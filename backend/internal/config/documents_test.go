package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeFileInfo is a stat result for the fake filesystem below.
type fakeFileInfo struct {
	name string
	mode fs.FileMode
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 1 }
func (f fakeFileInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

// fakeFS builds a fileLookup over an in-memory set of files, so detection is
// tested without touching (or depending on) the real machine.
type fakeFS struct {
	goos  string
	home  string
	files map[string]fs.FileMode
	path  map[string]string
	// statCalls records every path that was looked at.
	statCalls []string
}

func (f *fakeFS) lookup() fileLookup {
	return fileLookup{
		lookPath: func(file string) (string, error) {
			if found, ok := f.path[file]; ok {
				return found, nil
			}
			return "", errors.New("executable file not found in $PATH")
		},
		stat: func(name string) (os.FileInfo, error) {
			f.statCalls = append(f.statCalls, name)
			mode, ok := f.files[name]
			if !ok {
				return nil, fs.ErrNotExist
			}
			return fakeFileInfo{name: filepath.Base(name), mode: mode}, nil
		},
		glob: func(pattern string) ([]string, error) {
			var out []string
			for name := range f.files {
				if ok, _ := filepath.Match(pattern, name); ok {
					out = append(out, name)
				}
			}
			return out, nil
		},
		goos: f.goos,
		home: func() (string, error) {
			if f.home == "" {
				return "", errors.New("no home")
			}
			return f.home, nil
		},
	}
}

const (
	execFile  fs.FileMode = 0o755
	plainFile fs.FileMode = 0o644
)

func TestResolveSoffice(t *testing.T) {
	const macApp = "/Applications/LibreOffice.app/Contents/MacOS/soffice"
	tests := []struct {
		name       string
		raw        string
		fs         fakeFS
		wantBin    string
		wantSource string
	}{
		{name: "off", raw: "off", fs: fakeFS{goos: "darwin", files: map[string]fs.FileMode{macApp: execFile}}, wantSource: SofficeSourceDisabled},
		{name: "OFF upper case with spaces", raw: "  OFF ", fs: fakeFS{goos: "darwin", files: map[string]fs.FileMode{macApp: execFile}}, wantSource: SofficeSourceDisabled},
		{name: "none", raw: "none", fs: fakeFS{goos: "linux"}, wantSource: SofficeSourceDisabled},
		{name: "disabled", raw: "Disabled", fs: fakeFS{goos: "linux"}, wantSource: SofficeSourceDisabled},
		{name: "false", raw: "false", fs: fakeFS{goos: "linux"}, wantSource: SofficeSourceDisabled},
		{name: "zero", raw: "0", fs: fakeFS{goos: "linux"}, wantSource: SofficeSourceDisabled},
		{
			name:       "explicit valid path",
			raw:        "/opt/custom/soffice",
			fs:         fakeFS{goos: "linux", files: map[string]fs.FileMode{"/opt/custom/soffice": execFile, "/usr/bin/soffice": execFile}},
			wantBin:    "/opt/custom/soffice",
			wantSource: SofficeSourceEnv,
		},
		{
			name:       "explicit missing path is invalid, no fallback to auto",
			raw:        "/nope/soffice",
			fs:         fakeFS{goos: "linux", files: map[string]fs.FileMode{"/usr/bin/soffice": execFile}},
			wantSource: SofficeSourceEnvInvalid,
		},
		{
			name:       "explicit non-executable file is invalid",
			raw:        "/opt/custom/soffice",
			fs:         fakeFS{goos: "linux", files: map[string]fs.FileMode{"/opt/custom/soffice": plainFile}},
			wantSource: SofficeSourceEnvInvalid,
		},
		{
			name:       "explicit directory is invalid",
			raw:        "/Applications/LibreOffice.app",
			fs:         fakeFS{goos: "darwin", files: map[string]fs.FileMode{"/Applications/LibreOffice.app": fs.ModeDir | 0o755}},
			wantSource: SofficeSourceEnvInvalid,
		},
		{
			name:       "explicit bare name found on PATH",
			raw:        "soffice",
			fs:         fakeFS{goos: "linux", path: map[string]string{"soffice": "/usr/bin/soffice"}, files: map[string]fs.FileMode{"/usr/bin/soffice": execFile}},
			wantBin:    "/usr/bin/soffice",
			wantSource: SofficeSourceEnv,
		},
		{
			name:       "explicit bare name not on PATH",
			raw:        "soffice",
			fs:         fakeFS{goos: "linux"},
			wantSource: SofficeSourceEnvInvalid,
		},
		{
			name:       "auto via PATH soffice",
			fs:         fakeFS{goos: "darwin", path: map[string]string{"soffice": "/opt/homebrew/bin/soffice"}, files: map[string]fs.FileMode{"/opt/homebrew/bin/soffice": execFile, macApp: execFile}},
			wantBin:    "/opt/homebrew/bin/soffice",
			wantSource: SofficeSourceAuto,
		},
		{
			name:       "auto via PATH libreoffice",
			fs:         fakeFS{goos: "linux", path: map[string]string{"libreoffice": "/usr/local/bin/libreoffice"}, files: map[string]fs.FileMode{"/usr/local/bin/libreoffice": execFile}},
			wantBin:    "/usr/local/bin/libreoffice",
			wantSource: SofficeSourceAuto,
		},
		{
			name:       "auto ignores a relative PATH hit",
			fs:         fakeFS{goos: "linux", path: map[string]string{"soffice": "bin/soffice"}, files: map[string]fs.FileMode{"bin/soffice": execFile}},
			wantSource: SofficeSourceNotFound,
		},
		{
			name:       "auto darwin app bundle",
			fs:         fakeFS{goos: "darwin", home: "/Users/dev", files: map[string]fs.FileMode{macApp: execFile}},
			wantBin:    macApp,
			wantSource: SofficeSourceAuto,
		},
		{
			name:       "auto darwin per-user app bundle",
			fs:         fakeFS{goos: "darwin", home: "/Users/dev", files: map[string]fs.FileMode{"/Users/dev/Applications/LibreOffice.app/Contents/MacOS/soffice": execFile}},
			wantBin:    "/Users/dev/Applications/LibreOffice.app/Contents/MacOS/soffice",
			wantSource: SofficeSourceAuto,
		},
		{
			name:       "auto darwin skips a non-executable bundle binary",
			fs:         fakeFS{goos: "darwin", home: "/Users/dev", files: map[string]fs.FileMode{macApp: plainFile}},
			wantSource: SofficeSourceNotFound,
		},
		{
			name:       "auto darwin does not use linux paths",
			fs:         fakeFS{goos: "darwin", files: map[string]fs.FileMode{"/usr/bin/soffice": execFile}},
			wantSource: SofficeSourceNotFound,
		},
		{
			name:       "auto linux /usr/bin first",
			fs:         fakeFS{goos: "linux", files: map[string]fs.FileMode{"/usr/bin/soffice": execFile, "/usr/lib/libreoffice/program/soffice": execFile}},
			wantBin:    "/usr/bin/soffice",
			wantSource: SofficeSourceAuto,
		},
		{
			name:       "auto linux distro program dir",
			fs:         fakeFS{goos: "linux", files: map[string]fs.FileMode{"/usr/lib64/libreoffice/program/soffice": execFile}},
			wantBin:    "/usr/lib64/libreoffice/program/soffice",
			wantSource: SofficeSourceAuto,
		},
		{
			name: "auto linux /opt glob takes the lexicographically last",
			fs: fakeFS{goos: "linux", files: map[string]fs.FileMode{
				"/opt/libreoffice7.6/program/soffice":  execFile,
				"/opt/libreoffice24.8/program/soffice": execFile,
				"/opt/libreoffice25.2/program/soffice": execFile,
				"/snap/bin/libreoffice":                execFile,
			}},
			wantBin:    "/opt/libreoffice7.6/program/soffice",
			wantSource: SofficeSourceAuto,
		},
		{
			name:       "auto linux snap",
			fs:         fakeFS{goos: "linux", files: map[string]fs.FileMode{"/snap/bin/libreoffice": execFile}},
			wantBin:    "/snap/bin/libreoffice",
			wantSource: SofficeSourceAuto,
		},
		{
			name:       "auto NixOS system profile",
			fs:         fakeFS{goos: "linux", files: map[string]fs.FileMode{"/run/current-system/sw/bin/soffice": execFile}},
			wantBin:    "/run/current-system/sw/bin/soffice",
			wantSource: SofficeSourceAuto,
		},
		{
			name:       "auto windows program files",
			fs:         fakeFS{goos: "windows", files: map[string]fs.FileMode{`C:\Program Files\LibreOffice\program\soffice.exe`: plainFile}},
			wantBin:    `C:\Program Files\LibreOffice\program\soffice.exe`,
			wantSource: SofficeSourceAuto,
		},
		{name: "none found darwin", fs: fakeFS{goos: "darwin", home: "/Users/dev"}, wantSource: SofficeSourceNotFound},
		{name: "none found linux", fs: fakeFS{goos: "linux"}, wantSource: SofficeSourceNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := tc.fs
			got := resolveSoffice(tc.raw, fake.lookup())
			if got.Bin != tc.wantBin || got.Source != tc.wantSource {
				t.Fatalf("resolveSoffice(%q) = %+v, want bin %q source %q", tc.raw, got, tc.wantBin, tc.wantSource)
			}
			if (got.Bin != "") != (tc.wantSource == SofficeSourceAuto || tc.wantSource == SofficeSourceEnv) {
				t.Fatalf("Bin must be set exactly for auto/env: %+v", got)
			}
			if got.Source == SofficeSourceEnvInvalid && got.Reason == "" {
				t.Fatal("env_invalid must carry a reason for the startup WARN")
			}
		})
	}
}

// Lexicographic order decides between /opt installs: "libreoffice7.6" sorts
// after "libreoffice25.2", so the vendor dirs with the same digit count are
// what "newest" reliably means.
func TestResolveSofficeOptGlobSameWidth(t *testing.T) {
	fake := fakeFS{goos: "linux", files: map[string]fs.FileMode{
		"/opt/libreoffice24.2/program/soffice": execFile,
		"/opt/libreoffice25.8/program/soffice": execFile,
		"/opt/libreoffice25.2/program/soffice": execFile,
	}}
	got := resolveSoffice("", fake.lookup())
	if got.Bin != "/opt/libreoffice25.8/program/soffice" {
		t.Fatalf("Bin = %q, want the lexicographically last /opt install", got.Bin)
	}
}

// Detection looks only at PATH and the fixed candidate list.
func TestResolveSofficeLooksOnlyAtFixedCandidates(t *testing.T) {
	fake := fakeFS{goos: "linux"}
	resolveSoffice("", fake.lookup())
	allowed := map[string]bool{}
	for _, c := range (&fakeFS{goos: "linux"}).lookup().sofficeCandidates() {
		allowed[c] = true
	}
	for _, path := range fake.statCalls {
		if !allowed[path] {
			t.Fatalf("detection looked at %q, outside the fixed candidate list", path)
		}
	}
	if len(fake.statCalls) == 0 {
		t.Fatal("expected the fixed candidates to be checked")
	}
}

// An explicit relative path is made absolute: soffice runs with its job dir
// as working directory.
func TestResolveSofficeExplicitRelativePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "soffice"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveSoffice("./soffice", systemFileLookup())
	if got.Source != SofficeSourceEnv || !filepath.IsAbs(got.Bin) || filepath.Base(got.Bin) != "soffice" {
		t.Fatalf("resolveSoffice(./soffice) = %+v", got)
	}
}

func TestLoadDocumentsConfigInjectedLookup(t *testing.T) {
	setMinimalEnv(t)
	fake := fakeFS{goos: "darwin", files: map[string]fs.FileMode{"/Applications/LibreOffice.app/Contents/MacOS/soffice": execFile}}
	d, err := loadDocumentsConfig("8080", fake.lookup())
	if err != nil {
		t.Fatal(err)
	}
	if !d.PDFEnabled() || d.SofficeSource != SofficeSourceAuto || d.SofficeBin != "/Applications/LibreOffice.app/Contents/MacOS/soffice" {
		t.Fatalf("auto-detected config = %+v", d)
	}

	empty := fakeFS{goos: "linux"}
	d, err = loadDocumentsConfig("8080", empty.lookup())
	if err != nil {
		t.Fatal(err)
	}
	if d.PDFEnabled() || d.SofficeSource != SofficeSourceNotFound {
		t.Fatalf("nothing installed = %+v", d)
	}
}

func TestDefaultLOProfileDir(t *testing.T) {
	cache := func() (string, error) { return "/Users/dev/Library/Caches", nil }
	noCache := func() (string, error) { return "", errors.New("$HOME is not defined") }
	temp := func() string { return "/tmp" }
	tempProfile := "/tmp/kantor-lo-profile-8080"
	if uid := os.Getuid(); uid >= 0 {
		tempProfile = fmt.Sprintf("/tmp/kantor-lo-profile-%d-8080", uid)
	}
	tests := []struct {
		name  string
		port  string
		cache func() (string, error)
		want  string
	}{
		{name: "user cache dir per port", port: "8080", cache: cache, want: "/Users/dev/Library/Caches/kantor/lo-profile-8080"},
		{name: "second backend gets its own profile", port: "18080", cache: cache, want: "/Users/dev/Library/Caches/kantor/lo-profile-18080"},
		{name: "port keeps digits only", port: ":8080", cache: cache, want: "/Users/dev/Library/Caches/kantor/lo-profile-8080"},
		{name: "port traversal is stripped", port: "../../etc", cache: cache, want: "/Users/dev/Library/Caches/kantor/lo-profile-default"},
		{name: "cache dir error falls back to temp", port: "8080", cache: noCache, want: tempProfile},
		{name: "empty cache dir falls back to temp", port: "8080", cache: func() (string, error) { return " ", nil }, want: tempProfile},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultLOProfileDir(tc.port, tc.cache, temp); got != tc.want {
				t.Fatalf("defaultLOProfileDir(%q) = %q, want %q", tc.port, got, tc.want)
			}
		})
	}
}

// The default profile dir is never inside the repository (the working
// directory of `go run ./cmd/server`).
func TestDefaultLOProfileDirOutsideWorkingDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	got := defaultLOProfileDir("8080", os.UserCacheDir, os.TempDir)
	if !filepath.IsAbs(got) || strings.HasPrefix(got, wd+string(filepath.Separator)) {
		t.Fatalf("default profile dir %q must be absolute and outside %q", got, wd)
	}
}

func TestResolveMailCapture(t *testing.T) {
	tests := []struct {
		name        string
		appEnv      string
		raw         string
		wantAddr    string
		wantSource  string
		wantWarning bool
	}{
		{name: "development unset -> Mailpit default", appEnv: "development", raw: "", wantAddr: "localhost:1025", wantSource: MailCaptureDefault},
		{name: "development blank -> Mailpit default", appEnv: "development", raw: "   ", wantAddr: "localhost:1025", wantSource: MailCaptureDefault},
		{name: "development off", appEnv: "development", raw: "off", wantSource: MailCaptureOff},
		{name: "development OFF", appEnv: "development", raw: " OFF ", wantSource: MailCaptureOff},
		{name: "development none", appEnv: "development", raw: "none", wantSource: MailCaptureOff},
		{name: "development disabled", appEnv: "development", raw: "disabled", wantSource: MailCaptureOff},
		{name: "development false", appEnv: "development", raw: "false", wantSource: MailCaptureOff},
		{name: "development 0", appEnv: "development", raw: "0", wantSource: MailCaptureOff},
		{name: "development host:port", appEnv: "development", raw: " mailpit:1025 ", wantAddr: "mailpit:1025", wantSource: MailCaptureEnv},
		{name: "development ipv6 host:port", appEnv: "development", raw: "[::1]:2525", wantAddr: "[::1]:2525", wantSource: MailCaptureEnv},
		{name: "development missing port -> default + reason", appEnv: "development", raw: "localhost", wantAddr: "localhost:1025", wantSource: MailCaptureEnvInvalid, wantWarning: true},
		{name: "development non-numeric port -> default + reason", appEnv: "development", raw: "localhost:smtp", wantAddr: "localhost:1025", wantSource: MailCaptureEnvInvalid, wantWarning: true},
		{name: "development port out of range -> default + reason", appEnv: "development", raw: "localhost:70000", wantAddr: "localhost:1025", wantSource: MailCaptureEnvInvalid, wantWarning: true},
		{name: "development empty host -> default + reason", appEnv: "development", raw: ":1025", wantAddr: "localhost:1025", wantSource: MailCaptureEnvInvalid, wantWarning: true},
		{name: "development url -> default + reason", appEnv: "development", raw: "smtp://localhost:1025", wantAddr: "localhost:1025", wantSource: MailCaptureEnvInvalid, wantWarning: true},
		{name: "production set -> none", appEnv: "production", raw: "localhost:1025", wantSource: MailCaptureNotDevelopment, wantWarning: true},
		{name: "production unset -> none", appEnv: "production", raw: "", wantSource: MailCaptureNotDevelopment},
		{name: "production off -> none, no warning", appEnv: "production", raw: "off", wantSource: MailCaptureNotDevelopment},
		{name: "staging set -> none", appEnv: "staging", raw: "localhost:1025", wantSource: MailCaptureNotDevelopment, wantWarning: true},
		{name: "test unset -> none", appEnv: "test", raw: "", wantSource: MailCaptureNotDevelopment},
		{name: "blank APP_ENV set -> none", appEnv: "  ", raw: "localhost:1025", wantSource: MailCaptureNotDevelopment, wantWarning: true},
		{name: "unset APP_ENV unset -> none", appEnv: "", raw: "", wantSource: MailCaptureNotDevelopment},
		{name: "Development is not development", appEnv: "Development", raw: "", wantSource: MailCaptureNotDevelopment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Config{AppEnv: "development", RawAppEnv: tc.appEnv, DocumentMailDevSMTPAddr: tc.raw}.DocumentMailCapture()
			if got.Addr != tc.wantAddr || got.Source != tc.wantSource {
				t.Fatalf("capture = %+v, want addr %q source %q", got, tc.wantAddr, tc.wantSource)
			}
			if (got.Warning != "") != tc.wantWarning {
				t.Fatalf("warning = %q, want warning=%v", got.Warning, tc.wantWarning)
			}
		})
	}
}

func TestIsOffValue(t *testing.T) {
	for _, value := range []string{"off", "OFF", " none ", "Disabled", "FALSE", "0"} {
		if !IsOffValue(value) {
			t.Fatalf("IsOffValue(%q) = false", value)
		}
	}
	for _, value := range []string{"", "no", "1", "localhost:1025", "/usr/bin/soffice", "offline"} {
		if IsOffValue(value) {
			t.Fatalf("IsOffValue(%q) = true", value)
		}
	}
}

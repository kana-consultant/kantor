package config

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Zero-env defaults for the HR document features: none of SOFFICE_BIN,
// DOCUMENT_LO_PROFILE_DIR or DOCUMENT_MAIL_DEV_SMTP_ADDR has to be set. Each
// is an optional override with a working default. Executable paths,
// filesystem paths and SMTP hosts are never settable from the admin UI; the
// UI only shows the effective status.

// SofficeSource says where the LibreOffice binary came from.
const (
	// SofficeSourceAuto: SOFFICE_BIN unset, a binary was found on PATH or in
	// a well-known install location.
	SofficeSourceAuto = "auto"
	// SofficeSourceEnv: SOFFICE_BIN points at a usable binary.
	SofficeSourceEnv = "env"
	// SofficeSourceDisabled: SOFFICE_BIN=off (or none/disabled/false/0).
	SofficeSourceDisabled = "disabled"
	// SofficeSourceNotFound: SOFFICE_BIN unset and nothing was detected.
	SofficeSourceNotFound = "not_found"
	// SofficeSourceEnvInvalid: SOFFICE_BIN is set but is not an executable
	// file; PDF stays disabled instead of failing every job later.
	SofficeSourceEnvInvalid = "env_invalid"
)

// DefaultDevSMTPAddr is where document email is captured with an explicit
// APP_ENV=development when DOCUMENT_MAIL_DEV_SMTP_ADDR is unset (Mailpit).
const DefaultDevSMTPAddr = "localhost:1025"

// Mail capture sources (DocumentMailCapture).
const (
	// MailCaptureDefault: development, variable unset -> DefaultDevSMTPAddr.
	MailCaptureDefault = "default"
	// MailCaptureEnv: development, variable is a valid host:port.
	MailCaptureEnv = "env"
	// MailCaptureEnvInvalid: development, variable is not a valid host:port;
	// capture falls back to DefaultDevSMTPAddr (never to real Gmail).
	MailCaptureEnvInvalid = "env_invalid"
	// MailCaptureOff: development, variable is off -> real Gmail.
	MailCaptureOff = "off"
	// MailCaptureNotDevelopment: APP_ENV is not explicitly development;
	// capture never happens.
	MailCaptureNotDevelopment = "not_development"
)

// IsOffValue reports whether an override variable is set to one of the
// "switch it off" words: off, none, disabled, false, 0 (case-insensitive).
func IsOffValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "off", "none", "disabled", "false", "0":
		return true
	}
	return false
}

// SofficeResolution is the outcome of resolving SOFFICE_BIN.
type SofficeResolution struct {
	// Bin is the usable soffice executable; empty means PDF is disabled.
	Bin string
	// Source is one of the SofficeSource* constants.
	Source string
	// Requested is the raw SOFFICE_BIN value (trimmed), for logging.
	Requested string
	// Reason explains env_invalid.
	Reason string
}

// fileLookup holds the filesystem/OS lookups used by detection, injectable
// so tests never depend on the machine they run on. Detection never
// executes anything: it only looks at PATH and a fixed list of locations.
type fileLookup struct {
	lookPath func(file string) (string, error)
	stat     func(name string) (os.FileInfo, error)
	glob     func(pattern string) ([]string, error)
	goos     string
	home     func() (string, error)
}

func systemFileLookup() fileLookup {
	return fileLookup{
		lookPath: exec.LookPath,
		stat:     os.Stat,
		glob:     filepath.Glob,
		goos:     runtime.GOOS,
		home:     os.UserHomeDir,
	}
}

// resolveSoffice applies the SOFFICE_BIN rules: an off word disables PDF, any
// other value is an explicit path (a bare name is looked up on PATH) that
// must be an executable file, and an empty value triggers auto-detection.
func resolveSoffice(raw string, lookup fileLookup) SofficeResolution {
	raw = strings.TrimSpace(raw)
	if IsOffValue(raw) {
		return SofficeResolution{Source: SofficeSourceDisabled, Requested: raw}
	}
	if raw != "" {
		candidate := raw
		if !strings.ContainsAny(raw, `/\`) {
			found, err := lookup.lookPath(raw)
			if err != nil {
				return SofficeResolution{Source: SofficeSourceEnvInvalid, Requested: raw, Reason: "not found on PATH"}
			}
			candidate = found
		}
		if ok, reason := lookup.isExecutableFile(candidate); !ok {
			return SofficeResolution{Source: SofficeSourceEnvInvalid, Requested: raw, Reason: reason}
		}
		// soffice runs with its job dir as working directory, so a relative
		// path would no longer point at the same file.
		if abs, err := filepath.Abs(candidate); err == nil {
			candidate = abs
		}
		return SofficeResolution{Bin: candidate, Source: SofficeSourceEnv, Requested: raw}
	}

	for _, name := range []string{"soffice", "libreoffice"} {
		found, err := lookup.lookPath(name)
		if err != nil || found == "" {
			continue
		}
		if !filepath.IsAbs(found) {
			continue
		}
		if ok, _ := lookup.isExecutableFile(found); ok {
			return SofficeResolution{Bin: found, Source: SofficeSourceAuto}
		}
	}
	for _, candidate := range lookup.sofficeCandidates() {
		if ok, _ := lookup.isExecutableFile(candidate); ok {
			return SofficeResolution{Bin: candidate, Source: SofficeSourceAuto}
		}
	}
	return SofficeResolution{Source: SofficeSourceNotFound}
}

// sofficeCandidates is the fixed list of install locations for the current
// OS, in priority order.
func (l fileLookup) sofficeCandidates() []string {
	switch l.goos {
	case "darwin":
		candidates := []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
		if home, err := l.home(); err == nil && strings.TrimSpace(home) != "" {
			candidates = append(candidates, filepath.Join(home, "Applications", "LibreOffice.app", "Contents", "MacOS", "soffice"))
		}
		return candidates
	case "windows":
		return []string{`C:\Program Files\LibreOffice\program\soffice.exe`}
	default:
		candidates := []string{
			"/usr/bin/soffice",
			"/usr/local/bin/soffice",
			"/usr/lib/libreoffice/program/soffice",
			"/usr/lib64/libreoffice/program/soffice",
		}
		// Vendor tarball installs (/opt/libreoffice7.6, /opt/libreoffice24.2):
		// the lexicographically last one is taken as the newest.
		if matches, err := l.glob("/opt/libreoffice*/program/soffice"); err == nil && len(matches) > 0 {
			sorted := append([]string(nil), matches...)
			sort.Sort(sort.Reverse(sort.StringSlice(sorted)))
			candidates = append(candidates, sorted...)
		}
		return append(candidates,
			"/snap/bin/libreoffice",
			"/run/current-system/sw/bin/soffice",
		)
	}
}

// isExecutableFile reports whether path is an existing regular file with an
// executable bit (Windows has no executable bit; a regular file is enough).
func (l fileLookup) isExecutableFile(path string) (bool, string) {
	info, err := l.stat(path)
	if err != nil {
		return false, "file does not exist or cannot be read"
	}
	if !info.Mode().IsRegular() {
		return false, "not a regular file"
	}
	if l.goos != "windows" && info.Mode().Perm()&0o111 == 0 {
		return false, "file is not executable"
	}
	return true, ""
}

// defaultLOProfileDir is the LibreOffice profile used when
// DOCUMENT_LO_PROFILE_DIR is unset: a per-user cache dir, one per port, so
// two backends on one machine never share a profile lock. It is never inside
// the repository. The shared temp dir fallback also carries the user id, so
// another local user cannot pre-create the predictable path (the converter
// refuses a profile dir it does not own anyway, see
// docgen.SecureProfileDir).
func defaultLOProfileDir(port string, userCacheDir func() (string, error), tempDir func() string) string {
	suffix := portDigits(port)
	if cacheDir, err := userCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "kantor", "lo-profile-"+suffix)
	}
	if uid := os.Getuid(); uid >= 0 {
		suffix = strconv.Itoa(uid) + "-" + suffix
	}
	return filepath.Join(tempDir(), "kantor-lo-profile-"+suffix)
}

func portDigits(port string) string {
	var b strings.Builder
	for _, r := range port {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

// MailCapture is the effective document-mail capture setting.
type MailCapture struct {
	// Addr is the local capture server (host:port); empty means document
	// email goes to smtp.gmail.com.
	Addr string
	// Source is one of the MailCapture* constants.
	Source string
	// Warning explains why an explicit DOCUMENT_MAIL_DEV_SMTP_ADDR was
	// ignored or replaced by the default (empty otherwise).
	Warning string
}

// resolveMailCapture applies the DOCUMENT_MAIL_DEV_SMTP_ADDR rules. Only an
// explicit APP_ENV=development captures; there, unset means
// DefaultDevSMTPAddr, an off word means real Gmail, a valid host:port is used
// as-is and an invalid value falls back to the default capture (never
// silently to Gmail from a dev machine).
func resolveMailCapture(rawAppEnv string, rawAddr string) MailCapture {
	raw := strings.TrimSpace(rawAddr)
	if strings.TrimSpace(rawAppEnv) != "development" {
		capture := MailCapture{Source: MailCaptureNotDevelopment}
		if raw != "" && !IsOffValue(raw) {
			capture.Warning = "APP_ENV is not explicitly development"
		}
		return capture
	}
	switch {
	case raw == "":
		return MailCapture{Addr: DefaultDevSMTPAddr, Source: MailCaptureDefault}
	case IsOffValue(raw):
		return MailCapture{Source: MailCaptureOff}
	}
	if reason := validateHostPort(raw); reason != "" {
		return MailCapture{Addr: DefaultDevSMTPAddr, Source: MailCaptureEnvInvalid, Warning: reason}
	}
	return MailCapture{Addr: raw, Source: MailCaptureEnv}
}

func validateHostPort(raw string) string {
	host, port, err := net.SplitHostPort(raw)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return "value is not a valid host:port"
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "port is not a number between 1 and 65535"
	}
	if strings.ContainsAny(host, " \t/") {
		return "value is not a valid host:port"
	}
	return ""
}

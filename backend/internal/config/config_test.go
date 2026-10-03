package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Load defaults AppEnv to "development"; capture must still require an
// explicit APP_ENV=development, whatever DOCUMENT_MAIL_DEV_SMTP_ADDR says.
func TestLoadDocumentMailCaptureNeedsExplicitAppEnv(t *testing.T) {
	for _, tc := range []struct {
		name        string
		appEnv      *string
		raw         *string
		wantAddr    string
		wantSource  string
		wantWarning bool
	}{
		{name: "unset APP_ENV, variable set", appEnv: nil, raw: strPtr("relay.example.net:25"), wantSource: MailCaptureNotDevelopment, wantWarning: true},
		{name: "unset APP_ENV, variable unset", appEnv: nil, wantSource: MailCaptureNotDevelopment},
		{name: "blank APP_ENV", appEnv: strPtr("  "), raw: strPtr("relay.example.net:25"), wantSource: MailCaptureNotDevelopment, wantWarning: true},
		{name: "production, variable set", appEnv: strPtr("production"), raw: strPtr("localhost:1025"), wantSource: MailCaptureNotDevelopment, wantWarning: true},
		{name: "production, variable unset", appEnv: strPtr("production"), wantSource: MailCaptureNotDevelopment},
		{name: "explicit development, variable unset", appEnv: strPtr("development"), wantAddr: DefaultDevSMTPAddr, wantSource: MailCaptureDefault},
		{name: "explicit development, host:port", appEnv: strPtr("development"), raw: strPtr("relay.example.net:25"), wantAddr: "relay.example.net:25", wantSource: MailCaptureEnv},
		{name: "explicit development, off", appEnv: strPtr("development"), raw: strPtr("off"), wantSource: MailCaptureOff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setMinimalEnv(t)
			if tc.raw != nil {
				t.Setenv("DOCUMENT_MAIL_DEV_SMTP_ADDR", *tc.raw)
			}
			t.Setenv("APP_ENV", "")
			if tc.appEnv == nil {
				_ = os.Unsetenv("APP_ENV")
			} else {
				t.Setenv("APP_ENV", *tc.appEnv)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			capture := cfg.DocumentMailCapture()
			if capture.Addr != tc.wantAddr || capture.Source != tc.wantSource {
				t.Fatalf("capture = %+v, want addr %q source %q", capture, tc.wantAddr, tc.wantSource)
			}
			if (capture.Warning != "") != tc.wantWarning {
				t.Fatalf("warning = %q, want warning=%v (an ignored value must report a reason so the startup WARN fires)", capture.Warning, tc.wantWarning)
			}
		})
	}
}

func strPtr(value string) *string { return &value }

func setMinimalEnv(t *testing.T) {
	t.Helper()
	// Run from an empty directory so no .env file is loaded.
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://x@localhost/x")
	t.Setenv("JWT_SECRET", "test-secret-0123456789abcdef0123456789")
	t.Setenv("DATA_ENCRYPTION_KEY", "test-data-key-0123456789abcdef012345")
	for _, key := range []string{"SOFFICE_BIN", "DOCUMENT_RENDER_TIMEOUT", "DOCUMENT_LO_PROFILE_DIR", "DOCUMENTS_ALLOW_DOCX_SEND", "DOCUMENT_MAIL_DEV_SMTP_ADDR", "PORT", "BANK_ACCOUNT_CLEAR_PLAINTEXT", "AUDIT_SCRUB_EXISTING"} {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}
}

func TestLoadDocumentsDefaults(t *testing.T) {
	setMinimalEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := cfg.Documents
	// SOFFICE_BIN unset auto-detects; the outcome depends on the machine,
	// so only its consistency is checked here (the rules are table-tested
	// with injected lookups in documents_test.go).
	switch d.SofficeSource {
	case SofficeSourceAuto:
		if !d.PDFEnabled() {
			t.Fatalf("auto-detected soffice must enable PDF: %+v", d)
		}
	case SofficeSourceNotFound:
		if d.PDFEnabled() {
			t.Fatalf("not_found must disable PDF: %+v", d)
		}
	default:
		t.Fatalf("SofficeSource = %q with SOFFICE_BIN unset", d.SofficeSource)
	}
	if d.RenderTimeout != 90*time.Second {
		t.Fatalf("RenderTimeout = %s, want 90s", d.RenderTimeout)
	}
	want := defaultLOProfileDir("8080", os.UserCacheDir, os.TempDir)
	if d.LOProfileDir != want || !d.LOProfileDirDefaulted || !strings.HasSuffix(d.LOProfileDir, "lo-profile-8080") {
		t.Fatalf("LOProfileDir = %q (defaulted %v), want %q", d.LOProfileDir, d.LOProfileDirDefaulted, want)
	}
	if strings.HasPrefix(d.LOProfileDir, "/var/lib") {
		t.Fatalf("the /var/lib default is gone: %q", d.LOProfileDir)
	}
	if d.AllowDocxSend {
		t.Fatal("DOCUMENTS_ALLOW_DOCX_SEND must default to false")
	}
}

func TestLoadDocumentsSofficeOff(t *testing.T) {
	for _, value := range []string{"off", " OFF ", "none", "Disabled", "false", "0"} {
		t.Run(value, func(t *testing.T) {
			setMinimalEnv(t)
			t.Setenv("SOFFICE_BIN", value)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			d := cfg.Documents
			if d.PDFEnabled() || d.SofficeBin != "" || d.SofficeSource != SofficeSourceDisabled {
				t.Fatalf("SOFFICE_BIN=%q must disable PDF: %+v", value, d)
			}
		})
	}
}

func TestLoadDocumentsProfileDirPerPort(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("PORT", "18080")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasSuffix(cfg.Documents.LOProfileDir, "lo-profile-18080") {
		t.Fatalf("LOProfileDir = %q, want a per-port dir", cfg.Documents.LOProfileDir)
	}
}

func TestLoadDocumentsFromEnv(t *testing.T) {
	setMinimalEnv(t)
	bin := fakeExecutable(t)
	t.Setenv("SOFFICE_BIN", " "+bin+" ")
	t.Setenv("DOCUMENT_RENDER_TIMEOUT", "2m")
	t.Setenv("DOCUMENT_LO_PROFILE_DIR", "/tmp/lo")
	t.Setenv("DOCUMENTS_ALLOW_DOCX_SEND", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := cfg.Documents
	if d.SofficeBin != bin || !d.PDFEnabled() || d.SofficeSource != SofficeSourceEnv {
		t.Fatalf("SofficeBin = %q source %q", d.SofficeBin, d.SofficeSource)
	}
	if d.RenderTimeout != 2*time.Minute || d.LOProfileDir != "/tmp/lo" || d.LOProfileDirDefaulted || !d.AllowDocxSend {
		t.Fatalf("unexpected documents config: %+v", d)
	}
}

// An explicit SOFFICE_BIN that is not an executable file disables PDF with a
// clear state instead of failing every conversion later.
func TestLoadDocumentsInvalidSoffice(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("SOFFICE_BIN", filepath.Join(t.TempDir(), "missing-soffice"))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := cfg.Documents
	if d.PDFEnabled() || d.SofficeSource != SofficeSourceEnvInvalid || d.SofficeInvalidReason == "" {
		t.Fatalf("invalid SOFFICE_BIN: %+v", d)
	}
}

// fakeExecutable creates an executable file standing in for soffice; it is
// never run.
func fakeExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "soffice")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDocumentsRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"DOCUMENT_RENDER_TIMEOUT", "soon"},
		{"DOCUMENT_RENDER_TIMEOUT", "0s"},
		{"DOCUMENT_RENDER_TIMEOUT", "-5s"},
		{"DOCUMENTS_ALLOW_DOCX_SEND", "maybe"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			setMinimalEnv(t)
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted %s=%q", tc.key, tc.value)
			}
		})
	}
}

// The clean-ups that rewrite existing rows are opt-in: unset, blank or
// invalid values keep them off, and an invalid value does not stop the start.
func TestLoadDataMaintenanceOptIn(t *testing.T) {
	for _, tc := range []struct {
		name        string
		clear       *string
		scrub       *string
		wantClear   bool
		wantScrub   bool
		wantInvalid []string
	}{
		{name: "unset"},
		{name: "blank", clear: strPtr(" "), scrub: strPtr("")},
		{name: "explicit off", clear: strPtr("false"), scrub: strPtr("0")},
		{name: "clear on", clear: strPtr("true"), wantClear: true},
		{name: "scrub on", scrub: strPtr("yes"), wantScrub: true},
		{name: "both on", clear: strPtr("1"), scrub: strPtr("ON"), wantClear: true, wantScrub: true},
		{name: "invalid values stay off", clear: strPtr("ture"), scrub: strPtr("enable"), wantInvalid: []string{"BANK_ACCOUNT_CLEAR_PLAINTEXT", "AUDIT_SCRUB_EXISTING"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setMinimalEnv(t)
			if tc.clear != nil {
				t.Setenv("BANK_ACCOUNT_CLEAR_PLAINTEXT", *tc.clear)
			}
			if tc.scrub != nil {
				t.Setenv("AUDIT_SCRUB_EXISTING", *tc.scrub)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			got := cfg.DataMaintenance
			if got.BankAccountClearPlaintext != tc.wantClear || got.AuditScrubExisting != tc.wantScrub {
				t.Fatalf("data maintenance = %+v, want clear=%v scrub=%v", got, tc.wantClear, tc.wantScrub)
			}
			if strings.Join(got.Invalid, ",") != strings.Join(tc.wantInvalid, ",") {
				t.Fatalf("invalid = %v, want %v", got.Invalid, tc.wantInvalid)
			}
		})
	}
}

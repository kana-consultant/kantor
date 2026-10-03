package docgen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PDF conversion through headless LibreOffice (soffice).
//
// One Converter serialises all conversions (semaphore of 1) and reuses a
// persistent LibreOffice profile so jobs do not pay the first-start cost.
// Each call runs in its own job dir that is removed on every path, with a
// minimal environment so secrets such as DATA_ENCRYPTION_KEY, JWT_SECRET or
// DATABASE_URL are never inherited by soffice. soffice runs in its own
// process group, which is killed as a whole (soffice -> oosplash ->
// soffice.bin) on timeout or cancellation.
//
// Job dirs hold plaintext documents (the rendered DOCX and soffice's PDF), so
// they live under <TempDir>/kantor-docgen/inst-*/, one instance dir per
// Converter, held under an exclusive lock for the life of the process. A
// crash or SIGKILL skips the deferred cleanup; the next Converter (at server
// start) removes every instance dir whose lock is no longer held.

// ErrConverterUnavailable is returned when no soffice binary was resolved
// (SOFFICE_BIN=off, invalid, or LibreOffice not found by auto-detection).
var ErrConverterUnavailable = errors.New("docgen: PDF converter (LibreOffice soffice) is not available")

const (
	DefaultRenderTimeout = 90 * time.Second
	// perExtraDocTimeout extends the timeout for each document beyond the
	// first in a batch; a warm soffice converts a template in about a second.
	perExtraDocTimeout = 5 * time.Second
	waitDelay          = 5 * time.Second
	maxCapturedOutput  = 16 << 10

	workRootName   = "kantor-docgen"
	instancePrefix = "inst-"
	lockFileName   = ".lock"
	// instanceGrace protects an instance dir that is being created (lock
	// file not written yet) from a concurrent sweep.
	instanceGrace = time.Minute
)

// ConverterConfig configures a Converter.
type ConverterConfig struct {
	// Bin is the resolved soffice executable (SOFFICE_BIN or
	// auto-detected, see config). Empty disables PDF.
	Bin string
	// ProfileDir is the persistent LibreOffice user profile
	// (DOCUMENT_LO_PROFILE_DIR, or the per-user cache default). It is created
	// with mode 0700 on first use. Empty uses a throwaway profile per job.
	ProfileDir string
	// Timeout bounds one soffice call (DOCUMENT_RENDER_TIMEOUT). Batches get
	// a few extra seconds per additional document.
	Timeout time.Duration
	// TempDir is the base of the per-job directories, which are created
	// under TempDir/kantor-docgen. Empty uses os.TempDir().
	TempDir string
	// FontconfigFile is passed to soffice as FONTCONFIG_FILE. Empty falls
	// back to the process's FONTCONFIG_FILE, if any.
	FontconfigFile string
}

// Converter converts DOCX to PDF. It is safe for concurrent use; calls are
// serialised.
type Converter struct {
	cfg ConverterConfig
	sem chan struct{}

	// instDir and instLock are this converter's locked instance dir,
	// created on first use; guarded by sem.
	instDir  string
	instLock *os.File
	// profileWarned: the rejected-profile warning was logged; guarded by
	// sem.
	profileWarned bool
}

// NewConverter returns a converter. With an empty Bin it is unavailable and
// every conversion returns ErrConverterUnavailable. An available converter
// removes the job dirs a crashed process left behind (best effort).
func NewConverter(cfg ConverterConfig) *Converter {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultRenderTimeout
	}
	if cfg.FontconfigFile == "" {
		cfg.FontconfigFile = os.Getenv("FONTCONFIG_FILE")
	}
	c := &Converter{cfg: cfg, sem: make(chan struct{}, 1)}
	if c.Available() {
		_ = sweepStaleInstances(c.workRoot())
	}
	return c
}

func (c *Converter) workRoot() string {
	base := c.cfg.TempDir
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, workRootName)
}

// sweepStaleInstances removes the instance dirs under root that no live
// process holds. A missing root is not an error.
func sweepStaleInstances(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var firstErr error
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), instancePrefix) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		release, stale := claimStaleInstance(dir)
		if !stale {
			continue
		}
		if err := os.RemoveAll(dir); err != nil && firstErr == nil {
			firstErr = err
		}
		release()
	}
	return firstErr
}

// instanceDir returns this converter's locked instance dir, creating it on
// first use. Callers hold sem.
func (c *Converter) instanceDir() (string, error) {
	if c.instDir != "" {
		if _, err := os.Stat(c.instDir); err == nil {
			return c.instDir, nil
		}
		// Removed behind our back: start a new one.
		if c.instLock != nil {
			c.instLock.Close()
		}
		c.instDir, c.instLock = "", nil
	}
	root := c.workRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	for attempt := 0; attempt < 3; attempt++ {
		dir, err := os.MkdirTemp(root, instancePrefix+"*")
		if err != nil {
			return "", err
		}
		lock, err := lockInstance(dir)
		if err != nil {
			os.RemoveAll(dir)
			if errors.Is(err, errInstanceLost) {
				continue
			}
			return "", err
		}
		c.instDir, c.instLock = dir, lock
		return dir, nil
	}
	return "", errInstanceLost
}

// Available reports whether a soffice binary is configured.
func (c *Converter) Available() bool {
	return c != nil && c.cfg.Bin != ""
}

// Convert converts one DOCX document to PDF.
func (c *Converter) Convert(ctx context.Context, docx []byte) ([]byte, error) {
	out, err := c.ConvertBatch(ctx, [][]byte{docx})
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

// profileDir returns the LibreOffice user profile for one job: the
// persistent ProfileDir when it is safe to use, otherwise a throwaway
// profile inside the job dir. soffice loads macros and settings from the
// profile, so a persistent one that another local user could have prepared
// (a symlink, someone else's dir, group/other-writable) is refused with a
// warning (logged once) instead of being loaded for salary documents.
func (c *Converter) profileDir(jobDir string) (string, error) {
	throwaway := filepath.Join(jobDir, "profile")
	profile := c.cfg.ProfileDir
	if profile != "" {
		abs, err := filepath.Abs(profile)
		if err != nil {
			return "", fmt.Errorf("docgen: profile dir: %w", err)
		}
		if err := os.MkdirAll(abs, 0o700); err != nil {
			return "", fmt.Errorf("docgen: create profile dir: %w", err)
		}
		if err := SecureProfileDir(abs); err != nil {
			if !c.profileWarned {
				c.profileWarned = true
				slog.Warn("LibreOffice profile dir rejected; using a throwaway profile per conversion (slower)",
					"dir", abs,
					"reason", err.Error(),
				)
			}
			profile = ""
		} else {
			profile = abs
		}
	}
	if profile == "" {
		profile = throwaway
		if err := os.MkdirAll(profile, 0o700); err != nil {
			return "", fmt.Errorf("docgen: create profile dir: %w", err)
		}
	}
	return profile, nil
}

// ConvertBatch converts all documents in a single soffice call and returns
// the PDFs in input order. It fails as a whole if any document fails.
func (c *Converter) ConvertBatch(ctx context.Context, docs [][]byte) ([][]byte, error) {
	if !c.Available() {
		return nil, ErrConverterUnavailable
	}
	if len(docs) == 0 {
		return nil, nil
	}

	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.sem }()

	inst, err := c.instanceDir()
	if err != nil {
		return nil, fmt.Errorf("docgen: create work dir: %w", err)
	}
	jobDir, err := os.MkdirTemp(inst, "job-*")
	if err != nil {
		return nil, fmt.Errorf("docgen: create job dir: %w", err)
	}
	defer os.RemoveAll(jobDir)

	inDir := filepath.Join(jobDir, "in")
	outDir := filepath.Join(jobDir, "out")
	tmpDir := filepath.Join(jobDir, "tmp")
	for _, d := range []string{inDir, outDir, tmpDir} {
		if err := os.Mkdir(d, 0o700); err != nil {
			return nil, fmt.Errorf("docgen: create job dir: %w", err)
		}
	}

	profile, err := c.profileDir(jobDir)
	if err != nil {
		return nil, err
	}

	inputs := make([]string, len(docs))
	for i, doc := range docs {
		inputs[i] = filepath.Join(inDir, fmt.Sprintf("doc-%04d.docx", i))
		if err := os.WriteFile(inputs[i], doc, 0o600); err != nil {
			return nil, fmt.Errorf("docgen: write input: %w", err)
		}
	}

	timeout := c.cfg.Timeout + time.Duration(len(docs)-1)*perExtraDocTimeout
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"--headless", "--invisible", "--nologo", "--norestore", "--nolockcheck",
		"--nodefault", "--nofirststartwizard",
		"-env:UserInstallation=" + (&url.URL{Scheme: "file", Path: filepath.ToSlash(profile)}).String(),
		"--convert-to", "pdf:writer_pdf_Export",
		"--outdir", outDir,
	}
	args = append(args, inputs...)

	cmd := exec.CommandContext(runCtx, c.cfg.Bin, args...)
	cmd.Dir = jobDir
	cmd.Env = c.env(jobDir, tmpDir)
	stdout := &cappedBuffer{max: maxCapturedOutput}
	stderr := &cappedBuffer{max: maxCapturedOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = waitDelay
	configureProcessGroup(cmd)

	runErr := cmd.Run()
	// Reap anything the launcher left behind in the group, success or not.
	killProcessGroup(cmd)
	if runCtx.Err() != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return nil, fmt.Errorf("docgen: soffice timed out after %s", timeout)
		}
		return nil, fmt.Errorf("docgen: soffice cancelled: %w", runCtx.Err())
	}
	if runErr != nil {
		return nil, fmt.Errorf("docgen: soffice failed: %v: %s", runErr, bytes.TrimSpace(stderr.Bytes()))
	}

	out := make([][]byte, len(docs))
	for i := range docs {
		path := filepath.Join(outDir, fmt.Sprintf("doc-%04d.pdf", i))
		pdf, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("docgen: soffice produced no PDF for document %d: %s %s",
				i, bytes.TrimSpace(stdout.Bytes()), bytes.TrimSpace(stderr.Bytes()))
		}
		if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
			return nil, fmt.Errorf("docgen: output for document %d is not a PDF", i)
		}
		out[i] = pdf
	}
	return out, nil
}

// env is the complete environment of soffice. Nothing else from the server
// process is inherited.
func (c *Converter) env(home, tmp string) []string {
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}
	env := []string{
		"HOME=" + home,
		"TMPDIR=" + tmp,
		"PATH=" + path,
		"LANG=C.UTF-8",
		// The nixpkgs soffice wrapper is a `bash -e` script that, with no
		// session bus, mkdirs /run/user/<uid>/libreoffice-dbus. A systemd
		// system user (ProtectHome, no login session) cannot, so the wrapper
		// would exit before soffice starts. Any non-empty value skips that.
		"DBUS_SESSION_BUS_ADDRESS=disabled:",
	}
	if c.cfg.FontconfigFile != "" {
		env = append(env, "FONTCONFIG_FILE="+c.cfg.FontconfigFile)
	}
	return env
}

// cappedBuffer keeps the first max bytes written and discards the rest, so a
// chatty soffice cannot grow memory without bound.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *cappedBuffer) Bytes() []byte { return b.buf.Bytes() }

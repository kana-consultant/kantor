//go:build unix

package docgen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fakeSoffice writes an executable shell script standing in for soffice. It
// records its environment, arguments and working directory under dir and
// then runs body. The default body writes "%PDF-1.4 <name>" for every input.
func fakeSoffice(t *testing.T, dir, body string) string {
	t.Helper()
	if body == "" {
		body = `for a in "$@"; do
  case "$a" in *.docx) b=$(basename "$a" .docx); printf '%%PDF-1.4 %s\n' "$b" > "$outdir/$b.pdf";; esac
done`
	}
	script := `#!/bin/sh
env > "` + dir + `/env.txt"
pwd > "` + dir + `/pwd.txt"
printf '%s\n' "$@" > "` + dir + `/args.txt"
outdir=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--outdir" ]; then outdir="$a"; fi
  prev="$a"
done
` + body + "\n"
	path := filepath.Join(dir, "soffice")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConverterBatchWithFakeSoffice(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSoffice(t, dir, "")
	t.Setenv("DATA_ENCRYPTION_KEY", "must-not-leak")
	t.Setenv("JWT_SECRET", "must-not-leak")
	t.Setenv("DATABASE_URL", "postgres://must-not-leak")
	t.Setenv("FONTCONFIG_FILE", "/etc/fonts/test.conf")

	profile := filepath.Join(dir, "profile dir")
	tmpBase := filepath.Join(dir, "jobs")
	if err := os.Mkdir(tmpBase, 0o700); err != nil {
		t.Fatal(err)
	}
	c := NewConverter(ConverterConfig{Bin: bin, ProfileDir: profile, TempDir: tmpBase})
	pdfs, err := c.ConvertBatch(context.Background(), [][]byte{[]byte("a"), []byte("b"), []byte("c")})
	if err != nil {
		t.Fatalf("ConvertBatch: %v", err)
	}
	for i, pdf := range pdfs {
		if want := "%PDF-1.4 doc-000" + strconv.Itoa(i) + "\n"; string(pdf) != want {
			t.Errorf("pdf %d = %q, want %q", i, pdf, want)
		}
	}

	env, _ := os.ReadFile(filepath.Join(dir, "env.txt"))
	for _, secret := range []string{"DATA_ENCRYPTION_KEY", "JWT_SECRET", "DATABASE_URL", "must-not-leak"} {
		if strings.Contains(string(env), secret) {
			t.Errorf("soffice environment leaks %s", secret)
		}
	}
	jobDir := strings.TrimSpace(readFile(t, filepath.Join(dir, "pwd.txt")))
	for _, want := range []string{"LANG=C.UTF-8", "FONTCONFIG_FILE=/etc/fonts/test.conf", "HOME=", "DBUS_SESSION_BUS_ADDRESS=disabled:"} {
		if !strings.Contains(string(env), want) {
			t.Errorf("environment lacks %s", want)
		}
	}
	realBase, err := filepath.EvalSymlinks(tmpBase)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(jobDir, filepath.Join(realBase, workRootName, instancePrefix)) {
		t.Errorf("job dir %s not under %s/%s/%s*", jobDir, tmpBase, workRootName, instancePrefix)
	}
	if _, err := os.Stat(jobDir); !os.IsNotExist(err) {
		t.Errorf("job dir %s not removed: %v", jobDir, err)
	}
	// Only the locked instance dir with its lock file remains.
	var left []string
	filepath.WalkDir(tmpBase, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			left = append(left, path)
		}
		return nil
	})
	if len(left) != 1 || filepath.Base(left[0]) != lockFileName {
		t.Errorf("files left after the job: %v", left)
	}

	args := readFile(t, filepath.Join(dir, "args.txt"))
	if !strings.Contains(args, "-env:UserInstallation=file://"+filepath.ToSlash(dir)+"/profile%20dir") {
		t.Errorf("persistent profile not passed: %s", args)
	}
	if strings.Count(args, ".docx") != 3 {
		t.Errorf("expected all three inputs in one call: %s", args)
	}
	if st, err := os.Stat(profile); err != nil || !st.IsDir() {
		t.Errorf("profile dir not created: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestConverterReportsFailures(t *testing.T) {
	dir := t.TempDir()
	failing := NewConverter(ConverterConfig{Bin: fakeSoffice(t, dir, `echo "boom: source file could not be loaded" >&2; exit 1`)})
	if _, err := failing.Convert(context.Background(), []byte("x")); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("failing soffice: %v", err)
	}

	dir2 := t.TempDir()
	silent := NewConverter(ConverterConfig{Bin: fakeSoffice(t, dir2, `exit 0`)})
	if _, err := silent.Convert(context.Background(), []byte("x")); err == nil || !strings.Contains(err.Error(), "no PDF") {
		t.Errorf("soffice without output: %v", err)
	}

	dir3 := t.TempDir()
	notPDF := NewConverter(ConverterConfig{Bin: fakeSoffice(t, dir3, `echo nope > "$outdir/doc-0000.pdf"`)})
	if _, err := notPDF.Convert(context.Background(), []byte("x")); err == nil || !strings.Contains(err.Error(), "not a PDF") {
		t.Errorf("non-PDF output: %v", err)
	}
}

// TestConverterKillsProcessGroupOnTimeout: soffice forks a grandchild (like
// oosplash -> soffice.bin); on timeout the whole group must die and the job
// dir must still be removed. The script records the grandchild's pid
// synchronously (before it blocks) and the timeout leaves ample headroom, so
// the pid file exists even when the machine is loaded.
func TestConverterKillsProcessGroupOnTimeout(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	bin := fakeSoffice(t, dir, `sleep 60 &
echo $! > "`+pidFile+`"
exec sleep 60`)
	c := NewConverter(ConverterConfig{Bin: bin, Timeout: 5 * time.Second})

	start := time.Now()
	_, err := c.Convert(context.Background(), []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(readFile(t, pidFile)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("grandchild %d survived the timeout", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}

	jobDir := strings.TrimSpace(readFile(t, filepath.Join(dir, "pwd.txt")))
	if _, err := os.Stat(jobDir); !os.IsNotExist(err) {
		t.Errorf("job dir %s not removed after timeout", jobDir)
	}
}

func TestConverterCancelledContext(t *testing.T) {
	c := NewConverter(ConverterConfig{Bin: fakeSoffice(t, t.TempDir(), "")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Occupy the semaphore so the call has to wait for it.
	c.sem <- struct{}{}
	defer func() { <-c.sem }()
	if _, err := c.Convert(ctx, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestConverterSerialisesCalls: concurrent callers never run soffice at the
// same time (the profile directory cannot be shared by two instances).
func TestConverterSerialisesCalls(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "running")
	bin := fakeSoffice(t, dir, `mkdir "`+lock+`" || exit 3
sleep 0.2
rmdir "`+lock+`"
for a in "$@"; do
  case "$a" in *.docx) b=$(basename "$a" .docx); printf '%%PDF-1.4\n' > "$outdir/$b.pdf";; esac
done`)
	c := NewConverter(ConverterConfig{Bin: bin})

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Convert(context.Background(), []byte("x"))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent convert: %v", err)
		}
	}
}

// TestConverterSweepsStaleWorkDirs: job dirs hold plaintext documents. A
// process killed mid-conversion leaves its instance dir behind; the next
// converter removes it, but never the dir of a converter that is alive.
func TestConverterSweepsStaleWorkDirs(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSoffice(t, dir, "")
	base := filepath.Join(dir, "tmp")
	if err := os.Mkdir(base, 0o700); err != nil {
		t.Fatal(err)
	}

	live := NewConverter(ConverterConfig{Bin: bin, TempDir: base, ProfileDir: filepath.Join(dir, "p1")})
	if _, err := live.Convert(context.Background(), []byte("x")); err != nil {
		t.Fatal(err)
	}
	liveDir := live.instDir

	root := filepath.Join(base, workRootName)
	// Remains of a crashed process: unlocked lock file, plaintext input.
	crashed := filepath.Join(root, instancePrefix+"crashed")
	if err := os.MkdirAll(filepath.Join(crashed, "job-1", "in"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{filepath.Join(crashed, lockFileName), filepath.Join(crashed, "job-1", "in", "doc-0000.docx")} {
		if err := os.WriteFile(f, []byte("plaintext"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Crashed before taking the lock, long ago.
	noLock := filepath.Join(root, instancePrefix+"nolock")
	if err := os.Mkdir(noLock, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * instanceGrace)
	if err := os.Chtimes(noLock, old, old); err != nil {
		t.Fatal(err)
	}
	// Being created right now (no lock file yet, fresh).
	fresh := filepath.Join(root, instancePrefix+"fresh")
	if err := os.Mkdir(fresh, 0o700); err != nil {
		t.Fatal(err)
	}
	// Not ours.
	other := filepath.Join(root, "unrelated")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}

	// An unavailable converter does not touch anything.
	NewConverter(ConverterConfig{TempDir: base})
	if _, err := os.Stat(crashed); err != nil {
		t.Fatalf("unavailable converter swept: %v", err)
	}

	next := NewConverter(ConverterConfig{Bin: bin, TempDir: base, ProfileDir: filepath.Join(dir, "p2")})
	for _, gone := range []string{crashed, noLock} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("stale %s not removed: %v", filepath.Base(gone), err)
		}
	}
	for _, kept := range []string{liveDir, fresh, other} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s removed: %v", filepath.Base(kept), err)
		}
	}
	if _, err := next.Convert(context.Background(), []byte("y")); err != nil {
		t.Fatal(err)
	}
	if next.instDir == liveDir {
		t.Fatal("two converters share an instance dir")
	}
	// The live converter still works in its own dir.
	if _, err := live.Convert(context.Background(), []byte("z")); err != nil || live.instDir != liveDir {
		t.Fatalf("live converter after sweep: %v (%s)", err, live.instDir)
	}

	// Once the live converter's lock is released (process gone), its dir
	// is stale too.
	live.instLock.Close()
	NewConverter(ConverterConfig{Bin: bin, TempDir: base})
	if _, err := os.Stat(liveDir); !os.IsNotExist(err) {
		t.Errorf("released instance dir not removed: %v", err)
	}
	if _, err := os.Stat(next.instDir); err != nil {
		t.Errorf("held instance dir removed: %v", err)
	}
}

func TestSecureProfileDir(t *testing.T) {
	base := t.TempDir()

	own := filepath.Join(base, "own")
	if err := os.Mkdir(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(own, 0o755); err != nil {
		t.Fatal(err)
	}
	// Own dir with read/search bits for others: tightened to 0700.
	if err := SecureProfileDir(own); err != nil {
		t.Fatalf("own 0755 dir: %v", err)
	}
	if info, _ := os.Stat(own); info.Mode().Perm() != 0o700 {
		t.Errorf("mode after check = %#o", info.Mode().Perm())
	}

	// Writable by others: someone may have planted a profile.
	open := filepath.Join(base, "open")
	if err := os.Mkdir(open, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := SecureProfileDir(open); err == nil {
		t.Error("0777 dir accepted")
	}

	// A symlink to an own dir is refused too.
	link := filepath.Join(base, "link")
	if err := os.Symlink(own, link); err != nil {
		t.Fatal(err)
	}
	if err := SecureProfileDir(link); err == nil {
		t.Error("symlink accepted")
	}

	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SecureProfileDir(file); err == nil {
		t.Error("regular file accepted")
	}
}

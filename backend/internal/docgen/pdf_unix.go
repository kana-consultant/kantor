//go:build unix

package docgen

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// configureProcessGroup starts soffice in its own process group and makes
// context cancellation kill the whole group, not just the launcher.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
}

// killProcessGroup sends SIGKILL to the process group of cmd. A group that is
// already gone is not an error.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// errInstanceLost means a concurrent sweep removed an instance dir between
// its creation and its lock; the caller retries with a new one.
var errInstanceLost = errors.New("docgen: instance dir removed while locking")

// lockInstance creates dir's lock file and holds an exclusive flock on it.
// The kernel drops the lock when the process dies, which is what marks the
// dir as stale for the next sweep. The descriptor is close-on-exec, so
// soffice never inherits it.
func lockInstance(dir string) (*os.File, error) {
	path := filepath.Join(dir, lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			// Only a sweep that is about to remove the new dir can hold it.
			return nil, errInstanceLost
		}
		return nil, fmt.Errorf("docgen: lock work dir: %w", err)
	}
	// A sweep may have claimed and removed the dir between OpenFile and
	// Flock; then the lock is on an unlinked file and worthless.
	held, err1 := f.Stat()
	onDisk, err2 := os.Stat(path)
	if err1 != nil || err2 != nil || !os.SameFile(held, onDisk) {
		f.Close()
		return nil, errInstanceLost
	}
	return f, nil
}

// claimStaleInstance reports whether no live converter holds dir. When it
// does not, the returned release must be called after removing the dir.
func claimStaleInstance(dir string) (release func(), stale bool) {
	f, err := os.Open(filepath.Join(dir, lockFileName))
	if err != nil {
		// No lock file: either a dir being created right now or the
		// remains of a crash before the lock was taken.
		st, serr := os.Stat(dir)
		if errors.Is(err, os.ErrNotExist) && serr == nil && time.Since(st.ModTime()) > instanceGrace {
			return func() {}, true
		}
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, false
	}
	return func() { f.Close() }, true
}

// SecureProfileDir checks that dir may hold the persistent LibreOffice
// profile: a real directory (not a symlink) owned by this process's user
// and not writable by group or others. Group/other read or search bits on
// an own directory are removed (chmod 0700); anything else is refused.
func SecureProfileDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("is a symbolic link")
	}
	if !info.IsDir() {
		return errors.New("is not a directory")
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("is owned by uid %d, not by the backend user (uid %d)", st.Uid, os.Getuid())
	}
	perm := info.Mode().Perm()
	if perm&0o022 != 0 {
		return fmt.Errorf("is writable by group or others (mode %#o); remove it or chmod 700 it", perm)
	}
	if perm&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("tighten mode %#o to 0700: %w", perm, err)
		}
	}
	return nil
}

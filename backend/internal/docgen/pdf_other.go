//go:build !unix

package docgen

import (
	"errors"
	"os"
	"os/exec"
)

// Process groups are a Unix concept; elsewhere only the launcher is killed.
func configureProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) error { return nil }

var errInstanceLost = errors.New("docgen: instance dir removed while locking")

// Without flock there is no way to tell a live instance dir from a stale
// one, so nothing is swept.
func lockInstance(dir string) (*os.File, error) { return nil, nil }

func claimStaleInstance(dir string) (release func(), stale bool) { return nil, false }

// SecureProfileDir checks that dir may hold the persistent LibreOffice
// profile. Without Unix ownership and mode bits only symlinks and
// non-directories are refused.
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
	return nil
}

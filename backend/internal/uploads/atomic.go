package uploads

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to a temp file next to path and renames it into
// place, so a reader never sees a half-written file. Missing parent
// directories are created with dirMode, and the file gets fileMode regardless
// of the process umask.
func WriteFileAtomic(path string, data []byte, fileMode, dirMode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if err != nil {
			if !closed {
				_ = tmp.Close()
			}
			_ = os.Remove(tmpName)
		}
	}()
	if err = tmp.Chmod(fileMode); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	closed = true
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

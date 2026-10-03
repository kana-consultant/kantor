package uploads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "file.bin")

	if err := WriteFileAtomic(target, []byte("first"), 0o640, 0o750); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	if err := WriteFileAtomic(target, []byte("second"), 0o640, 0o750); err != nil {
		t.Fatalf("WriteFileAtomic (replace): %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil || string(got) != "second" {
		t.Fatalf("content = %q, %v", got, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o640 {
		t.Fatalf("file mode = %o, want 640", mode)
	}
	dirInfo, err := os.Stat(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o750 {
		t.Fatalf("dir mode = %o, want 750", mode)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
)

func TestLocalStorage_CRUD(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	ctx := context.Background()
	key := filepath.Join("tenant_123", "avatars", "avatar.png")
	data := []byte("fake-image-bytes")

	// 1. Put
	if err := store.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "image/png"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 2. Exists
	exists, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if !exists {
		t.Fatalf("Expected file to exist")
	}

	// 3. Get
	rc, meta, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	gotData, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	_ = rc.Close()

	if !bytes.Equal(gotData, data) {
		t.Fatalf("expected data %q, got %q", data, gotData)
	}
	if meta.ContentLength != int64(len(data)) {
		t.Fatalf("expected length %d, got %d", len(data), meta.ContentLength)
	}

	// 4. Delete
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	// 5. Exists after delete
	existsAfter, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists error after delete: %v", err)
	}
	if existsAfter {
		t.Fatalf("Expected file to not exist after delete")
	}

	// 6. Get after delete returns ErrNotFound
	_, _, err = store.Get(ctx, key)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestLocalStorage_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	ctx := context.Background()
	badKeys := []string{
		"../escape.txt",
		"folder/../../escape.txt",
		"",
		"   ",
	}

	for _, k := range badKeys {
		if err := store.Put(ctx, k, bytes.NewReader([]byte("hack")), 4, "text/plain"); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("expected ErrInvalidPath for key %q, got %v", k, err)
		}
	}
}

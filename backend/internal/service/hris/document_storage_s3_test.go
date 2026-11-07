package hris

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kana-consultant/kantor/backend/internal/security"
	"github.com/kana-consultant/kantor/backend/internal/storage"
)

func TestDocumentStore_WithStorageProvider(t *testing.T) {
	tmpDir := t.TempDir()
	localStorage, err := storage.NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	encrypter, err := security.NewEncrypter("test-secret-key-32-chars-long-12")
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}

	store := NewDocumentStoreWithStorage(localStorage, encrypter)

	tenantID := uuid.New().String()
	docID := uuid.New().String()
	pdfContent := []byte("%PDF-1.4 test document content for object storage")

	// 1. Write
	stored, err := store.Write(tenantID, "payslip", docID, "slip", pdfContent)
	if err != nil {
		t.Fatalf("store.Write failed: %v", err)
	}
	if stored.Path == "" || stored.SHA256 == "" {
		t.Fatalf("invalid stored document result: %+v", stored)
	}

	// 2. Read
	decrypted, err := store.Read(tenantID, stored.Path, stored.SHA256)
	if err != nil {
		t.Fatalf("store.Read failed: %v", err)
	}
	if !bytes.Equal(decrypted, pdfContent) {
		t.Fatalf("decrypted content mismatch: got %q, want %q", decrypted, pdfContent)
	}

	// 3. Exists on underlying storage
	exists, err := localStorage.Exists(context.Background(), stored.Path)
	if err != nil || !exists {
		t.Fatalf("underlying object should exist in storage: %v", err)
	}

	// 4. Delete
	if err := store.Delete(tenantID, stored.Path); err != nil {
		t.Fatalf("store.Delete failed: %v", err)
	}

	// 5. Exists after delete
	existsAfter, err := localStorage.Exists(context.Background(), stored.Path)
	if err != nil || existsAfter {
		t.Fatalf("underlying object should not exist after delete: %v", err)
	}

	// 6. Read after delete returns ErrDocumentNotFound
	_, err = store.Read(tenantID, stored.Path, stored.SHA256)
	if err != ErrDocumentNotFound {
		t.Fatalf("expected ErrDocumentNotFound, got %v", err)
	}
}

package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotFound is returned when an object is not found in the storage.
	ErrNotFound = errors.New("storage: object not found")
	// ErrInvalidPath is returned when a path is empty or attempts directory traversal.
	ErrInvalidPath = errors.New("storage: invalid path")
)

// StorageMetadata contains metadata about a stored file.
type StorageMetadata struct {
	ContentType   string
	ContentLength int64
	ModTime       time.Time
}

// Storage is the abstraction for file storage operations (local disk or S3-compatible).
type Storage interface {
	// Put writes data from the given reader into the specified key.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// Get retrieves the object data and metadata for the specified key.
	// The caller is responsible for closing the returned ReadCloser.
	Get(ctx context.Context, key string) (io.ReadCloser, StorageMetadata, error)

	// Delete removes the object at the specified key. If the object does not exist,
	// Delete returns nil (idempotent).
	Delete(ctx context.Context, key string) error

	// Exists checks if an object exists at the specified key.
	Exists(ctx context.Context, key string) (bool, error)
}

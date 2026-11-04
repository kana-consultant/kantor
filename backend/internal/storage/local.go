package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/uploads"
)

// LocalStorage implements the Storage interface for local filesystem / Docker volumes.
type LocalStorage struct {
	baseDir string
}

// NewLocalStorage initializes a local filesystem storage rooted at baseDir.
func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create local storage dir: %w", err)
	}
	return &LocalStorage{baseDir: abs}, nil
}

func (s *LocalStorage) resolvePath(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", ErrInvalidPath
	}
	// Clean and normalize path separators
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", ErrInvalidPath
	}

	target := filepath.Join(s.baseDir, cleaned)
	prefix := s.baseDir + string(os.PathSeparator)
	if target != s.baseDir && !strings.HasPrefix(target, prefix) {
		return "", ErrInvalidPath
	}
	return target, nil
}

// Put writes data atomically to the specified key.
func (s *LocalStorage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	targetPath, err := s.resolvePath(key)
	if err != nil {
		return err
	}

	var data []byte
	if size > 0 {
		data = make([]byte, 0, size)
	}
	data, err = io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read stream: %w", err)
	}

	if err := uploads.WriteFileAtomic(targetPath, data, 0o640, 0o750); err != nil {
		return fmt.Errorf("write atomic: %w", err)
	}
	return nil
}

// Get opens the file and returns a ReadCloser along with its metadata.
func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, StorageMetadata, error) {
	select {
	case <-ctx.Done():
		return nil, StorageMetadata{}, ctx.Err()
	default:
	}

	targetPath, err := s.resolvePath(key)
	if err != nil {
		return nil, StorageMetadata{}, err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, StorageMetadata{}, ErrNotFound
		}
		return nil, StorageMetadata{}, err
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return nil, StorageMetadata{}, err
	}

	ct := mime.TypeByExtension(filepath.Ext(targetPath))
	if ct == "" {
		ct = "application/octet-stream"
	}

	return file, StorageMetadata{
		ContentType:   ct,
		ContentLength: info.Size(),
		ModTime:       info.ModTime(),
	}, nil
}

// Delete removes the file. It is idempotent (returns nil if not found).
func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	targetPath, err := s.resolvePath(key)
	if err != nil {
		return err
	}

	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Exists checks if the file exists and is not a directory.
func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	targetPath, err := s.resolvePath(key)
	if err != nil {
		return false, err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

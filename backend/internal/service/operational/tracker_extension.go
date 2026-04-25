package operational

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var ErrTrackerExtensionUnavailable = errors.New("tracker extension package is unavailable")

func (s *TrackerService) BuildExtensionArchive(ctx context.Context) ([]byte, string, error) {
	extensionDir, err := resolveExtensionDir()
	if err != nil {
		slog.ErrorContext(ctx, "failed to resolve extension directory", "error", err)
		return nil, "", ErrTrackerExtensionUnavailable
	}
	slog.InfoContext(ctx, "resolved extension directory", "path", extensionDir)

	buffer := bytes.NewBuffer(nil)
	archive := zip.NewWriter(buffer)

	if err := filepath.WalkDir(extensionDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(extensionDir, path)
		if err != nil {
			return fmt.Errorf("resolve relative extension path: %w", err)
		}

		zipPath := filepath.ToSlash(relativePath)
		writer, err := archive.Create(zipPath)
		if err != nil {
			return fmt.Errorf("create zip entry %s: %w", zipPath, err)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open extension file %s: %w", path, err)
		}
		defer file.Close()

		if _, err := io.Copy(writer, file); err != nil {
			return fmt.Errorf("write extension file %s: %w", path, err)
		}

		return nil
	}); err != nil {
		_ = archive.Close()
		slog.ErrorContext(ctx, "failed to walk extension directory", "error", err, "dir", extensionDir)
		return nil, "", fmt.Errorf("archive tracker extension: %w", err)
	}

	if err := archive.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize tracker extension archive: %w", err)
	}

	return buffer.Bytes(), "kantor-activity-tracker.zip", nil
}

func resolveExtensionDir() (string, error) {
	candidates := []string{
		"extension",
		filepath.Join("..", "extension"),
		filepath.Join("..", "..", "extension"),
		filepath.Join("..", "..", "..", "extension"),
		filepath.Join(string(filepath.Separator), "app", "extension"),
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			continue
		}

		manifestPath := filepath.Join(candidate, "manifest.json")
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}

		absolutePath, err := filepath.Abs(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve extension path: %w", err)
		}

		return absolutePath, nil
	}

	return "", errors.New("extension directory not found")
}

func sanitizeContentDispositionFilename(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "download.zip"
	}

	return strings.ReplaceAll(trimmed, "\"", "")
}

package auth

import (
	"errors"
	"testing"

	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
)

func TestNormalizeRefreshRotationError_MapsNotFoundToInvalidRefreshToken(t *testing.T) {
	t.Parallel()

	got := normalizeRefreshRotationError(authrepo.ErrNotFound)
	if !errors.Is(got, ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken, got %v", got)
	}
}

func TestNormalizeRefreshRotationError_PassthroughUnknownError(t *testing.T) {
	t.Parallel()

	unknown := errors.New("boom")
	got := normalizeRefreshRotationError(unknown)
	if !errors.Is(got, unknown) {
		t.Fatalf("expected original error to pass through, got %v", got)
	}
}

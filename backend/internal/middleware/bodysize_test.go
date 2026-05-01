package middleware

import (
	"errors"
	"net/http"
	"testing"
)

func TestIsBodyTooLargeError_WithMaxBytesError(t *testing.T) {
	err := &http.MaxBytesError{Limit: 1024}
	if !IsBodyTooLargeError(err) {
		t.Fatal("expected max bytes error to be detected")
	}
}

func TestIsBodyTooLargeError_WithGenericError(t *testing.T) {
	if IsBodyTooLargeError(errors.New("invalid multipart")) {
		t.Fatal("did not expect generic multipart error to be treated as body too large")
	}
}

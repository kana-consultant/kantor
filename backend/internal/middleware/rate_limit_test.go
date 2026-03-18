package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewIPRateLimitBlocksAfterConfiguredAttempts(t *testing.T) {
	middleware := NewIPRateLimit(2, time.Minute, "RATE_LIMITED", "Too many requests")
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		request.RemoteAddr = "203.0.113.10:12345"
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected first requests to pass, got %d", recorder.Code)
		}
	}

	blockedRequest := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	blockedRequest.RemoteAddr = "203.0.113.10:12345"
	blockedRecorder := httptest.NewRecorder()

	handler.ServeHTTP(blockedRecorder, blockedRequest)

	if blockedRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit is exceeded, got %d", blockedRecorder.Code)
	}

	if blockedRecorder.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header to be set")
	}
}

func TestNewIPRateLimitIsScopedPerIP(t *testing.T) {
	middleware := NewIPRateLimit(1, time.Minute, "RATE_LIMITED", "Too many requests")
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	firstRequest := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	firstRequest.RemoteAddr = "203.0.113.10:1111"
	firstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstRecorder, firstRequest)

	secondRequest := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	secondRequest.RemoteAddr = "203.0.113.11:2222"
	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, secondRequest)

	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("expected first IP to pass, got %d", firstRecorder.Code)
	}

	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("expected second IP to pass independently, got %d", secondRecorder.Code)
	}
}

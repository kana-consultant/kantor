package middleware

import (
	"context"
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

func TestRateLimitResetsAfterWindow(t *testing.T) {
	limiter := &ipRateLimiter{
		maxRequests: 1,
		window:      time.Minute,
		entries:     make(map[string]rateLimitEntry),
	}
	now := time.Now().UTC()

	if d := limiter.retryAfter("10.0.0.1", now); d > 0 {
		t.Fatal("expected first request to pass")
	}
	if d := limiter.retryAfter("10.0.0.1", now.Add(30*time.Second)); d == 0 {
		t.Fatal("expected block within window")
	}
	if d := limiter.retryAfter("10.0.0.1", now.Add(61*time.Second)); d > 0 {
		t.Fatal("expected pass after window reset")
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

func requestWithPrincipal(userID string, remoteAddr string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r.RemoteAddr = remoteAddr
	ctx := context.WithValue(r.Context(), principalContextKey, Principal{UserID: userID})
	return r.WithContext(ctx)
}

func TestNewUserRateLimitScopesByUserID(t *testing.T) {
	mw := NewUserRateLimit(2, time.Minute, "RATE_LIMITED", "Too many requests")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for range 2 {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, requestWithPrincipal("user-a", "10.0.0.1:1111"))
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected pass for user-a, got %d", recorder.Code)
		}
	}

	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, requestWithPrincipal("user-a", "10.0.0.1:1111"))
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for user-a after limit, got %d", blocked.Code)
	}

	// Different user from the same IP must not be punished by user-a's quota.
	other := httptest.NewRecorder()
	handler.ServeHTTP(other, requestWithPrincipal("user-b", "10.0.0.1:1111"))
	if other.Code != http.StatusOK {
		t.Fatalf("expected user-b to pass independently, got %d", other.Code)
	}
}

func TestNewUserRateLimitFallsBackToIP(t *testing.T) {
	// When no principal is set, the limiter should still bound traffic by IP.
	mw := NewUserRateLimit(1, time.Minute, "RATE_LIMITED", "Too many requests")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/x", nil)
	r1.RemoteAddr = "10.0.0.2:1234"
	handler.ServeHTTP(first, r1)
	if first.Code != http.StatusOK {
		t.Fatalf("expected first to pass, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	r2.RemoteAddr = "10.0.0.2:1234"
	handler.ServeHTTP(second, r2)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected anonymous fallback to throttle by IP, got %d", second.Code)
	}
}

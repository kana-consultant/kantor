package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	backendauth "github.com/kana-consultant/kantor/backend/internal/auth"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	"github.com/kana-consultant/kantor/backend/internal/repository"
)

type testDBTX struct{}

func (testDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (testDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }
func (testDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	var tag pgconn.CommandTag
	return tag, nil
}
func (testDBTX) Begin(context.Context) (pgx.Tx, error)                  { return nil, nil }
func (testDBTX) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }

func TestAuthMiddlewareRejectsInactiveUser(t *testing.T) {
	parseToken := func(string) (*backendauth.AccessClaims, error) {
		return &backendauth.AccessClaims{
			TenantID: "tenant-1",
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "user-1",
				ID:      "jti-1",
			},
		}, nil
	}
	loadPermissions := func(context.Context, string) (*rbac.CachedPermissions, error) {
		return &rbac.CachedPermissions{
			IsActive:     false,
			IsSuperAdmin: false,
			ModuleRoles:  map[string]rbac.ModuleRole{},
			Permissions:  map[string]bool{},
			CachedAt:     time.Now().UTC(),
			TTL:          time.Minute,
		}, nil
	}

	nextCalled := false
	handler := AuthMiddleware(parseToken, loadPermissions, nil, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = req.WithContext(repository.WithConn(req.Context(), testDBTX{}))
	req.Header.Set("Authorization", "Bearer dummy-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for inactive user, got %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("expected request pipeline to stop for inactive user")
	}
	if !strings.Contains(rec.Body.String(), "INACTIVE_USER") {
		t.Fatalf("expected INACTIVE_USER response code, got body: %s", rec.Body.String())
	}
}

func TestAuthMiddlewareAllowsActiveUserAndInjectsPrincipal(t *testing.T) {
	parseToken := func(string) (*backendauth.AccessClaims, error) {
		return &backendauth.AccessClaims{
			TenantID: "tenant-1",
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "user-1",
				ID:      "jti-1",
			},
		}, nil
	}
	loadPermissions := func(context.Context, string) (*rbac.CachedPermissions, error) {
		return &rbac.CachedPermissions{
			IsActive:     true,
			IsSuperAdmin: true,
			ModuleRoles:  map[string]rbac.ModuleRole{},
			Permissions: map[string]bool{
				"dashboard:view": true,
			},
			CachedAt: time.Now().UTC(),
			TTL:      time.Minute,
		}, nil
	}

	handler := AuthMiddleware(parseToken, loadPermissions, nil, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("expected principal in context")
		}
		if principal.UserID != "user-1" {
			t.Fatalf("expected user-1, got %s", principal.UserID)
		}
		if principal.TenantID != "tenant-1" {
			t.Fatalf("expected tenant-1, got %s", principal.TenantID)
		}
		if !principal.IsSuperAdmin {
			t.Fatal("expected super admin flag to be true")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = req.WithContext(repository.WithConn(req.Context(), testDBTX{}))
	req.Header.Set("Authorization", "Bearer dummy-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}


func TestPATAuthRateLimiter(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		limiter := NewPATAuthRateLimiter(5, 3, time.Minute)
		
		// Should allow first 3 requests from same IP with DIFFERENT tokens
		// (token limit is per-token, so we need different tokens to test IP limit)
		for i := 0; i < 3; i++ {
			token := "pat_test_token_" + string(rune('a'+i))
			retryAfter := limiter.checkLimit("192.168.1.1", token)
			if retryAfter > 0 {
				t.Fatalf("request %d should be allowed, got retry after %v", i+1, retryAfter)
			}
		}
	})
	
	t.Run("blocks requests over IP limit", func(t *testing.T) {
		limiter := NewPATAuthRateLimiter(3, 10, time.Minute)
		
		// Exhaust IP limit
		for i := 0; i < 3; i++ {
			limiter.checkLimit("192.168.1.1", "pat_token_"+string(rune(i)))
		}
		
		// Next request should be blocked
		retryAfter := limiter.checkLimit("192.168.1.1", "pat_token_new")
		if retryAfter <= 0 {
			t.Fatal("expected request to be rate limited by IP")
		}
	})
	
	t.Run("blocks requests over token limit", func(t *testing.T) {
		limiter := NewPATAuthRateLimiter(10, 2, time.Minute)
		
		// Exhaust token limit from different IPs
		limiter.checkLimit("192.168.1.1", "pat_same_token_xyz")
		limiter.checkLimit("192.168.1.2", "pat_same_token_xyz")
		
		// Third attempt with same token should be blocked
		retryAfter := limiter.checkLimit("192.168.1.3", "pat_same_token_xyz")
		if retryAfter <= 0 {
			t.Fatal("expected request to be rate limited by token")
		}
	})
	
	t.Run("different IPs and tokens are independent", func(t *testing.T) {
		limiter := NewPATAuthRateLimiter(2, 2, time.Minute)
		
		// Use up limits for IP1 + Token1
		limiter.checkLimit("192.168.1.1", "pat_token1")
		limiter.checkLimit("192.168.1.1", "pat_token1")
		
		// IP2 + Token2 should still work
		retryAfter := limiter.checkLimit("192.168.1.2", "pat_token2")
		if retryAfter > 0 {
			t.Fatal("different IP and token should not be rate limited")
		}
	})
	
	t.Run("resets after window expires", func(t *testing.T) {
		limiter := NewPATAuthRateLimiter(1, 1, 10*time.Millisecond)
		
		// Exhaust limit
		limiter.checkLimit("192.168.1.1", "pat_token")
		
		// Should be blocked
		retryAfter := limiter.checkLimit("192.168.1.1", "pat_token")
		if retryAfter <= 0 {
			t.Fatal("expected to be rate limited")
		}
		
		// Wait for window to expire
		time.Sleep(15 * time.Millisecond)
		
		// Should be allowed again
		retryAfter = limiter.checkLimit("192.168.1.1", "pat_token")
		if retryAfter > 0 {
			t.Fatal("should be allowed after window reset")
		}
	})
}

func TestAuthMiddlewarePATRateLimiting(t *testing.T) {
	parseToken := func(string) (*backendauth.AccessClaims, error) {
		return nil, errors.New("not a PAT")
	}
	loadPermissions := func(context.Context, string) (*rbac.CachedPermissions, error) {
		return &rbac.CachedPermissions{
			IsActive:     true,
			IsSuperAdmin: false,
			ModuleRoles:  map[string]rbac.ModuleRole{},
			Permissions:  map[string]bool{},
			CachedAt:     time.Now().UTC(),
			TTL:          time.Minute,
		}, nil
	}
	authenticatePAT := func(ctx context.Context, token string) (string, string, *string, error) {
		return "user-1", "tenant-1", nil, nil
	}
	
	limiter := NewPATAuthRateLimiter(2, 2, time.Minute)
	
	handler := AuthMiddleware(parseToken, loadPermissions, nil, authenticatePAT, limiter)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)
	
	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req = req.WithContext(repository.WithConn(req.Context(), testDBTX{}))
		req.Header.Set("Authorization", "Bearer kantor_pat_test_token_12345")
		rec := httptest.NewRecorder()
		
		handler.ServeHTTP(rec, req)
		
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d, body: %s", i+1, rec.Code, rec.Body.String())
		}
	}
	
	// Third request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req = req.WithContext(repository.WithConn(req.Context(), testDBTX{}))
	req.Header.Set("Authorization", "Bearer kantor_pat_test_token_12345")
	rec := httptest.NewRecorder()
	
	handler.ServeHTTP(rec, req)
	
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	
	body := rec.Body.String()
	if !strings.Contains(body, "PAT_RATE_LIMIT_EXCEEDED") {
		t.Fatalf("expected PAT_RATE_LIMIT_EXCEEDED error code, got: %s", body)
	}
	if !strings.Contains(body, "retry_after_seconds") {
		t.Fatalf("expected retry_after_seconds in response, got: %s", body)
	}
}

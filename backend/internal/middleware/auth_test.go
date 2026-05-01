package middleware

import (
	"context"
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
	handler := AuthMiddleware(parseToken, loadPermissions, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	handler := AuthMiddleware(parseToken, loadPermissions, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	backendauth "github.com/kana-consultant/kantor/backend/internal/auth"
	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
)

// serviceRevocationTestDB is a tiny DBTX for the service-level Revoke tests.
// It records every Exec call and returns whatever the test pre-loads.
type serviceRevocationTestDB struct {
	execFn func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (d *serviceRevocationTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("Query not used by service revocation tests")
}
func (d *serviceRevocationTestDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}
func (d *serviceRevocationTestDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return d.execFn(ctx, sql, args...)
}
func (d *serviceRevocationTestDB) Begin(context.Context) (pgx.Tx, error) { return nil, nil }
func (d *serviceRevocationTestDB) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

// serviceRevocationTestPool wraps a serviceRevocationTestDB as a
// *pgxpool.Pool-shaped Acquire. The pool is only used by background jobs;
// our revocation path uses the injected store directly, so an empty
// pool is fine for these tests.
func newServiceRevocationTestPool() *pgxpool.Pool { return nil }

// unusedStore exists to satisfy the linter when the test pool is nil; the
// service never calls it because the tests use the injected store.
var _ = rbac.NewPermissionCache

// TestServiceRevokeAccessTokenPersistsJTI confirms that calling
// RevokeAccessToken writes the parsed JTI to the revocation store with
// the token's own expiry, and the user/tenant advisory metadata.
func TestServiceRevokeAccessTokenPersistsJTI(t *testing.T) {
	t.Parallel()

	secret := "test-secret-32-chars-long-enough!!"
	tm := backendauth.NewTokenManager(secret, time.Hour, 24*time.Hour)
	signed, _, err := tm.GenerateAccessToken("user-42", "tenant-7", time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	var capturedJTI string
	var capturedExp time.Time
	var capturedUserID, capturedTenantID string
	db := &serviceRevocationTestDB{
		execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
			if len(args) < 5 {
				t.Fatalf("expected 5 args (jti, expires, now, user, tenant), got %d", len(args))
			}
			capturedJTI, _ = args[0].(string)
			capturedExp, _ = args[1].(time.Time)
			capturedUserID, _ = args[3].(string)
			capturedTenantID, _ = args[4].(string)
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	cfg := config.Config{JWTSecret: secret, JWTAccessExpiry: time.Hour, JWTRefreshExpiry: 24 * time.Hour}
	store := backendauth.NewPostgresRevocationStore(db)
	svc := NewServiceForTest(cfg, tm, store)

	svc.RevokeAccessToken(context.Background(), signed)

	if capturedJTI == "" {
		t.Fatal("expected store to receive a JTI")
	}
	if capturedUserID != "user-42" {
		t.Fatalf("user_id = %q, want user-42", capturedUserID)
	}
	if capturedTenantID != "tenant-7" {
		t.Fatalf("tenant_id = %q, want tenant-7", capturedTenantID)
	}
	// JWT stores exp as a Unix timestamp with second precision, so the
	// round-tripped value loses any sub-second component of the original
	// exp. Compare with second-truncation tolerance.
	expectedExp := time.Now().UTC().Add(time.Hour)
	if delta := capturedExp.Sub(expectedExp); delta > time.Second || delta < -time.Second {
		t.Fatalf("expires_at = %v, want ~%v (delta %v)", capturedExp, expectedExp, delta)
	}
}

// TestServiceRevokeAccessTokenSilentlyIgnoresBadToken confirms that a
// garbage token is a no-op (no store call, no error). Logout must not
// fail because the incoming token is malformed.
func TestServiceRevokeAccessTokenSilentlyIgnoresBadToken(t *testing.T) {
	t.Parallel()

	execCalled := false
	db := &serviceRevocationTestDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			execCalled = true
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	cfg := config.Config{JWTSecret: "test-secret-32-chars-long-enough!!", JWTAccessExpiry: time.Hour, JWTRefreshExpiry: 24 * time.Hour}
	tm := backendauth.NewTokenManager(cfg.JWTSecret, cfg.JWTAccessExpiry, cfg.JWTRefreshExpiry)
	store := backendauth.NewPostgresRevocationStore(db)
	svc := NewServiceForTest(cfg, tm, store)

	svc.RevokeAccessToken(context.Background(), "definitely.not.a.jwt")

	if execCalled {
		t.Fatal("Exec must not be called for a malformed token")
	}
}

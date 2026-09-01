package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// revocationTestDB is a minimal DBTX for the revocation store tests. It
// captures the SQL + args from Exec and returns whatever the test pre-loads
// via execFn/queryRowFn. Mirrors the hand-rolled stub pattern in
// repository/auth/repository_rotate_test.go so the new tests fit the codebase.
type revocationTestDB struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (d *revocationTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("Query not used by revocation store")
}

func (d *revocationTestDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return d.queryRowFn(ctx, sql, args...)
}

func (d *revocationTestDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return d.execFn(ctx, sql, args...)
}

func (d *revocationTestDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("Begin not used by revocation store")
}

func (d *revocationTestDB) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

// revocationTestRow is a tiny pgx.Row implementation that delegates Scan to
// the test's scanFn closure.
type revocationTestRow struct {
	scanFn func(dest ...any) error
}

func (r *revocationTestRow) Scan(dest ...any) error { return r.scanFn(dest...) }

func TestPostgresRevocationStore_RevokePersistsJTI(t *testing.T) {
	t.Parallel()

	var capturedSQL string
	var capturedArgs []any

	db := &revocationTestDB{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			capturedSQL = sql
			capturedArgs = args
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	store := NewPostgresRevocationStore(db)
	expiry := time.Now().UTC().Add(time.Hour)
	store.Revoke(context.Background(), "jti-abc", expiry, "user-1", "tenant-1")

	if capturedSQL == "" {
		t.Fatal("expected Exec to be called for Revoke")
	}
	if len(capturedArgs) < 3 {
		t.Fatalf("expected at least JTI + expiry + now args, got %d", len(capturedArgs))
	}
	if capturedArgs[0] != "jti-abc" {
		t.Fatalf("first arg = %v, want jti-abc", capturedArgs[0])
	}
}

func TestPostgresRevocationStore_RevokeSkipsAlreadyExpired(t *testing.T) {
	t.Parallel()

	execCalled := false
	db := &revocationTestDB{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalled = true
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	store := NewPostgresRevocationStore(db)
	// expiry in the past — Revoke should be a no-op
	store.Revoke(context.Background(), "jti-abc", time.Now().UTC().Add(-time.Hour), "user-1", "tenant-1")

	if execCalled {
		t.Fatal("Exec must not be called when expires_at is already in the past")
	}
}

func TestPostgresRevocationStore_RevokeSkipsEmptyJTI(t *testing.T) {
	t.Parallel()

	execCalled := false
	db := &revocationTestDB{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalled = true
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	store := NewPostgresRevocationStore(db)
	store.Revoke(context.Background(), "", time.Now().UTC().Add(time.Hour), "", "")

	if execCalled {
		t.Fatal("Exec must not be called when JTI is empty")
	}
}

func TestPostgresRevocationStore_IsRevokedReturnsTrueWhenRowExists(t *testing.T) {
	t.Parallel()

	db := &revocationTestDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &revocationTestRow{scanFn: func(dest ...any) error {
				if len(dest) != 1 {
					t.Fatalf("expected 1 dest, got %d", len(dest))
				}
				out, ok := dest[0].(*bool)
				if !ok {
					t.Fatalf("dest[0] type = %T, want *bool", dest[0])
				}
				*out = true
				return nil
			}}
		},
	}

	store := NewPostgresRevocationStore(db)
	if !store.IsRevoked(context.Background(), "jti-abc") {
		t.Fatal("expected IsRevoked to return true when DB row exists")
	}
}

func TestPostgresRevocationStore_IsRevokedReturnsFalseOnNoRows(t *testing.T) {
	t.Parallel()

	db := &revocationTestDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &revocationTestRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}

	store := NewPostgresRevocationStore(db)
	if store.IsRevoked(context.Background(), "jti-missing") {
		t.Fatal("expected IsRevoked to return false when no row exists")
	}
}

func TestPostgresRevocationStore_IsRevokedReturnsFalseOnEmptyJTI(t *testing.T) {
	t.Parallel()

	queryRowCalled := false
	db := &revocationTestDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			queryRowCalled = true
			return &revocationTestRow{scanFn: func(_ ...any) error { return nil }}
		},
	}

	store := NewPostgresRevocationStore(db)
	if store.IsRevoked(context.Background(), "") {
		t.Fatal("expected IsRevoked to return false on empty JTI")
	}
	if queryRowCalled {
		t.Fatal("QueryRow must not be called on empty JTI")
	}
}

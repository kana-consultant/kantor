package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RevocationStore persists revoked JWT identifiers (JTIs) so an explicit
// logout — or any other revocation event — is honoured across every backend
// instance, not just the one that received the request. The contract is
// intentionally minimal: a JTI is either revoked (until its own expires_at)
// or not. Implementations must treat unknown JTIs as "not revoked" and must
// not fail the caller on a transient store error: a failed lookup MUST
// return false (fail-open) so an outage of the revocation store does not
// take the whole API down. Revoke() is the one method that may surface
// errors so the caller can audit them.
type RevocationStore interface {
	// Revoke records the JTI as revoked until at least expiresAt. Calls with
	// an empty jti or with expiresAt already in the past are no-ops. userID
	// and tenantID are advisory metadata for audit/diagnostic purposes and
	// may be empty.
	Revoke(ctx context.Context, jti string, expiresAt time.Time, userID string, tenantID string) error
	// IsRevoked reports whether the JTI is currently revoked. Must return
	// false (not an error) when the store cannot reach its backing data,
	// and must return false for an empty jti.
	IsRevoked(ctx context.Context, jti string) bool
}

// PostgresRevocationStore is the production RevocationStore. It writes
// revoked JTIs to the global `jwt_revocations` table and reads them back
// for every authenticated request. The table is global (no RLS) on
// purpose: a token's tenant is not known before the middleware decides
// whether to honour the JTI, and the JTI itself is unique enough to be the
// only key needed.
type PostgresRevocationStore struct {
	db RevocationDBTX
}

// RevocationDBTX is the subset of *pgxpool.Pool/pgx.Conn/pgx.Tx that the
// store uses. Defined here so hand-rolled test stubs can satisfy it
// without pulling pgx into the public surface.
type RevocationDBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// NewPostgresRevocationStore returns a store backed by the supplied DBTX.
// In production this is the pgxpool.Pool from app.go.
func NewPostgresRevocationStore(db RevocationDBTX) *PostgresRevocationStore {
	return &PostgresRevocationStore{db: db}
}

// Revoke inserts a row into jwt_revocations. ON CONFLICT (jti) DO UPDATE
// keeps the latest expires_at so a token re-revoked with a later expiry
// is honoured. user_id and tenant_id are diagnostic only.
func (s *PostgresRevocationStore) Revoke(ctx context.Context, jti string, expiresAt time.Time, userID string, tenantID string) error {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return nil
	}
	if !expiresAt.After(time.Now().UTC()) {
		return nil
	}

	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO jwt_revocations (jti, expires_at, revoked_at, user_id, tenant_id)
		VALUES ($1, $2, $3, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid)
		ON CONFLICT (jti) DO UPDATE
		SET expires_at = GREATEST(jwt_revocations.expires_at, EXCLUDED.expires_at),
		    revoked_at = EXCLUDED.revoked_at,
		    user_id    = COALESCE(EXCLUDED.user_id, jwt_revocations.user_id),
		    tenant_id  = COALESCE(EXCLUDED.tenant_id, jwt_revocations.tenant_id)
	`, jti, expiresAt.UTC(), now, strings.TrimSpace(userID), strings.TrimSpace(tenantID))
	if err != nil {
		return fmt.Errorf("revoke jti: %w", err)
	}
	return nil
}

// IsRevoked returns true when a non-expired row exists for the JTI. Expired
// rows are ignored — the natural JWT expiry has already passed, so the
// token cannot be used regardless. A lookup error or empty JTI returns
// false (fail-open) so a revocation-store outage does not break
// authentication for every user.
func (s *PostgresRevocationStore) IsRevoked(ctx context.Context, jti string) bool {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return false
	}

	var found bool
	row := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM jwt_revocations
			WHERE jti = $1 AND expires_at > NOW()
		)
	`, jti)
	if err := row.Scan(&found); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.WarnContext(ctx, "jwt revocation lookup failed; failing open", "error", err)
		}
		return false
	}
	return found
}

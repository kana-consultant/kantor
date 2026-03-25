package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/repository"
	"github.com/kana-consultant/kantor/backend/internal/response"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// setupTenantConn configures a pooled connection for RLS.
// It sets the app.current_tenant GUC using a parameterized query (guaranteed
// clean pgx protocol) and, when running as superuser (Docker dev), also sets
// the session role to kantor_app so that RLS policies are enforced.
func setupTenantConn(ctx context.Context, conn *pgxpool.Conn, tenantID string) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		return fmt.Errorf("invalid tenant id: %w", err)
	}

	// set_config via parameterized QueryRow — fully consumes the response.
	var ignored string
	if err := conn.QueryRow(ctx,
		"SELECT set_config('app.current_tenant', $1, false)", tenantID,
	).Scan(&ignored); err != nil {
		return fmt.Errorf("set tenant guc: %w", err)
	}

	// In Docker dev the DB user is superuser; SET ROLE so RLS applies.
	// In production (NixOS) the user is already non-superuser, so this is a no-op.
	var isSuperuser bool
	if err := conn.QueryRow(ctx,
		"SELECT current_setting('is_superuser') = 'on'",
	).Scan(&isSuperuser); err != nil {
		return fmt.Errorf("check superuser: %w", err)
	}
	if isSuperuser {
		if _, err := conn.Exec(ctx, "SET ROLE kantor_app"); err != nil {
			return fmt.Errorf("set role: %w", err)
		}
	}

	return nil
}

// TenantMiddleware resolves the tenant from the Host header, acquires a
// dedicated connection from the pool, and configures RLS by setting the
// app.current_tenant GUC and (when superuser) downgrading to kantor_app.
func TenantMiddleware(pool *pgxpool.Pool, resolver *tenant.Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info, err := resolver.Resolve(r.Context(), r.Host)
			if err != nil {
				response.WriteError(w, http.StatusBadRequest, "UNKNOWN_TENANT", "Could not resolve tenant for this domain", nil)
				return
			}

			conn, err := pool.Acquire(r.Context())
			if err != nil {
				response.WriteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Could not acquire database connection", nil)
				return
			}

			if err := setupTenantConn(r.Context(), conn, info.ID); err != nil {
				conn.Release()
				response.WriteError(w, http.StatusInternalServerError, "TENANT_SETUP_FAILED", "Failed to configure tenant context", nil)
				return
			}

			// Store tenant info and the scoped connection in context.
			ctx := tenant.WithInfo(r.Context(), info)
			ctx = repository.WithConn(ctx, conn)

			// Ensure we clean up the connection regardless of outcome.
			defer func() {
				_, _ = conn.Exec(context.Background(), "RESET ALL")
				conn.Release()
			}()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ForEachTenant runs fn once per active tenant, setting up proper RLS context.
// Used for background jobs that need to iterate over all tenants.
func ForEachTenant(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context, t tenant.Info) error) error {
	rows, err := pool.Query(ctx, "SELECT id::text, slug, name FROM tenants WHERE is_active = true")
	if err != nil {
		return err
	}
	defer rows.Close()

	var tenants []tenant.Info
	for rows.Next() {
		var t tenant.Info
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name); err != nil {
			return err
		}
		tenants = append(tenants, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, t := range tenants {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return err
		}

		if err := setupTenantConn(ctx, conn, t.ID); err != nil {
			conn.Release()
			return err
		}

		tenantCtx := tenant.WithInfo(ctx, t)
		tenantCtx = repository.WithConn(tenantCtx, conn)

		fnErr := fn(tenantCtx, t)

		_, _ = conn.Exec(context.Background(), "RESET ALL")
		conn.Release()

		if fnErr != nil {
			return fnErr
		}
	}

	return nil
}

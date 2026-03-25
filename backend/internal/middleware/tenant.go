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

// tenantSetSQL returns the SET statements for RLS context with a validated UUID.
// SET does not support $1 parameters, so we interpolate after validation.
// When running as superuser (Docker dev), it also does SET ROLE kantor_app so
// that RLS is enforced. In production (NixOS), the DB user is already
// non-superuser, so only the GUC is needed.
func tenantSetSQL(tenantID string) (string, error) {
	if _, err := uuid.Parse(tenantID); err != nil {
		return "", fmt.Errorf("invalid tenant id: %w", err)
	}
	return fmt.Sprintf(
		"DO $$ BEGIN IF current_setting('is_superuser')='on' THEN EXECUTE 'SET ROLE kantor_app'; END IF; END $$; SET app.current_tenant = '%s'",
		tenantID,
	), nil
}

// TenantMiddleware resolves the tenant from the Host header, acquires a
// dedicated connection from the pool, and configures RLS by setting the
// app.current_tenant GUC and downgrading to the non-superuser kantor_app role.
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

			// SET ROLE to non-superuser so RLS policies are enforced.
			// SET app.current_tenant so RLS knows which tenant's data to expose.
			setSQL, err := tenantSetSQL(info.ID)
			if err != nil {
				conn.Release()
				response.WriteError(w, http.StatusInternalServerError, "TENANT_SETUP_FAILED", "Invalid tenant ID", nil)
				return
			}
			_, err = conn.Exec(r.Context(), setSQL)
			if err != nil {
				conn.Release()
				response.WriteError(w, http.StatusInternalServerError, "TENANT_SETUP_FAILED", "Failed to configure tenant context", nil)
				return
			}

			// Store tenant info and the scoped connection in context.
			ctx := tenant.WithInfo(r.Context(), info)
			ctx = repository.WithConn(ctx, conn)

			// Ensure we clean up the connection regardless of outcome.
			defer func() {
				// RESET ALL restores the session role and clears all GUC variables.
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

		setSQL, err := tenantSetSQL(t.ID)
		if err != nil {
			conn.Release()
			return err
		}
		_, err = conn.Exec(ctx, setSQL)
		if err != nil {
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

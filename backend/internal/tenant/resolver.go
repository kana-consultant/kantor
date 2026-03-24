package tenant

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Resolver maps a request Host to a tenant.
// It uses an in-memory cache that refreshes periodically.
type Resolver struct {
	pool         *pgxpool.Pool
	mu           sync.RWMutex
	domainCache  map[string]Info // domain → tenant info
	lastRefresh  time.Time
	cacheTTL     time.Duration
}

// NewResolver creates a tenant resolver backed by the database.
func NewResolver(pool *pgxpool.Pool) *Resolver {
	return &Resolver{
		pool:        pool,
		domainCache: make(map[string]Info),
		cacheTTL:    5 * time.Minute,
	}
}

// Resolve looks up the tenant for the given host.
// The host may include a port (e.g. "localhost:3000") which is stripped.
func (r *Resolver) Resolve(ctx context.Context, host string) (Info, error) {
	domain := stripPort(host)

	// Try cache first.
	r.mu.RLock()
	info, ok := r.domainCache[domain]
	expired := time.Since(r.lastRefresh) > r.cacheTTL
	r.mu.RUnlock()

	if ok && !expired {
		return info, nil
	}

	// Refresh cache.
	if err := r.refresh(ctx); err != nil {
		// If refresh fails but we have a stale entry, use it.
		if ok {
			return info, nil
		}
		return Info{}, fmt.Errorf("tenant resolver: %w", err)
	}

	r.mu.RLock()
	info, ok = r.domainCache[domain]
	r.mu.RUnlock()

	if !ok {
		return Info{}, fmt.Errorf("tenant resolver: no tenant for domain %q", domain)
	}

	return info, nil
}

func (r *Resolver) refresh(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `
		SELECT td.domain, t.id::text, t.slug, t.name
		FROM tenant_domains td
		INNER JOIN tenants t ON t.id = td.tenant_id
		WHERE t.is_active = true
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	cache := make(map[string]Info)
	for rows.Next() {
		var domain string
		var info Info
		if err := rows.Scan(&domain, &info.ID, &info.Slug, &info.Name); err != nil {
			return err
		}
		cache[domain] = info
	}
	if err := rows.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	r.domainCache = cache
	r.lastRefresh = time.Now()
	r.mu.Unlock()

	return nil
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i != -1 {
		return host[:i]
	}
	return host
}

package hris

import (
	"context"
	"sync"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// PayrollCache memoises the per-tenant total monthly payroll computation.
// The aggregation requires decrypting every active salary row in process,
// which is O(n) per overview hit. A short TTL plus explicit invalidation on
// salary mutations keeps the overview snappy while still surfacing fresh
// numbers within seconds of an admin saving a new salary.
type PayrollCache struct {
	mu      sync.RWMutex
	entries map[string]payrollCacheEntry
	ttl     time.Duration
	now     func() time.Time
}

type payrollCacheEntry struct {
	value     int64
	expiresAt time.Time
}

// NewPayrollCache returns a cache. ttl=0 disables the cache (useful in tests).
func NewPayrollCache(ttl time.Duration) *PayrollCache {
	return &PayrollCache{
		entries: make(map[string]payrollCacheEntry),
		ttl:     ttl,
		now:     time.Now,
	}
}

// Get returns the cached total payroll for the tenant in ctx and true on hit.
// Misses (no entry, expired entry, no tenant) return (0, false).
func (c *PayrollCache) Get(ctx context.Context) (int64, bool) {
	if c == nil || c.ttl <= 0 {
		return 0, false
	}
	key := payrollCacheKey(ctx)
	if key == "" {
		return 0, false
	}
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return 0, false
	}
	if c.now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return 0, false
	}
	return entry.value, true
}

// Set memoises the total payroll for the tenant in ctx.
func (c *PayrollCache) Set(ctx context.Context, value int64) {
	if c == nil || c.ttl <= 0 {
		return
	}
	key := payrollCacheKey(ctx)
	if key == "" {
		return
	}
	c.mu.Lock()
	c.entries[key] = payrollCacheEntry{
		value:     value,
		expiresAt: c.now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Invalidate drops the cached value for the tenant in ctx. Compensation
// services call this after every create/update/delete on salary rows.
func (c *PayrollCache) Invalidate(ctx context.Context) {
	if c == nil {
		return
	}
	key := payrollCacheKey(ctx)
	if key == "" {
		return
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// InvalidateAll wipes the cache across every tenant. Useful for ops actions
// like an encryption key rotation that revalidates every ciphertext.
func (c *PayrollCache) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[string]payrollCacheEntry)
	c.mu.Unlock()
}

func payrollCacheKey(ctx context.Context) string {
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return ""
	}
	return info.ID
}

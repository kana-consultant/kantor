package auth

import (
	"sync"
	"time"
)

// AccessTokenBlacklist tracks JWT IDs (JTI) that have been explicitly revoked
// before their natural expiry. The store is in-memory and process-local; for
// a multi-instance deployment this should be replaced with a shared store
// (Redis SET, postgres table) but the API stays the same.
//
// Entries auto-expire when their record passes the access-token expiry, so
// the map cannot grow unbounded under normal traffic. A periodic GC runs in
// the background to drop stale records eagerly.
type AccessTokenBlacklist struct {
	mu      sync.RWMutex
	entries map[string]time.Time
	now     func() time.Time
	gcStop  chan struct{}
}

// NewAccessTokenBlacklist returns a blacklist with a goroutine that sweeps
// expired entries every gcInterval. Pass 0 to skip the goroutine — useful in
// tests where the caller drives time manually.
func NewAccessTokenBlacklist(gcInterval time.Duration) *AccessTokenBlacklist {
	b := &AccessTokenBlacklist{
		entries: make(map[string]time.Time),
		now:     time.Now,
		gcStop:  make(chan struct{}),
	}
	if gcInterval > 0 {
		go b.runGC(gcInterval)
	}
	return b
}

func (b *AccessTokenBlacklist) runGC(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-b.gcStop:
			return
		case <-ticker.C:
			b.gc()
		}
	}
}

// Stop terminates the background sweeper. Idempotent.
func (b *AccessTokenBlacklist) Stop() {
	select {
	case <-b.gcStop:
		return
	default:
		close(b.gcStop)
	}
}

// Revoke marks the given JTI as invalid until its corresponding access token
// would have expired anyway. Calls after expiresAt are no-ops.
func (b *AccessTokenBlacklist) Revoke(jti string, expiresAt time.Time) {
	if jti == "" {
		return
	}
	if expiresAt.Before(b.now().UTC()) {
		return
	}
	b.mu.Lock()
	b.entries[jti] = expiresAt
	b.mu.Unlock()
}

// IsRevoked reports whether the given JTI has been revoked and is still
// within its expiry window. Expired entries are dropped lazily on read.
func (b *AccessTokenBlacklist) IsRevoked(jti string) bool {
	if jti == "" {
		return false
	}
	b.mu.RLock()
	exp, ok := b.entries[jti]
	b.mu.RUnlock()
	if !ok {
		return false
	}
	if exp.Before(b.now().UTC()) {
		b.mu.Lock()
		delete(b.entries, jti)
		b.mu.Unlock()
		return false
	}
	return true
}

// Size returns the current number of tracked entries. Intended for tests and
// observability — the production code does not consult it.
func (b *AccessTokenBlacklist) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.entries)
}

func (b *AccessTokenBlacklist) gc() {
	now := b.now().UTC()
	b.mu.Lock()
	defer b.mu.Unlock()
	for jti, exp := range b.entries {
		if exp.Before(now) {
			delete(b.entries, jti)
		}
	}
}

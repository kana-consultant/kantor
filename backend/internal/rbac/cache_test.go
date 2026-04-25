package rbac

import (
	"context"
	"testing"
	"time"
)

func newCacheForTest(ttl time.Duration) *PermissionCache {
	return &PermissionCache{
		store: make(map[string]*CachedPermissions),
		ttl:   ttl,
	}
}

func TestNewPermissionCache_DefaultTTLWhenZero(t *testing.T) {
	c := NewPermissionCache(nil, 0)
	if c.ttl <= 0 {
		t.Fatalf("expected positive default TTL, got %v", c.ttl)
	}
}

func TestPermissionCache_GetReturnsCachedValue(t *testing.T) {
	c := newCacheForTest(time.Minute)
	key := c.cacheKey(context.Background(), "user-1")
	c.store[key] = &CachedPermissions{
		IsSuperAdmin: true,
		CachedAt:     time.Now().UTC(),
		TTL:          time.Minute,
	}

	got := c.Get(context.Background(), "user-1")
	if got == nil || !got.IsSuperAdmin {
		t.Fatalf("expected cached entry to be returned")
	}
}

func TestPermissionCache_GetExpiresAfterTTL(t *testing.T) {
	c := newCacheForTest(time.Minute)
	key := c.cacheKey(context.Background(), "user-1")
	c.store[key] = &CachedPermissions{
		IsSuperAdmin: true,
		CachedAt:     time.Now().UTC().Add(-2 * time.Minute),
		TTL:          time.Minute,
	}

	if got := c.Get(context.Background(), "user-1"); got != nil {
		t.Fatalf("expected expired entry to be evicted, got %+v", got)
	}
	if _, exists := c.store[key]; exists {
		t.Fatalf("expected expired entry to be removed from store")
	}
}

func TestPermissionCache_InvalidateDropsEntry(t *testing.T) {
	c := newCacheForTest(time.Minute)
	key := c.cacheKey(context.Background(), "user-1")
	c.store[key] = &CachedPermissions{CachedAt: time.Now().UTC(), TTL: time.Minute}

	c.Invalidate(context.Background(), "user-1")
	if _, exists := c.store[key]; exists {
		t.Fatalf("Invalidate should drop the entry")
	}
}

func TestPermissionCache_InvalidateByRoleDropsAffected(t *testing.T) {
	c := newCacheForTest(time.Minute)

	c.store["a"] = &CachedPermissions{
		ModuleRoles: map[string]ModuleRole{
			"hris": {RoleID: "role-1", RoleSlug: "viewer"},
		},
		CachedAt: time.Now().UTC(),
		TTL:      time.Minute,
	}
	c.store["b"] = &CachedPermissions{
		ModuleRoles: map[string]ModuleRole{
			"hris": {RoleID: "role-2", RoleSlug: "editor"},
		},
		CachedAt: time.Now().UTC(),
		TTL:      time.Minute,
	}

	c.InvalidateByRole("role-1")

	if _, exists := c.store["a"]; exists {
		t.Fatalf("entry holding role-1 should be evicted")
	}
	if _, exists := c.store["b"]; !exists {
		t.Fatalf("unrelated entry should not be evicted")
	}
}

func TestPermissionCache_InvalidateAllClearsStore(t *testing.T) {
	c := newCacheForTest(time.Minute)
	c.store["a"] = &CachedPermissions{}
	c.store["b"] = &CachedPermissions{}

	c.InvalidateAll()
	if len(c.store) != 0 {
		t.Fatalf("expected store to be empty, got %d entries", len(c.store))
	}
}

package hris

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

func ctxWithTenant(id string) context.Context {
	return tenant.WithInfo(context.Background(), tenant.Info{ID: id})
}

func TestPayrollCache_HitWithinTTL(t *testing.T) {
	c := NewPayrollCache(time.Minute)
	ctx := ctxWithTenant("tenant-a")

	c.Set(ctx, 12_345)
	got, ok := c.Get(ctx)
	if !ok || got != 12_345 {
		t.Fatalf("expected hit with 12345, got (%d, %v)", got, ok)
	}
}

func TestPayrollCache_MissAfterTTL(t *testing.T) {
	c := NewPayrollCache(time.Minute)
	ctx := ctxWithTenant("tenant-a")

	now := time.Now()
	clock := atomic.Pointer[time.Time]{}
	clock.Store(&now)
	c.now = func() time.Time { return *clock.Load() }

	c.Set(ctx, 999)
	advanced := now.Add(2 * time.Minute)
	clock.Store(&advanced)

	if _, ok := c.Get(ctx); ok {
		t.Fatal("entry should expire after TTL")
	}
}

func TestPayrollCache_InvalidateDropsEntry(t *testing.T) {
	c := NewPayrollCache(time.Minute)
	ctx := ctxWithTenant("tenant-a")

	c.Set(ctx, 1)
	c.Invalidate(ctx)
	if _, ok := c.Get(ctx); ok {
		t.Fatal("Invalidate should drop the entry")
	}
}

func TestPayrollCache_TenantsAreIsolated(t *testing.T) {
	c := NewPayrollCache(time.Minute)
	a := ctxWithTenant("tenant-a")
	b := ctxWithTenant("tenant-b")

	c.Set(a, 100)
	c.Set(b, 200)

	if got, _ := c.Get(a); got != 100 {
		t.Fatalf("tenant-a expected 100, got %d", got)
	}
	if got, _ := c.Get(b); got != 200 {
		t.Fatalf("tenant-b expected 200, got %d", got)
	}

	c.Invalidate(a)
	if _, ok := c.Get(a); ok {
		t.Fatal("tenant-a should have been invalidated")
	}
	if got, _ := c.Get(b); got != 200 {
		t.Fatalf("tenant-b must not be touched by tenant-a Invalidate, got %d", got)
	}
}

func TestPayrollCache_DisabledByZeroTTL(t *testing.T) {
	c := NewPayrollCache(0)
	ctx := ctxWithTenant("tenant-a")

	c.Set(ctx, 42)
	if _, ok := c.Get(ctx); ok {
		t.Fatal("ttl=0 should disable the cache")
	}
}

func TestPayrollCache_NoTenantSilentlyMisses(t *testing.T) {
	c := NewPayrollCache(time.Minute)
	c.Set(context.Background(), 7) // no tenant on the context
	if _, ok := c.Get(context.Background()); ok {
		t.Fatal("missing tenant must always miss")
	}
}

package auth

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestAccessTokenBlacklist_RevokeAndCheck(t *testing.T) {
	b := NewAccessTokenBlacklist(0)
	defer b.Stop()

	b.Revoke("jti-1", time.Now().Add(time.Hour))
	if !b.IsRevoked("jti-1") {
		t.Fatal("expected jti-1 to be revoked")
	}
	if b.IsRevoked("jti-2") {
		t.Fatal("unrelated jti must not appear revoked")
	}
}

func TestAccessTokenBlacklist_EmptyJTIIsIgnored(t *testing.T) {
	b := NewAccessTokenBlacklist(0)
	defer b.Stop()

	b.Revoke("", time.Now().Add(time.Hour))
	if b.Size() != 0 {
		t.Fatalf("empty JTI should not be stored, got size %d", b.Size())
	}
	if b.IsRevoked("") {
		t.Fatal("empty JTI lookup must not match")
	}
}

func TestAccessTokenBlacklist_DropsExpiredOnRead(t *testing.T) {
	b := NewAccessTokenBlacklist(0)
	defer b.Stop()

	now := time.Now().UTC()
	clock := atomic.Pointer[time.Time]{}
	clock.Store(&now)
	b.now = func() time.Time { return *clock.Load() }

	b.Revoke("jti-1", now.Add(time.Second))
	if !b.IsRevoked("jti-1") {
		t.Fatal("expected jti-1 to be revoked at t=0")
	}

	advanced := now.Add(2 * time.Second)
	clock.Store(&advanced)
	if b.IsRevoked("jti-1") {
		t.Fatal("expired entry must not appear revoked")
	}
	if b.Size() != 0 {
		t.Fatalf("expired entry should be evicted, got size %d", b.Size())
	}
}

func TestAccessTokenBlacklist_RejectsAlreadyExpiredEntries(t *testing.T) {
	b := NewAccessTokenBlacklist(0)
	defer b.Stop()

	b.Revoke("jti-1", time.Now().Add(-time.Hour))
	if b.Size() != 0 {
		t.Fatal("Revoke must drop entries with past expiry")
	}
}

func TestGenerateAccessToken_IncludesJTI(t *testing.T) {
	tm := NewTokenManager("super-secret-please-replace", 5*time.Minute, time.Hour)
	signed, _, err := tm.GenerateAccessToken("user-1", "tenant-1", time.Now())
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := tm.ParseAccessToken(signed)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if claims.ID == "" {
		t.Fatal("expected non-empty JTI in access token")
	}

	// Two consecutive tokens must carry distinct JTIs so individual revocation
	// is meaningful.
	signed2, _, err := tm.GenerateAccessToken("user-1", "tenant-1", time.Now())
	if err != nil {
		t.Fatalf("second GenerateAccessToken: %v", err)
	}
	claims2, err := tm.ParseAccessToken(signed2)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if claims.ID == claims2.ID {
		t.Fatalf("expected distinct JTIs, both were %q", claims.ID)
	}
}

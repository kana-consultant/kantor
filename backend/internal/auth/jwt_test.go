package auth

import (
	"testing"
	"time"
)

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

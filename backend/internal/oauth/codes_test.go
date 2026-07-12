package oauth

import (
	"testing"
)

func TestNewCodeReturnsDistinctValues(t *testing.T) {
	plaintext, hash, err := NewCode()
	if err != nil {
		t.Fatalf("NewCode: %v", err)
	}
	if plaintext == "" {
		t.Fatal("plaintext must not be empty")
	}
	if hash == "" {
		t.Fatal("hash must not be empty")
	}
	if plaintext == hash {
		t.Error("plaintext must differ from hash")
	}
}

func TestHashCodeConsistent(t *testing.T) {
	plaintext, hash, err := NewCode()
	if err != nil {
		t.Fatalf("NewCode: %v", err)
	}
	if got := HashCode(plaintext); got != hash {
		t.Errorf("HashCode mismatch: got %q, want %q", got, hash)
	}
}

func TestNewCodeUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		plaintext, _, err := NewCode()
		if err != nil {
			t.Fatalf("NewCode: %v", err)
		}
		if seen[plaintext] {
			t.Fatalf("duplicate code on iteration %d", i)
		}
		seen[plaintext] = true
	}
}

func TestVerifyPKCES256(t *testing.T) {
	// RFC 7636 Appendix B example verifier/challenge pair.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	if !VerifyPKCE("S256", challenge, verifier) {
		t.Error("expected valid PKCE verification")
	}
	if VerifyPKCE("S256", challenge, "wrong-verifier") {
		t.Error("expected PKCE rejection for wrong verifier")
	}
	if VerifyPKCE("plain", challenge, verifier) {
		t.Error("expected PKCE rejection for unsupported method")
	}
}

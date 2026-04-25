package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

const testSecret = "test-secret-please-replace-32chars!"

func TestEncrypter_RoundTrip(t *testing.T) {
	e, err := NewEncrypter(testSecret)
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}

	cipher, err := e.EncryptString("hello world")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if !strings.HasPrefix(cipher, "v1:") {
		t.Fatalf("expected versioned ciphertext, got %q", cipher)
	}

	plain, err := e.DecryptString(cipher)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}
	if plain != "hello world" {
		t.Fatalf("expected hello world, got %q", plain)
	}
}

func TestEncrypter_DecryptsLegacySHA256Ciphertext(t *testing.T) {
	// Re-create what the previous (sha256-only) code would produce: derive
	// the AES key with bare SHA-256 and encrypt directly. The new Encrypter
	// must still decrypt it via the legacy fallback.
	legacyKey := deriveKeySHA256(testSecret)
	rawCipher, err := encrypt(legacyKey, []byte("legacy payload"))
	if err != nil {
		t.Fatalf("encrypt legacy: %v", err)
	}
	versioned := "v1:" + base64.StdEncoding.EncodeToString(rawCipher)

	e, err := NewEncrypter(testSecret)
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}

	got, err := e.DecryptString(versioned)
	if err != nil {
		t.Fatalf("expected legacy ciphertext to decrypt, got error: %v", err)
	}
	if got != "legacy payload" {
		t.Fatalf("expected legacy payload, got %q", got)
	}
}

func TestEncrypter_KeyRotation(t *testing.T) {
	// Encrypt with the previous secret (legacy sha256-style ciphertext at v1).
	prevKey := deriveKeySHA256("old-secret-old-secret-old-secret!")
	rawOld, err := encrypt(prevKey, []byte("aged data"))
	if err != nil {
		t.Fatalf("encrypt old: %v", err)
	}
	oldVersioned := "v1:" + base64.StdEncoding.EncodeToString(rawOld)

	// Boot the encrypter with the new secret as primary and the old as previous.
	e, err := NewEncrypter("new-secret-new-secret-new-secret!", "old-secret-old-secret-old-secret!")
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}

	got, err := e.DecryptString(oldVersioned)
	if err != nil {
		t.Fatalf("rotation decrypt failed: %v", err)
	}
	if got != "aged data" {
		t.Fatalf("expected aged data, got %q", got)
	}

	// New writes go through Argon2id with primary version 2.
	cipher, err := e.EncryptString("fresh data")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if !strings.HasPrefix(cipher, "v2:") {
		t.Fatalf("expected v2 prefix on new ciphertext, got %q", cipher)
	}

	plain, err := e.DecryptString(cipher)
	if err != nil {
		t.Fatalf("DecryptString new: %v", err)
	}
	if plain != "fresh data" {
		t.Fatalf("expected fresh data, got %q", plain)
	}
}

func TestEncrypter_DerivedKeyDiffersBetweenSchemes(t *testing.T) {
	sha := deriveKeySHA256(testSecret)
	argon, err := deriveKeyArgon2id(testSecret, 1)
	if err != nil {
		t.Fatalf("deriveKeyArgon2id: %v", err)
	}
	if string(sha) == string(argon) {
		t.Fatal("Argon2id-derived key must differ from SHA-256-derived key")
	}
}

func TestEncrypter_RejectsEmptySecret(t *testing.T) {
	if _, err := NewEncrypter(""); err == nil {
		t.Fatal("expected error for empty secret")
	}
	if _, err := NewEncrypter("   "); err == nil {
		t.Fatal("expected error for whitespace-only secret")
	}
}

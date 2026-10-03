package security

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncrypter_BytesRoundTrip(t *testing.T) {
	e, err := NewEncrypter(testSecret)
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}

	plain := make([]byte, 64<<10)
	if _, err := rand.Read(plain); err != nil {
		t.Fatal(err)
	}
	plain = append([]byte("%PDF-1.7\n"), plain...)

	sealed, err := e.EncryptBytes(plain)
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}
	if !bytes.HasPrefix(sealed, []byte("v1:")) {
		t.Fatalf("expected v1: header, got %q", sealed[:8])
	}
	// Raw GCM output, no base64: header + 12-byte nonce + 16-byte tag.
	if want := len(plain) + 3 + 12 + 16; len(sealed) != want {
		t.Fatalf("expected %d bytes, got %d", want, len(sealed))
	}
	if bytes.Contains(sealed, []byte("%PDF-")) {
		t.Fatal("ciphertext contains plaintext")
	}

	got, err := e.DecryptBytes(sealed)
	if err != nil {
		t.Fatalf("DecryptBytes: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("round trip mismatch")
	}

	empty, err := e.EncryptBytes(nil)
	if err != nil {
		t.Fatalf("EncryptBytes(nil): %v", err)
	}
	if got, err := e.DecryptBytes(empty); err != nil || len(got) != 0 {
		t.Fatalf("empty round trip: %v %d", err, len(got))
	}
}

func TestEncrypter_BytesKeyRotation(t *testing.T) {
	old, err := NewEncrypter("old-secret-old-secret-old-secret!")
	if err != nil {
		t.Fatal(err)
	}
	sealedOld, err := old.EncryptBytes([]byte("aged pdf"))
	if err != nil {
		t.Fatal(err)
	}

	e, err := NewEncrypter("new-secret-new-secret-new-secret!", "old-secret-old-secret-old-secret!")
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.DecryptBytes(sealedOld)
	if err != nil || string(got) != "aged pdf" {
		t.Fatalf("rotation decrypt: %q %v", got, err)
	}

	sealedNew, err := e.EncryptBytes([]byte("fresh pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(sealedNew, []byte("v2:")) {
		t.Fatalf("expected v2: header, got %q", sealedNew[:3])
	}
	if _, err := old.DecryptBytes(sealedNew); err == nil {
		t.Fatal("old encrypter must not know key version 2")
	}
}

func TestEncrypter_DecryptBytesRejectsBadInput(t *testing.T) {
	e, err := NewEncrypter(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := e.EncryptBytes([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}

	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0x01

	other, err := NewEncrypter("another-secret-another-secret-123")
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]func() ([]byte, error){
		"empty":        func() ([]byte, error) { return e.DecryptBytes(nil) },
		"no header":    func() ([]byte, error) { return e.DecryptBytes(sealed[3:]) },
		"bad version":  func() ([]byte, error) { return e.DecryptBytes(append([]byte("vx:"), sealed[3:]...)) },
		"unknown key":  func() ([]byte, error) { return e.DecryptBytes(append([]byte("v9:"), sealed[3:]...)) },
		"tampered":     func() ([]byte, error) { return e.DecryptBytes(tampered) },
		"truncated":    func() ([]byte, error) { return e.DecryptBytes(sealed[:10]) },
		"wrong secret": func() ([]byte, error) { return other.DecryptBytes(sealed) },
	}
	for name, fn := range cases {
		if _, err := fn(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

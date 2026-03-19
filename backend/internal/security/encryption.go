package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Encrypter uses AES-256-GCM for application-layer encryption.
// It supports key rotation by maintaining a primary key for encryption
// and optional previous keys for decrypting older ciphertext.
//
// Ciphertext format: "v<version>:<base64 payload>"
// Unversioned ciphertext (legacy) is treated as version 1.
type Encrypter struct {
	primaryVersion int
	primaryKey     []byte
	keys           map[int][]byte // version -> key
}

// NewEncrypter creates an encrypter with the primary key for encryption.
// Previous keys are used only for decryption during key rotation.
//
// Key rotation procedure (requires application restart):
//  1. Set DATA_ENCRYPTION_KEY to the new key
//  2. Set DATA_ENCRYPTION_KEY_OLD to the previous key
//  3. Restart the application
//  4. New writes use the new key; old data decrypts via the old key
//  5. Once all data is re-encrypted, DATA_ENCRYPTION_KEY_OLD can be removed
func NewEncrypter(secret string, previousSecrets ...string) (*Encrypter, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("DATA_ENCRYPTION_KEY is required")
	}

	e := &Encrypter{
		primaryVersion: len(previousSecrets) + 1,
		primaryKey:     deriveKey(secret),
		keys:           make(map[int][]byte),
	}

	// Previous keys get versions 1, 2, ... in order
	for i, prev := range previousSecrets {
		if strings.TrimSpace(prev) == "" {
			continue
		}
		e.keys[i+1] = deriveKey(prev)
	}

	// Current key gets the highest version
	e.keys[e.primaryVersion] = e.primaryKey

	return e, nil
}

func deriveKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key
}

func (e *Encrypter) EncryptString(plaintext string) (string, error) {
	ciphertext, err := encrypt(e.primaryKey, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("v%d:%s", e.primaryVersion, base64.StdEncoding.EncodeToString(ciphertext)), nil
}

func (e *Encrypter) DecryptString(ciphertext string) (string, error) {
	version, payload, hasVersion := parseVersionedCiphertext(ciphertext)

	if hasVersion {
		key, ok := e.keys[version]
		if !ok {
			return "", fmt.Errorf("unknown encryption key version: %d", version)
		}
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", fmt.Errorf("decode ciphertext: %w", err)
		}
		plaintext, err := decrypt(key, decoded)
		if err != nil {
			return "", fmt.Errorf("decrypt ciphertext (v%d): %w", version, err)
		}
		return string(plaintext), nil
	}

	// Legacy unversioned ciphertext (pre-rotation data without "v1:" prefix):
	// try all known keys to find the one that decrypts it.
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	for v := 1; v <= e.primaryVersion; v++ {
		key, ok := e.keys[v]
		if !ok {
			continue
		}
		plaintext, err := decrypt(key, decoded)
		if err == nil {
			return string(plaintext), nil
		}
	}

	return "", fmt.Errorf("decrypt ciphertext: no matching key found")
}

func parseVersionedCiphertext(s string) (version int, payload string, ok bool) {
	if !strings.HasPrefix(s, "v") {
		return 0, "", false
	}
	idx := strings.Index(s, ":")
	if idx < 2 {
		return 0, "", false
	}
	var v int
	if _, err := fmt.Sscanf(s[:idx], "v%d", &v); err != nil {
		return 0, "", false
	}
	return v, s[idx+1:], true
}

func encrypt(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decrypt(key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, encrypted, nil)
}

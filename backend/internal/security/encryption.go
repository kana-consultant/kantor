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

	"golang.org/x/crypto/argon2"
)

// Encrypter uses AES-256-GCM for application-layer encryption.
// It supports key rotation by maintaining a primary key for encryption
// and optional previous keys for decrypting older ciphertext.
//
// Ciphertext format: "v<version>:<base64 payload>"
// Unversioned ciphertext (legacy) is treated as version 1.
//
// Key derivation:
//   - New writes use Argon2id (memory-hard) over the secret with a
//     deterministic per-version salt. Argon2id parameters target ~250 ms on
//     a server-class CPU, which makes brute-force of low-entropy
//     DATA_ENCRYPTION_KEY values prohibitively expensive.
//   - For each registered secret we ALSO keep a SHA-256 key. Existing
//     ciphertext was written when the project derived keys via a single
//     SHA-256 pass, and we must remain able to decrypt it without forcing
//     an immediate re-encryption sweep. Decryption tries Argon2id first,
//     then falls back to SHA-256.
type Encrypter struct {
	primaryVersion int
	primaryKey     []byte
	keys           map[int]derivedKeys // version -> {argon2id, sha256}
}

type derivedKeys struct {
	primary []byte // Argon2id-derived key (new scheme)
	legacy  []byte // SHA-256 of the same secret (legacy scheme)
}

// argon2KDFParams are the Argon2id parameters used for the primary key
// derivation. These are intentionally exported via constants so any review
// catches when they change. The values target ~250 ms on a 4-core server CPU
// while keeping memory pressure under 64 MiB.
const (
	argon2Time    uint32 = 2
	argon2Memory  uint32 = 64 * 1024 // KiB
	argon2Threads uint8  = 4
	argon2KeyLen  uint32 = 32 // AES-256
)

// NewEncrypter creates an encrypter with the primary key for encryption.
// Previous keys are used only for decryption during key rotation.
//
// Key rotation procedure (requires application restart):
//  1. Set DATA_ENCRYPTION_KEY to the new key
//  2. Set DATA_ENCRYPTION_KEY_PREVIOUS to the previous key
//  3. Restart the application
//  4. New writes use the new key; old data decrypts via the old key
//  5. Once all data is re-encrypted, DATA_ENCRYPTION_KEY_PREVIOUS can be removed
func NewEncrypter(secret string, previousSecrets ...string) (*Encrypter, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("DATA_ENCRYPTION_KEY is required")
	}

	e := &Encrypter{
		primaryVersion: len(previousSecrets) + 1,
		keys:           make(map[int]derivedKeys),
	}

	// Previous keys get versions 1, 2, ... in order
	for i, prev := range previousSecrets {
		if strings.TrimSpace(prev) == "" {
			continue
		}
		version := i + 1
		argonKey, err := deriveKeyArgon2id(prev, version)
		if err != nil {
			return nil, fmt.Errorf("derive argon2id key for version %d: %w", version, err)
		}
		e.keys[version] = derivedKeys{
			primary: argonKey,
			legacy:  deriveKeySHA256(prev),
		}
	}

	// Current key gets the highest version
	primaryArgon, err := deriveKeyArgon2id(secret, e.primaryVersion)
	if err != nil {
		return nil, fmt.Errorf("derive argon2id primary key: %w", err)
	}
	e.primaryKey = primaryArgon
	e.keys[e.primaryVersion] = derivedKeys{
		primary: primaryArgon,
		legacy:  deriveKeySHA256(secret),
	}

	return e, nil
}

func deriveKeySHA256(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key
}

// deriveKeyArgon2id produces a 32-byte AES-256 key from secret using
// Argon2id with a deterministic, per-version salt. The salt deliberately
// includes a project-specific domain string so two services that happen to
// share the same DATA_ENCRYPTION_KEY do not derive the same AES key.
func deriveKeyArgon2id(secret string, version int) ([]byte, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("secret is empty")
	}
	salt := sha256.Sum256([]byte(fmt.Sprintf("kantor.security.encryption/v%d", version)))
	return argon2.IDKey([]byte(secret), salt[:16], argon2Time, argon2Memory, argon2Threads, argon2KeyLen), nil
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
		entry, ok := e.keys[version]
		if !ok {
			return "", fmt.Errorf("unknown encryption key version: %d", version)
		}
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", fmt.Errorf("decode ciphertext: %w", err)
		}
		if plaintext, err := decrypt(entry.primary, decoded); err == nil {
			return string(plaintext), nil
		}
		// Legacy data written before Argon2id was introduced still uses the
		// SHA-256-derived key for the same secret. Try it before giving up.
		if entry.legacy != nil {
			if plaintext, err := decrypt(entry.legacy, decoded); err == nil {
				return string(plaintext), nil
			}
		}
		return "", fmt.Errorf("decrypt ciphertext (v%d): no matching key", version)
	}

	// Legacy unversioned ciphertext (pre-rotation data without "v1:" prefix):
	// try every known key, both schemes, until one succeeds.
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	for v := 1; v <= e.primaryVersion; v++ {
		entry, ok := e.keys[v]
		if !ok {
			continue
		}
		if plaintext, err := decrypt(entry.primary, decoded); err == nil {
			return string(plaintext), nil
		}
		if entry.legacy != nil {
			if plaintext, err := decrypt(entry.legacy, decoded); err == nil {
				return string(plaintext), nil
			}
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

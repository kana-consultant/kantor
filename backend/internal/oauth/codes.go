package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const CodeTTL = 2 * time.Minute

// AuthCode holds the metadata for an in-flight authorization code grant.
type AuthCode struct {
	UserID              string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	ExpiresAt           time.Time
}

func newRandomToken(byteLen int) (string, error) {
	buffer := make([]byte, byteLen)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// NewCode generates a random authorization code value and its SHA-256 hash.
// Only the hash is stored; the plaintext is returned once to the caller.
func NewCode() (plaintext string, hash string, err error) {
	plaintext, err = newRandomToken(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
}

// HashCode returns the SHA-256 hex digest of a code value, used to look up
// a previously issued code in the database.
func HashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// VerifyPKCE checks an S256 code_verifier against the stored code_challenge.
func VerifyPKCE(method string, challenge string, verifier string) bool {
	if method != "S256" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// NewClientID mints a public client identifier for dynamic client registration.
func NewClientID() (string, error) {
	value, err := newRandomToken(24)
	if err != nil {
		return "", err
	}
	return "kantor_mcp_" + value, nil
}

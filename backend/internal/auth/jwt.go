package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AccessClaims struct {
	Type     string `json:"type"`
	TenantID string `json:"tenant_id,omitempty"`
	Source   string `json:"source,omitempty"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret        []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

func NewTokenManager(secret string, accessExpiry time.Duration, refreshExpiry time.Duration) *TokenManager {
	return &TokenManager{
		secret:        []byte(secret),
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
	}
}

func (m *TokenManager) GenerateAccessToken(userID string, tenantID string, source string, now time.Time) (string, time.Time, error) {
	expiresAt := now.Add(m.accessExpiry)
	claims := AccessClaims{
		Type:     "access",
		TenantID: tenantID,
		Source:   source,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}

func (m *TokenManager) ParseAccessToken(token string) (*AccessClaims, error) {
	claims := &AccessClaims{}

	parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (interface{}, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}

	if !parsed.Valid {
		return nil, fmt.Errorf("invalid access token")
	}
	if claims.Type != "access" {
		return nil, fmt.Errorf("invalid access token type")
	}

	return claims, nil
}

func (m *TokenManager) GenerateRefreshToken() (string, time.Time, error) {
	return m.GenerateRefreshTokenWithExpiry(m.refreshExpiry)
}

func (m *TokenManager) GenerateRefreshTokenWithExpiry(expiry time.Duration) (string, time.Time, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", time.Time{}, err
	}

	token := base64.RawURLEncoding.EncodeToString(bytes)
	return token, time.Now().UTC().Add(expiry), nil
}

func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

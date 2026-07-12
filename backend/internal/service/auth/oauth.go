package auth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/oauth"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
)

var (
	ErrOAuthInvalidClient   = errors.New("invalid client")
	ErrOAuthInvalidRedirect = errors.New("invalid redirect uri")
	ErrOAuthInvalidRequest  = errors.New("invalid request")
	ErrOAuthInvalidGrant    = errors.New("invalid grant")
)

type oauthClientRepository interface {
	CreateOAuthClient(ctx context.Context, clientID string, clientName string, redirectURIs []string) error
	GetOAuthClient(ctx context.Context, clientID string) (authrepo.OAuthClient, error)
}

type oauthCodeRepository interface {
	CreateOAuthCode(ctx context.Context, params authrepo.CreateOAuthCodeParams) error
	ConsumeOAuthCode(ctx context.Context, codeHash string, now time.Time) (authrepo.OAuthCodeRow, error)
	PurgeExpiredOAuthCodes(ctx context.Context, now time.Time) (int64, error)
}

type OAuthService struct {
	repo  oauthClientRepository
	codes oauthCodeRepository
	auth  *Service
}

func NewOAuthService(repo oauthClientRepository, codes oauthCodeRepository, auth *Service) *OAuthService {
	return &OAuthService{repo: repo, codes: codes, auth: auth}
}

type RegisterClientResult struct {
	ClientID     string
	ClientName   string
	RedirectURIs []string
}

func (s *OAuthService) RegisterClient(ctx context.Context, clientName string, redirectURIs []string) (RegisterClientResult, error) {
	if len(redirectURIs) == 0 {
		return RegisterClientResult{}, ErrOAuthInvalidRequest
	}
	for _, uri := range redirectURIs {
		if err := oauth.ValidateRedirectURI(uri); err != nil {
			return RegisterClientResult{}, ErrOAuthInvalidRedirect
		}
	}

	name := strings.TrimSpace(clientName)
	if name == "" {
		name = "MCP Client"
	}

	clientID, err := oauth.NewClientID()
	if err != nil {
		return RegisterClientResult{}, err
	}
	if err := s.repo.CreateOAuthClient(ctx, clientID, name, redirectURIs); err != nil {
		return RegisterClientResult{}, err
	}
	return RegisterClientResult{ClientID: clientID, ClientName: name, RedirectURIs: redirectURIs}, nil
}

func (s *OAuthService) ValidateAuthorize(ctx context.Context, clientID string, redirectURI string) error {
	client, err := s.repo.GetOAuthClient(ctx, clientID)
	if err != nil {
		return ErrOAuthInvalidClient
	}
	if !slices.Contains(client.RedirectURIs, redirectURI) {
		return ErrOAuthInvalidRedirect
	}
	return nil
}

type GrantParams struct {
	UserID              string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
}

func (s *OAuthService) Grant(ctx context.Context, params GrantParams, now time.Time) (string, error) {
	if err := s.ValidateAuthorize(ctx, params.ClientID, params.RedirectURI); err != nil {
		return "", err
	}
	if params.CodeChallengeMethod != "S256" || strings.TrimSpace(params.CodeChallenge) == "" {
		return "", ErrOAuthInvalidRequest
	}

	plaintext, hash, err := oauth.NewCode()
	if err != nil {
		return "", err
	}

	if err := s.codes.CreateOAuthCode(ctx, authrepo.CreateOAuthCodeParams{
		CodeHash:            hash,
		UserID:              params.UserID,
		ClientID:            params.ClientID,
		RedirectURI:         params.RedirectURI,
		CodeChallenge:       params.CodeChallenge,
		CodeChallengeMethod: params.CodeChallengeMethod,
		Scope:               params.Scope,
		ExpiresAt:           now.Add(oauth.CodeTTL),
	}); err != nil {
		return "", err
	}
	return plaintext, nil
}

func (s *OAuthService) ExchangeCode(ctx context.Context, code string, clientID string, redirectURI string, codeVerifier string, userAgent string, ipAddress string, now time.Time) (dto.TokenPair, error) {
	hash := oauth.HashCode(code)
	stored, err := s.codes.ConsumeOAuthCode(ctx, hash, now)
	if err != nil {
		return dto.TokenPair{}, ErrOAuthInvalidGrant
	}
	if stored.ClientID != clientID || stored.RedirectURI != redirectURI {
		return dto.TokenPair{}, ErrOAuthInvalidGrant
	}
	if !oauth.VerifyPKCE(stored.CodeChallengeMethod, stored.CodeChallenge, codeVerifier) {
		return dto.TokenPair{}, ErrOAuthInvalidGrant
	}

	result, err := s.auth.IssueTokensForUser(ctx, stored.UserID, userAgent, ipAddress)
	if err != nil {
		return dto.TokenPair{}, err
	}
	return result.Tokens, nil
}

func (s *OAuthService) RefreshGrant(ctx context.Context, refreshToken string, userAgent string, ipAddress string) (dto.TokenPair, error) {
	result, err := s.auth.Refresh(ctx, refreshToken, userAgent, ipAddress)
	if err != nil {
		return dto.TokenPair{}, err
	}
	return result.Tokens, nil
}

// PurgeExpiredCodes removes stale authorization codes from the database.
// Called by the background scheduler to prevent table bloat.
func (s *OAuthService) PurgeExpiredCodes(ctx context.Context, now time.Time) (int64, error) {
	return s.codes.PurgeExpiredOAuthCodes(ctx, now)
}

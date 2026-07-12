package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var ErrOAuthCodeNotFound = errors.New("oauth authorization code not found")

type CreateOAuthCodeParams struct {
	CodeHash            string
	UserID              string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	ExpiresAt           time.Time
}

type OAuthCodeRow struct {
	UserID              string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	ExpiresAt           time.Time
}

// CreateOAuthCode persists a single-use authorization code.
func (r *Repository) CreateOAuthCode(ctx context.Context, params CreateOAuthCodeParams) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO oauth_authorization_codes
			(code_hash, user_id, client_id, redirect_uri, code_challenge, code_challenge_method, scope, expires_at)
		VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8)
	`, params.CodeHash, params.UserID, params.ClientID, params.RedirectURI,
		params.CodeChallenge, params.CodeChallengeMethod, params.Scope, params.ExpiresAt)
	return err
}

// ConsumeOAuthCode atomically fetches and deletes an authorization code.
// Returns ErrOAuthCodeNotFound when no matching unexpired row exists.
func (r *Repository) ConsumeOAuthCode(ctx context.Context, codeHash string, now time.Time) (OAuthCodeRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var row OAuthCodeRow
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		DELETE FROM oauth_authorization_codes
		WHERE code_hash = $1 AND expires_at > $2
		RETURNING user_id::text, client_id, redirect_uri, code_challenge, code_challenge_method, scope, expires_at
	`, codeHash, now).Scan(
		&row.UserID, &row.ClientID, &row.RedirectURI,
		&row.CodeChallenge, &row.CodeChallengeMethod, &row.Scope, &row.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OAuthCodeRow{}, ErrOAuthCodeNotFound
		}
		return OAuthCodeRow{}, err
	}
	return row, nil
}

// PurgeExpiredOAuthCodes removes authorization codes that have passed their TTL.
func (r *Repository) PurgeExpiredOAuthCodes(ctx context.Context, now time.Time) (int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `
		DELETE FROM oauth_authorization_codes WHERE expires_at <= $1
	`, now)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

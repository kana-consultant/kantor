package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type CreatePasswordResetTokenParams struct {
	UserID      string
	TokenHash   string
	ExpiresAt   time.Time
	RequestedIP string
	UserAgent   string
}

func (r *Repository) CreatePasswordResetToken(ctx context.Context, params CreatePasswordResetTokenParams) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		UPDATE password_reset_tokens
		SET used_at = NOW()
		WHERE user_id = $1::uuid AND used_at IS NULL
	`, params.UserID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at, requested_ip, user_agent)
		VALUES ($1::uuid, $2, $3, NULLIF($4, '')::inet, NULLIF($5, ''))
	`, params.UserID, params.TokenHash, params.ExpiresAt, params.RequestedIP, params.UserAgent); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (model.PasswordResetToken, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var token model.PasswordResetToken
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT id::text, user_id::text, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.UsedAt,
		&token.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.PasswordResetToken{}, ErrNotFound
		}
		return model.PasswordResetToken{}, err
	}

	return token, nil
}

func (r *Repository) UsePasswordResetToken(ctx context.Context, tokenID string, userID string, passwordHash string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	tag, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $1, updated_at = NOW()
		WHERE id = $2::uuid
	`, passwordHash, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE user_id = $1::uuid AND revoked_at IS NULL
	`, userID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE password_reset_tokens
		SET used_at = NOW()
		WHERE user_id = $1::uuid AND used_at IS NULL
	`, userID); err != nil {
		return err
	}

	tag, err = tx.Exec(ctx, `
		UPDATE password_reset_tokens
		SET used_at = COALESCE(used_at, NOW())
		WHERE id = $1::uuid
	`, tokenID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return tx.Commit(ctx)
}

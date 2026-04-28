package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type rotateTestDB struct {
	tx pgx.Tx
}

func (d *rotateTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (d *rotateTestDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func (d *rotateTestDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}

func (d *rotateTestDB) Begin(context.Context) (pgx.Tx, error) {
	return d.tx, nil
}

func (d *rotateTestDB) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

type rotateTestTx struct {
	updateRows int64
	updateErr  error
	insertErr  error
	commitErr  error

	execCalls  int
	committed  bool
	rolledBack bool
}

func (tx *rotateTestTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}

func (tx *rotateTestTx) Commit(context.Context) error {
	tx.committed = true
	return tx.commitErr
}

func (tx *rotateTestTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

func (tx *rotateTestTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}

func (tx *rotateTestTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (tx *rotateTestTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (tx *rotateTestTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}

func (tx *rotateTestTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	tx.execCalls++
	switch tx.execCalls {
	case 1:
		if tx.updateErr != nil {
			return pgconn.CommandTag{}, tx.updateErr
		}
		return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tx.updateRows)), nil
	case 2:
		if tx.insertErr != nil {
			return pgconn.CommandTag{}, tx.insertErr
		}
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	default:
		return pgconn.CommandTag{}, errors.New("unexpected exec call")
	}
}

func (tx *rotateTestTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (tx *rotateTestTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func (tx *rotateTestTx) Conn() *pgx.Conn {
	return nil
}

func TestRotateRefreshToken_ReturnsNotFoundWhenAlreadyRevoked(t *testing.T) {
	t.Parallel()

	tx := &rotateTestTx{updateRows: 0}
	repo := New(&rotateTestDB{tx: tx})

	err := repo.RotateRefreshToken(context.Background(), "old-token-hash", CreateRefreshTokenParams{
		UserID:    "user-1",
		TokenHash: "new-token-hash",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if tx.execCalls != 1 {
		t.Fatalf("expected only revoke query to run, got %d exec calls", tx.execCalls)
	}
	if tx.committed {
		t.Fatal("transaction must not commit when old token is already revoked")
	}
}

func TestRotateRefreshToken_SucceedsWhenOldTokenIsActive(t *testing.T) {
	t.Parallel()

	tx := &rotateTestTx{updateRows: 1}
	repo := New(&rotateTestDB{tx: tx})

	err := repo.RotateRefreshToken(context.Background(), "old-token-hash", CreateRefreshTokenParams{
		UserID:    "user-1",
		TokenHash: "new-token-hash",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		UserAgent: "Mozilla/5.0",
		IPAddress: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if tx.execCalls != 2 {
		t.Fatalf("expected revoke and insert queries, got %d exec calls", tx.execCalls)
	}
	if !tx.committed {
		t.Fatal("expected transaction to commit")
	}
}

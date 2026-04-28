package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type setUserActiveTestDB struct {
	tx pgx.Tx
}

func (d *setUserActiveTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (d *setUserActiveTestDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func (d *setUserActiveTestDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}

func (d *setUserActiveTestDB) Begin(context.Context) (pgx.Tx, error) {
	return d.tx, nil
}

func (d *setUserActiveTestDB) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

type setUserActiveTestTx struct {
	updateRows int64
	updateErr  error
	revokeErr  error
	commitErr  error

	execSQL    []string
	committed  bool
	rolledBack bool
}

func (tx *setUserActiveTestTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}

func (tx *setUserActiveTestTx) Commit(context.Context) error {
	tx.committed = true
	return tx.commitErr
}

func (tx *setUserActiveTestTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

func (tx *setUserActiveTestTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}

func (tx *setUserActiveTestTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (tx *setUserActiveTestTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (tx *setUserActiveTestTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}

func (tx *setUserActiveTestTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.execSQL = append(tx.execSQL, sql)
	switch len(tx.execSQL) {
	case 1:
		if tx.updateErr != nil {
			return pgconn.CommandTag{}, tx.updateErr
		}
		return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tx.updateRows)), nil
	case 2:
		if tx.revokeErr != nil {
			return pgconn.CommandTag{}, tx.revokeErr
		}
		return pgconn.NewCommandTag("UPDATE 1"), nil
	default:
		return pgconn.CommandTag{}, errors.New("unexpected exec call")
	}
}

func (tx *setUserActiveTestTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (tx *setUserActiveTestTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func (tx *setUserActiveTestTx) Conn() *pgx.Conn {
	return nil
}

func TestSetUserActive_DeactivateRevokesActiveRefreshTokens(t *testing.T) {
	t.Parallel()

	tx := &setUserActiveTestTx{updateRows: 1}
	repo := New(&setUserActiveTestDB{tx: tx})

	if err := repo.SetUserActive(context.Background(), "user-1", false); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(tx.execSQL) != 2 {
		t.Fatalf("expected 2 exec calls (update user + revoke tokens), got %d", len(tx.execSQL))
	}
	if !strings.Contains(tx.execSQL[1], "UPDATE refresh_tokens") {
		t.Fatalf("expected refresh token revoke query, got %q", tx.execSQL[1])
	}
	if !tx.committed {
		t.Fatal("expected transaction to commit")
	}
}

func TestSetUserActive_ActivateDoesNotRevokeRefreshTokens(t *testing.T) {
	t.Parallel()

	tx := &setUserActiveTestTx{updateRows: 1}
	repo := New(&setUserActiveTestDB{tx: tx})

	if err := repo.SetUserActive(context.Background(), "user-1", true); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(tx.execSQL) != 1 {
		t.Fatalf("expected 1 exec call (update user only), got %d", len(tx.execSQL))
	}
	if !tx.committed {
		t.Fatal("expected transaction to commit")
	}
}

func TestSetUserActive_ReturnsNotFoundWhenUserMissing(t *testing.T) {
	t.Parallel()

	tx := &setUserActiveTestTx{updateRows: 0}
	repo := New(&setUserActiveTestDB{tx: tx})

	err := repo.SetUserActive(context.Background(), "missing-user", false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if len(tx.execSQL) != 1 {
		t.Fatalf("expected only user update query, got %d exec calls", len(tx.execSQL))
	}
	if tx.committed {
		t.Fatal("transaction must not commit when user is missing")
	}
}

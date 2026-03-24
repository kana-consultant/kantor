package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const DefaultQueryTimeout = 5 * time.Second

// QueryContext returns a context with a default query timeout.
// If the parent context already has an earlier deadline, it is preserved.
func QueryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, DefaultQueryTimeout)
}

// DBTX is the common interface satisfied by *pgxpool.Pool, *pgxpool.Conn, and pgx.Tx.
type DBTX interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type connCtxKey struct{}

// WithConn stores a tenant-scoped database connection in the context.
func WithConn(ctx context.Context, conn DBTX) context.Context {
	return context.WithValue(ctx, connCtxKey{}, conn)
}

// DB returns the tenant-scoped connection from the context.
// If no connection is found, it falls back to the provided default (typically the pool).
func DB(ctx context.Context, fallback DBTX) DBTX {
	if conn, ok := ctx.Value(connCtxKey{}).(DBTX); ok {
		return conn
	}
	return fallback
}

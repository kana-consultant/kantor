package repository

import (
	"context"
	"time"
)

const DefaultQueryTimeout = 5 * time.Second

// QueryContext returns a context with a default query timeout.
// If the parent context already has an earlier deadline, it is preserved.
func QueryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, DefaultQueryTimeout)
}

package hris

import (
	"context"
	"errors"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// These tests need a migrated database: set KANTOR_TEST_DATABASE_URL (e.g.
// the isolated verify database). They are skipped otherwise.
func sequenceTestPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("KANTOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KANTOR_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	var tenantID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenantID); err != nil {
		t.Fatalf("find a tenant: %v", err)
	}
	return pool, tenantID
}

// withTenantTx runs fn in its own transaction on its own connection with the
// tenant GUC set, like a document generation request would.
func withTenantTx(ctx context.Context, pool *pgxpool.Pool, tenantID string, commit bool, fn func(context.Context) error) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant', $1, false)`, tenantID); err != nil {
		return err
	}
	defer func() { _, _ = conn.Exec(context.Background(), "RESET ALL") }()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(repository.WithConn(ctx, tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if !commit {
		return tx.Rollback(ctx)
	}
	return tx.Commit(ctx)
}

func TestDocumentSequenceConcurrentIncrementsAreUnique(t *testing.T) {
	pool, tenantID := sequenceTestPool(t)
	repo := NewDocumentSequencesRepository(pool)
	docType := "TEST_" + uuid.NewString()[:8]
	t.Cleanup(func() {
		_ = withTenantTx(context.Background(), pool, tenantID, true, func(ctx context.Context) error {
			_, err := repository.DB(ctx, nil).Exec(ctx, `DELETE FROM document_sequences WHERE doc_type = $1`, docType)
			return err
		})
	})

	const workers, perWorker = 8, 10
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var (
		mu     sync.Mutex
		values []int
		wg     sync.WaitGroup
		errs   = make(chan error, workers*perWorker)
	)
	start := make(chan struct{})
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < perWorker; i++ {
				err := withTenantTx(ctx, pool, tenantID, true, func(txCtx context.Context) error {
					value, err := repo.Next(txCtx, docType, "2026-10")
					if err != nil {
						return err
					}
					// Hold the row lock a moment so transactions overlap.
					time.Sleep(2 * time.Millisecond)
					mu.Lock()
					values = append(values, value)
					mu.Unlock()
					return nil
				})
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("increment failed: %v", err)
	}

	sort.Ints(values)
	if len(values) != workers*perWorker {
		t.Fatalf("got %d values, want %d", len(values), workers*perWorker)
	}
	for i, value := range values {
		if value != i+1 {
			t.Fatalf("values are not unique and gap-free: %v", values)
		}
	}

	// Another period starts its own counter.
	var other int
	if err := withTenantTx(ctx, pool, tenantID, true, func(txCtx context.Context) (err error) {
		other, err = repo.Next(txCtx, docType, "2026-11")
		return err
	}); err != nil || other != 1 {
		t.Fatalf("new period value = %d, %v; want 1", other, err)
	}

	// A rolled-back document gives its number back.
	var rolledBack, next int
	if err := withTenantTx(ctx, pool, tenantID, false, func(txCtx context.Context) (err error) {
		rolledBack, err = repo.Next(txCtx, docType, "2026-10")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := withTenantTx(ctx, pool, tenantID, true, func(txCtx context.Context) (err error) {
		next, err = repo.Next(txCtx, docType, "2026-10")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if rolledBack != workers*perWorker+1 || next != rolledBack {
		t.Fatalf("rolled back %d, next %d; want both %d", rolledBack, next, workers*perWorker+1)
	}

	if _, err := repo.Next(ctx, " ", "x"); !errors.Is(err, ErrInvalidDocumentSequenceKey) {
		t.Fatalf("blank doc type: %v", err)
	}
	t.Logf("%d concurrent increments, all unique and gap-free", len(values))
}

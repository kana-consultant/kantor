package hris

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// createFixtureEmployee inserts a throw-away employee in the (rolled-back)
// test transaction in ctx, so the DB-gated tests never depend on, or
// collide with, rows that already exist in the database. bankAccount is
// stored in the legacy plaintext column (nil = none).
func createFixtureEmployee(ctx context.Context, t *testing.T, bankAccount *string) string {
	t.Helper()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	var employeeID string
	if err := repository.DB(ctx, nil).QueryRow(ctx, `
		INSERT INTO employees (full_name, email, position, date_joined, employment_status, bank_account_number, bank_name)
		VALUES ($1, $2, 'Full Time', DATE '2098-01-05', 'active', $3, 'BCA')
		RETURNING id::text
	`, "Fixture "+suffix, "fixture-"+suffix+"@example.test", bankAccount).Scan(&employeeID); err != nil {
		t.Fatalf("create fixture employee: %v", err)
	}
	return employeeID
}

// testRunSuffix makes document numbers unique per test run.
func testRunSuffix() string {
	return strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
}

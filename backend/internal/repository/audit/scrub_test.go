package audit

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// TestScrubRedactedKeys needs a migrated database (KANTOR_TEST_DATABASE_URL,
// e.g. the isolated verify database). It runs in one rolled-back
// transaction with the tenant GUC set, so it never changes real rows.
func TestScrubRedactedKeys(t *testing.T) {
	dsn := os.Getenv("KANTOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KANTOR_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	var tenantID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenantID); err != nil {
		t.Fatalf("find a tenant: %v", err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant', $1, false)`, tenantID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), "RESET ALL") }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	txCtx := repository.WithConn(ctx, tx)

	repo := NewRepository(pool)
	// Pretend the scrub never ran for this tenant.
	if _, err := tx.Exec(ctx, `DELETE FROM system_settings WHERE key = $1`, auditRedactionSettingKey); err != nil {
		t.Fatal(err)
	}

	// A legacy row written before the denylist (raw SQL, like old code did)
	// and a harmless row.
	var legacyID, harmlessID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO audit_logs (action, module, resource, resource_id, old_value, new_value)
		VALUES ('update', 'hris', 'employee', 'scrub-test',
		        '{"bank_account_number": "1234567890", "bank_name": "BCA"}',
		        '{"BaseSalary": 15000000, "items": [{"amount": 500000, "reason": "THR"}], "full_name": "Budi"}')
		RETURNING id::text
	`).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO audit_logs (action, module, resource, resource_id, new_value)
		VALUES ('view', 'hris', 'salary', 'scrub-test', '{"records_count": 3}')
		RETURNING id::text
	`).Scan(&harmlessID); err != nil {
		t.Fatal(err)
	}

	// A registration-settings update from before the handler dropped the
	// live code: "code" is only redacted on that resource.
	var registrationID, otherCodeID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO audit_logs (action, module, resource, resource_id, old_value, new_value)
		VALUES ('update', 'admin', 'system_setting', 'registration',
		        '{"code": "OLD-SECRET-CODE", "has_code": true}',
		        '{"code": "NEW-SECRET-CODE", "has_code": true, "enabled": true}')
		RETURNING id::text
	`).Scan(&registrationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO audit_logs (action, module, resource, resource_id, new_value)
		VALUES ('create', 'marketing', 'campaign', 'scrub-test', '{"code": "PROMO10"}')
		RETURNING id::text
	`).Scan(&otherCodeID); err != nil {
		t.Fatal(err)
	}

	// A run interrupted after the last row resumes there and only finishes
	// the marker: nothing before its position is scanned again.
	if _, err := tx.Exec(ctx, `
		INSERT INTO system_settings (key, value)
		VALUES ($1, jsonb_build_object('version', 1, 'scrubbed_rows', 7, 'resume_version', $2::int, 'resume_after_id', 'ffffffff-ffff-ffff-ffff-ffffffffffff'))
	`, auditRedactionSettingKey, auditRedactionVersion); err != nil {
		t.Fatal(err)
	}
	resumed, err := repo.ScrubRedactedKeys(txCtx)
	if err != nil || !resumed.Resumed || resumed.Scanned != 0 || resumed.Updated != 7 {
		t.Fatalf("resumed run = %+v, %v", resumed, err)
	}
	if again, err := repo.ScrubRedactedKeys(txCtx); err != nil || !again.AlreadyDone {
		t.Fatalf("run after resume = %+v, %v", again, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM system_settings WHERE key = $1`, auditRedactionSettingKey); err != nil {
		t.Fatal(err)
	}
	// An older finished version is scrubbed again for the new keys.
	if _, err := tx.Exec(ctx, `
		INSERT INTO system_settings (key, value) VALUES ($1, '{"version": 1, "scrubbed_rows": 0}')
	`, auditRedactionSettingKey); err != nil {
		t.Fatal(err)
	}

	result, err := repo.ScrubRedactedKeys(txCtx)
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyDone || result.Updated < 1 || result.Scanned < result.Updated {
		t.Fatalf("result = %+v", result)
	}

	var oldValue, newValue string
	if err := tx.QueryRow(ctx, `SELECT old_value::text, new_value::text FROM audit_logs WHERE id = $1::uuid`, legacyID).Scan(&oldValue, &newValue); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"1234567890", "15000000", "500000"} {
		if strings.Contains(oldValue+newValue, secret) {
			t.Errorf("scrubbed row still holds %q: %s / %s", secret, oldValue, newValue)
		}
	}
	if !strings.Contains(oldValue, `"bank_name": "BCA"`) || !strings.Contains(newValue, `"reason": "THR"`) || !strings.Contains(newValue, `"BaseSalary": "[redacted]"`) {
		t.Errorf("scrubbed row = %s / %s", oldValue, newValue)
	}
	var harmless string
	if err := tx.QueryRow(ctx, `SELECT new_value::text FROM audit_logs WHERE id = $1::uuid`, harmlessID).Scan(&harmless); err != nil {
		t.Fatal(err)
	}
	if harmless != `{"records_count": 3}` {
		t.Errorf("harmless row changed: %s", harmless)
	}

	var registrationOld, registrationNew, otherCode string
	if err := tx.QueryRow(ctx, `SELECT old_value::text, new_value::text FROM audit_logs WHERE id = $1::uuid`, registrationID).Scan(&registrationOld, &registrationNew); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(registrationOld+registrationNew, "SECRET-CODE") || !strings.Contains(registrationNew, `"enabled": true`) {
		t.Errorf("registration row = %s / %s", registrationOld, registrationNew)
	}
	if err := tx.QueryRow(ctx, `SELECT new_value::text FROM audit_logs WHERE id = $1::uuid`, otherCodeID).Scan(&otherCode); err != nil {
		t.Fatal(err)
	}
	if otherCode != `{"code": "PROMO10"}` {
		t.Errorf("unrelated code row changed: %s", otherCode)
	}

	// The marker makes the next start skip the scan.
	again, err := repo.ScrubRedactedKeys(txCtx)
	if err != nil || !again.AlreadyDone || again.Scanned != 0 {
		t.Errorf("second run = %+v, %v", again, err)
	}

	// New rows go through the insert path and are redacted on write.
	var userID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM users ORDER BY created_at LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("find a user: %v", err)
	}
	if err := repo.Insert(txCtx, Entry{UserID: userID, Action: "create", Module: "hris", Resource: "bonus", ResourceID: "scrub-test", NewValue: map[string]any{"amount": 1000, "reason": "Bonus"}}); err != nil {
		t.Fatal(err)
	}
	var inserted string
	if err := tx.QueryRow(ctx, `SELECT new_value::text FROM audit_logs WHERE resource = 'bonus' AND resource_id = 'scrub-test'`).Scan(&inserted); err != nil {
		t.Fatal(err)
	}
	if inserted != `{"amount": "[redacted]", "reason": "Bonus"}` {
		t.Errorf("inserted row = %s", inserted)
	}

	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

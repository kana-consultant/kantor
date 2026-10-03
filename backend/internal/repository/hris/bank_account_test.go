package hris

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

var (
	testEncrypterOnce sync.Once
	testEncrypter     *security.Encrypter
	testEncrypterErr  error
)

func bankAccountTestEncrypter(t *testing.T) *security.Encrypter {
	t.Helper()
	testEncrypterOnce.Do(func() {
		testEncrypter, testEncrypterErr = security.NewEncrypter("bank-account-test-key")
	})
	if testEncrypterErr != nil {
		t.Fatal(testEncrypterErr)
	}
	return testEncrypter
}

func strPtr(value string) *string { return &value }

func TestOpenAndSealBankAccount(t *testing.T) {
	encrypter := bankAccountTestEncrypter(t)

	sealed, err := sealBankAccount(encrypter, strPtr("  1234567890 "))
	if err != nil || sealed == nil {
		t.Fatalf("seal = %v, %v", sealed, err)
	}
	if strings.Contains(*sealed, "1234567890") || !strings.HasPrefix(*sealed, "v1:") {
		t.Fatalf("ciphertext = %q", *sealed)
	}
	opened, err := openBankAccount(encrypter, nil, sealed)
	if err != nil || opened == nil || *opened != "1234567890" {
		t.Fatalf("open = %v, %v", opened, err)
	}

	// A plaintext next to a ciphertext was written later (out of band) and
	// is the newer value; a blank plaintext does not hide the ciphertext.
	if opened, _ := openBankAccount(encrypter, strPtr("1111"), sealed); *opened != "1111" {
		t.Errorf("ciphertext + plaintext = %q", *opened)
	}
	if opened, _ := openBankAccount(encrypter, strPtr(" "), sealed); *opened != "1234567890" {
		t.Errorf("ciphertext + blank plaintext = %q", *opened)
	}
	if opened, err := openBankAccount(encrypter, strPtr(" 1111 "), nil); err != nil || *opened != "1111" {
		t.Errorf("plaintext fallback = %v, %v", opened, err)
	}
	if opened, err := openBankAccount(nil, strPtr("1111"), nil); err != nil || *opened != "1111" {
		t.Errorf("plaintext fallback without encrypter = %v, %v", opened, err)
	}
	if opened, err := openBankAccount(encrypter, strPtr("  "), nil); err != nil || opened != nil {
		t.Errorf("blank = %v, %v", opened, err)
	}

	// Empty input stores NULL and needs no key.
	for _, value := range []*string{nil, strPtr(""), strPtr("   ")} {
		if sealed, err := sealBankAccount(nil, value); err != nil || sealed != nil {
			t.Errorf("seal empty = %v, %v", sealed, err)
		}
	}
	if _, err := sealBankAccount(nil, strPtr("123")); !errors.Is(err, ErrBankAccountCipherMissing) {
		t.Errorf("seal without encrypter err = %v", err)
	}
	if _, err := openBankAccount(nil, nil, sealed); !errors.Is(err, ErrBankAccountCipherMissing) {
		t.Errorf("open without encrypter err = %v", err)
	}
	other, err := security.NewEncrypter("another-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openBankAccount(other, nil, sealed); err == nil || strings.Contains(err.Error(), "1234567890") {
		t.Errorf("open with the wrong key err = %v", err)
	}
	// Row reads fail soft: an unreadable ciphertext reads as missing.
	if value, unreadable := readBankAccount(other, "employee-id", nil, sealed); value != nil || !unreadable {
		t.Errorf("read with the wrong key = %v, %v", value, unreadable)
	}
	if value, unreadable := readBankAccount(encrypter, "employee-id", nil, sealed); value == nil || *value != "1234567890" || unreadable {
		t.Errorf("read = %v, %v", value, unreadable)
	}
}

// bankAccountColumns reads both bank account columns and updated_at of one
// employee straight from the table.
type bankAccountColumns struct {
	plain     *string
	sealed    *string
	updatedAt time.Time
}

func readBankAccountColumns(ctx context.Context, t *testing.T, employeeID string) bankAccountColumns {
	t.Helper()
	var cols bankAccountColumns
	if err := repository.DB(ctx, nil).QueryRow(ctx,
		`SELECT bank_account_number, bank_account_encrypted, updated_at FROM employees WHERE id = $1::uuid`,
		employeeID,
	).Scan(&cols.plain, &cols.sealed, &cols.updatedAt); err != nil {
		t.Fatalf("read bank account columns of %s: %v", employeeID, err)
	}
	return cols
}

// assertSealedHolds checks that the stored ciphertext decrypts to want and
// does not contain it in clear.
func assertSealedHolds(t *testing.T, encrypter *security.Encrypter, label string, sealed *string, want string) {
	t.Helper()
	if sealed == nil {
		t.Errorf("%s: no ciphertext, want %q", label, want)
		return
	}
	if strings.Contains(*sealed, want) {
		t.Errorf("%s: ciphertext contains the number", label)
	}
	opened, err := encrypter.DecryptString(*sealed)
	if err != nil || opened != want {
		t.Errorf("%s: ciphertext opens to %q, %v; want %q", label, opened, err, want)
	}
}

func assertPlain(t *testing.T, label string, plain *string, want *string) {
	t.Helper()
	switch {
	case want == nil && plain != nil:
		t.Errorf("%s: plaintext = %q, want NULL", label, *plain)
	case want != nil && (plain == nil || *plain != *want):
		t.Errorf("%s: plaintext = %v, want %q", label, plain, *want)
	}
}

// TestBankAccountBackfill runs against a migrated database
// (KANTOR_TEST_DATABASE_URL) in one rolled-back transaction, with the
// default (non-destructive) setting: the backfill only fills the ciphertext,
// never changes the plaintext column or updated_at, re-encrypts a stale or
// unreadable ciphertext from the plaintext, and a re-run changes nothing;
// writes store both columns; the guarded update refuses a row that changed
// meanwhile.
func TestBankAccountBackfill(t *testing.T) {
	pool, tenantID := sequenceTestPool(t)
	encrypter := bankAccountTestEncrypter(t)
	repo := NewEmployeesRepository(pool, encrypter)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	errRollback := errors.New("rollback")
	err := withTenantTx(ctx, pool, tenantID, false, func(ctx context.Context) error {
		db := repository.DB(ctx, nil)
		legacy := createFixtureEmployee(ctx, t, strPtr("1234567890"))
		blank := createFixtureEmployee(ctx, t, strPtr("  "))
		// Whitespace that SQL btrim does not strip (written by SQL or an
		// import): blank for reads and the backfill, so never encrypted and
		// not reported as unsealed.
		tabOnly := createFixtureEmployee(ctx, t, strPtr("\t"))
		nbspOnly := createFixtureEmployee(ctx, t, strPtr("\u00a0"))
		none := createFixtureEmployee(ctx, t, nil)
		// A plaintext written out of band next to an older ciphertext: the
		// newer number, re-encrypted by the backfill.
		superseded := createFixtureEmployee(ctx, t, nil)
		olderSealed, err := sealBankAccount(encrypter, strPtr("1111000000"))
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_encrypted = $2, bank_account_number = '2222000000' WHERE id = $1::uuid`, superseded, *olderSealed); err != nil {
			return err
		}
		// Already in sync (as this code writes it): left alone.
		inSync := createFixtureEmployee(ctx, t, strPtr("7777000000"))
		syncSealed, err := sealBankAccount(encrypter, strPtr("7777000000"))
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_encrypted = $2 WHERE id = $1::uuid`, inSync, *syncSealed); err != nil {
			return err
		}
		// A ciphertext nobody can decrypt next to a plaintext: re-encrypted
		// from the plaintext.
		garbled := createFixtureEmployee(ctx, t, strPtr("8888000000"))
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_encrypted = 'v1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' WHERE id = $1::uuid`, garbled); err != nil {
			return err
		}
		fixtures := []string{legacy, blank, tabOnly, nbspOnly, none, superseded, inSync, garbled}
		before := make(map[string]bankAccountColumns, len(fixtures))
		for _, id := range fixtures {
			before[id] = readBankAccountColumns(ctx, t, id)
		}

		// Before: the legacy row is read through the plaintext.
		got, err := repo.GetEmployeeByID(ctx, legacy)
		if err != nil || got.BankAccountNumber == nil || *got.BankAccountNumber != "1234567890" {
			t.Fatalf("before = %v, %v", got.BankAccountNumber, err)
		}

		result, err := repo.BackfillBankAccountEncryption(ctx)
		if err != nil {
			return err
		}
		// Other rows of the database may be anything, so only lower bounds
		// are checked globally and the fixtures are checked one by one.
		if result.ClearPlaintext || result.Cleared != 0 || result.Encrypted < 3 || result.Superseded < 2 || result.InSync < 1 || result.Remaining < 7 {
			t.Errorf("result = %+v", result)
		}
		// Every non-blank number of the tenant has its ciphertext now; the
		// TAB- and NBSP-only rows are blank and do not count.
		if result.Unsealed != 0 {
			t.Errorf("unsealed = %d, want 0 (whitespace-only plaintexts are not numbers)", result.Unsealed)
		}

		// The plaintext column and updated_at are untouched on every fixture.
		for _, id := range fixtures {
			after := readBankAccountColumns(ctx, t, id)
			assertPlain(t, "fixture "+id, after.plain, before[id].plain)
			if !after.updatedAt.Equal(before[id].updatedAt) {
				t.Errorf("fixture %s: updated_at changed %v -> %v", id, before[id].updatedAt, after.updatedAt)
			}
		}
		assertSealedHolds(t, encrypter, "legacy", readBankAccountColumns(ctx, t, legacy).sealed, "1234567890")
		assertSealedHolds(t, encrypter, "superseded", readBankAccountColumns(ctx, t, superseded).sealed, "2222000000")
		assertSealedHolds(t, encrypter, "garbled", readBankAccountColumns(ctx, t, garbled).sealed, "8888000000")
		if after := readBankAccountColumns(ctx, t, inSync); after.sealed == nil || *after.sealed != *syncSealed {
			t.Errorf("in-sync row: ciphertext rewritten")
		}
		for _, id := range []string{blank, tabOnly, nbspOnly, none} {
			if after := readBankAccountColumns(ctx, t, id); after.sealed != nil {
				t.Errorf("row %s without a number got a ciphertext", id)
			}
			if got, err := repo.GetEmployeeByID(ctx, id); err != nil || got.BankAccountNumber != nil {
				t.Errorf("row %s without a number reads %v, %v", id, got.BankAccountNumber, err)
			}
		}

		// Reads return the same numbers as before the backfill.
		for id, want := range map[string]string{legacy: "1234567890", superseded: "2222000000", inSync: "7777000000", garbled: "8888000000"} {
			if got, err := repo.GetEmployeeByID(ctx, id); err != nil || got.BankAccountNumber == nil || *got.BankAccountNumber != want || got.BankAccountUnreadable {
				t.Errorf("read %s = %v, %v", id, got.BankAccountNumber, err)
			}
		}
		listed, _, err := repo.ListEmployees(ctx, ListEmployeesParams{Page: 1, PerPage: 1000, Search: got.FullName})
		if err != nil || len(listed) != 1 || listed[0].BankAccountNumber == nil || *listed[0].BankAccountNumber != "1234567890" {
			t.Errorf("list after backfill = %+v, %v", listed, err)
		}
		// The PKWT and the payslip read the same columns (the PKWT prints the
		// full number, the slip masks it in the service).
		contractEmployee, err := NewContractsRepository(pool, encrypter).GetEmployee(ctx, legacy)
		if err != nil || contractEmployee.BankAccountNumber == nil || *contractEmployee.BankAccountNumber != "1234567890" {
			t.Errorf("contract employee = %v, %v", contractEmployee.BankAccountNumber, err)
		}
		payslipEmployee, err := NewPayslipsRepository(pool, encrypter).GetEmployee(ctx, legacy)
		if err != nil || payslipEmployee.BankAccountNumber == nil || *payslipEmployee.BankAccountNumber != "1234567890" {
			t.Errorf("payslip employee = %v, %v", payslipEmployee.BankAccountNumber, err)
		}
		if empty, err := repo.GetEmployeeByID(ctx, none); err != nil || empty.BankAccountNumber != nil {
			t.Errorf("no account = %v, %v", empty.BankAccountNumber, err)
		}

		// Idempotent: nothing left to do (rows outside the fixtures that
		// the first run could not convert are not counted here).
		again, err := repo.BackfillBankAccountEncryption(ctx)
		if err != nil || again.Encrypted != 0 || again.Cleared != 0 || again.InSync < 4 || again.Unsealed != 0 {
			t.Errorf("second run = %+v, %v", again, err)
		}

		// Known limitation, pinned here and documented in
		// docs/deployment.md ("Bank account numbers"): a number deleted
		// outside this code by clearing ONLY the plaintext column (SQL, or the
		// previous release, which cannot start on this schema) leaves the
		// ciphertext, and reads fall back to it, exactly like a row cleared by
		// the BANK_ACCOUNT_CLEAR_PLAINTEXT opt-in. A deletion has to clear
		// both columns; saving the employee with an empty number does that.
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_number = NULL WHERE id = $1::uuid`, legacy); err != nil {
			return err
		}
		if got, err := repo.GetEmployeeByID(ctx, legacy); err != nil || got.BankAccountNumber == nil || *got.BankAccountNumber != "1234567890" {
			t.Errorf("plaintext-only deletion: read = %v, %v (ciphertext fallback expected)", got.BankAccountNumber, err)
		}
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_number = NULL, bank_account_encrypted = NULL WHERE id = $1::uuid`, legacy); err != nil {
			return err
		}
		if got, err := repo.GetEmployeeByID(ctx, legacy); err != nil || got.BankAccountNumber != nil {
			t.Errorf("deletion of both columns: read = %v, %v", got.BankAccountNumber, err)
		}

		// An unreadable ciphertext without a plaintext does not fail the
		// read; a save that keeps the stored number leaves both columns
		// untouched, a new number replaces it in both.
		unreadable := createFixtureEmployee(ctx, t, nil)
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_encrypted = 'v1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' WHERE id = $1::uuid`, unreadable); err != nil {
			return err
		}
		broken, err := repo.GetEmployeeByID(ctx, unreadable)
		if err != nil || broken.BankAccountNumber != nil || !broken.BankAccountUnreadable {
			t.Fatalf("unreadable row = %v, %v, %v", broken.BankAccountNumber, broken.BankAccountUnreadable, err)
		}
		if _, err := repo.UpdateEmployee(ctx, unreadable, UpsertEmployeeParams{
			FullName: broken.FullName, Email: broken.Email, Position: broken.Position,
			DateJoined: broken.DateJoined, EmploymentStatus: broken.EmploymentStatus,
			KeepStoredBankAccount: true,
		}); err != nil {
			t.Fatalf("keep update: %v", err)
		}
		if kept := readBankAccountColumns(ctx, t, unreadable); kept.sealed == nil || *kept.sealed != "v1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" || kept.plain != nil {
			t.Errorf("kept columns = %v, %v", kept.plain, kept.sealed)
		}
		if fixed, err := repo.UpdateEmployee(ctx, unreadable, UpsertEmployeeParams{
			FullName: broken.FullName, Email: broken.Email, Position: broken.Position,
			DateJoined: broken.DateJoined, EmploymentStatus: broken.EmploymentStatus,
			BankAccountNumber: strPtr("3333000000"),
		}); err != nil || fixed.BankAccountNumber == nil || *fixed.BankAccountNumber != "3333000000" || fixed.BankAccountUnreadable {
			t.Errorf("repair update = %+v, %v", fixed.BankAccountNumber, err)
		}
		repaired := readBankAccountColumns(ctx, t, unreadable)
		assertPlain(t, "repaired", repaired.plain, strPtr("3333000000"))
		assertSealedHolds(t, encrypter, "repaired", repaired.sealed, "3333000000")

		// The guarded update refuses a row whose plaintext changed after
		// it was read.
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_number = '5555', bank_account_encrypted = NULL WHERE id = $1::uuid`, none); err != nil {
			return err
		}
		stale, err := sealBankAccount(encrypter, strPtr("4444"))
		if err != nil {
			return err
		}
		if updated, err := repo.sealPlaintextBankAccount(ctx, bankAccountBackfillRow{id: none, plain: "4444"}, stale, false); err != nil || updated {
			t.Errorf("stale guarded update = %v, %v", updated, err)
		}
		// ... and one whose ciphertext changed after it was read.
		if updated, err := repo.sealPlaintextBankAccount(ctx, bankAccountBackfillRow{id: none, plain: "5555", sealed: syncSealed}, stale, false); err != nil || updated {
			t.Errorf("stale ciphertext guarded update = %v, %v", updated, err)
		}

		// Writes store both columns (dual-write), trimmed.
		employee, err := repo.GetEmployeeByID(ctx, none)
		if err != nil {
			return err
		}
		updated, err := repo.UpdateEmployee(ctx, none, UpsertEmployeeParams{
			FullName: employee.FullName, Email: employee.Email, Position: employee.Position,
			DateJoined: employee.DateJoined, EmploymentStatus: employee.EmploymentStatus,
			BankAccountNumber: strPtr(" 9876543210 "), BankName: strPtr("BNI"),
		})
		if err != nil || updated.BankAccountNumber == nil || *updated.BankAccountNumber != "9876543210" {
			t.Fatalf("update = %v, %v", updated.BankAccountNumber, err)
		}
		written := readBankAccountColumns(ctx, t, none)
		assertPlain(t, "after update", written.plain, strPtr("9876543210"))
		assertSealedHolds(t, encrypter, "after update", written.sealed, "9876543210")

		// Emptying the number empties both columns.
		if _, err := repo.UpdateEmployee(ctx, none, UpsertEmployeeParams{
			FullName: employee.FullName, Email: employee.Email, Position: employee.Position,
			DateJoined: employee.DateJoined, EmploymentStatus: employee.EmploymentStatus,
			BankAccountNumber: strPtr("  "),
		}); err != nil {
			t.Fatalf("empty update: %v", err)
		}
		if emptied := readBankAccountColumns(ctx, t, none); emptied.plain != nil || emptied.sealed != nil {
			t.Errorf("after emptying: plain=%v sealed=%v", emptied.plain, emptied.sealed)
		}

		suffix := testRunSuffix()
		created, err := repo.CreateEmployee(ctx, UpsertEmployeeParams{
			FullName: "Fixture Create " + suffix, Email: "fixture-create-" + strings.ToLower(suffix) + "@example.test",
			Position: "Full Time", DateJoined: time.Date(2098, 1, 5, 0, 0, 0, 0, time.UTC), EmploymentStatus: "active",
			BankAccountNumber: strPtr(" 5550001111 "),
		})
		if err != nil || created.BankAccountNumber == nil || *created.BankAccountNumber != "5550001111" {
			t.Fatalf("create = %v, %v", created.BankAccountNumber, err)
		}
		createdCols := readBankAccountColumns(ctx, t, created.ID)
		assertPlain(t, "after create", createdCols.plain, strPtr("5550001111"))
		assertSealedHolds(t, encrypter, "after create", createdCols.sealed, "5550001111")
		return errRollback
	})
	if err != nil && !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
}

// TestBankAccountBackfillClearPlaintextOptIn covers the opt-in
// BANK_ACCOUNT_CLEAR_PLAINTEXT=true (rolled back): the backfill encrypts and
// then clears the plaintext column, blank plaintexts included, and writes
// store the ciphertext only.
func TestBankAccountBackfillClearPlaintextOptIn(t *testing.T) {
	pool, tenantID := sequenceTestPool(t)
	encrypter := bankAccountTestEncrypter(t)
	repo := NewEmployeesRepository(pool, encrypter)
	repo.SetClearBankAccountPlaintext(true)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	errRollback := errors.New("rollback")
	err := withTenantTx(ctx, pool, tenantID, false, func(ctx context.Context) error {
		db := repository.DB(ctx, nil)
		legacy := createFixtureEmployee(ctx, t, strPtr("1234567890"))
		blank := createFixtureEmployee(ctx, t, strPtr("  "))
		inSync := createFixtureEmployee(ctx, t, strPtr("7777000000"))
		syncSealed, err := sealBankAccount(encrypter, strPtr("7777000000"))
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_encrypted = $2 WHERE id = $1::uuid`, inSync, *syncSealed); err != nil {
			return err
		}

		result, err := repo.BackfillBankAccountEncryption(ctx)
		if err != nil {
			return err
		}
		if !result.ClearPlaintext || result.Encrypted < 1 || result.Cleared < 3 || result.InSync < 1 {
			t.Errorf("result = %+v", result)
		}
		for _, id := range []string{legacy, blank, inSync} {
			if cols := readBankAccountColumns(ctx, t, id); cols.plain != nil {
				t.Errorf("row %s still holds plaintext", id)
			}
		}
		assertSealedHolds(t, encrypter, "legacy", readBankAccountColumns(ctx, t, legacy).sealed, "1234567890")
		if cols := readBankAccountColumns(ctx, t, inSync); cols.sealed == nil || *cols.sealed != *syncSealed {
			t.Errorf("in-sync row: ciphertext rewritten")
		}
		if cols := readBankAccountColumns(ctx, t, blank); cols.sealed != nil {
			t.Errorf("blank row got a ciphertext")
		}
		if got, err := repo.GetEmployeeByID(ctx, legacy); err != nil || got.BankAccountNumber == nil || *got.BankAccountNumber != "1234567890" {
			t.Errorf("read after clearing = %v, %v", got.BankAccountNumber, err)
		}

		again, err := repo.BackfillBankAccountEncryption(ctx)
		if err != nil || again.Encrypted != 0 || again.Cleared != 0 {
			t.Errorf("second run = %+v, %v", again, err)
		}

		employee, err := repo.GetEmployeeByID(ctx, legacy)
		if err != nil {
			return err
		}
		if _, err := repo.UpdateEmployee(ctx, legacy, UpsertEmployeeParams{
			FullName: employee.FullName, Email: employee.Email, Position: employee.Position,
			DateJoined: employee.DateJoined, EmploymentStatus: employee.EmploymentStatus,
			BankAccountNumber: strPtr("9876543210"),
		}); err != nil {
			t.Fatalf("update: %v", err)
		}
		written := readBankAccountColumns(ctx, t, legacy)
		assertPlain(t, "after update", written.plain, nil)
		assertSealedHolds(t, encrypter, "after update", written.sealed, "9876543210")
		return errRollback
	})
	if err != nil && !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
}

// TestBankAccountDownMigration runs the down migration of
// 20260930150000 inside a rolled-back transaction: it drops
// bank_account_encrypted when every row with a ciphertext still has its
// plaintext (the default dual-write state), and refuses while a row keeps
// its number only in the ciphertext.
func TestBankAccountDownMigration(t *testing.T) {
	pool, tenantID := sequenceTestPool(t)
	encrypter := bankAccountTestEncrypter(t)
	downSQL, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "20260930150000_employee_bank_account_encrypted.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	columnExists := func(ctx context.Context) bool {
		t.Helper()
		var exists bool
		if err := repository.DB(ctx, nil).QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = 'employees' AND column_name = 'bank_account_encrypted'
			)`).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		return exists
	}

	errRollback := errors.New("rollback")
	err = withTenantTx(ctx, pool, tenantID, false, func(ctx context.Context) error {
		db := repository.DB(ctx, nil)
		// Dual-written row: plaintext and ciphertext.
		dual := createFixtureEmployee(ctx, t, strPtr("1234567890"))
		sealed, err := sealBankAccount(encrypter, strPtr("1234567890"))
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_encrypted = $2 WHERE id = $1::uuid`, dual, *sealed); err != nil {
			return err
		}

		// Rows outside this test (any tenant) that already keep their number
		// only in the ciphertext make the success path impossible to check.
		if _, err := db.Exec(ctx, `SAVEPOINT probe`); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `ALTER TABLE employees NO FORCE ROW LEVEL SECURITY`); err != nil {
			return err
		}
		var sealedOnlyElsewhere int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*) FROM employees
			WHERE bank_account_encrypted IS NOT NULL
			  AND (bank_account_number IS NULL OR btrim(bank_account_number) = '')`).Scan(&sealedOnlyElsewhere); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `ROLLBACK TO SAVEPOINT probe`); err != nil {
			return err
		}

		if sealedOnlyElsewhere > 0 {
			t.Logf("database already holds %d sealed-only row(s); success path not checked", sealedOnlyElsewhere)
		} else {
			if _, err := db.Exec(ctx, `SAVEPOINT down_ok`); err != nil {
				return err
			}
			if _, err := db.Exec(ctx, string(downSQL)); err != nil {
				t.Errorf("down migration with every ciphertext backed by a plaintext: %v", err)
			} else if columnExists(ctx) {
				t.Errorf("bank_account_encrypted still exists after the down migration")
			}
			if _, err := db.Exec(ctx, `ROLLBACK TO SAVEPOINT down_ok`); err != nil {
				return err
			}
		}

		// A row whose plaintext was cleared (opt-in) keeps its only copy in
		// the ciphertext: the down migration must refuse.
		sealedOnly := createFixtureEmployee(ctx, t, nil)
		if _, err := db.Exec(ctx, `UPDATE employees SET bank_account_encrypted = $2 WHERE id = $1::uuid`, sealedOnly, *sealed); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `SAVEPOINT down_refused`); err != nil {
			return err
		}
		_, downErr := db.Exec(ctx, string(downSQL))
		if downErr == nil || !strings.Contains(downErr.Error(), "refusing to drop employees.bank_account_encrypted") {
			t.Errorf("down migration with a sealed-only row: err = %v", downErr)
		}
		if _, err := db.Exec(ctx, `ROLLBACK TO SAVEPOINT down_refused`); err != nil {
			return err
		}
		if !columnExists(ctx) {
			t.Errorf("bank_account_encrypted dropped although a row keeps its number only there")
		}
		return errRollback
	})
	if err != nil && !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
}

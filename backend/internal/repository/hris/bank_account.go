package hris

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

// Bank account numbers are stored as security.Encrypter ciphertext in
// employees.bank_account_encrypted (migration
// 20260930150000_employee_bank_account_encrypted). By default the plaintext
// column employees.bank_account_number is KEPT and written next to it
// (dual-write), so existing data is never removed and an older binary still
// reads correct numbers. Clearing the plaintext is an explicit opt-in
// (BANK_ACCOUNT_CLEAR_PLAINTEXT, EmployeesRepository.SetClearBankAccountPlaintext),
// to be run only after a verified backup; with it, writes store the
// ciphertext only and the startup backfill clears the plaintext column.

// ErrBankAccountCipherMissing is returned when a bank account number has to
// be encrypted or decrypted but the repository has no encrypter.
var ErrBankAccountCipherMissing = errors.New("bank account encryption is not configured")

// openBankAccount returns the readable account number of a row. A non-blank
// plaintext wins: this code always writes both columns with the same number
// (or, with the clear opt-in, the ciphertext only), so a plaintext that
// differs from the ciphertext can only come from a later out-of-band write
// (an older binary after a rollback, a manual fix) and is the newer value;
// the startup backfill re-encrypts it. Otherwise the ciphertext is
// decrypted.
func openBankAccount(encrypter *security.Encrypter, plaintext *string, ciphertext *string) (*string, error) {
	if plaintext != nil && strings.TrimSpace(*plaintext) != "" {
		value := strings.TrimSpace(*plaintext)
		return &value, nil
	}
	if ciphertext != nil && strings.TrimSpace(*ciphertext) != "" {
		if encrypter == nil {
			return nil, ErrBankAccountCipherMissing
		}
		value, err := encrypter.DecryptString(*ciphertext)
		if err != nil {
			return nil, fmt.Errorf("decrypt bank account number: %w", err)
		}
		return &value, nil
	}
	return nil, nil
}

// readBankAccount is openBankAccount for row reads: a ciphertext that cannot
// be decrypted (sealed under another DATA_ENCRYPTION_KEY, corrupted) must not
// take the whole employee record down with it, so it is logged with the
// employee id (never the value) and read as "no account number"; unreadable
// reports that case. Payslip and PKWT preflight then report the number as
// missing, and the stored ciphertext is kept until a new number is saved
// (see UpsertEmployeeParams.KeepStoredBankAccount).
func readBankAccount(encrypter *security.Encrypter, employeeID string, plaintext *string, ciphertext *string) (value *string, unreadable bool) {
	value, err := openBankAccount(encrypter, plaintext, ciphertext)
	if err != nil {
		slog.Warn("bank account number unreadable; treated as missing",
			"employee_id", employeeID,
			"error", err,
		)
		return nil, true
	}
	return value, false
}

// sealBankAccount encrypts a submitted account number for
// bank_account_encrypted. An empty value is stored as NULL.
func sealBankAccount(encrypter *security.Encrypter, value *string) (*string, error) {
	plain := nullableString(value)
	if plain == "" {
		return nil, nil
	}
	if encrypter == nil {
		return nil, ErrBankAccountCipherMissing
	}
	sealed, err := encrypter.EncryptString(plain)
	if err != nil {
		return nil, fmt.Errorf("encrypt bank account number: %w", err)
	}
	return &sealed, nil
}

// plaintextBankAccount is the value written to bank_account_number next to
// the ciphertext: the trimmed number (dual-write), or NULL when it is empty
// or when the clear-plaintext opt-in is on.
func (r *EmployeesRepository) plaintextBankAccount(value *string) *string {
	if r.clearBankAccountPlaintext {
		return nil
	}
	plain := nullableString(value)
	if plain == "" {
		return nil
	}
	return &plain
}

// BankAccountBackfillResult counts what one backfill run did. It never
// carries account numbers.
type BankAccountBackfillResult struct {
	// ClearPlaintext: the run cleared the plaintext column (opt-in).
	ClearPlaintext bool
	// Candidates: rows with a plaintext value (blank included).
	Candidates int
	// InSync: rows whose ciphertext already holds the plaintext number.
	InSync int
	// Encrypted: rows whose ciphertext was written by this run.
	Encrypted int
	// Superseded: the part of Encrypted whose row already had a ciphertext
	// that held another number or could not be decrypted; the plaintext is
	// the newer value and replaced it.
	Superseded int
	// Cleared: rows whose plaintext column was set to NULL (opt-in only).
	Cleared int
	// Skipped: rows changed by someone else between the read and the
	// guarded update; the next run picks them up if still needed.
	Skipped int
	// Unsealed: plaintext numbers that are not blank (after
	// strings.TrimSpace, the rule reads and the backfill use) and still have
	// no ciphertext after the run; 0 when the backfill is complete. A
	// whitespace-only plaintext (spaces, TAB, NBSP, ...) reads as no number
	// and is never encrypted, so it is not counted. Counted from the table
	// after the run, so a skipped row that is still unsealed counts too.
	Unsealed int
	// Remaining: plaintext values still stored after the run. With the
	// plaintext kept (the default) this is every row with a number.
	Remaining int
}

type bankAccountBackfillRow struct {
	id string
	// plain is bank_account_number exactly as stored (the guard compares it).
	plain string
	// sealed is the stored ciphertext, nil when there is none.
	sealed *string
}

// SetClearBankAccountPlaintext switches the clear-plaintext opt-in
// (config DataMaintenance.BankAccountClearPlaintext). Off (the default):
// writes store both columns and the backfill only fills the ciphertext. On:
// writes store the ciphertext only and the backfill clears the plaintext
// column once the ciphertext holds the same number. Call it before the
// repository is used.
func (r *EmployeesRepository) SetClearBankAccountPlaintext(clear bool) {
	r.clearBankAccountPlaintext = clear
}

// BackfillBankAccountEncryption fills bank_account_encrypted from the
// plaintext bank account numbers of the tenant in ctx. A row is written only
// when its ciphertext is missing, holds another number or cannot be
// decrypted; each write is ONE guarded statement that applies only if both
// columns are still what was read, so it is idempotent and safe to run on
// every start (and next to live writes). The plaintext column is left
// exactly as it is and updated_at is not touched, unless the
// clear-plaintext opt-in is on: then the plaintext is set to NULL in the
// same statement (blank plaintexts included).
func (r *EmployeesRepository) BackfillBankAccountEncryption(ctx context.Context) (BankAccountBackfillResult, error) {
	result := BankAccountBackfillResult{ClearPlaintext: r.clearBankAccountPlaintext}
	if r.encrypter == nil {
		return result, ErrBankAccountCipherMissing
	}

	pending, err := r.listPlaintextBankAccounts(ctx)
	if err != nil {
		return result, err
	}
	result.Candidates = len(pending)

	for _, row := range pending {
		value := strings.TrimSpace(row.plain)
		if value == "" && !r.clearBankAccountPlaintext {
			// Blank plaintext: nothing to encrypt, and the row is left alone.
			continue
		}

		var sealed *string
		if value != "" && !r.ciphertextHolds(row.sealed, value) {
			ciphertext, err := r.encrypter.EncryptString(value)
			if err != nil {
				return result, fmt.Errorf("encrypt bank account number of employee %s: %w", row.id, err)
			}
			sealed = &ciphertext
		}
		if sealed == nil && !r.clearBankAccountPlaintext {
			result.InSync++
			continue
		}

		updated, err := r.sealPlaintextBankAccount(ctx, row, sealed, r.clearBankAccountPlaintext)
		if err != nil {
			return result, fmt.Errorf("backfill bank account number of employee %s: %w", row.id, err)
		}
		if !updated {
			result.Skipped++
			continue
		}
		if sealed != nil {
			result.Encrypted++
			if row.sealed != nil {
				result.Superseded++
			}
		} else if value != "" {
			result.InSync++
		}
		if r.clearBankAccountPlaintext {
			result.Cleared++
		}
	}

	if err := r.countBankAccountColumns(ctx, &result); err != nil {
		return result, err
	}
	return result, nil
}

// ciphertextHolds reports whether sealed decrypts to value. A ciphertext
// that cannot be decrypted does not hold it (the plaintext is re-encrypted).
func (r *EmployeesRepository) ciphertextHolds(sealed *string, value string) bool {
	if sealed == nil || strings.TrimSpace(*sealed) == "" {
		return false
	}
	opened, err := r.encrypter.DecryptString(*sealed)
	if err != nil {
		return false
	}
	return opened == value
}

func (r *EmployeesRepository) listPlaintextBankAccounts(ctx context.Context) ([]bankAccountBackfillRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, bank_account_number, bank_account_encrypted
		FROM employees
		WHERE bank_account_number IS NOT NULL
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list plaintext bank account numbers: %w", err)
	}
	defer rows.Close()

	items := make([]bankAccountBackfillRow, 0)
	for rows.Next() {
		var item bankAccountBackfillRow
		if err := rows.Scan(&item.id, &item.plain, &item.sealed); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// sealPlaintextBankAccount writes one row of the backfill: the ciphertext
// when sealed is set (otherwise the stored one is kept) and, only with
// clearPlaintext, NULL into the plaintext column. The guard applies it only
// if both columns still hold what was read. updated_at is left alone: this
// is a storage change, not an edit.
func (r *EmployeesRepository) sealPlaintextBankAccount(ctx context.Context, row bankAccountBackfillRow, sealed *string, clearPlaintext bool) (bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE employees
		SET bank_account_encrypted = COALESCE($2, bank_account_encrypted),
		    bank_account_number = CASE WHEN $5::boolean THEN NULL ELSE bank_account_number END
		WHERE id = $1::uuid
		  AND bank_account_number = $3
		  AND bank_account_encrypted IS NOT DISTINCT FROM $4
	`, row.id, sealed, row.plain, row.sealed, clearPlaintext)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// countBankAccountColumns fills Remaining and Unsealed after a run, from
// the table as it is then (rows written meanwhile included). Unsealed applies
// the same blank rule as reads and the backfill (strings.TrimSpace, any
// Unicode whitespace) in Go: SQL btrim strips only spaces, so a TAB- or
// NBSP-only plaintext, which reads as no number and is never encrypted,
// would otherwise be reported as unsealed on every start.
func (r *EmployeesRepository) countBankAccountColumns(ctx context.Context, result *BankAccountBackfillResult) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	db := repository.DB(ctx, r.db)
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*) FROM employees WHERE bank_account_number IS NOT NULL
	`).Scan(&result.Remaining); err != nil {
		return fmt.Errorf("count bank account columns: %w", err)
	}

	rows, err := db.Query(ctx, `
		SELECT bank_account_number
		FROM employees
		WHERE bank_account_number IS NOT NULL
		  AND (bank_account_encrypted IS NULL OR btrim(bank_account_encrypted) = '')
	`)
	if err != nil {
		return fmt.Errorf("count unsealed bank account numbers: %w", err)
	}
	defer rows.Close()
	unsealed := 0
	for rows.Next() {
		var plain string
		if err := rows.Scan(&plain); err != nil {
			return fmt.Errorf("count unsealed bank account numbers: %w", err)
		}
		if strings.TrimSpace(plain) != "" {
			unsealed++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("count unsealed bank account numbers: %w", err)
	}
	result.Unsealed = unsealed
	return nil
}

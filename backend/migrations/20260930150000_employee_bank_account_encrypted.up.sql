-- Phase 5: employees.bank_account_number is encrypted at rest.
--
-- bank_account_encrypted holds security.Encrypter ciphertext (AES-256-GCM,
-- key DATA_ENCRYPTION_KEY). Encryption needs the application key, so the
-- existing numbers are encrypted by a Go routine at server start, per
-- tenant (EmployeesRepository.BackfillBankAccountEncryption). This release is
-- additive: the backfill only fills this column, and every write stores the
-- number in both columns (dual-write), so no existing value is changed or
-- removed and the previous release still reads correct numbers. Clearing the
-- plaintext column is a separate opt-in (BANK_ACCOUNT_CLEAR_PLAINTEXT) for
-- after a verified backup.
--
-- The plaintext column is NOT dropped here (see docs/deployment.md).
-- employees already has tenant_id + RLS; nothing to add for isolation.
ALTER TABLE employees
    ADD COLUMN IF NOT EXISTS bank_account_encrypted TEXT NULL;

COMMENT ON COLUMN employees.bank_account_encrypted IS
    'security.Encrypter ciphertext of the bank account number (kept in sync with bank_account_number)';

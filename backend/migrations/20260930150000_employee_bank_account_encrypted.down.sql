-- Development databases only: never run a 20260930* down migration on
-- production data (docs/deployment.md, "Down migrations").
--
-- By default the backend keeps employees.bank_account_number next to the
-- ciphertext (dual-write), so dropping bank_account_encrypted loses nothing.
-- The opt-in BANK_ACCOUNT_CLEAR_PLAINTEXT clears the plaintext column; rows
-- cleared that way keep their bank account number ONLY in
-- bank_account_encrypted, and no tool writes it back. Dropping the column
-- would silently delete those numbers, so this rollback refuses to run while
-- any row (of any tenant) has a ciphertext but no plaintext number. It cannot
-- tell those rows from a number deleted outside the app by clearing only the
-- plaintext column, so such a row makes it refuse as well.
--
-- When it refuses, golang-migrate leaves the database dirty at version
-- 20260930140000 with the column still present; run
-- `migrate ... force 20260930150000` before starting the backend again.
--
-- employees has FORCE ROW LEVEL SECURITY, which also applies to the table
-- owner running migrations, so the check lifts FORCE for its own query and
-- restores it. Everything happens inside one DO block: if the check raises,
-- the ALTERs are rolled back with it.
DO $$
DECLARE
    was_forced boolean;
    sealed_only_rows bigint := 0;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'employees'
          AND column_name = 'bank_account_encrypted'
    ) THEN
        RETURN;
    END IF;

    SELECT relforcerowsecurity INTO was_forced
    FROM pg_class WHERE oid = 'employees'::regclass;
    IF was_forced THEN
        EXECUTE 'ALTER TABLE employees NO FORCE ROW LEVEL SECURITY';
    END IF;

    EXECUTE 'SELECT COUNT(*) FROM employees
             WHERE bank_account_encrypted IS NOT NULL
               AND (bank_account_number IS NULL OR btrim(bank_account_number) = '''')'
        INTO sealed_only_rows;

    IF was_forced THEN
        EXECUTE 'ALTER TABLE employees FORCE ROW LEVEL SECURITY';
    END IF;

    IF sealed_only_rows > 0 THEN
        RAISE EXCEPTION 'refusing to drop employees.bank_account_encrypted: % employee(s) keep their bank account number only there (encrypted; the plaintext column is empty)', sealed_only_rows
            USING HINT = 'Rolling back would delete those numbers. Keep this migration applied: after golang-migrate marked the database dirty, run "migrate ... force 20260930150000"; see docs/deployment.md, "Down migrations".';
    END IF;
END $$;

ALTER TABLE employees
    DROP COLUMN IF EXISTS bank_account_encrypted;

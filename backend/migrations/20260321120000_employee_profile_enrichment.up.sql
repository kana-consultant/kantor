ALTER TABLE employees
    ADD COLUMN IF NOT EXISTS bank_account_number TEXT,
    ADD COLUMN IF NOT EXISTS bank_name TEXT,
    ADD COLUMN IF NOT EXISTS linkedin_profile TEXT,
    ADD COLUMN IF NOT EXISTS ssh_keys TEXT;

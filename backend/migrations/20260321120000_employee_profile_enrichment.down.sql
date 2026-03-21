ALTER TABLE employees
    DROP COLUMN IF EXISTS ssh_keys,
    DROP COLUMN IF EXISTS linkedin_profile,
    DROP COLUMN IF EXISTS bank_name,
    DROP COLUMN IF EXISTS bank_account_number;

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS failed_login_attempts INTEGER,
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;

UPDATE users
SET failed_login_attempts = 0
WHERE failed_login_attempts IS NULL;

ALTER TABLE users
  ALTER COLUMN failed_login_attempts SET DEFAULT 0,
  ALTER COLUMN failed_login_attempts SET NOT NULL;

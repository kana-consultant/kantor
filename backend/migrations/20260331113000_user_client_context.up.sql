ALTER TABLE users
ADD COLUMN IF NOT EXISTS browser_timezone VARCHAR(64),
ADD COLUMN IF NOT EXISTS browser_timezone_offset_minutes INT,
ADD COLUMN IF NOT EXISTS browser_locale VARCHAR(32),
ADD COLUMN IF NOT EXISTS tracker_extension_version VARCHAR(32),
ADD COLUMN IF NOT EXISTS tracker_extension_reported_at TIMESTAMPTZ;

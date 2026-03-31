ALTER TABLE users
DROP COLUMN IF EXISTS tracker_extension_reported_at,
DROP COLUMN IF EXISTS tracker_extension_version,
DROP COLUMN IF EXISTS browser_locale,
DROP COLUMN IF EXISTS browser_timezone_offset_minutes,
DROP COLUMN IF EXISTS browser_timezone;

DROP INDEX IF EXISTS idx_activity_sessions_user_date_timezone;
ALTER TABLE activity_sessions DROP COLUMN IF EXISTS timezone_offset_minutes;

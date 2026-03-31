ALTER TABLE activity_sessions
ADD COLUMN timezone_offset_minutes INT NOT NULL DEFAULT 0;

CREATE INDEX idx_activity_sessions_user_date_timezone
ON activity_sessions(user_id, date DESC, timezone_offset_minutes);

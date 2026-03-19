DROP INDEX IF EXISTS idx_wa_broadcast_logs_template;
DROP INDEX IF EXISTS idx_wa_broadcast_logs_status;
DROP INDEX IF EXISTS idx_wa_broadcast_logs_dedup;
DROP TABLE IF EXISTS wa_broadcast_logs;
DROP TABLE IF EXISTS wa_broadcast_schedules;
DROP TABLE IF EXISTS wa_message_templates;
ALTER TABLE users DROP COLUMN IF EXISTS phone;

-- Tighten enum-like VARCHAR columns on the WA broadcast tables with DB-level
-- CHECK constraints. Until now the allowed values were enforced only by Go DTO
-- validation; direct INSERTs or future code paths could bypass it.
--
-- The allowed values mirror the Go `oneof` tags in
-- backend/internal/dto/whatsapp/dto.go and the statuses actually written by
-- backend/internal/service/whatsapp/*.

ALTER TABLE wa_message_templates
    ADD CONSTRAINT wa_message_templates_category_check
        CHECK (category IN ('operational', 'hris', 'marketing', 'general'));

ALTER TABLE wa_message_templates
    ADD CONSTRAINT wa_message_templates_trigger_type_check
        CHECK (trigger_type IN ('auto_scheduled', 'event_triggered', 'manual'));

ALTER TABLE wa_broadcast_schedules
    ADD CONSTRAINT wa_broadcast_schedules_schedule_type_check
        CHECK (schedule_type IN ('daily', 'weekly', 'monthly', 'once'));

ALTER TABLE wa_broadcast_schedules
    ADD CONSTRAINT wa_broadcast_schedules_target_type_check
        CHECK (target_type IN ('all_employees', 'department', 'specific_users', 'project_members'));

ALTER TABLE wa_broadcast_logs
    ADD CONSTRAINT wa_broadcast_logs_status_check
        CHECK (status IN ('queued', 'sent', 'failed', 'skipped_disabled', 'skipped_no_phone'));

ALTER TABLE wa_broadcast_logs
    ADD CONSTRAINT wa_broadcast_logs_trigger_type_check
        CHECK (trigger_type IN ('auto_scheduled', 'event_triggered', 'manual', 'manual_quick_send'));

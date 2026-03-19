-- Add phone column to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone VARCHAR(20);

-- WA message templates
CREATE TABLE IF NOT EXISTS wa_message_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(50) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL,
    trigger_type VARCHAR(20) NOT NULL,
    body_template TEXT NOT NULL,
    description TEXT,
    available_variables TEXT[],
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- WA broadcast schedules
CREATE TABLE IF NOT EXISTS wa_broadcast_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    template_id UUID NOT NULL REFERENCES wa_message_templates(id),
    schedule_type VARCHAR(20) NOT NULL,
    cron_expression VARCHAR(50),
    target_type VARCHAR(20) NOT NULL,
    target_config JSONB,
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- WA broadcast logs
CREATE TABLE IF NOT EXISTS wa_broadcast_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id UUID REFERENCES wa_broadcast_schedules(id),
    template_id UUID REFERENCES wa_message_templates(id),
    template_slug VARCHAR(50),
    trigger_type VARCHAR(20) NOT NULL DEFAULT 'auto_scheduled',
    recipient_user_id UUID REFERENCES users(id),
    recipient_phone VARCHAR(20) NOT NULL,
    message_body TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'queued',
    error_message TEXT,
    reference_type VARCHAR(50),
    reference_id UUID,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wa_broadcast_logs_dedup
    ON wa_broadcast_logs(recipient_user_id, template_slug, reference_id, created_at);

CREATE INDEX IF NOT EXISTS idx_wa_broadcast_logs_status
    ON wa_broadcast_logs(status, created_at);

CREATE INDEX IF NOT EXISTS idx_wa_broadcast_logs_template
    ON wa_broadcast_logs(template_slug, created_at);

-- Seed default system templates
INSERT INTO wa_message_templates (name, slug, category, trigger_type, body_template, description, available_variables, is_active, is_system) VALUES
('Task Jatuh Tempo Hari Ini', 'task_due_today', 'operational', 'auto_scheduled',
 E'Halo {{name}}, task *{{task_title}}* di project *{{project_name}}* jatuh tempo hari ini dan belum selesai. Segera diselesaikan ya!\n\nAkses KANTOR: {{app_url}}',
 'Dikirim otomatis setiap pagi untuk task yang due date hari ini',
 '{name,task_title,project_name,due_date,app_url}', true, true),

('Task Overdue', 'task_overdue', 'operational', 'auto_scheduled',
 E'Halo {{name}}, task *{{task_title}}* di project *{{project_name}}* sudah melewati deadline ({{due_date}}). Mohon segera update progress.\n\nAkses KANTOR: {{app_url}}',
 'Dikirim otomatis setiap pagi untuk task yang sudah melewati deadline',
 '{name,task_title,project_name,due_date,app_url}', true, true),

('Task Baru Di-Assign', 'task_assigned', 'operational', 'event_triggered',
 E'Halo {{name}}, kamu di-assign ke task baru:\n\n📋 *{{task_title}}*\n📁 Project: {{project_name}}\n📅 Deadline: {{due_date}}\n🏷️ Prioritas: {{priority}}\n\nCek detail di KANTOR: {{app_url}}',
 'Dikirim saat task di-assign ke seseorang (manual atau auto-assign)',
 '{name,task_title,project_name,due_date,priority,app_url}', true, true),

('Project Deadline H-3', 'project_deadline_warning', 'operational', 'auto_scheduled',
 E'⚠️ Reminder: Project *{{project_name}}* deadline dalam 3 hari ({{deadline}}).\n\nStatus saat ini: {{project_status}}\nTask belum selesai: {{open_tasks_count}} dari {{total_tasks_count}}\n\nPastikan semua task terselesaikan tepat waktu. Akses: {{app_url}}',
 'Dikirim otomatis H-3 sebelum deadline project ke semua member project',
 '{name,project_name,deadline,project_status,open_tasks_count,total_tasks_count,app_url}', true, true),

('Weekly Digest', 'weekly_digest', 'operational', 'auto_scheduled',
 E'📊 *Weekly Digest KANTOR* ({{week_start}} - {{week_end}})\n\nHalo {{name}}, ini ringkasan minggu lalu:\n\n✅ Task selesai: {{completed_count}}\n🔄 Task masih open: {{open_count}}\n⚠️ Task overdue: {{overdue_count}}\n\nSemangat untuk minggu ini! 💪\nAkses: {{app_url}}',
 'Dikirim setiap Senin pagi, ringkasan task per user untuk minggu sebelumnya',
 '{name,week_start,week_end,completed_count,open_count,overdue_count,app_url}', true, true),

('Reimbursement Status Update', 'reimbursement_status', 'hris', 'event_triggered',
 E'Halo {{name}}, update status reimbursement kamu:\n\n📝 *{{reimbursement_title}}*\n💰 Nominal: {{amount}}\n📌 Status: {{new_status}}\n{{reviewer_notes_section}}\n\nCek detail di KANTOR: {{app_url}}',
 'Dikirim saat status reimbursement berubah (approved/rejected/paid)',
 '{name,reimbursement_title,amount,new_status,reviewer_notes_section,app_url}', true, true)
ON CONFLICT (slug) DO NOTHING;

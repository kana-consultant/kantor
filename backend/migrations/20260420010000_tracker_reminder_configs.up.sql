CREATE TABLE tracker_reminder_configs (
    tenant_id UUID PRIMARY KEY DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    start_hour SMALLINT NOT NULL DEFAULT 9 CHECK (start_hour BETWEEN 0 AND 23),
    end_hour SMALLINT NOT NULL DEFAULT 17 CHECK (end_hour BETWEEN 1 AND 24),
    weekdays_only BOOLEAN NOT NULL DEFAULT TRUE,
    timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    heartbeat_stale_minutes INT NOT NULL DEFAULT 10 CHECK (heartbeat_stale_minutes >= 1),
    notify_in_app BOOLEAN NOT NULL DEFAULT TRUE,
    notify_whatsapp BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_tracker_reminder_hours CHECK (end_hour > start_hour)
);

ALTER TABLE tracker_reminder_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE tracker_reminder_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tracker_reminder_configs
  USING  (tenant_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Row is seeded by the application at startup (per-tenant).

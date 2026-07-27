CREATE TABLE discord_reminder_configs (
    tenant_id UUID PRIMARY KEY DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    webhook_url TEXT NOT NULL DEFAULT '',
    shared_secret TEXT NOT NULL DEFAULT '',
    send_hour SMALLINT NOT NULL DEFAULT 8 CHECK (send_hour BETWEEN 0 AND 23),
    weekdays_only BOOLEAN NOT NULL DEFAULT TRUE,
    timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE discord_reminder_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE discord_reminder_configs FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON discord_reminder_configs
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON discord_reminder_configs TO kantor_app';
    END IF;
END $$;

-- =============================================================================
-- Multi-Tenancy: RLS-based tenant isolation
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. Global tables
-- ---------------------------------------------------------------------------
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tenant_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain TEXT NOT NULL UNIQUE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenant_domains_tenant ON tenant_domains(tenant_id);

-- ---------------------------------------------------------------------------
-- 2. Placeholder tenant for backfilling existing data
-- ---------------------------------------------------------------------------
-- The real name, slug, and domains are seeded by the application at startup
-- from environment variables (TENANT_NAME, TENANT_SLUG, TENANT_DOMAINS).
-- We insert a placeholder here so that _mt_setup can backfill tenant_id.
INSERT INTO tenants (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default', 'default');

-- ---------------------------------------------------------------------------
-- 3. Non-superuser role for RLS enforcement
--    Only created when running as superuser (Docker dev).
--    In NixOS production the DB user is already non-superuser, so RLS applies.
-- ---------------------------------------------------------------------------
DO $$ BEGIN
    IF current_setting('is_superuser') = 'on' THEN
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
            CREATE ROLE kantor_app NOLOGIN;
        END IF;
    END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 4. Helper function: add tenant_id + RLS to a table
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION _mt_setup(tbl TEXT, default_tid UUID) RETURNS void AS $$
BEGIN
    EXECUTE format('ALTER TABLE %I ADD COLUMN tenant_id UUID REFERENCES tenants(id)', tbl);
    EXECUTE format('UPDATE %I SET tenant_id = $1 WHERE tenant_id IS NULL', tbl) USING default_tid;
    EXECUTE format('ALTER TABLE %I ALTER COLUMN tenant_id SET NOT NULL', tbl);
    EXECUTE format('ALTER TABLE %I ALTER COLUMN tenant_id SET DEFAULT current_setting(''app.current_tenant'')::uuid', tbl);
    EXECUTE format('CREATE INDEX idx_%s_tenant_id ON %I(tenant_id)', tbl, tbl);
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', tbl);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', tbl);
    EXECUTE format(
        'CREATE POLICY tenant_isolation ON %I '
        'USING (tenant_id = current_setting(''app.current_tenant'', true)::uuid) '
        'WITH CHECK (tenant_id = current_setting(''app.current_tenant'', true)::uuid)',
        tbl
    );
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- 5. Add tenant_id + RLS to all tenant-scoped tables
-- ---------------------------------------------------------------------------
SELECT _mt_setup('users',                       '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('refresh_tokens',              '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('audit_logs',                  '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('employees',                   '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('departments',                 '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('salaries',                    '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('bonuses',                     '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('subscriptions',               '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('subscription_alerts',         '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('projects',                    '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('project_members',             '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('kanban_columns',              '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('kanban_tasks',                '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('campaigns',                   '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('campaign_columns',            '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('campaign_column_assignments', '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('campaign_attachments',        '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('ads_metrics',                 '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('leads',                       '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('lead_activities',             '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('finance_categories',          '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('finance_records',             '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('reimbursements',              '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('notifications',               '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('wa_message_templates',        '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('wa_broadcast_schedules',      '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('wa_broadcast_logs',           '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('activity_sessions',           '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('activity_entries',            '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('domain_categories',           '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('activity_consents',           '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('roles',                       '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('role_permissions',            '00000000-0000-0000-0000-000000000001');
SELECT _mt_setup('user_module_roles',           '00000000-0000-0000-0000-000000000001');

-- system_settings needs special handling (PK on key)
ALTER TABLE system_settings DROP CONSTRAINT system_settings_pkey;
ALTER TABLE system_settings ADD COLUMN tenant_id UUID REFERENCES tenants(id);
UPDATE system_settings SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
ALTER TABLE system_settings ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE system_settings ALTER COLUMN tenant_id SET DEFAULT current_setting('app.current_tenant')::uuid;
ALTER TABLE system_settings ADD PRIMARY KEY (tenant_id, key);
ALTER TABLE system_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE system_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON system_settings
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Cleanup helper
DROP FUNCTION _mt_setup;

-- ---------------------------------------------------------------------------
-- 6. Update unique constraints to be tenant-aware
-- ---------------------------------------------------------------------------

-- users: UNIQUE(email) → UNIQUE(tenant_id, email)
ALTER TABLE users DROP CONSTRAINT users_email_key;
ALTER TABLE users ADD CONSTRAINT uq_users_tenant_email UNIQUE (tenant_id, email);

-- refresh_tokens: UNIQUE(token_hash) → UNIQUE(tenant_id, token_hash)
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_token_hash_key;
ALTER TABLE refresh_tokens ADD CONSTRAINT uq_refresh_tokens_tenant_hash UNIQUE (tenant_id, token_hash);

-- employees: UNIQUE(user_id) + UNIQUE(email)
ALTER TABLE employees DROP CONSTRAINT employees_user_id_key;
ALTER TABLE employees ADD CONSTRAINT uq_employees_tenant_user UNIQUE (tenant_id, user_id);
ALTER TABLE employees DROP CONSTRAINT uq_employees_email;
ALTER TABLE employees ADD CONSTRAINT uq_employees_tenant_email UNIQUE (tenant_id, email);

-- departments: UNIQUE(name)
ALTER TABLE departments DROP CONSTRAINT uq_departments_name;
ALTER TABLE departments ADD CONSTRAINT uq_departments_tenant_name UNIQUE (tenant_id, name);

-- kanban_columns: UNIQUE(project_id, position)
ALTER TABLE kanban_columns DROP CONSTRAINT uq_kanban_columns_project_position;
ALTER TABLE kanban_columns ADD CONSTRAINT uq_kanban_columns_tenant_project_pos UNIQUE (tenant_id, project_id, position);

-- finance_categories: UNIQUE(name, type)
ALTER TABLE finance_categories DROP CONSTRAINT uq_finance_categories_name_type;
ALTER TABLE finance_categories ADD CONSTRAINT uq_finance_categories_tenant_name_type UNIQUE (tenant_id, name, type);

-- campaign_columns: UNIQUE(name) + UNIQUE(position)
ALTER TABLE campaign_columns DROP CONSTRAINT uq_campaign_columns_name;
ALTER TABLE campaign_columns ADD CONSTRAINT uq_campaign_columns_tenant_name UNIQUE (tenant_id, name);
ALTER TABLE campaign_columns DROP CONSTRAINT uq_campaign_columns_position;
ALTER TABLE campaign_columns ADD CONSTRAINT uq_campaign_columns_tenant_pos UNIQUE (tenant_id, position);

-- wa_message_templates: UNIQUE(slug)
ALTER TABLE wa_message_templates DROP CONSTRAINT wa_message_templates_slug_key;
ALTER TABLE wa_message_templates ADD CONSTRAINT uq_wa_templates_tenant_slug UNIQUE (tenant_id, slug);

-- domain_categories: UNIQUE(domain_pattern)
ALTER TABLE domain_categories DROP CONSTRAINT domain_categories_domain_pattern_key;
ALTER TABLE domain_categories ADD CONSTRAINT uq_domain_categories_tenant_pattern UNIQUE (tenant_id, domain_pattern);

-- activity_consents: UNIQUE(user_id)
ALTER TABLE activity_consents DROP CONSTRAINT activity_consents_user_id_key;
ALTER TABLE activity_consents ADD CONSTRAINT uq_activity_consents_tenant_user UNIQUE (tenant_id, user_id);

-- roles (was roles_v2): UNIQUE(slug)
ALTER TABLE roles DROP CONSTRAINT roles_v2_slug_key;
ALTER TABLE roles ADD CONSTRAINT uq_roles_tenant_slug UNIQUE (tenant_id, slug);

-- user_module_roles: UNIQUE(user_id, module_id)
ALTER TABLE user_module_roles DROP CONSTRAINT user_module_roles_user_id_module_id_key;
ALTER TABLE user_module_roles ADD CONSTRAINT uq_user_module_roles_tenant_user_mod UNIQUE (tenant_id, user_id, module_id);

-- ---------------------------------------------------------------------------
-- 7. Grant kantor_app access to all tables (only if role exists)
-- ---------------------------------------------------------------------------
DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT USAGE ON SCHEMA public TO kantor_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO kantor_app';
        EXECUTE 'GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO kantor_app';
        EXECUTE 'ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO kantor_app';
        EXECUTE 'ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO kantor_app';
    END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 8. Per-tenant WhatsApp (WAHA) configuration
-- ---------------------------------------------------------------------------
CREATE TABLE tenant_wa_configs (
    tenant_id UUID PRIMARY KEY DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    api_url TEXT NOT NULL DEFAULT 'http://localhost:3000',
    api_key TEXT NOT NULL DEFAULT '',
    session_name TEXT NOT NULL DEFAULT 'default',
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    max_daily_messages INT NOT NULL DEFAULT 50,
    min_delay_ms INT NOT NULL DEFAULT 2000,
    max_delay_ms INT NOT NULL DEFAULT 5000,
    reminder_cron TEXT NOT NULL DEFAULT '0 8 * * 1-5',
    weekly_digest_cron TEXT NOT NULL DEFAULT '0 8 * * 1',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tenant_wa_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_wa_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_wa_configs
  USING  (tenant_id = current_setting('app.current_tenant', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- WA config is seeded by the application at startup (per-tenant).

-- ---------------------------------------------------------------------------
-- 9. Drop deprecated RBAC v1 tables (no longer needed)
-- ---------------------------------------------------------------------------
DROP TABLE IF EXISTS user_roles_deprecated CASCADE;
DROP TABLE IF EXISTS role_permissions_deprecated CASCADE;
DROP TABLE IF EXISTS permissions_deprecated CASCADE;
DROP TABLE IF EXISTS roles_deprecated CASCADE;

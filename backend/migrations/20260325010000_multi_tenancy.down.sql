-- =============================================================================
-- Rollback Multi-Tenancy
-- =============================================================================

-- 1. Drop RLS policies and disable RLS on all tenant-scoped tables
DO $$
DECLARE
    tbl TEXT;
BEGIN
    FOR tbl IN
        SELECT unnest(ARRAY[
            'users','refresh_tokens','audit_logs','employees','departments',
            'salaries','bonuses','subscriptions','subscription_alerts',
            'projects','project_members','kanban_columns','kanban_tasks',
            'campaigns','campaign_columns','campaign_column_assignments',
            'campaign_attachments','ads_metrics','leads','lead_activities',
            'finance_categories','finance_records','reimbursements','notifications',
            'wa_message_templates','wa_broadcast_schedules','wa_broadcast_logs',
            'activity_sessions','activity_entries','domain_categories','activity_consents',
            'roles','role_permissions','user_module_roles','system_settings',
            'tenant_wa_configs'
        ])
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', tbl);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', tbl);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', tbl);
    END LOOP;
END $$;

-- 2. Restore original unique constraints
ALTER TABLE users DROP CONSTRAINT IF EXISTS uq_users_tenant_email;
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);

ALTER TABLE refresh_tokens DROP CONSTRAINT IF EXISTS uq_refresh_tokens_tenant_hash;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_token_hash_key UNIQUE (token_hash);

ALTER TABLE employees DROP CONSTRAINT IF EXISTS uq_employees_tenant_user;
ALTER TABLE employees ADD CONSTRAINT employees_user_id_key UNIQUE (user_id);
ALTER TABLE employees DROP CONSTRAINT IF EXISTS uq_employees_tenant_email;
ALTER TABLE employees ADD CONSTRAINT uq_employees_email UNIQUE (email);

ALTER TABLE departments DROP CONSTRAINT IF EXISTS uq_departments_tenant_name;
ALTER TABLE departments ADD CONSTRAINT uq_departments_name UNIQUE (name);

ALTER TABLE kanban_columns DROP CONSTRAINT IF EXISTS uq_kanban_columns_tenant_project_pos;
ALTER TABLE kanban_columns ADD CONSTRAINT uq_kanban_columns_project_position UNIQUE (project_id, position);

ALTER TABLE finance_categories DROP CONSTRAINT IF EXISTS uq_finance_categories_tenant_name_type;
ALTER TABLE finance_categories ADD CONSTRAINT uq_finance_categories_name_type UNIQUE (name, type);

ALTER TABLE campaign_columns DROP CONSTRAINT IF EXISTS uq_campaign_columns_tenant_name;
ALTER TABLE campaign_columns ADD CONSTRAINT uq_campaign_columns_name UNIQUE (name);
ALTER TABLE campaign_columns DROP CONSTRAINT IF EXISTS uq_campaign_columns_tenant_pos;
ALTER TABLE campaign_columns ADD CONSTRAINT uq_campaign_columns_position UNIQUE (position);

ALTER TABLE wa_message_templates DROP CONSTRAINT IF EXISTS uq_wa_templates_tenant_slug;
ALTER TABLE wa_message_templates ADD CONSTRAINT wa_message_templates_slug_key UNIQUE (slug);

ALTER TABLE domain_categories DROP CONSTRAINT IF EXISTS uq_domain_categories_tenant_pattern;
ALTER TABLE domain_categories ADD CONSTRAINT domain_categories_domain_pattern_key UNIQUE (domain_pattern);

ALTER TABLE activity_consents DROP CONSTRAINT IF EXISTS uq_activity_consents_tenant_user;
ALTER TABLE activity_consents ADD CONSTRAINT activity_consents_user_id_key UNIQUE (user_id);

ALTER TABLE roles DROP CONSTRAINT IF EXISTS uq_roles_tenant_slug;
ALTER TABLE roles ADD CONSTRAINT roles_v2_slug_key UNIQUE (slug);

ALTER TABLE user_module_roles DROP CONSTRAINT IF EXISTS uq_user_module_roles_tenant_user_mod;
ALTER TABLE user_module_roles ADD CONSTRAINT user_module_roles_user_id_module_id_key UNIQUE (user_id, module_id);

-- 3. system_settings: restore PK(key)
ALTER TABLE system_settings DROP CONSTRAINT IF EXISTS system_settings_pkey;
ALTER TABLE system_settings DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE system_settings ADD PRIMARY KEY (key);

-- 4. Drop tenant_id column from all tables
DO $$
DECLARE
    tbl TEXT;
BEGIN
    FOR tbl IN
        SELECT unnest(ARRAY[
            'users','refresh_tokens','audit_logs','employees','departments',
            'salaries','bonuses','subscriptions','subscription_alerts',
            'projects','project_members','kanban_columns','kanban_tasks',
            'campaigns','campaign_columns','campaign_column_assignments',
            'campaign_attachments','ads_metrics','leads','lead_activities',
            'finance_categories','finance_records','reimbursements','notifications',
            'wa_message_templates','wa_broadcast_schedules','wa_broadcast_logs',
            'activity_sessions','activity_entries','domain_categories','activity_consents',
            'roles','role_permissions','user_module_roles'
        ])
    LOOP
        EXECUTE format('DROP INDEX IF EXISTS idx_%s_tenant_id', tbl);
        EXECUTE format('ALTER TABLE %I DROP COLUMN IF EXISTS tenant_id', tbl);
    END LOOP;
END $$;

-- 5. Drop new tables and global tables
DROP TABLE IF EXISTS tenant_wa_configs CASCADE;
DROP TABLE IF EXISTS tenant_domains CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;

-- 6. Drop app role (if exists)
DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'REVOKE ALL ON ALL TABLES IN SCHEMA public FROM kantor_app';
        EXECUTE 'REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM kantor_app';
    END IF;
END $$;
DROP ROLE IF EXISTS kantor_app;

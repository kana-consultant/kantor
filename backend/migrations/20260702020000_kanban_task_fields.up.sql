CREATE TABLE kanban_task_fields (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    task_id UUID NOT NULL REFERENCES kanban_tasks (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kanban_task_fields_name CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT chk_kanban_task_fields_value CHECK (length(btrim(value)) > 0)
);

CREATE INDEX idx_kanban_task_fields_task_id ON kanban_task_fields (task_id, position);

ALTER TABLE kanban_task_fields
    ADD CONSTRAINT uq_kanban_task_fields_task_name UNIQUE (task_id, name);

ALTER TABLE kanban_task_fields ENABLE ROW LEVEL SECURITY;
ALTER TABLE kanban_task_fields FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON kanban_task_fields
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE INDEX idx_kanban_task_fields_tenant_id ON kanban_task_fields (tenant_id);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON kanban_task_fields TO kantor_app';
    END IF;
END $$;

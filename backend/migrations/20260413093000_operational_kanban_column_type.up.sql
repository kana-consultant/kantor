ALTER TABLE kanban_columns
ADD COLUMN column_type TEXT;

DO $$
DECLARE
    current_tenant_id UUID;
BEGIN
    FOR current_tenant_id IN SELECT id FROM tenants LOOP
        PERFORM set_config('app.current_tenant', current_tenant_id::text, true);

        UPDATE kanban_columns
        SET column_type = CASE
            WHEN LOWER(REPLACE(name, ' ', '_')) IN ('backlog', 'todo', 'to_do') THEN 'todo'
            WHEN LOWER(REPLACE(name, ' ', '_')) IN ('in_progress', 'inprogress', 'working', 'doing') THEN 'in_progress'
            WHEN LOWER(REPLACE(name, ' ', '_')) IN ('done', 'completed', 'complete', 'closed') THEN 'done'
            ELSE 'custom'
        END
        WHERE tenant_id = current_tenant_id
          AND column_type IS NULL;
    END LOOP;
END $$;

ALTER TABLE kanban_columns
ALTER COLUMN column_type SET DEFAULT 'custom';

DO $$
DECLARE
    current_tenant_id UUID;
BEGIN
    FOR current_tenant_id IN SELECT id FROM tenants LOOP
        PERFORM set_config('app.current_tenant', current_tenant_id::text, true);

        UPDATE kanban_columns
        SET column_type = 'custom'
        WHERE tenant_id = current_tenant_id
          AND column_type IS NULL;
    END LOOP;
END $$;

ALTER TABLE kanban_columns
ALTER COLUMN column_type SET NOT NULL;

ALTER TABLE kanban_columns
ADD CONSTRAINT chk_kanban_columns_column_type
CHECK (column_type IN ('todo', 'in_progress', 'done', 'custom'));

CREATE INDEX idx_kanban_columns_project_type
ON kanban_columns (project_id, column_type);

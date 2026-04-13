DROP INDEX IF EXISTS idx_kanban_columns_project_type;

ALTER TABLE kanban_columns
DROP CONSTRAINT IF EXISTS chk_kanban_columns_column_type;

ALTER TABLE kanban_columns
DROP COLUMN IF EXISTS column_type;

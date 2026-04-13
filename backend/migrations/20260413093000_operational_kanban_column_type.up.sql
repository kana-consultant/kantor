ALTER TABLE kanban_columns
ADD COLUMN column_type TEXT;

UPDATE kanban_columns
SET column_type = CASE
    WHEN LOWER(REPLACE(name, ' ', '_')) IN ('backlog', 'todo', 'to_do') THEN 'todo'
    WHEN LOWER(REPLACE(name, ' ', '_')) IN ('in_progress', 'inprogress', 'working', 'doing') THEN 'in_progress'
    WHEN LOWER(REPLACE(name, ' ', '_')) IN ('done', 'completed', 'complete', 'closed') THEN 'done'
    ELSE 'custom'
END
WHERE column_type IS NULL;

ALTER TABLE kanban_columns
ALTER COLUMN column_type SET DEFAULT 'custom';

UPDATE kanban_columns
SET column_type = 'custom'
WHERE column_type IS NULL;

ALTER TABLE kanban_columns
ALTER COLUMN column_type SET NOT NULL;

ALTER TABLE kanban_columns
ADD CONSTRAINT chk_kanban_columns_column_type
CHECK (column_type IN ('todo', 'in_progress', 'done', 'custom'));

CREATE INDEX idx_kanban_columns_project_type
ON kanban_columns (project_id, column_type);

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_kanban_tasks_title_trgm
    ON kanban_tasks USING GIN (title gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_kanban_tasks_description_trgm
    ON kanban_tasks USING GIN (description gin_trgm_ops);

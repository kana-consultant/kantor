DROP INDEX IF EXISTS idx_kanban_tasks_due_date;
DROP INDEX IF EXISTS idx_kanban_tasks_assignee_id;
DROP INDEX IF EXISTS idx_kanban_tasks_column_id;
DROP INDEX IF EXISTS idx_kanban_tasks_project_id;
DROP INDEX IF EXISTS idx_kanban_columns_project_id;

DROP TABLE IF EXISTS kanban_tasks;
DROP TABLE IF EXISTS kanban_columns;

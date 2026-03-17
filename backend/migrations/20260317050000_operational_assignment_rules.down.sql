DROP INDEX IF EXISTS idx_assignment_rules_active;
DROP INDEX IF EXISTS idx_assignment_rules_project_priority;
DROP INDEX IF EXISTS idx_assignment_rules_project_id;

DROP TABLE IF EXISTS assignment_rules;

ALTER TABLE kanban_tasks
    DROP CONSTRAINT IF EXISTS chk_kanban_tasks_assigned_via,
    DROP COLUMN IF EXISTS assigned_via;

ALTER TABLE users
    DROP COLUMN IF EXISTS skills,
    DROP COLUMN IF EXISTS department;

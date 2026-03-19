-- Add auto_assign_mode to projects (off = manual, round_robin, least_busy)
ALTER TABLE projects
    ADD COLUMN auto_assign_mode TEXT NOT NULL DEFAULT 'off',
    ADD CONSTRAINT chk_projects_auto_assign_mode CHECK (auto_assign_mode IN ('off', 'round_robin', 'least_busy'));

-- Track round-robin pointer per project so distribution is fair
ALTER TABLE projects
    ADD COLUMN auto_assign_cursor INT NOT NULL DEFAULT 0;

-- Drop assignment_rules table (no longer needed)
DROP INDEX IF EXISTS idx_assignment_rules_active;
DROP INDEX IF EXISTS idx_assignment_rules_project_priority;
DROP INDEX IF EXISTS idx_assignment_rules_project_id;
DROP TABLE IF EXISTS assignment_rules;

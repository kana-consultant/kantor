ALTER TABLE users
    ADD COLUMN department TEXT,
    ADD COLUMN skills TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE kanban_tasks
    ADD COLUMN assigned_via TEXT NOT NULL DEFAULT 'manual',
    ADD CONSTRAINT chk_kanban_tasks_assigned_via CHECK (assigned_via IN ('manual', 'auto'));

CREATE TABLE assignment_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    rule_type TEXT NOT NULL,
    rule_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    priority INT NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_assignment_rules_type CHECK (rule_type IN ('by_department', 'by_skill', 'by_workload'))
);

CREATE INDEX idx_assignment_rules_project_id ON assignment_rules (project_id);
CREATE INDEX idx_assignment_rules_project_priority ON assignment_rules (project_id, priority);
CREATE INDEX idx_assignment_rules_active ON assignment_rules (is_active);

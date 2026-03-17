CREATE TABLE kanban_columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    color TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kanban_columns_project_position UNIQUE (project_id, position)
);

CREATE TABLE kanban_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id UUID NOT NULL REFERENCES kanban_columns (id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    assignee_id UUID REFERENCES users (id) ON DELETE SET NULL,
    due_date TIMESTAMPTZ,
    priority TEXT NOT NULL DEFAULT 'medium',
    label TEXT,
    position INTEGER NOT NULL,
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kanban_tasks_priority CHECK (priority IN ('low', 'medium', 'high', 'critical'))
);

CREATE INDEX idx_kanban_columns_project_id ON kanban_columns (project_id);
CREATE INDEX idx_kanban_tasks_project_id ON kanban_tasks (project_id);
CREATE INDEX idx_kanban_tasks_column_id ON kanban_tasks (column_id);
CREATE INDEX idx_kanban_tasks_assignee_id ON kanban_tasks (assignee_id);
CREATE INDEX idx_kanban_tasks_due_date ON kanban_tasks (due_date);

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    deadline TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'draft',
    priority TEXT NOT NULL DEFAULT 'medium',
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_projects_status CHECK (status IN ('draft', 'active', 'on_hold', 'completed', 'archived')),
    CONSTRAINT chk_projects_priority CHECK (priority IN ('low', 'medium', 'high', 'critical'))
);

CREATE TABLE project_members (
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_in_project TEXT NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_projects_status ON projects (status);
CREATE INDEX idx_projects_priority ON projects (priority);
CREATE INDEX idx_projects_deadline ON projects (deadline);
CREATE INDEX idx_projects_created_by ON projects (created_by);
CREATE INDEX idx_project_members_user_id ON project_members (user_id);

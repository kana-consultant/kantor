CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    channel TEXT NOT NULL,
    budget_amount BIGINT NOT NULL DEFAULT 0,
    budget_currency TEXT NOT NULL DEFAULT 'IDR',
    pic_employee_id UUID REFERENCES employees (id) ON DELETE SET NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    brief_text TEXT,
    status TEXT NOT NULL DEFAULT 'ideation',
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_campaigns_channel CHECK (channel IN ('instagram', 'facebook', 'google_ads', 'tiktok', 'youtube', 'email', 'other')),
    CONSTRAINT chk_campaigns_status CHECK (status IN ('ideation', 'planning', 'in_production', 'live', 'completed', 'archived')),
    CONSTRAINT chk_campaigns_budget_amount CHECK (budget_amount >= 0),
    CONSTRAINT chk_campaigns_date_range CHECK (end_date >= start_date)
);

CREATE TABLE campaign_columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    color TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_campaign_columns_name UNIQUE (name),
    CONSTRAINT uq_campaign_columns_position UNIQUE (position)
);

CREATE TABLE campaign_column_assignments (
    campaign_id UUID PRIMARY KEY REFERENCES campaigns (id) ON DELETE CASCADE,
    column_id UUID NOT NULL REFERENCES campaign_columns (id) ON DELETE RESTRICT,
    position INTEGER NOT NULL,
    moved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    moved_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT
);

CREATE TABLE campaign_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_type TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    uploaded_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_campaign_attachments_file_size CHECK (file_size >= 0)
);

CREATE INDEX idx_campaigns_channel ON campaigns (channel);
CREATE INDEX idx_campaigns_status ON campaigns (status);
CREATE INDEX idx_campaigns_pic_employee_id ON campaigns (pic_employee_id);
CREATE INDEX idx_campaigns_start_date ON campaigns (start_date);
CREATE INDEX idx_campaigns_end_date ON campaigns (end_date);
CREATE INDEX idx_campaigns_created_by ON campaigns (created_by);
CREATE INDEX idx_campaign_column_assignments_column_id ON campaign_column_assignments (column_id);
CREATE INDEX idx_campaign_column_assignments_position ON campaign_column_assignments (column_id, position);
CREATE INDEX idx_campaign_attachments_campaign_id ON campaign_attachments (campaign_id);

INSERT INTO campaign_columns (name, position, color)
VALUES
    ('Ideation', 1, '#8B5CF6'),
    ('Planning', 2, '#0EA5E9'),
    ('In Production', 3, '#F59E0B'),
    ('Live', 4, '#10B981'),
    ('Completed', 5, '#334155'),
    ('Archived', 6, '#94A3B8')
ON CONFLICT (name) DO NOTHING;

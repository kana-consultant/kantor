CREATE TABLE leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(180) NOT NULL,
    phone VARCHAR(32),
    email VARCHAR(255),
    source_channel VARCHAR(32) NOT NULL CHECK (source_channel IN ('whatsapp', 'email', 'instagram', 'facebook', 'website', 'referral', 'other')),
    pipeline_status VARCHAR(32) NOT NULL CHECK (pipeline_status IN ('new', 'contacted', 'qualified', 'proposal', 'negotiation', 'won', 'lost')),
    assigned_to UUID REFERENCES employees(id) ON DELETE SET NULL,
    notes TEXT,
    company_name VARCHAR(180),
    estimated_value BIGINT NOT NULL DEFAULT 0 CHECK (estimated_value >= 0),
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_leads_pipeline_status ON leads (pipeline_status);
CREATE INDEX idx_leads_source_channel ON leads (source_channel);
CREATE INDEX idx_leads_assigned_to ON leads (assigned_to);
CREATE INDEX idx_leads_created_at ON leads (created_at DESC);

CREATE TABLE lead_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    activity_type VARCHAR(32) NOT NULL CHECK (activity_type IN ('status_change', 'note_added', 'call', 'email_sent', 'whatsapp_sent', 'meeting', 'follow_up')),
    description TEXT NOT NULL,
    old_status VARCHAR(32) CHECK (old_status IN ('new', 'contacted', 'qualified', 'proposal', 'negotiation', 'won', 'lost')),
    new_status VARCHAR(32) CHECK (new_status IN ('new', 'contacted', 'qualified', 'proposal', 'negotiation', 'won', 'lost')),
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_lead_activities_lead_id ON lead_activities (lead_id, created_at DESC);

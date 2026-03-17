CREATE TABLE finance_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('income', 'outcome')),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_finance_categories_name_type UNIQUE (name, type)
);

CREATE TABLE finance_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id UUID NOT NULL REFERENCES finance_categories(id) ON DELETE RESTRICT,
    type TEXT NOT NULL CHECK (type IN ('income', 'outcome')),
    amount BIGINT NOT NULL CHECK (amount >= 0),
    description TEXT NOT NULL,
    record_date DATE NOT NULL,
    record_month INT NOT NULL CHECK (record_month BETWEEN 1 AND 12),
    record_year INT NOT NULL CHECK (record_year BETWEEN 2000 AND 2100),
    approval_status TEXT NOT NULL DEFAULT 'draft' CHECK (approval_status IN ('draft', 'pending_review', 'approved', 'rejected')),
    submitted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    approved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_finance_records_record_period ON finance_records (record_year, record_month);
CREATE INDEX idx_finance_records_status ON finance_records (approval_status);
CREATE INDEX idx_finance_records_type ON finance_records (type);
CREATE INDEX idx_finance_records_category_id ON finance_records (category_id);

INSERT INTO finance_categories (name, type, is_default) VALUES
    ('project revenue', 'income', TRUE),
    ('service fee', 'income', TRUE),
    ('other income', 'income', TRUE),
    ('gaji', 'outcome', TRUE),
    ('sewa', 'outcome', TRUE),
    ('utilitas', 'outcome', TRUE),
    ('marketing spend', 'outcome', TRUE),
    ('subscription', 'outcome', TRUE),
    ('operational', 'outcome', TRUE),
    ('other expense', 'outcome', TRUE)
ON CONFLICT (name, type) DO NOTHING;

CREATE TABLE salaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id UUID NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    base_salary TEXT NOT NULL,
    allowances TEXT NOT NULL,
    deductions TEXT NOT NULL,
    net_salary TEXT NOT NULL,
    effective_date DATE NOT NULL,
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE bonuses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id UUID NOT NULL REFERENCES employees (id) ON DELETE CASCADE,
    amount TEXT NOT NULL,
    reason TEXT NOT NULL,
    period_month INTEGER NOT NULL,
    period_year INTEGER NOT NULL,
    approval_status TEXT NOT NULL DEFAULT 'pending',
    approved_by UUID REFERENCES users (id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_bonuses_period_month CHECK (period_month BETWEEN 1 AND 12),
    CONSTRAINT chk_bonuses_status CHECK (approval_status IN ('pending', 'approved', 'rejected'))
);

CREATE INDEX idx_salaries_employee_id ON salaries (employee_id);
CREATE INDEX idx_salaries_effective_date ON salaries (effective_date DESC);
CREATE INDEX idx_bonuses_employee_id ON bonuses (employee_id);
CREATE INDEX idx_bonuses_status ON bonuses (approval_status);
CREATE INDEX idx_bonuses_period ON bonuses (period_year, period_month);

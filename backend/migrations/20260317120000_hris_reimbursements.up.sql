CREATE TABLE reimbursements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    category TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount >= 0),
    transaction_date DATE NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'manager_review', 'finance_approval', 'approved', 'rejected', 'paid')),
    attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
    submitted_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    manager_id UUID REFERENCES users(id) ON DELETE SET NULL,
    manager_action_at TIMESTAMPTZ,
    manager_notes TEXT,
    finance_id UUID REFERENCES users(id) ON DELETE SET NULL,
    finance_action_at TIMESTAMPTZ,
    finance_notes TEXT,
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reimbursements_employee_id ON reimbursements (employee_id);
CREATE INDEX idx_reimbursements_status ON reimbursements (status);
CREATE INDEX idx_reimbursements_transaction_date ON reimbursements (transaction_date DESC);

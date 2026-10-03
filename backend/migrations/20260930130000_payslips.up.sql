-- Slip gaji (payslips). Every row is a snapshot: the amounts (earnings,
-- deductions, reimbursements, manual lines, totals) and the full template
-- payload are encrypted with security.Encrypter, because backdated salary
-- rows, bonus approval flips and late paid_at would otherwise change a slip
-- after it was sent. Sent slips are never edited: they are voided and
-- reissued with a '-R<n>' number.
--
-- reimbursement_ids / bonus_ids are the carry-over key: an item listed on a
-- sent (non-void) slip is never put on another one.
CREATE TABLE payslips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    employee_id UUID NOT NULL REFERENCES employees (id) ON DELETE RESTRICT,
    period_year INTEGER NOT NULL,
    period_month INTEGER NOT NULL,
    revision INTEGER NOT NULL DEFAULT 0,
    doc_number TEXT NOT NULL,
    replaces_payslip_id UUID REFERENCES payslips (id),
    salary_id UUID REFERENCES salaries (id) ON DELETE SET NULL,
    -- FK to employment_contracts is added by the contracts migration.
    contract_id UUID,
    status TEXT NOT NULL DEFAULT 'draft',
    pay_date DATE NOT NULL,
    amounts_encrypted TEXT NOT NULL,
    payload_encrypted TEXT NOT NULL,
    reimbursement_ids UUID[] NOT NULL DEFAULT '{}',
    bonus_ids UUID[] NOT NULL DEFAULT '{}',
    note TEXT,
    warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    template_version TEXT,
    render_status TEXT NOT NULL DEFAULT 'none',
    render_error TEXT,
    -- Worker claim: a fresh token per claim, so a render finished after the
    -- row changed (edited, regenerated) is discarded instead of recorded.
    render_claim UUID,
    render_started_at TIMESTAMPTZ,
    pdf_path TEXT,
    pdf_sha256 TEXT,
    generated_by UUID REFERENCES users (id) ON DELETE SET NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_sent_at TIMESTAMPTZ,
    voided_by UUID REFERENCES users (id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ,
    void_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_payslips_period_month CHECK (period_month BETWEEN 1 AND 12),
    CONSTRAINT chk_payslips_period_year CHECK (period_year BETWEEN 2000 AND 2100),
    CONSTRAINT chk_payslips_revision CHECK (revision >= 0),
    CONSTRAINT chk_payslips_status CHECK (status IN ('draft', 'sent', 'void')),
    CONSTRAINT chk_payslips_render_status CHECK (render_status IN ('none', 'pending', 'rendering', 'ready', 'failed')),
    CONSTRAINT chk_payslips_doc_number CHECK (length(btrim(doc_number)) BETWEEN 1 AND 60),
    CONSTRAINT chk_payslips_note CHECK (note IS NULL OR length(note) <= 200),
    CONSTRAINT chk_payslips_void CHECK ((status = 'void') = (voided_at IS NOT NULL)),
    CONSTRAINT chk_payslips_pdf CHECK ((pdf_path IS NULL) = (pdf_sha256 IS NULL)),
    CONSTRAINT chk_payslips_warnings CHECK (jsonb_typeof(warnings) = 'array')
);

-- One active (draft or sent) slip per employee and period; voided slips are
-- kept for the record.
CREATE UNIQUE INDEX uq_payslips_active
    ON payslips (tenant_id, employee_id, period_year, period_month)
    WHERE status <> 'void';
CREATE UNIQUE INDEX uq_payslips_doc_number ON payslips (tenant_id, doc_number);
CREATE INDEX idx_payslips_period ON payslips (tenant_id, period_year, period_month);
CREATE INDEX idx_payslips_employee ON payslips (tenant_id, employee_id, period_year DESC, period_month DESC);
CREATE INDEX idx_payslips_render_pending ON payslips (tenant_id, render_status)
    WHERE render_status IN ('pending', 'rendering');

ALTER TABLE payslips ENABLE ROW LEVEL SECURITY;
ALTER TABLE payslips FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON payslips
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE INDEX idx_payslips_tenant_id ON payslips (tenant_id);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON payslips TO kantor_app';
    END IF;
END $$;

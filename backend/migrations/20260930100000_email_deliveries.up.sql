-- Log of every document email attempt (payslips, contracts and the admin
-- test email). Bodies and attachment bytes are never stored; the sha256 lets
-- HR check a presented copy against what was actually sent.
CREATE TABLE email_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    reference_type TEXT,
    reference_id UUID,
    recipient TEXT NOT NULL,
    recipient_source TEXT NOT NULL,
    cc TEXT[] NOT NULL DEFAULT '{}',
    subject TEXT NOT NULL,
    attachment_names TEXT[] NOT NULL DEFAULT '{}',
    attachment_sha256 TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'queued',
    error TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    batch_id UUID,
    requested_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ,
    CONSTRAINT chk_email_deliveries_kind CHECK (kind IN ('contract', 'payslip', 'test')),
    CONSTRAINT chk_email_deliveries_recipient_source CHECK (recipient_source IN ('login', 'employee', 'personal', 'self')),
    CONSTRAINT chk_email_deliveries_status CHECK (status IN ('queued', 'sending', 'sent', 'failed')),
    CONSTRAINT chk_email_deliveries_attempts CHECK (attempts >= 0),
    CONSTRAINT chk_email_deliveries_reference CHECK ((reference_type IS NULL) = (reference_id IS NULL)),
    CONSTRAINT chk_email_deliveries_attachments CHECK (cardinality(attachment_names) = cardinality(attachment_sha256))
);

CREATE INDEX idx_email_deliveries_reference ON email_deliveries (tenant_id, reference_type, reference_id);
CREATE INDEX idx_email_deliveries_batch ON email_deliveries (tenant_id, batch_id);
CREATE INDEX idx_email_deliveries_recipient ON email_deliveries (tenant_id, recipient);
CREATE INDEX idx_email_deliveries_tenant_created ON email_deliveries (tenant_id, created_at DESC);

-- Double-send guard: at most one queued/sending attempt per document.
CREATE UNIQUE INDEX uq_email_deliveries_in_flight
    ON email_deliveries (tenant_id, reference_type, reference_id)
    WHERE status IN ('queued', 'sending') AND reference_id IS NOT NULL;

ALTER TABLE email_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE email_deliveries FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON email_deliveries
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE INDEX idx_email_deliveries_tenant_id ON email_deliveries (tenant_id);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON email_deliveries TO kantor_app';
    END IF;
END $$;

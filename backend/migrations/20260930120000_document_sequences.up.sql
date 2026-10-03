-- Per-tenant document counters. Incremented inside the document transaction
-- with INSERT ... ON CONFLICT DO UPDATE ... RETURNING, which takes a row
-- lock, so concurrent generations never get the same number.
--   doc_type 'EMPLOYMENT', period_key 'YYYY-MM': shared by a PKWT + NDA pair
--   doc_type 'EMPLOYEE',   period_key 'ALL':     employee numbers
CREATE TABLE document_sequences (
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    doc_type TEXT NOT NULL,
    period_key TEXT NOT NULL,
    last_value INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, doc_type, period_key),
    CONSTRAINT chk_document_sequences_doc_type CHECK (length(btrim(doc_type)) BETWEEN 1 AND 40),
    CONSTRAINT chk_document_sequences_period_key CHECK (length(btrim(period_key)) BETWEEN 1 AND 40),
    CONSTRAINT chk_document_sequences_last_value CHECK (last_value >= 0)
);

ALTER TABLE document_sequences ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_sequences FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON document_sequences
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE INDEX idx_document_sequences_tenant_id ON document_sequences (tenant_id);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON document_sequences TO kantor_app';
    END IF;
END $$;

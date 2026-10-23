-- Dokumen Legal (Operasional): agreements with parties outside the company,
-- generated from templates 03-07 of the legal pack (freelance partnership,
-- subcontract, statement of work, service level agreement, handover report).
--
-- New tables only, plus two widened CHECKs on email_deliveries; no existing
-- row is touched.

-- Pihak: freelancers, subcontractors and clients. The identity number (NIK or
-- company registration), the NPWP and the account number are encrypted with
-- security.Encrypter and only ever returned masked.
CREATE TABLE legal_parties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    party_type TEXT NOT NULL DEFAULT 'person',
    name TEXT NOT NULL,
    address TEXT NOT NULL DEFAULT '',
    -- Company: the law it was established under, as printed
    -- ("Negara Republik Indonesia / the Republic of Indonesia").
    jurisdiction TEXT NOT NULL DEFAULT '',
    id_number_encrypted TEXT,
    tax_id_encrypted TEXT,
    representative_name TEXT NOT NULL DEFAULT '',
    representative_title TEXT NOT NULL DEFAULT '',
    contact_name TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    bank_name TEXT NOT NULL DEFAULT '',
    bank_account_encrypted TEXT,
    bank_account_holder TEXT NOT NULL DEFAULT '',
    bank_swift TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_legal_parties_type CHECK (party_type IN ('person', 'company')),
    CONSTRAINT chk_legal_parties_name CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    CONSTRAINT chk_legal_parties_lengths CHECK (
        length(address) <= 500 AND length(jurisdiction) <= 200
        AND length(representative_name) <= 200 AND length(representative_title) <= 200
        AND length(contact_name) <= 200 AND length(email) <= 254 AND length(phone) <= 50
        AND length(bank_name) <= 100 AND length(bank_account_holder) <= 200
        AND length(bank_swift) <= 20 AND length(notes) <= 2000
    )
);

CREATE INDEX idx_legal_parties_tenant_id ON legal_parties (tenant_id);
CREATE INDEX idx_legal_parties_name ON legal_parties (tenant_id, lower(name));

ALTER TABLE legal_parties ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_parties FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON legal_parties
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- One document per row. fields holds the form values (no identity or account
-- numbers: those stay on the party). Generating snapshots the full template
-- payload, encrypted like the contract payloads, and the document worker
-- renders it to PDF.
--
-- Numbering: one monthly sequence per document type in document_sequences
-- (doc_type 'LEGAL_<CODE>', period_key 'YYYY-MM'), assigned at the first
-- generate and kept by later edits.
CREATE TABLE legal_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    doc_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    title TEXT NOT NULL,
    -- The other party; a party with documents cannot be deleted.
    party_id UUID NOT NULL REFERENCES legal_parties (id) ON DELETE RESTRICT,
    -- The document this one builds on (a handover report on its SOW).
    related_document_id UUID REFERENCES legal_documents (id) ON DELETE SET NULL,
    fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    document_date DATE,
    document_city TEXT,
    seq_no INTEGER,
    doc_number TEXT,
    template_version TEXT,
    payload_encrypted TEXT,

    -- Render (document worker)
    render_status TEXT NOT NULL DEFAULT 'none',
    render_error TEXT,
    render_claim UUID,
    render_started_at TIMESTAMPTZ,
    pdf_path TEXT,
    pdf_sha256 TEXT,

    -- The signed copy (a PDF scan), stored encrypted like generated PDFs.
    signed_at DATE,
    signed_pdf_path TEXT,
    signed_pdf_sha256 TEXT,

    generated_by UUID REFERENCES users (id) ON DELETE SET NULL,
    generated_at TIMESTAMPTZ,
    last_sent_at TIMESTAMPTZ,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_legal_documents_type CHECK (doc_type IN ('freelance', 'subcontract', 'sow', 'sla', 'bast')),
    CONSTRAINT chk_legal_documents_status CHECK (status IN ('draft', 'generated', 'sent', 'signed', 'cancelled')),
    CONSTRAINT chk_legal_documents_title CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    CONSTRAINT chk_legal_documents_fields CHECK (jsonb_typeof(fields) = 'object'),
    CONSTRAINT chk_legal_documents_city CHECK (document_city IS NULL OR length(document_city) <= 100),
    CONSTRAINT chk_legal_documents_render_status CHECK (render_status IN ('none', 'pending', 'rendering', 'ready', 'failed')),
    CONSTRAINT chk_legal_documents_numbers CHECK ((seq_no IS NULL) = (doc_number IS NULL)),
    CONSTRAINT chk_legal_documents_pdf CHECK ((pdf_path IS NULL) = (pdf_sha256 IS NULL)),
    CONSTRAINT chk_legal_documents_signed_pdf CHECK ((signed_pdf_path IS NULL) = (signed_pdf_sha256 IS NULL)),
    CONSTRAINT chk_legal_documents_signed CHECK (status <> 'signed' OR signed_at IS NOT NULL),
    CONSTRAINT uq_legal_documents_doc_number UNIQUE (tenant_id, doc_number)
);

CREATE INDEX idx_legal_documents_tenant_id ON legal_documents (tenant_id);
CREATE INDEX idx_legal_documents_type_status ON legal_documents (tenant_id, doc_type, status);
CREATE INDEX idx_legal_documents_party ON legal_documents (party_id);
CREATE INDEX idx_legal_documents_related ON legal_documents (related_document_id)
    WHERE related_document_id IS NOT NULL;
CREATE INDEX idx_legal_documents_render_pending ON legal_documents (tenant_id, render_status)
    WHERE render_status IN ('pending', 'rendering');

ALTER TABLE legal_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_documents FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON legal_documents
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON legal_parties TO kantor_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON legal_documents TO kantor_app';
    END IF;
END $$;

-- Legal documents are e-mailed to the party's address: a new delivery kind
-- and recipient source. Widening a CHECK keeps every existing row valid.
ALTER TABLE email_deliveries DROP CONSTRAINT IF EXISTS chk_email_deliveries_kind;
ALTER TABLE email_deliveries
    ADD CONSTRAINT chk_email_deliveries_kind CHECK (kind IN ('contract', 'payslip', 'test', 'legal'));

ALTER TABLE email_deliveries DROP CONSTRAINT IF EXISTS chk_email_deliveries_recipient_source;
ALTER TABLE email_deliveries
    ADD CONSTRAINT chk_email_deliveries_recipient_source CHECK (recipient_source IN ('login', 'employee', 'personal', 'self', 'party'));

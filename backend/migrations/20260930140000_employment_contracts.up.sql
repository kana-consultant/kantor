-- Kontrak kerja (PKWT + NDA/HKI). A contract row holds the terms HR enters;
-- generating it snapshots both document payloads (encrypted with
-- security.Encrypter, like payslips) and renders the PKWT and the NDA/HKI to
-- PDF in the background. Record-only rows (PKWTT, Magang, paper history)
-- never produce documents but still feed payslips and the PKWT 5-year check.
--
-- Numbering: the PKWT and the NDA share one monthly EMPLOYMENT sequence
-- value (seq_no), assigned once at the first generate and kept by every
-- revision (editing a sent contract returns it to draft with revision+1).
CREATE TABLE employment_contracts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    -- Legal records: an employee with contracts cannot be deleted (409).
    employee_id UUID NOT NULL REFERENCES employees (id) ON DELETE RESTRICT,
    -- The Perpanjang (renewal) chain.
    previous_contract_id UUID REFERENCES employment_contracts (id) ON DELETE RESTRICT,
    contract_type TEXT NOT NULL,
    is_record_only BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'draft',
    revision INTEGER NOT NULL DEFAULT 0,
    start_date DATE NOT NULL,
    end_date DATE,

    -- Posisi
    job_title TEXT NOT NULL,
    department TEXT,
    supervisor_name TEXT,
    work_location TEXT NOT NULL DEFAULT '',
    work_mode TEXT NOT NULL DEFAULT 'wfo',
    work_mode_detail TEXT,
    pkwt_basis TEXT,
    job_description TEXT NOT NULL DEFAULT '',

    -- Jam kerja
    work_days TEXT NOT NULL DEFAULT 'Senin–Jumat / Monday–Friday',
    work_hours TEXT NOT NULL DEFAULT '09.00–18.00 WIB',
    weekly_hours INTEGER NOT NULL DEFAULT 40,
    notice_days INTEGER NOT NULL DEFAULT 30,

    -- Kompensasi (security.Encrypter JSON {base_salary, fixed_allowance};
    -- returned only to callers with hris:salary:view) and benefits.
    compensation_encrypted TEXT,
    benefits JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- NDA & HKI
    incident_report_hours INTEGER NOT NULL DEFAULT 24,
    non_solicit_months INTEGER NOT NULL DEFAULT 12,
    confidentiality_years INTEGER NOT NULL DEFAULT 3,
    prior_works JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- Dokumen
    document_date DATE,
    document_city TEXT,
    seq_no INTEGER,
    doc_number TEXT,
    nda_doc_number TEXT,
    template_version TEXT,
    payload_encrypted TEXT,

    -- Render (document worker). pdf_sha256 holds the digests of the PKWT and
    -- the NDA PDFs, in that order.
    render_status TEXT NOT NULL DEFAULT 'none',
    render_error TEXT,
    render_claim UUID,
    render_started_at TIMESTAMPTZ,
    pkwt_pdf_path TEXT,
    nda_pdf_path TEXT,
    pdf_sha256 TEXT[],

    generated_by UUID REFERENCES users (id) ON DELETE SET NULL,
    generated_at TIMESTAMPTZ,
    last_sent_at TIMESTAMPTZ,
    signed_at DATE,
    ended_at DATE,
    end_notes TEXT,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_employment_contracts_type CHECK (contract_type IN ('PKWT', 'PKWTT', 'MAGANG')),
    -- Only a PKWT generates documents; PKWTT and Magang are always records.
    CONSTRAINT chk_employment_contracts_record_only CHECK (contract_type = 'PKWT' OR is_record_only),
    CONSTRAINT chk_employment_contracts_status CHECK (status IN ('draft', 'generated', 'sent', 'signed', 'ended', 'cancelled')),
    CONSTRAINT chk_employment_contracts_revision CHECK (revision >= 0),
    CONSTRAINT chk_employment_contracts_pkwt_end CHECK (contract_type <> 'PKWT' OR end_date IS NOT NULL),
    CONSTRAINT chk_employment_contracts_dates CHECK (end_date IS NULL OR end_date >= start_date),
    CONSTRAINT chk_employment_contracts_job_title CHECK (length(btrim(job_title)) BETWEEN 1 AND 120),
    CONSTRAINT chk_employment_contracts_work_mode CHECK (work_mode IN ('wfo', 'hybrid', 'remote')),
    CONSTRAINT chk_employment_contracts_weekly_hours CHECK (weekly_hours BETWEEN 1 AND 60),
    CONSTRAINT chk_employment_contracts_notice_days CHECK (notice_days BETWEEN 0 AND 180),
    CONSTRAINT chk_employment_contracts_incident_hours CHECK (incident_report_hours BETWEEN 1 AND 720),
    CONSTRAINT chk_employment_contracts_non_solicit CHECK (non_solicit_months BETWEEN 0 AND 60),
    CONSTRAINT chk_employment_contracts_confidentiality CHECK (confidentiality_years BETWEEN 0 AND 30),
    CONSTRAINT chk_employment_contracts_benefits CHECK (jsonb_typeof(benefits) = 'array'),
    CONSTRAINT chk_employment_contracts_prior_works CHECK (jsonb_typeof(prior_works) = 'array'),
    CONSTRAINT chk_employment_contracts_render_status CHECK (render_status IN ('none', 'pending', 'rendering', 'ready', 'failed')),
    -- Numbers are assigned together, only to documents.
    CONSTRAINT chk_employment_contracts_numbers CHECK (
        (seq_no IS NULL AND doc_number IS NULL AND nda_doc_number IS NULL)
        OR (seq_no IS NOT NULL AND doc_number IS NOT NULL AND nda_doc_number IS NOT NULL AND NOT is_record_only)
    ),
    CONSTRAINT chk_employment_contracts_pdf CHECK (
        (pkwt_pdf_path IS NULL AND nda_pdf_path IS NULL AND pdf_sha256 IS NULL)
        OR (pkwt_pdf_path IS NOT NULL AND nda_pdf_path IS NOT NULL AND cardinality(pdf_sha256) = 2)
    ),
    CONSTRAINT chk_employment_contracts_signed CHECK (status <> 'signed' OR signed_at IS NOT NULL),
    CONSTRAINT chk_employment_contracts_ended CHECK (status <> 'ended' OR ended_at IS NOT NULL),
    CONSTRAINT chk_employment_contracts_end_notes CHECK (end_notes IS NULL OR length(end_notes) <= 500),
    CONSTRAINT uq_employment_contracts_doc_number UNIQUE (tenant_id, doc_number),
    CONSTRAINT uq_employment_contracts_nda_doc_number UNIQUE (tenant_id, nda_doc_number)
);

CREATE INDEX idx_employment_contracts_employee ON employment_contracts (tenant_id, employee_id);
CREATE INDEX idx_employment_contracts_status ON employment_contracts (tenant_id, status);
CREATE INDEX idx_employment_contracts_end_date ON employment_contracts (tenant_id, end_date);
CREATE INDEX idx_employment_contracts_previous ON employment_contracts (previous_contract_id)
    WHERE previous_contract_id IS NOT NULL;
CREATE INDEX idx_employment_contracts_render_pending ON employment_contracts (tenant_id, render_status)
    WHERE render_status IN ('pending', 'rendering');

ALTER TABLE employment_contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE employment_contracts FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON employment_contracts
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE INDEX idx_employment_contracts_tenant_id ON employment_contracts (tenant_id);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON employment_contracts TO kantor_app';
    END IF;
END $$;

-- payslips.contract_id (created by the payslips migration) now points at the
-- active contract the slip took its status kerja and jabatan from.
ALTER TABLE payslips
    ADD CONSTRAINT fk_payslips_contract
    FOREIGN KEY (contract_id) REFERENCES employment_contracts (id) ON DELETE SET NULL;

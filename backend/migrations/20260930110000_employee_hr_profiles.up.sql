-- HR data kept out of model.Employee (which every hris:employee:view holder
-- and the MCP tools can read). Identity data (NIK, birth place and date,
-- gender, bank account holder name, KTP address) is one security.Encrypter
-- JSON blob: there is deliberately no plaintext nik_last4, birth date or
-- gender column, because together they would rebuild the NIK.
-- employee_number / employee_code are assigned lazily from
-- document_sequences ('EMPLOYEE', 'ALL') at the first payslip or contract.
CREATE TABLE employee_hr_profiles (
    employee_id UUID PRIMARY KEY REFERENCES employees (id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    employee_number INTEGER,
    employee_code TEXT,
    job_title TEXT,
    personal_email TEXT,
    identity_encrypted TEXT,
    identity_updated_at TIMESTAMPTZ,
    updated_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_employee_hr_profiles_number UNIQUE (tenant_id, employee_number),
    CONSTRAINT uq_employee_hr_profiles_code UNIQUE (tenant_id, employee_code),
    CONSTRAINT chk_employee_hr_profiles_number CHECK (employee_number IS NULL OR employee_number > 0),
    CONSTRAINT chk_employee_hr_profiles_code CHECK ((employee_number IS NULL) = (employee_code IS NULL))
);

ALTER TABLE employee_hr_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE employee_hr_profiles FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON employee_hr_profiles
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE INDEX idx_employee_hr_profiles_tenant_id ON employee_hr_profiles (tenant_id);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON employee_hr_profiles TO kantor_app';
    END IF;
END $$;

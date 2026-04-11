CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    requested_ip INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);
CREATE INDEX idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at);

ALTER TABLE password_reset_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE password_reset_tokens FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON password_reset_tokens
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE password_reset_tokens
    ADD CONSTRAINT uq_password_reset_tokens_tenant_hash UNIQUE (tenant_id, token_hash);

DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON password_reset_tokens TO kantor_app';
    END IF;
END $$;

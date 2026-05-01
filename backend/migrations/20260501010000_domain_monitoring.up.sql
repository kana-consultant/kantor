-- Domain monitoring tables
--
-- Inventory + renewal tracking + DNS resolution check + WHOIS auto-sync.
-- Sister module to VPS monitoring — same RLS / tenant isolation pattern.
--
-- Schema is flatter than VPS: typically 1 DNS check per domain, so the
-- check fields live inline on `domains`. A separate `domain_health_events`
-- log keeps probe history for debugging (7-day retention).

CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    registrar TEXT NOT NULL DEFAULT '',
    nameservers TEXT[] NOT NULL DEFAULT '{}',
    expiry_date DATE,
    cost_amount BIGINT NOT NULL DEFAULT 0 CHECK (cost_amount >= 0),
    cost_currency TEXT NOT NULL DEFAULT 'IDR',
    billing_cycle TEXT NOT NULL DEFAULT 'yearly' CHECK (billing_cycle IN ('monthly', 'yearly')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'expired', 'transferring', 'parked')),
    tags TEXT[] NOT NULL DEFAULT '{}',
    notes TEXT NOT NULL DEFAULT '',

    -- DNS resolution check (Phase B)
    dns_check_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    dns_expected_ip TEXT NOT NULL DEFAULT '',  -- optional, '' means just check resolves
    dns_check_interval_seconds INT NOT NULL DEFAULT 3600 CHECK (dns_check_interval_seconds BETWEEN 60 AND 86400),
    dns_last_status TEXT NOT NULL DEFAULT 'unknown' CHECK (dns_last_status IN ('unknown', 'up', 'down')),
    dns_last_resolved_ips TEXT[] NOT NULL DEFAULT '{}',
    dns_last_error TEXT NOT NULL DEFAULT '',
    dns_last_check_at TIMESTAMPTZ,
    dns_last_status_changed_at TIMESTAMPTZ,
    dns_consecutive_fails INT NOT NULL DEFAULT 0,
    dns_alert_active BOOLEAN NOT NULL DEFAULT FALSE,
    dns_alert_last_sent_at TIMESTAMPTZ,

    -- WHOIS auto-sync (Phase C)
    whois_sync_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    whois_last_sync_at TIMESTAMPTZ,
    whois_last_error TEXT NOT NULL DEFAULT '',

    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_domains_tenant_name UNIQUE (tenant_id, name)
);

CREATE INDEX idx_domains_tenant_status ON domains (tenant_id, status);
CREATE INDEX idx_domains_expiry_date ON domains (expiry_date) WHERE expiry_date IS NOT NULL;
CREATE INDEX idx_domains_dns_due ON domains (dns_last_check_at NULLS FIRST) WHERE dns_check_enabled = TRUE;

ALTER TABLE domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE domains FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON domains
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);


CREATE TABLE domain_health_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    -- type: 'dns' | 'whois'
    event_type TEXT NOT NULL CHECK (event_type IN ('dns', 'whois')),
    status TEXT NOT NULL CHECK (status IN ('up', 'down', 'synced', 'error')),
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_domain_health_events_domain_created ON domain_health_events (domain_id, created_at DESC);
CREATE INDEX idx_domain_health_events_created_at ON domain_health_events (created_at);

ALTER TABLE domain_health_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE domain_health_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON domain_health_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);


DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON domains TO kantor_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON domain_health_events TO kantor_app';
    END IF;
END $$;

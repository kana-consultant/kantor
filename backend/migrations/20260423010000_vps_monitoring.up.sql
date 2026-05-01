-- VPS Monitoring tables
--
-- Inventory + uptime monitoring for the operational module. The owner runs
-- many VPS across providers and needs a single dashboard for: status,
-- per-app health, SSL expiry, billing renewals.
--
-- Five tables:
--   vps_servers              — inventory + last-known status snapshot
--   vps_apps                 — apps running on a VPS (with optional check FK)
--   vps_health_checks        — check definition (icmp/tcp/http) + interval
--   vps_health_events        — raw event log (cleanup ≥ 7 days, see job)
--   vps_health_daily_summary — pre-computed per-day uptime % (kept forever)

CREATE TABLE vps_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL DEFAULT '',
    cpu_cores INT NOT NULL DEFAULT 0 CHECK (cpu_cores >= 0),
    ram_mb INT NOT NULL DEFAULT 0 CHECK (ram_mb >= 0),
    disk_gb INT NOT NULL DEFAULT 0 CHECK (disk_gb >= 0),
    cost_amount BIGINT NOT NULL DEFAULT 0 CHECK (cost_amount >= 0),
    cost_currency TEXT NOT NULL DEFAULT 'IDR',
    billing_cycle TEXT NOT NULL DEFAULT 'monthly' CHECK (billing_cycle IN ('monthly', 'quarterly', 'yearly')),
    renewal_date DATE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'decommissioned')),
    tags TEXT[] NOT NULL DEFAULT '{}',
    notes TEXT NOT NULL DEFAULT '',
    -- Snapshot of last computed health (rolled up across all enabled checks)
    last_status TEXT NOT NULL DEFAULT 'unknown' CHECK (last_status IN ('unknown', 'up', 'degraded', 'down')),
    last_status_changed_at TIMESTAMPTZ,
    last_check_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vps_servers_tenant_status ON vps_servers (tenant_id, status);
CREATE INDEX idx_vps_servers_renewal_date ON vps_servers (renewal_date) WHERE renewal_date IS NOT NULL;

ALTER TABLE vps_servers ENABLE ROW LEVEL SECURITY;
ALTER TABLE vps_servers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vps_servers
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);


CREATE TABLE vps_health_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    vps_id UUID NOT NULL REFERENCES vps_servers(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('icmp', 'tcp', 'http', 'https')),
    -- target depends on type:
    --   icmp:  ip or hostname
    --   tcp:   ip:port
    --   http:  full URL
    --   https: full URL (also reads SSL cert)
    target TEXT NOT NULL,
    interval_seconds INT NOT NULL DEFAULT 60 CHECK (interval_seconds BETWEEN 30 AND 86400),
    timeout_seconds INT NOT NULL DEFAULT 5 CHECK (timeout_seconds BETWEEN 1 AND 60),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    -- runtime state
    last_status TEXT NOT NULL DEFAULT 'unknown' CHECK (last_status IN ('unknown', 'up', 'down')),
    last_latency_ms INT,
    last_error TEXT NOT NULL DEFAULT '',
    last_check_at TIMESTAMPTZ,
    last_status_changed_at TIMESTAMPTZ,
    consecutive_fails INT NOT NULL DEFAULT 0,
    consecutive_successes INT NOT NULL DEFAULT 0,
    -- alert dedup state
    alert_active BOOLEAN NOT NULL DEFAULT FALSE,
    alert_last_sent_at TIMESTAMPTZ,
    -- SSL cert metadata (only for https)
    ssl_expires_at TIMESTAMPTZ,
    ssl_issuer TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vps_health_checks_vps_id ON vps_health_checks (vps_id);
CREATE INDEX idx_vps_health_checks_due ON vps_health_checks (last_check_at NULLS FIRST) WHERE enabled = TRUE;

ALTER TABLE vps_health_checks ENABLE ROW LEVEL SECURITY;
ALTER TABLE vps_health_checks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vps_health_checks
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);


CREATE TABLE vps_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    vps_id UUID NOT NULL REFERENCES vps_servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    -- type free-form: 'database', 'web', 'cache', 'cron-job', etc — display only
    app_type TEXT NOT NULL DEFAULT '',
    port INT,
    url TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    -- optional FK to a check that monitors this app. NULL means "not monitored",
    -- the app is just listed for documentation.
    check_id UUID REFERENCES vps_health_checks(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vps_apps_vps_id ON vps_apps (vps_id);
CREATE INDEX idx_vps_apps_check_id ON vps_apps (check_id) WHERE check_id IS NOT NULL;

ALTER TABLE vps_apps ENABLE ROW LEVEL SECURITY;
ALTER TABLE vps_apps FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vps_apps
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);


CREATE TABLE vps_health_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    vps_id UUID NOT NULL REFERENCES vps_servers(id) ON DELETE CASCADE,
    check_id UUID NOT NULL REFERENCES vps_health_checks(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('up', 'down')),
    latency_ms INT,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vps_health_events_check_created ON vps_health_events (check_id, created_at DESC);
CREATE INDEX idx_vps_health_events_vps_created ON vps_health_events (vps_id, created_at DESC);
-- For retention sweeper
CREATE INDEX idx_vps_health_events_created_at ON vps_health_events (created_at);

ALTER TABLE vps_health_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE vps_health_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vps_health_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);


CREATE TABLE vps_health_daily_summary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT current_setting('app.current_tenant')::uuid REFERENCES tenants(id) ON DELETE CASCADE,
    vps_id UUID NOT NULL REFERENCES vps_servers(id) ON DELETE CASCADE,
    check_id UUID NOT NULL REFERENCES vps_health_checks(id) ON DELETE CASCADE,
    summary_date DATE NOT NULL,
    total_checks INT NOT NULL DEFAULT 0,
    up_count INT NOT NULL DEFAULT 0,
    down_count INT NOT NULL DEFAULT 0,
    uptime_pct NUMERIC(5, 2) NOT NULL DEFAULT 100.00,
    avg_latency_ms INT,
    p95_latency_ms INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vps_health_daily UNIQUE (tenant_id, check_id, summary_date)
);

CREATE INDEX idx_vps_health_daily_vps_date ON vps_health_daily_summary (vps_id, summary_date DESC);

ALTER TABLE vps_health_daily_summary ENABLE ROW LEVEL SECURITY;
ALTER TABLE vps_health_daily_summary FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vps_health_daily_summary
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);


-- Optional grants for the runtime role
DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'kantor_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON vps_servers TO kantor_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON vps_apps TO kantor_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON vps_health_checks TO kantor_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON vps_health_events TO kantor_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON vps_health_daily_summary TO kantor_app';
    END IF;
END $$;

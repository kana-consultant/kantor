CREATE TABLE ads_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    platform TEXT NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    amount_spent BIGINT NOT NULL DEFAULT 0,
    impressions BIGINT NOT NULL DEFAULT 0,
    clicks BIGINT NOT NULL DEFAULT 0,
    conversions BIGINT NOT NULL DEFAULT 0,
    revenue BIGINT NOT NULL DEFAULT 0,
    notes TEXT,
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ads_metrics_platform CHECK (platform IN ('instagram', 'facebook', 'google_ads', 'tiktok', 'youtube', 'other')),
    CONSTRAINT chk_ads_metrics_period CHECK (period_end >= period_start),
    CONSTRAINT chk_ads_metrics_amount_spent CHECK (amount_spent >= 0),
    CONSTRAINT chk_ads_metrics_impressions CHECK (impressions >= 0),
    CONSTRAINT chk_ads_metrics_clicks CHECK (clicks >= 0),
    CONSTRAINT chk_ads_metrics_conversions CHECK (conversions >= 0),
    CONSTRAINT chk_ads_metrics_revenue CHECK (revenue >= 0)
);

CREATE INDEX idx_ads_metrics_campaign_id ON ads_metrics (campaign_id);
CREATE INDEX idx_ads_metrics_platform ON ads_metrics (platform);
CREATE INDEX idx_ads_metrics_period_start ON ads_metrics (period_start);
CREATE INDEX idx_ads_metrics_period_end ON ads_metrics (period_end);
CREATE INDEX idx_ads_metrics_created_by ON ads_metrics (created_by);

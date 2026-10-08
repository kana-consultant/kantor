-- Campaign board: a stable stage key per column, and the Meta Ads channel.
--
-- DDL only. No row of campaigns, campaign_columns, campaign_column_assignments
-- or ads_metrics is rewritten here: `stage` starts NULL on every existing
-- column and the server fills it at boot (EnsureStageColumns adopts the
-- column whose name already matches a stage, or appends the missing one).

-- 1. campaign_columns.stage: which campaigns.status a lane stands for.
--    NULL = a custom lane, which never changes a campaign's status.
ALTER TABLE campaign_columns
    ADD COLUMN stage TEXT;

ALTER TABLE campaign_columns
    ADD CONSTRAINT chk_campaign_columns_stage
    CHECK (stage IS NULL OR stage IN ('ideation', 'planning', 'in_production', 'live', 'completed', 'archived'));

-- At most one lane per stage and tenant.
CREATE UNIQUE INDEX uq_campaign_columns_tenant_stage
    ON campaign_columns (tenant_id, stage)
    WHERE stage IS NOT NULL;

-- 2. 'meta_ads' as a campaign channel and as an ads-metrics platform.
--    Widening a CHECK keeps every existing row valid.
ALTER TABLE campaigns
    DROP CONSTRAINT IF EXISTS chk_campaigns_channel;

ALTER TABLE campaigns
    ADD CONSTRAINT chk_campaigns_channel
    CHECK (channel IN ('meta_ads', 'instagram', 'facebook', 'google_ads', 'tiktok', 'youtube', 'email', 'other'));

ALTER TABLE ads_metrics
    DROP CONSTRAINT IF EXISTS chk_ads_metrics_platform;

ALTER TABLE ads_metrics
    ADD CONSTRAINT chk_ads_metrics_platform
    CHECK (platform IN ('meta_ads', 'instagram', 'facebook', 'google_ads', 'tiktok', 'youtube', 'other'));

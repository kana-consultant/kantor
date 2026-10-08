-- Removes the stage key again. Lanes, campaigns and their placement stay as
-- they are; only the key is dropped.
DROP INDEX IF EXISTS uq_campaign_columns_tenant_stage;

ALTER TABLE campaign_columns
    DROP CONSTRAINT IF EXISTS chk_campaign_columns_stage;

ALTER TABLE campaign_columns
    DROP COLUMN IF EXISTS stage;

-- chk_campaigns_channel and chk_ads_metrics_platform are left widened on
-- purpose. Narrowing them would either fail (rows using 'meta_ads' exist) or,
-- as NOT VALID, make every later UPDATE of such a row fail, so those campaigns
-- could no longer be edited or moved. The older code never writes 'meta_ads',
-- so the wider CHECK is harmless to it. The up migration drops and re-adds
-- both constraints, so migrating up again works.

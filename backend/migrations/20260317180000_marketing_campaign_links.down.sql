DROP INDEX IF EXISTS idx_leads_campaign_id;

ALTER TABLE leads
DROP COLUMN IF EXISTS campaign_id;

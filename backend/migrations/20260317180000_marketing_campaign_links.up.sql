ALTER TABLE leads
ADD COLUMN campaign_id uuid REFERENCES campaigns(id) ON DELETE SET NULL;

CREATE INDEX idx_leads_campaign_id ON leads(campaign_id);

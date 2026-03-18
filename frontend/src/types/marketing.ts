export type CampaignChannel =
  | "instagram"
  | "facebook"
  | "google_ads"
  | "tiktok"
  | "youtube"
  | "email"
  | "other";

export type CampaignStatus =
  | "ideation"
  | "planning"
  | "in_production"
  | "live"
  | "completed"
  | "archived";

export interface Campaign {
  id: string;
  name: string;
  description?: string | null;
  channel: CampaignChannel;
  budget_amount: number;
  budget_currency: string;
  pic_employee_id?: string | null;
  pic_employee_name?: string | null;
  pic_avatar_url?: string | null;
  start_date: string;
  end_date: string;
  brief_text?: string | null;
  status: CampaignStatus;
  created_by: string;
  created_at: string;
  updated_at: string;
  column_id?: string | null;
  column_name?: string | null;
  column_color?: string | null;
  column_position?: number | null;
  attachment_count: number;
}

export interface CampaignAttachment {
  id: string;
  campaign_id: string;
  file_name: string;
  file_path: string;
  file_type: string;
  file_size: number;
  uploaded_by: string;
  created_at: string;
}

export interface CampaignDetail {
  campaign: Campaign;
  attachments: CampaignAttachment[];
}

export interface CampaignActivity {
  id: string;
  campaign_id: string;
  action: string;
  description: string;
  actor_id?: string | null;
  actor_name?: string | null;
  created_at: string;
}

export interface CampaignColumn {
  id: string;
  name: string;
  position: number;
  color?: string | null;
  created_at: string;
  campaigns?: Campaign[];
  campaign_count?: number;
}

export interface CampaignFilters {
  page: number;
  perPage: number;
  search: string;
  channel: string;
  status: string;
  pic: string;
  dateFrom: string;
  dateTo: string;
}

export interface CampaignFormValues {
  name: string;
  description: string;
  channel: CampaignChannel;
  budget_amount: number;
  budget_currency: string;
  pic_employee_id: string;
  start_date: string;
  end_date: string;
  brief_text: string;
  status: CampaignStatus;
}

export interface CampaignsListResponse {
  items: Campaign[];
  meta: {
    page: number;
    per_page: number;
    total: number;
  };
}

export type AdsMetricPlatform =
  | "instagram"
  | "facebook"
  | "google_ads"
  | "tiktok"
  | "youtube"
  | "other";

export interface AdsMetric {
  id: string;
  campaign_id: string;
  campaign_name?: string | null;
  platform: AdsMetricPlatform;
  period_start: string;
  period_end: string;
  amount_spent: number;
  impressions: number;
  clicks: number;
  conversions: number;
  revenue: number;
  notes?: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
  cpr?: number | null;
  roas?: number | null;
  ctr?: number | null;
  cpc?: number | null;
  cpm?: number | null;
}

export interface AdsMetricFilters {
  page: number;
  perPage: number;
  campaignId: string;
  platform: string;
  dateFrom: string;
  dateTo: string;
}

export interface AdsMetricFormValues {
  campaign_id: string;
  platform: AdsMetricPlatform;
  period_start: string;
  period_end: string;
  amount_spent: number;
  impressions: number;
  clicks: number;
  conversions: number;
  revenue: number;
  notes: string;
}

export interface AdsMetricsListResponse {
  items: AdsMetric[];
  meta: {
    page: number;
    per_page: number;
    total: number;
  };
}

export interface AdsMetricsSummaryRow {
  group_key: string;
  group_label: string;
  total_spent: number;
  total_impressions: number;
  total_clicks: number;
  total_conversions: number;
  total_revenue: number;
  cpr?: number | null;
  roas?: number | null;
  ctr?: number | null;
  cpc?: number | null;
  cpm?: number | null;
}

export interface AdsMetricsSummary {
  group_by: "campaign" | "platform" | "month";
  items: AdsMetricsSummaryRow[];
}

export type LeadSourceChannel =
  | "whatsapp"
  | "email"
  | "instagram"
  | "facebook"
  | "website"
  | "referral"
  | "other";

export type LeadPipelineStatus =
  | "new"
  | "contacted"
  | "qualified"
  | "proposal"
  | "negotiation"
  | "won"
  | "lost";

export interface Lead {
  id: string;
  name: string;
  phone?: string | null;
  email?: string | null;
  source_channel: LeadSourceChannel;
  pipeline_status: LeadPipelineStatus;
  campaign_id?: string | null;
  campaign_name?: string | null;
  assigned_to?: string | null;
  assigned_to_name?: string | null;
  assigned_to_avatar?: string | null;
  notes?: string | null;
  company_name?: string | null;
  estimated_value: number;
  created_by: string;
  created_by_name?: string | null;
  created_at: string;
  updated_at: string;
}

export interface LeadActivity {
  id: string;
  lead_id: string;
  activity_type: string;
  description: string;
  old_status?: string | null;
  new_status?: string | null;
  created_by: string;
  created_by_name?: string | null;
  created_at: string;
}

export interface LeadPipelineColumn {
  status: LeadPipelineStatus;
  label: string;
  leads: Lead[];
}

export interface LeadFilters {
  page: number;
  perPage: number;
  pipelineStatus: string;
  sourceChannel: string;
  campaignId: string;
  assignedTo: string;
  dateFrom: string;
  dateTo: string;
  search: string;
}

export interface LeadFormValues {
  name: string;
  phone: string;
  email: string;
  source_channel: LeadSourceChannel;
  pipeline_status: LeadPipelineStatus;
  campaign_id: string;
  assigned_to: string;
  notes: string;
  company_name: string;
  estimated_value: number;
}

export interface LeadsListResponse {
  items: Lead[];
  meta: {
    page: number;
    per_page: number;
    total: number;
  };
}

export interface LeadSummaryRow {
  status: LeadPipelineStatus;
  label: string;
  lead_count: number;
  estimated_value: number;
}

export interface LeadSummary {
  total_leads: number;
  won_leads: number;
  conversion_rate: number;
  by_status: LeadSummaryRow[];
}

export interface LeadImportError {
  row: number;
  message: string;
}

export interface LeadImportSummary {
  success_count: number;
  failed_count: number;
  errors: LeadImportError[];
}

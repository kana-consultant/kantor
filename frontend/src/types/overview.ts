export interface OverviewSeriesPoint {
  key: string;
  label: string;
  value: number;
}

export interface OperationalRecentTask {
  id: string;
  project_id: string;
  project_name: string;
  title: string;
  status: string;
  priority: string;
  assignee_id?: string | null;
  assignee_name?: string | null;
  assignee_avatar?: string | null;
  due_date?: string | null;
  updated_at: string;
}

export interface OperationalOverview {
  total_projects: number;
  active_tasks: number;
  overdue_tasks: number;
  team_members: number;
  completed_by_week: OverviewSeriesPoint[];
  recent_tasks: OperationalRecentTask[];
}

export interface FinanceOverviewPoint {
  key: string;
  label: string;
  income: number;
  outcome: number;
}

export interface HrisUpcomingRenewal {
  id: string;
  name: string;
  vendor: string;
  renewal_date: string;
  days_remaining: number;
  cost_amount: number;
  cost_currency: string;
  pic_employee_name?: string | null;
}

export interface HrisRecentReimbursement {
  id: string;
  employee_id: string;
  employee_name: string;
  title: string;
  category: string;
  amount: number;
  transaction_date: string;
  description: string;
  status: string;
  attachments: string[];
  created_at: string;
  updated_at: string;
}

export interface HrisOverview {
  total_employees: number;
  active_subscriptions: number;
  active_subscription_monthly_cost: number;
  monthly_net: number;
  pending_reimbursements: number;
  income_vs_outcome: FinanceOverviewPoint[];
  upcoming_renewals: HrisUpcomingRenewal[];
  recent_reimbursements: HrisRecentReimbursement[];
}

export interface MarketingRoasTrendPoint {
  key: string;
  label: string;
  spent: number;
  revenue: number;
  roas?: number | null;
}

export interface MarketingLeadStage {
  status: string;
  label: string;
  lead_count: number;
  estimated_value: number;
}

export interface MarketingTopCampaign {
  campaign_id: string;
  campaign_name: string;
  status: string;
  total_spent: number;
  total_revenue: number;
  roas?: number | null;
}

export interface MarketingOverview {
  active_campaigns: number;
  total_ads_spent: number;
  overall_roas?: number | null;
  total_leads: number;
  conversion_rate: number;
  roas_trend: MarketingRoasTrendPoint[];
  leads_by_stage: MarketingLeadStage[];
  top_campaigns: MarketingTopCampaign[];
}

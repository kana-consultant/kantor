export interface ActivityConsent {
  id?: string;
  user_id: string;
  consented: boolean;
  consented_at?: string | null;
  revoked_at?: string | null;
  ip_address?: string | null;
  created_at?: string;
}

export interface TrackerCategoryBreakdown {
  category: string;
  duration_seconds: number;
  is_productive: boolean;
}

export interface TrackerHourlyBreakdown {
  hour: number;
  label: string;
  duration_seconds: number;
}

export interface TrackerTopDomain {
  domain: string;
  category: string;
  duration_seconds: number;
  is_productive: boolean;
  percentage: number;
}

export interface TrackerActivityOverview {
  user_id: string;
  user_name: string;
  total_active_seconds: number;
  total_idle_seconds: number;
  productivity_score: number;
  most_used_domain?: string | null;
  category_breakdown: TrackerCategoryBreakdown[];
  hourly_breakdown: TrackerHourlyBreakdown[];
  top_domains: TrackerTopDomain[];
}

export interface TrackerUserSummary {
  user_id: string;
  user_name: string;
  active_seconds: number;
  idle_seconds: number;
  productivity_score: number;
  top_domain?: string | null;
  category_breakdown: Record<string, number>;
}

export interface TrackerTeamOverview {
  members_tracked: number;
  avg_active_seconds: number;
  top_productive_member?: string | null;
  least_productive_member?: string | null;
  users: TrackerUserSummary[];
}

export interface TrackerDailySummary {
  total_users: number;
  avg_active_seconds: number;
  top_productive_domains: TrackerTopDomain[];
  top_unproductive_domains: TrackerTopDomain[];
}

export interface DomainCategory {
  id: string;
  domain_pattern: string;
  category: string;
  is_productive: boolean;
  created_at: string;
}

export interface TrackerConsentAudit {
  user_id: string;
  user_name: string;
  user_email: string;
  consented: boolean;
  consented_at?: string | null;
  revoked_at?: string | null;
  ip_address?: string | null;
  browser_timezone?: string | null;
  tracker_extension_version?: string | null;
  tracker_extension_reported_at?: string | null;
  last_session_started_at?: string | null;
  last_activity_at?: string | null;
}
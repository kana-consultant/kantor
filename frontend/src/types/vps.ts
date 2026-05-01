export type VPSStatus = "active" | "suspended" | "decommissioned";
export type VPSBillingCycle = "monthly" | "quarterly" | "yearly";
export type VPSHealthStatus = "unknown" | "up" | "degraded" | "down";
export type VPSCheckStatus = "unknown" | "up" | "down";
export type VPSCheckType = "icmp" | "tcp" | "http" | "https";

export interface VPSServer {
  id: string;
  tenant_id: string;
  label: string;
  provider: string;
  hostname: string;
  ip_address: string;
  region: string;
  cpu_cores: number;
  ram_mb: number;
  disk_gb: number;
  cost_amount: number;
  cost_currency: string;
  billing_cycle: VPSBillingCycle;
  renewal_date?: string | null;
  status: VPSStatus;
  tags: string[];
  notes: string;
  last_status: VPSHealthStatus;
  last_status_changed_at?: string | null;
  last_check_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface VPSServerSummary extends VPSServer {
  apps_count: number;
  checks_count: number;
  down_checks_count: number;
}

export interface VPSHealthCheck {
  id: string;
  vps_id: string;
  label: string;
  type: VPSCheckType;
  target: string;
  interval_seconds: number;
  timeout_seconds: number;
  enabled: boolean;
  last_status: VPSCheckStatus;
  last_latency_ms?: number | null;
  last_error: string;
  last_check_at?: string | null;
  last_status_changed_at?: string | null;
  consecutive_fails: number;
  consecutive_successes: number;
  alert_active: boolean;
  alert_last_sent_at?: string | null;
  ssl_expires_at?: string | null;
  ssl_issuer: string;
  created_at: string;
  updated_at: string;
}

export interface VPSApp {
  id: string;
  vps_id: string;
  name: string;
  app_type: string;
  port?: number | null;
  url: string;
  notes: string;
  check_id?: string | null;
  created_at: string;
  updated_at: string;
}

export interface VPSHealthEvent {
  id: string;
  vps_id: string;
  check_id: string;
  status: "up" | "down";
  latency_ms?: number | null;
  error_message: string;
  created_at: string;
}

export interface VPSDailySummary {
  vps_id: string;
  check_id: string;
  summary_date: string;
  total_checks: number;
  up_count: number;
  down_count: number;
  uptime_pct: number;
  avg_latency_ms?: number | null;
  p95_latency_ms?: number | null;
}

export interface VPSDetail {
  server: VPSServer;
  checks: VPSHealthCheck[];
  apps: VPSApp[];
  events: VPSHealthEvent[];
  daily: VPSDailySummary[];
}

export interface VPSListFilters {
  search?: string;
  status?: VPSStatus | "";
  provider?: string;
  tag?: string;
}

export interface VPSFormValues {
  label: string;
  provider: string;
  hostname: string;
  ip_address: string;
  region: string;
  cpu_cores: number;
  ram_mb: number;
  disk_gb: number;
  cost_amount: number;
  cost_currency: string;
  billing_cycle: VPSBillingCycle;
  renewal_date?: string | null;
  status: VPSStatus;
  tags: string[];
  notes: string;
}

export interface VPSCheckFormValues {
  label: string;
  type: VPSCheckType;
  target: string;
  interval_seconds: number;
  timeout_seconds: number;
  enabled?: boolean;
}

export interface VPSAppFormValues {
  name: string;
  app_type: string;
  port?: number | null;
  url: string;
  notes: string;
  check_id?: string | null;
}

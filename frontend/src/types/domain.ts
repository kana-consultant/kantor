export type DomainStatus = "active" | "expired" | "transferring" | "parked";
export type DomainBillingCycle = "monthly" | "yearly";
export type DomainCheckStatus = "unknown" | "up" | "down";

export interface Domain {
  id: string;
  tenant_id: string;
  name: string;
  registrar: string;
  nameservers: string[];
  expiry_date?: string | null;
  cost_amount: number;
  cost_currency: string;
  billing_cycle: DomainBillingCycle;
  status: DomainStatus;
  tags: string[];
  notes: string;

  dns_check_enabled: boolean;
  dns_expected_ip: string;
  dns_check_interval_seconds: number;
  dns_last_status: DomainCheckStatus;
  dns_last_resolved_ips: string[];
  dns_last_error: string;
  dns_last_check_at?: string | null;
  dns_last_status_changed_at?: string | null;
  dns_consecutive_fails: number;
  dns_alert_active: boolean;
  dns_alert_last_sent_at?: string | null;

  whois_sync_enabled: boolean;
  whois_last_sync_at?: string | null;
  whois_last_error: string;

  created_at: string;
  updated_at: string;
}

export interface DomainHealthEvent {
  id: string;
  domain_id: string;
  event_type: "dns" | "whois";
  status: "up" | "down" | "synced" | "error";
  detail: string;
  created_at: string;
}

export interface DomainDetail {
  domain: Domain;
  events: DomainHealthEvent[];
}

export interface DomainListFilters {
  search?: string;
  status?: DomainStatus | "";
  registrar?: string;
  tag?: string;
}

export interface DomainFormValues {
  name: string;
  registrar: string;
  nameservers: string[];
  expiry_date?: string | null;
  cost_amount: number;
  cost_currency: string;
  billing_cycle: DomainBillingCycle;
  status: DomainStatus;
  tags: string[];
  notes: string;
  dns_check_enabled: boolean;
  dns_expected_ip: string;
  dns_check_interval_seconds: number;
  whois_sync_enabled: boolean;
}

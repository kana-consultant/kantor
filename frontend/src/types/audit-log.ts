import type { PaginationMeta } from "@/types/hris";

export interface AuditLogItem {
  id: string;
  user_id?: string | null;
  user_name: string;
  user_email: string;
  user_avatar_url?: string | null;
  action: string;
  module: "operational" | "hris" | "marketing" | "admin" | string;
  resource: string;
  resource_id: string;
  old_value?: unknown | null;
  new_value?: unknown | null;
  ip_address?: string;
  created_at: string;
}

export interface AuditLogFilters {
  module: string;
  action: string;
  userId: string;
  resource: string;
  resourceId: string;
  dateFrom: string;
  dateTo: string;
  search: string;
  page: number;
  perPage: number;
}

export interface AuditLogListResponse {
  items: AuditLogItem[];
  meta: PaginationMeta;
}

export interface AuditLogSummaryBucket {
  key: string;
  count: number;
}

export interface AuditLogTopUser {
  user_id?: string | null;
  user_name: string;
  user_email: string;
  count: number;
}

export interface AuditLogSummary {
  total_today: number;
  total_week: number;
  by_module: AuditLogSummaryBucket[];
  by_action: AuditLogSummaryBucket[];
  top_users: AuditLogTopUser[];
}

export interface AuditLogUserOption {
  user_id: string;
  user_name: string;
  user_email: string;
  user_avatar_url?: string | null;
}

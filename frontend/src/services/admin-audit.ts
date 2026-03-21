import { authDownload, authGetJSON, authRequestEnvelope } from "@/lib/api-client";
import type {
  AuditLogFilters,
  AuditLogListResponse,
  AuditLogSummary,
  AuditLogUserOption,
} from "@/types/audit-log";
import type { PaginationMeta } from "@/types/hris";

export const adminAuditKeys = {
  all: ["admin-audit"] as const,
  logs: () => [...adminAuditKeys.all, "logs"] as const,
  logList: (filters: AuditLogFilters) =>
    [...adminAuditKeys.logs(), "list", { ...filters }] as const,
  summary: () => [...adminAuditKeys.all, "summary"] as const,
  users: (search = "") => [...adminAuditKeys.all, "users", search] as const,
};

export async function listAuditLogs(filters: AuditLogFilters): Promise<AuditLogListResponse> {
  const params = new URLSearchParams();
  params.set("page", String(filters.page));
  params.set("per_page", String(filters.perPage));

  if (filters.module) {
    params.set("module", filters.module);
  }
  if (filters.action) {
    params.set("action", filters.action);
  }
  if (filters.userId) {
    params.set("user_id", filters.userId);
  }
  if (filters.resource) {
    params.set("resource", filters.resource);
  }
  if (filters.resourceId) {
    params.set("resource_id", filters.resourceId);
  }
  if (filters.dateFrom) {
    params.set("date_from", filters.dateFrom);
  }
  if (filters.dateTo) {
    params.set("date_to", filters.dateTo);
  }
  if (filters.search.trim()) {
    params.set("search", filters.search.trim());
  }

  const response = await authRequestEnvelope<AuditLogListResponse["items"]>(
    `/admin/audit-logs?${params.toString()}`,
    { method: "GET" },
  );

  return {
    items: response.data ?? [],
    meta:
      (response.meta as PaginationMeta | undefined) ?? {
        page: filters.page,
        per_page: filters.perPage,
        total: 0,
      },
  };
}

export function getAuditLogSummary() {
  return authGetJSON<AuditLogSummary>("/admin/audit-logs/summary");
}

export function listAuditLogUsers(search = "") {
  const suffix = search.trim() ? `?search=${encodeURIComponent(search.trim())}` : "";
  return authGetJSON<AuditLogUserOption[]>(`/admin/audit-logs/users${suffix}`);
}

export function exportAuditLogs(filters: AuditLogFilters) {
  const params = new URLSearchParams();

  if (filters.module) {
    params.set("module", filters.module);
  }
  if (filters.action) {
    params.set("action", filters.action);
  }
  if (filters.userId) {
    params.set("user_id", filters.userId);
  }
  if (filters.resource) {
    params.set("resource", filters.resource);
  }
  if (filters.resourceId) {
    params.set("resource_id", filters.resourceId);
  }
  if (filters.dateFrom) {
    params.set("date_from", filters.dateFrom);
  }
  if (filters.dateTo) {
    params.set("date_to", filters.dateTo);
  }
  if (filters.search.trim()) {
    params.set("search", filters.search.trim());
  }

  return authDownload(`/admin/audit-logs/export?${params.toString()}`);
}

import {
  authGetJSON,
  authPostJSON,
  authRequestEnvelope,
  authRequestJSON,
} from "@/lib/api-client";

// ---- Types ----

export interface WASessionStatus {
  name: string;
  status: string;
}

export interface WAAccountInfo {
  id: string;
  pushName: string;
}

export interface WADailyStats {
  sentToday: number;
  dailyLimit: number;
}

export interface WAStatusResponse {
  enabled: boolean;
  session: WASessionStatus;
}

export interface WAStatsResponse {
  enabled: boolean;
  daily_stats: WADailyStats;
  account: WAAccountInfo | null;
}

export interface WATemplate {
  id: string;
  name: string;
  slug: string;
  category: string;
  trigger_type: string;
  body_template: string;
  description: string | null;
  available_variables: string[];
  is_active: boolean;
  is_system: boolean;
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface WADefaultTemplatesSeedResult {
  inserted_count: number;
  existing_count: number;
  total_count: number;
  inserted_slugs?: string[];
}

export interface WASchedule {
  id: string;
  name: string;
  template_id: string;
  template_name: string;
  schedule_type: string;
  cron_expression: string | null;
  target_type: string;
  target_config: string | null;
  is_active: boolean;
  last_run_at: string | null;
  next_run_at: string | null;
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface WABroadcastLog {
  id: string;
  schedule_id: string | null;
  template_id: string | null;
  template_slug: string | null;
  trigger_type: string;
  recipient_user_id: string | null;
  recipient_phone: string;
  recipient_name: string | null;
  message_body: string;
  status: string;
  error_message: string | null;
  reference_type: string | null;
  reference_id: string | null;
  sent_at: string | null;
  created_at: string;
}

export interface WALogSummary {
  total_sent: number;
  total_failed: number;
  total_skipped: number;
  daily_limit: number;
  sent_today: number;
}

export interface WALogFilters {
  page: number;
  perPage: number;
  triggerType?: string;
  templateSlug?: string;
  status?: string;
  dateFrom?: string;
  dateTo?: string;
  search?: string;
}

export interface WAConfig {
  api_url: string;
  api_key: string;
  session_name: string;
  enabled: boolean;
  max_daily_messages: number;
  min_delay_ms: number;
  max_delay_ms: number;
  reminder_cron: string;
  weekly_digest_cron: string;
}

// ---- Query keys ----

export const waKeys = {
  all: ["wa"] as const,
  config: () => [...waKeys.all, "config"] as const,
  status: () => [...waKeys.all, "status"] as const,
  stats: () => [...waKeys.all, "stats"] as const,
  templates: (category?: string, triggerType?: string) => [...waKeys.all, "templates", category, triggerType] as const,
  schedules: () => [...waKeys.all, "schedules"] as const,
  logs: (filters: WALogFilters) => [...waKeys.all, "logs", filters] as const,
  logSummary: (date?: string) => [...waKeys.all, "logSummary", date] as const,
  phone: () => [...waKeys.all, "phone"] as const,
};

// ---- API functions ----

// Config
export function getWAConfig() {
  return authGetJSON<WAConfig>("/wa/config");
}

export function updateWAConfig(data: WAConfig) {
  return authRequestJSON<{ message: string }>("/wa/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

// Connection
export function getWAStatus() {
  return authGetJSON<WAStatusResponse>("/wa/status");
}

export function getWAQR() {
  return authGetJSON<{ qr: string }>("/wa/qr");
}

export function startWASession() {
  return authPostJSON<{ message: string }, undefined>("/wa/session/start", undefined);
}

export function stopWASession() {
  return authPostJSON<{ message: string }, undefined>("/wa/session/stop", undefined);
}

export function getWAStats() {
  return authGetJSON<WAStatsResponse>("/wa/stats");
}

// Templates
export async function listTemplates(category?: string, triggerType?: string) {
  const params = new URLSearchParams();
  if (category) params.set("category", category);
  if (triggerType) params.set("trigger_type", triggerType);
  const qs = params.toString();
  return authGetJSON<WATemplate[]>(`/wa/templates${qs ? "?" + qs : ""}`);
}

export function createTemplate(data: Partial<WATemplate>) {
  return authPostJSON<WATemplate, Partial<WATemplate>>("/wa/templates", data);
}

export function updateTemplate(id: string, data: Partial<WATemplate>) {
  return authRequestJSON<WATemplate>(`/wa/templates/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export function deleteTemplate(id: string) {
  return authRequestJSON<{ message: string }>(`/wa/templates/${id}`, { method: "DELETE" });
}

export function previewTemplate(id: string) {
  return authPostJSON<{ preview: string }, undefined>(`/wa/templates/${id}/preview`, undefined);
}

export function generateDefaultTemplates() {
  return authPostJSON<WADefaultTemplatesSeedResult, undefined>("/wa/templates/generate-defaults", undefined);
}

// Schedules
export function listSchedules() {
  return authGetJSON<WASchedule[]>("/wa/schedules");
}

export function createSchedule(data: Record<string, unknown>) {
  return authPostJSON<WASchedule, Record<string, unknown>>("/wa/schedules", data);
}

export function updateSchedule(id: string, data: Record<string, unknown>) {
  return authRequestJSON<WASchedule>(`/wa/schedules/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export function deleteSchedule(id: string) {
  return authRequestJSON<{ message: string }>(`/wa/schedules/${id}`, { method: "DELETE" });
}

export function triggerSchedule(id: string) {
  return authPostJSON<{ message: string }, undefined>(`/wa/schedules/${id}/trigger`, undefined);
}

export function toggleSchedule(id: string, isActive: boolean) {
  return authRequestJSON<WASchedule>(`/wa/schedules/${id}/toggle`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ is_active: isActive }),
  });
}

// Logs
export async function listLogs(filters: WALogFilters) {
  const params = new URLSearchParams({
    page: String(filters.page),
    per_page: String(filters.perPage),
  });
  if (filters.triggerType) params.set("trigger_type", filters.triggerType);
  if (filters.templateSlug) params.set("template_slug", filters.templateSlug);
  if (filters.status) params.set("status", filters.status);
  if (filters.dateFrom) params.set("date_from", filters.dateFrom);
  if (filters.dateTo) params.set("date_to", filters.dateTo);
  if (filters.search) params.set("search", filters.search);

  const envelope = await authRequestEnvelope<WABroadcastLog[]>(
    `/wa/logs?${params.toString()}`,
    { method: "GET" },
  );
  return {
    items: envelope.data,
    meta: envelope.meta as { page: number; per_page: number; total: number },
  };
}

export function getLogSummary(date?: string) {
  const qs = date ? `?date=${date}` : "";
  return authGetJSON<WALogSummary>(`/wa/logs/summary${qs}`);
}

// Quick Send
export function quickSend(phone: string, message: string) {
  return authPostJSON<{ message: string }, { phone: string; message: string }>("/wa/send", { phone, message });
}

// Phone
export function getUserPhone() {
  return authGetJSON<{ phone: string | null }>("/wa/phone");
}

export function updateUserPhone(phone: string | null) {
  return authRequestJSON<{ message: string }>("/wa/phone", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ phone }),
  });
}

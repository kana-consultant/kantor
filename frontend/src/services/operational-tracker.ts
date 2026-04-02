import { authDownload, authGetJSON, authPostJSON, authRequestJSON } from "@/lib/api-client";
import type {
  ActivityConsent,
  DomainCategory,
  TrackerActivityOverview,
  TrackerBulkClassifyDomainsResult,
  TrackerConsentAudit,
  TrackerDailySummary,
  TrackerObservedDomain,
  TrackerTeamOverview,
} from "@/types/tracker";

interface DomainCategoryPayload {
  domain_pattern: string;
  category: string;
  is_productive: boolean;
}

interface BulkClassifyTrackerDomainsPayload {
  domains: string[];
  is_productive: boolean;
  category?: string | null;
}

function createDateParams(dateFrom: string, dateTo: string) {
  const params = new URLSearchParams();
  params.set("date_from", dateFrom);
  params.set("date_to", dateTo);
  return params.toString();
}

export const trackerKeys = {
  consent: () => ["operational", "tracker", "consent"] as const,
  myActivity: (dateFrom: string, dateTo: string) =>
    ["operational", "tracker", "my-activity", dateFrom, dateTo] as const,
  teamActivity: (dateFrom: string, dateTo: string, userId?: string) =>
    ["operational", "tracker", "team-activity", dateFrom, dateTo, userId ?? "all"] as const,
  userActivity: (userId: string, dateFrom: string, dateTo: string) =>
    ["operational", "tracker", "user-activity", userId, dateFrom, dateTo] as const,
  summary: (date: string) => ["operational", "tracker", "summary", date] as const,
  consents: () => ["operational", "tracker", "consents"] as const,
  domains: () => ["operational", "tracker", "domains"] as const,
  observedDomains: () => ["operational", "tracker", "observed-domains"] as const,
};

export async function getTrackerConsent() {
  return authGetJSON<ActivityConsent>("/tracker/consent");
}

export async function giveTrackerConsent() {
  return authPostJSON<ActivityConsent, Record<string, never>>("/tracker/consent", {});
}

export async function revokeTrackerConsent() {
  return authRequestJSON<ActivityConsent>("/tracker/consent", {
    method: "DELETE",
  });
}

export async function getMyTrackerActivity(dateFrom: string, dateTo: string) {
  return authGetJSON<TrackerActivityOverview>(`/tracker/my-activity?${createDateParams(dateFrom, dateTo)}`);
}

export async function getTeamTrackerActivity(dateFrom: string, dateTo: string, userId?: string) {
  const params = new URLSearchParams(createDateParams(dateFrom, dateTo));
  if (userId) {
    params.set("user_id", userId);
  }
  return authGetJSON<TrackerTeamOverview>(`/tracker/team-activity?${params.toString()}`);
}

export async function getTrackerUserActivity(userId: string, dateFrom: string, dateTo: string) {
  return authGetJSON<TrackerActivityOverview>(`/tracker/activity/${userId}?${createDateParams(dateFrom, dateTo)}`);
}

export async function getTrackerSummary(date: string) {
  return authGetJSON<TrackerDailySummary>(`/tracker/summary?date=${date}`);
}

export async function listTrackerConsents() {
  return authGetJSON<TrackerConsentAudit[]>("/tracker/consents");
}

export async function listTrackerDomains() {
  return authGetJSON<DomainCategory[]>("/tracker/domains");
}

export async function listObservedTrackerDomains() {
  return authGetJSON<TrackerObservedDomain[]>("/tracker/domains/observed");
}

export async function downloadTrackerExtension() {
  return authDownload("/tracker/extension/download", {
    method: "GET",
  });
}

export async function createTrackerDomain(payload: DomainCategoryPayload) {
  return authPostJSON<DomainCategory, DomainCategoryPayload>("/tracker/domains", payload);
}

export async function updateTrackerDomain(domainId: string, payload: DomainCategoryPayload) {
  return authRequestJSON<DomainCategory>(`/tracker/domains/${domainId}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

export async function bulkClassifyObservedTrackerDomains(payload: BulkClassifyTrackerDomainsPayload) {
  return authRequestJSON<TrackerBulkClassifyDomainsResult>("/tracker/domains/observed/bulk-classify", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

export async function deleteTrackerDomain(domainId: string) {
  return authRequestJSON<{ message: string }>(`/tracker/domains/${domainId}`, {
    method: "DELETE",
  });
}

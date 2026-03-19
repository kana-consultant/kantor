import { ApiError, authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
import { getStoredSession } from "@/stores/auth-store";
import type {
  AdsMetric,
  AdsMetricFilters,
  AdsMetricFormValues,
  AdsMetricsListResponse,
  AdsMetricsSummary,
} from "@/types/marketing";
import type { PaginationMeta } from "@/types/project";

export const adsMetricsKeys = {
  all: ["marketing", "ads-metrics"] as const,
  list: (filters: AdsMetricFilters) => [...adsMetricsKeys.all, "list", { ...filters }] as const,
  detail: (metricId: string) => [...adsMetricsKeys.all, "detail", metricId] as const,
  summary: (groupBy: "campaign" | "platform" | "month", dateFrom: string, dateTo: string) =>
    [...adsMetricsKeys.all, "summary", groupBy, dateFrom, dateTo] as const,
};

export async function listAdsMetrics(filters: AdsMetricFilters): Promise<AdsMetricsListResponse> {
  const params = new URLSearchParams();

  params.set("page", String(filters.page));
  params.set("per_page", String(filters.perPage));
  if (filters.campaignId) {
    params.set("campaign_id", filters.campaignId);
  }
  if (filters.platform) {
    params.set("platform", filters.platform);
  }
  if (filters.dateFrom) {
    params.set("date_from", filters.dateFrom);
  }
  if (filters.dateTo) {
    params.set("date_to", filters.dateTo);
  }

  const payload = await authRequestEnvelope<AdsMetricsListResponse["items"]>(
    `/marketing/ads-metrics?${params.toString()}`,
    { method: "GET" },
  );

  return {
    items: payload.data,
    meta: (payload.meta as PaginationMeta | undefined) ?? {
      page: filters.page,
      per_page: filters.perPage,
      total: 0,
    },
  };
}

export async function createAdsMetric(input: AdsMetricFormValues) {
  return authRequestJSON<AdsMetric>(
    "/marketing/ads-metrics",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeAdsMetric(input)),
    },
  );
}

export async function batchCreateAdsMetrics(entries: AdsMetricFormValues[]) {
  return authRequestJSON<AdsMetric[]>(
    "/marketing/ads-metrics/batch",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        entries: entries.map(serializeAdsMetric),
      }),
    },
  );
}

export async function updateAdsMetric(metricId: string, input: AdsMetricFormValues) {
  return authRequestJSON<AdsMetric>(
    `/marketing/ads-metrics/${metricId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeAdsMetric(input)),
    },
  );
}

export async function deleteAdsMetric(metricId: string) {
  return authRequestJSON<{ message: string }>(
    `/marketing/ads-metrics/${metricId}`,
    { method: "DELETE" },
  );
}

export async function getAdsMetricsSummary(groupBy: "campaign" | "platform" | "month", dateFrom: string, dateTo: string) {
  const params = new URLSearchParams();
  params.set("group_by", groupBy);
  if (dateFrom) {
    params.set("date_from", dateFrom);
  }
  if (dateTo) {
    params.set("date_to", dateTo);
  }

  return authRequestJSON<AdsMetricsSummary>(
    `/marketing/ads-metrics/summary?${params.toString()}`,
    { method: "GET" },
  );
}

export async function exportAdsMetricsCSV(dateFrom: string, dateTo: string) {
  const session = getStoredSession();
  const params = new URLSearchParams({ format: "csv" });
  if (dateFrom) {
    params.set("date_from", dateFrom);
  }
  if (dateTo) {
    params.set("date_to", dateTo);
  }

  const response = await fetch(`${import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1"}/marketing/ads-metrics/export?${params.toString()}`, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${session?.tokens.access_token ?? ""}`,
    },
  });

  if (!response.ok) {
    throw new ApiError(response.status, "Failed to export ads metrics");
  }

  return response.blob();
}

function serializeAdsMetric(input: AdsMetricFormValues) {
  return {
    campaign_id: input.campaign_id.trim(),
    platform: input.platform,
    period_start: new Date(input.period_start).toISOString(),
    period_end: new Date(input.period_end).toISOString(),
    amount_spent: input.amount_spent,
    impressions: input.impressions,
    clicks: input.clicks,
    conversions: input.conversions,
    revenue: input.revenue,
    notes: input.notes.trim() || null,
  };
}

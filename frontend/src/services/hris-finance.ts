import { getJSON, postJSON, requestEnvelope, requestJSON } from "@/lib/api-client";
import { getStoredSession } from "@/stores/auth-store";
import type {
  FinanceCategory,
  FinanceCategoryFormValues,
  FinanceRecord,
  FinanceRecordFilters,
  FinanceRecordFormValues,
  FinanceSummary,
} from "@/types/hris";

export const financeKeys = {
  all: ["hris", "finance"] as const,
  categories: (type = "") => [...financeKeys.all, "categories", type] as const,
  records: (filters: FinanceRecordFilters) => [...financeKeys.all, "records", filters] as const,
  summary: (year: number) => [...financeKeys.all, "summary", year] as const,
};

export async function listFinanceCategories(type = "") {
  const session = getStoredSession();
  const query = type ? `?type=${encodeURIComponent(type)}` : "";
  return getJSON<FinanceCategory[]>(`/hris/finance/categories${query}`, session?.tokens.access_token);
}

export async function createFinanceCategory(values: FinanceCategoryFormValues) {
  const session = getStoredSession();
  return postJSON<FinanceCategory, FinanceCategoryFormValues>(
    "/hris/finance/categories",
    values,
    session?.tokens.access_token,
  );
}

export async function updateFinanceCategory(categoryId: string, values: FinanceCategoryFormValues) {
  const session = getStoredSession();
  return requestJSON<FinanceCategory>(
    `/hris/finance/categories/${categoryId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(values),
    },
    session?.tokens.access_token,
  );
}

export async function deleteFinanceCategory(categoryId: string) {
  const session = getStoredSession();
  return requestJSON<{ message: string }>(
    `/hris/finance/categories/${categoryId}`,
    { method: "DELETE" },
    session?.tokens.access_token,
  );
}

export async function listFinanceRecords(filters: FinanceRecordFilters) {
  const session = getStoredSession();
  const search = new URLSearchParams({
    page: String(filters.page),
    per_page: String(filters.perPage),
  });
  if (filters.type) search.set("type", filters.type);
  if (filters.category) search.set("category", filters.category);
  if (filters.month) search.set("month", filters.month);
  if (filters.year) search.set("year", filters.year);
  if (filters.status) search.set("status", filters.status);

  const envelope = await requestEnvelope<FinanceRecord[]>(
    `/hris/finance/records?${search.toString()}`,
    { method: "GET" },
    session?.tokens.access_token,
  );

  return {
    items: envelope.data,
    meta: envelope.meta as { page: number; per_page: number; total: number },
  };
}

export async function createFinanceRecord(values: FinanceRecordFormValues) {
  const session = getStoredSession();
  return postJSON<FinanceRecord, Record<string, unknown>>(
    "/hris/finance/records",
    {
      ...values,
      record_date: new Date(`${values.record_date}T00:00:00`).toISOString(),
    },
    session?.tokens.access_token,
  );
}

export async function updateFinanceRecord(recordId: string, values: FinanceRecordFormValues) {
  const session = getStoredSession();
  return requestJSON<FinanceRecord>(
    `/hris/finance/records/${recordId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        ...values,
        record_date: new Date(`${values.record_date}T00:00:00`).toISOString(),
      }),
    },
    session?.tokens.access_token,
  );
}

export async function deleteFinanceRecord(recordId: string) {
  const session = getStoredSession();
  return requestJSON<{ message: string }>(
    `/hris/finance/records/${recordId}`,
    { method: "DELETE" },
    session?.tokens.access_token,
  );
}

export async function submitFinanceRecord(recordId: string) {
  const session = getStoredSession();
  return requestJSON<FinanceRecord>(
    `/hris/finance/records/${recordId}/submit`,
    { method: "PATCH" },
    session?.tokens.access_token,
  );
}

export async function reviewFinanceRecord(recordId: string, decision: "approved" | "rejected") {
  const session = getStoredSession();
  return requestJSON<FinanceRecord>(
    `/hris/finance/records/${recordId}/review`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ decision }),
    },
    session?.tokens.access_token,
  );
}

export async function getFinanceSummary(year: number) {
  const session = getStoredSession();
  return getJSON<FinanceSummary>(`/hris/finance/summary?year=${year}`, session?.tokens.access_token);
}

export async function exportFinanceCSV(year: number, month?: string) {
  const session = getStoredSession();
  const query = new URLSearchParams({ year: String(year) });
  if (month) {
    query.set("month", month);
  }

  const response = await fetch(
    `${import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1"}/hris/finance/export?${query.toString()}`,
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${session?.tokens.access_token ?? ""}`,
      },
    },
  );

  if (!response.ok) {
    throw new Error("Export failed");
  }

  return response.blob();
}

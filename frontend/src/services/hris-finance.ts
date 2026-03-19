import { env } from "@/lib/env";
import { authGetJSON, authPostJSON, authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
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
  const query = type ? `?type=${encodeURIComponent(type)}` : "";
  return authGetJSON<FinanceCategory[]>(`/hris/finance/categories${query}`);
}

export async function createFinanceCategory(values: FinanceCategoryFormValues) {
  return authPostJSON<FinanceCategory, FinanceCategoryFormValues>(
    "/hris/finance/categories",
    values,
  );
}

export async function updateFinanceCategory(categoryId: string, values: FinanceCategoryFormValues) {
  return authRequestJSON<FinanceCategory>(
    `/hris/finance/categories/${categoryId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(values),
    },
  );
}

export async function deleteFinanceCategory(categoryId: string) {
  return authRequestJSON<{ message: string }>(
    `/hris/finance/categories/${categoryId}`,
    { method: "DELETE" },
  );
}

export async function listFinanceRecords(filters: FinanceRecordFilters) {
  const search = new URLSearchParams({
    page: String(filters.page),
    per_page: String(filters.perPage),
  });
  if (filters.type) search.set("type", filters.type);
  if (filters.category) search.set("category", filters.category);
  if (filters.month) search.set("month", filters.month);
  if (filters.year) search.set("year", filters.year);
  if (filters.status) search.set("status", filters.status);

  const envelope = await authRequestEnvelope<FinanceRecord[]>(
    `/hris/finance/records?${search.toString()}`,
    { method: "GET" },
  );

  return {
    items: envelope.data,
    meta: envelope.meta as { page: number; per_page: number; total: number },
  };
}

export async function createFinanceRecord(values: FinanceRecordFormValues) {
  return authPostJSON<FinanceRecord, Record<string, unknown>>(
    "/hris/finance/records",
    {
      ...values,
      record_date: new Date(`${values.record_date}T00:00:00`).toISOString(),
    },
  );
}

export async function updateFinanceRecord(recordId: string, values: FinanceRecordFormValues) {
  return authRequestJSON<FinanceRecord>(
    `/hris/finance/records/${recordId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        ...values,
        record_date: new Date(`${values.record_date}T00:00:00`).toISOString(),
      }),
    },
  );
}

export async function deleteFinanceRecord(recordId: string) {
  return authRequestJSON<{ message: string }>(
    `/hris/finance/records/${recordId}`,
    { method: "DELETE" },
  );
}

export async function submitFinanceRecord(recordId: string) {
  return authRequestJSON<FinanceRecord>(
    `/hris/finance/records/${recordId}/submit`,
    { method: "PATCH" },
  );
}

export async function reviewFinanceRecord(recordId: string, decision: "approved" | "rejected") {
  return authRequestJSON<FinanceRecord>(
    `/hris/finance/records/${recordId}/review`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ decision }),
    },
  );
}

export async function getFinanceSummary(year: number) {
  return authGetJSON<FinanceSummary>(`/hris/finance/summary?year=${year}`);
}

export async function exportFinanceCSV(year: number, month?: string) {
  const session = getStoredSession();
  const query = new URLSearchParams({ year: String(year) });
  if (month) {
    query.set("month", month);
  }

  const response = await fetch(
    `${env.VITE_API_BASE_URL}/hris/finance/export?${query.toString()}`,
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

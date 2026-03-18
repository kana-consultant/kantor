import { getJSON, postJSON, requestEnvelope, requestJSON } from "@/lib/api-client";
import { env } from "@/lib/env";
import { getStoredSession } from "@/stores/auth-store";
import type {
  Reimbursement,
  ReimbursementFilters,
  ReimbursementFormValues,
  ReimbursementSummary,
} from "@/types/hris";

export const reimbursementsKeys = {
  all: ["hris", "reimbursements"] as const,
  list: (filters: ReimbursementFilters) => [...reimbursementsKeys.all, "list", filters] as const,
  detail: (reimbursementId: string) => [...reimbursementsKeys.all, "detail", reimbursementId] as const,
  summary: (month: string, year: string) => [...reimbursementsKeys.all, "summary", month, year] as const,
};

export async function listReimbursements(filters: ReimbursementFilters) {
  const session = getStoredSession();
  const search = new URLSearchParams({
    page: String(filters.page),
    per_page: String(filters.perPage),
  });
  if (filters.status) search.set("status", filters.status);
  if (filters.employee) search.set("employee", filters.employee);
  if (filters.month) search.set("month", filters.month);
  if (filters.year) search.set("year", filters.year);

  const envelope = await requestEnvelope<Reimbursement[]>(
    `/hris/reimbursements?${search.toString()}`,
    { method: "GET" },
    session?.tokens.access_token,
  );

  return {
    items: envelope.data,
    meta: envelope.meta as { page: number; per_page: number; total: number },
  };
}

export async function getReimbursement(reimbursementId: string) {
  const session = getStoredSession();
  return getJSON<Reimbursement>(`/hris/reimbursements/${reimbursementId}`, session?.tokens.access_token);
}

export async function createReimbursement(values: ReimbursementFormValues) {
  const session = getStoredSession();
  return postJSON<Reimbursement, Record<string, unknown>>(
    "/hris/reimbursements",
    {
      ...values,
      transaction_date: new Date(`${values.transaction_date}T00:00:00`).toISOString(),
    },
    session?.tokens.access_token,
  );
}

export async function uploadReimbursementAttachments(reimbursementId: string, files: File[]) {
  const session = getStoredSession();
  const payload = new FormData();
  files.forEach((file) => payload.append("files", file));
  const response = await fetch(
    `${env.VITE_API_BASE_URL}/hris/reimbursements/${reimbursementId}/attachments`,
    {
      method: "POST",
      headers: {
        Authorization: `Bearer ${session?.tokens.access_token ?? ""}`,
      },
      body: payload,
    },
  );
  const json = await response.json();
  if (!response.ok || !json.success) {
    throw new Error(json.error?.message ?? "Attachment upload failed");
  }
  return json.data as Reimbursement;
}

export async function reviewReimbursement(reimbursementId: string, decision: "approved" | "rejected", notes = "") {
  const session = getStoredSession();
  return requestJSON<Reimbursement>(
    `/hris/reimbursements/${reimbursementId}/review`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ decision, notes }),
    },
    session?.tokens.access_token,
  );
}

export async function markReimbursementPaid(reimbursementId: string, notes = "") {
  const session = getStoredSession();
  return requestJSON<Reimbursement>(
    `/hris/reimbursements/${reimbursementId}/mark-paid`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ notes }),
    },
    session?.tokens.access_token,
  );
}

export async function getReimbursementSummary(month: string, year: string) {
  const session = getStoredSession();
  const search = new URLSearchParams();
  if (month) search.set("month", month);
  if (year) search.set("year", year);
  return getJSON<ReimbursementSummary>(`/hris/reimbursements/summary?${search.toString()}`, session?.tokens.access_token);
}

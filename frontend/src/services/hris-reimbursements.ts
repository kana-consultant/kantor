import { authGetJSON, authPostJSON, authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
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
  const search = new URLSearchParams({
    page: String(filters.page),
    per_page: String(filters.perPage),
  });
  if (filters.status) search.set("status", filters.status);
  if (filters.employee) search.set("employee", filters.employee);
  if (filters.month) search.set("month", filters.month);
  if (filters.year) search.set("year", filters.year);

  const envelope = await authRequestEnvelope<Reimbursement[]>(
    `/hris/reimbursements?${search.toString()}`,
    { method: "GET" },
  );

  return {
    items: envelope.data,
    meta: envelope.meta as { page: number; per_page: number; total: number },
  };
}

export async function getReimbursement(reimbursementId: string) {
  return authGetJSON<Reimbursement>(`/hris/reimbursements/${reimbursementId}`);
}

export async function createReimbursement(values: ReimbursementFormValues) {
  return authPostJSON<Reimbursement, Record<string, unknown>>(
    "/hris/reimbursements",
    {
      ...values,
      transaction_date: new Date(`${values.transaction_date}T00:00:00`).toISOString(),
    },
  );
}

export async function uploadReimbursementAttachments(reimbursementId: string, files: File[]) {
  const payload = new FormData();
  files.forEach((file) => payload.append("files", file));
  return authRequestJSON<Reimbursement>(
    `/hris/reimbursements/${reimbursementId}/attachments`,
    {
      method: "POST",
      body: payload,
    },
  );
}

export async function reviewReimbursement(reimbursementId: string, decision: "approved" | "rejected", notes = "") {
  return authRequestJSON<Reimbursement>(
    `/hris/reimbursements/${reimbursementId}/review`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ decision, notes }),
    },
  );
}

export async function markReimbursementPaid(reimbursementId: string, notes = "") {
  return authRequestJSON<Reimbursement>(
    `/hris/reimbursements/${reimbursementId}/mark-paid`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ notes }),
    },
  );
}

export async function updateReimbursement(
  reimbursementId: string,
  values: Omit<ReimbursementFormValues, "employee_id">,
  keptAttachments?: string[],
) {
  return authRequestJSON<Reimbursement>(
    `/hris/reimbursements/${reimbursementId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        ...values,
        transaction_date: new Date(`${values.transaction_date}T00:00:00`).toISOString(),
        kept_attachments: keptAttachments ?? null,
      }),
    },
  );
}

export async function deleteReimbursement(reimbursementId: string) {
  return authRequestJSON<{ deleted: boolean }>(
    `/hris/reimbursements/${reimbursementId}`,
    { method: "DELETE" },
  );
}

export async function getReimbursementSummary(month: string, year: string) {
  const search = new URLSearchParams();
  if (month) search.set("month", month);
  if (year) search.set("year", year);
  return authGetJSON<ReimbursementSummary>(`/hris/reimbursements/summary?${search.toString()}`);
}

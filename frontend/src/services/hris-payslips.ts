import { authDownload, authGetJSON, authPostJSON, authRequestJSON } from "@/lib/api-client";
import type {
  GeneratePayslipsPayload,
  GeneratePayslipsResponse,
  PayslipBatchResponse,
  PayslipDetail,
  PayslipHistoryItem,
  PayslipListResponse,
  PayslipRecipient,
  PayslipRecipientChoice,
  PayslipSendPreviewResponse,
  PayslipSendResponse,
  UpdatePayslipPayload,
  VoidReissuePayslipResponse,
} from "@/types/documents";

// Every read of these endpoints writes a salary access row on the server, so
// the queries using these keys do not refetch on window focus.
export const payslipsKeys = {
  all: ["hris", "payslips"] as const,
  period: (year: number, month: number) => [...payslipsKeys.all, "period", year, month] as const,
  detail: (payslipId: string) => [...payslipsKeys.all, "detail", payslipId] as const,
  employee: (employeeId: string) => [...payslipsKeys.all, "employee", employeeId] as const,
  recipient: (payslipId: string, source: PayslipRecipientChoice) =>
    [...payslipsKeys.all, "recipient", payslipId, source] as const,
};

export function listPayslips(year: number, month: number) {
  return authGetJSON<PayslipListResponse>(`/hris/payslips?year=${year}&month=${month}`);
}

export function listEmployeePayslips(employeeId: string, limit = 6) {
  return authGetJSON<PayslipHistoryItem[]>(`/hris/payslips/employee/${employeeId}?limit=${limit}`);
}

export function getPayslip(payslipId: string) {
  return authGetJSON<PayslipDetail>(`/hris/payslips/${payslipId}`);
}

/** Builds (or rebuilds) draft slips; PDFs render in the background. */
export function generatePayslips(payload: GeneratePayslipsPayload) {
  return authPostJSON<GeneratePayslipsResponse, GeneratePayslipsPayload>("/hris/payslips/generate", payload);
}

/** Replaces the note and manual lines of a draft and re-renders it. */
export function updatePayslip(payslipId: string, payload: UpdatePayslipPayload) {
  return authRequestJSON<PayslipDetail>(`/hris/payslips/${payslipId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

/** Fetches the slip PDF. The caller owns (and must revoke) any object URL made from it. */
export function downloadPayslipPDF(payslipId: string, disposition: "inline" | "attachment") {
  return authDownload(`/hris/payslips/${payslipId}/pdf?disposition=${disposition}`, { method: "GET" });
}

export function downloadPayslipDocx(payslipId: string) {
  return authDownload(`/hris/payslips/${payslipId}/docx`, { method: "GET" });
}

export function getPayslipRecipient(payslipId: string, source: PayslipRecipientChoice) {
  return authGetJSON<PayslipRecipient>(`/hris/payslips/${payslipId}/recipient?source=${source}`);
}

export function previewPayslipSend(ids: string[], includeAlreadySent: boolean) {
  return authPostJSON<PayslipSendPreviewResponse, { ids: string[]; include_already_sent: boolean }>(
    "/hris/payslips/send-preview",
    { ids, include_already_sent: includeAlreadySent },
  );
}

/** Sends one slip synchronously (Kirim / Kirim ulang). */
export function sendPayslip(payslipId: string, source: PayslipRecipientChoice) {
  const body = source === "default" ? {} : { recipient_source: source };
  return authPostJSON<PayslipSendResponse, typeof body>(`/hris/payslips/${payslipId}/send`, body);
}

/** Queues a background batch send; slips already sent are skipped unless includeAlreadySent. */
export function sendPayslipBatch(ids: string[], includeAlreadySent: boolean) {
  return authPostJSON<PayslipBatchResponse, { ids: string[]; include_already_sent: boolean }>(
    "/hris/payslips/send-batch",
    { ids, include_already_sent: includeAlreadySent },
  );
}

export function voidReissuePayslip(payslipId: string, reason: string) {
  return authPostJSON<VoidReissuePayslipResponse, { reason: string }>(
    `/hris/payslips/${payslipId}/void-reissue`,
    { reason },
  );
}

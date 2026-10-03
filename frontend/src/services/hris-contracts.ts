import { authDownload, authGetJSON, authPostJSON, authRequestJSON } from "@/lib/api-client";
import type {
  ContractDetail,
  ContractFieldsPayload,
  ContractListFilters,
  ContractListResponse,
  ContractPart,
  ContractPreflight,
  ContractRecipient,
  ContractRecipientChoice,
  ContractSendResponse,
  CreateContractPayload,
  SendContractPayload,
  UpdateContractStatusPayload,
} from "@/types/contracts";

// Detail reads with hris:salary:view and preflight reads (identity) are
// access-logged on the server, so the queries using these keys do not
// refetch on window focus.
export const contractsKeys = {
  all: ["hris", "contracts"] as const,
  list: (filters: ContractListFilters) => [...contractsKeys.all, "list", { ...filters }] as const,
  detail: (contractId: string) => [...contractsKeys.all, "detail", contractId] as const,
  preflight: (contractId: string) => [...contractsKeys.all, "preflight", contractId] as const,
  recipient: (contractId: string, source: ContractRecipientChoice) =>
    [...contractsKeys.all, "recipient", contractId, source] as const,
};

export function listContracts(filters: ContractListFilters = {}) {
  const params = new URLSearchParams();
  if (filters.employee_id) {
    params.set("employee_id", filters.employee_id);
  }
  if (filters.status) {
    params.set("status", filters.status);
  }
  if (filters.type) {
    params.set("type", filters.type);
  }
  if (filters.search?.trim()) {
    params.set("search", filters.search.trim());
  }
  if (filters.limit) {
    params.set("limit", String(filters.limit));
  }
  const query = params.toString();
  return authGetJSON<ContractListResponse>(`/hris/contracts${query ? `?${query}` : ""}`);
}

export function getContract(contractId: string) {
  return authGetJSON<ContractDetail>(`/hris/contracts/${contractId}`);
}

/** Creates a draft (or a record-only entry). */
export function createContract(payload: CreateContractPayload) {
  return authPostJSON<ContractDetail, CreateContractPayload>("/hris/contracts", payload);
}

/**
 * Replaces the terms (Edit / Revisi). A sent, unsigned contract becomes a
 * draft with revision+1 and keeps its numbers.
 */
export function updateContract(contractId: string, payload: ContractFieldsPayload) {
  return authRequestJSON<ContractDetail>(`/hris/contracts/${contractId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function getContractPreflight(contractId: string) {
  return authGetJSON<ContractPreflight>(`/hris/contracts/${contractId}/preflight`);
}

/** Assigns the numbers (first time) and queues the PKWT + NDA render. */
export function generateContract(contractId: string) {
  return authPostJSON<ContractDetail, Record<string, never>>(`/hris/contracts/${contractId}/generate`, {});
}

/** Perpanjang: a linked draft starting the day after the end, same terms. */
export function renewContract(contractId: string) {
  return authPostJSON<ContractDetail, Record<string, never>>(`/hris/contracts/${contractId}/renew`, {});
}

export function updateContractStatus(contractId: string, payload: UpdateContractStatusPayload) {
  return authRequestJSON<ContractDetail>(`/hris/contracts/${contractId}/status`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

/** Fetches one PDF. The caller owns (and must revoke) any object URL made from it. */
export function downloadContractPDF(contractId: string, part: ContractPart, disposition: "inline" | "attachment") {
  return authDownload(`/hris/contracts/${contractId}/files/${part}/pdf?disposition=${disposition}`, { method: "GET" });
}

/** Re-renders one DOCX from the stored snapshot (HR only). */
export function downloadContractDocx(contractId: string, part: ContractPart) {
  return authDownload(`/hris/contracts/${contractId}/files/${part}/docx`, { method: "GET" });
}

export function getContractRecipient(contractId: string, source: ContractRecipientChoice) {
  return authGetJSON<ContractRecipient>(`/hris/contracts/${contractId}/recipient?source=${source}`);
}

/** E-mails the PKWT and the NDA together (synchronous). */
export function sendContract(contractId: string, payload: SendContractPayload) {
  return authPostJSON<ContractSendResponse, SendContractPayload>(`/hris/contracts/${contractId}/send`, payload);
}

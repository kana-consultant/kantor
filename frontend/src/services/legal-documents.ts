import { authDownload, authGetJSON, authPostJSON, authRequestJSON } from "@/lib/api-client";
import type {
  LegalDocSpec,
  LegalDocType,
  LegalDocumentDetail,
  LegalDocumentListItem,
  LegalDocumentPayload,
  LegalParty,
  LegalPartyPayload,
  LegalSendResponse,
} from "@/types/legal";

export interface LegalDocumentFilters {
  type?: LegalDocType | "";
  status?: string;
  party_id?: string;
  search?: string;
}

// Detail reads decrypt the party's numbers for masking only; still, the
// queries do not refetch on window focus (like contracts).
export const legalKeys = {
  all: ["operational", "legal"] as const,
  types: () => [...legalKeys.all, "types"] as const,
  documents: (filters: LegalDocumentFilters) => [...legalKeys.all, "documents", { ...filters }] as const,
  document: (documentId: string) => [...legalKeys.all, "document", documentId] as const,
  parties: (search: string) => [...legalKeys.all, "parties", search] as const,
};

export function listLegalDocTypes() {
  return authGetJSON<LegalDocSpec[]>("/operational/legal-documents/types");
}

export function listLegalDocuments(filters: LegalDocumentFilters = {}) {
  const params = new URLSearchParams();
  if (filters.type) params.set("type", filters.type);
  if (filters.status) params.set("status", filters.status);
  if (filters.party_id) params.set("party_id", filters.party_id);
  if (filters.search?.trim()) params.set("search", filters.search.trim());
  const query = params.toString();
  return authGetJSON<LegalDocumentListItem[]>(`/operational/legal-documents${query ? `?${query}` : ""}`);
}

export function getLegalDocument(documentId: string) {
  return authGetJSON<LegalDocumentDetail>(`/operational/legal-documents/${documentId}`);
}

export function createLegalDocument(payload: LegalDocumentPayload) {
  return authPostJSON<LegalDocumentDetail, LegalDocumentPayload>("/operational/legal-documents", payload);
}

/** Saves the form; an issued document goes back to draft (its number is kept). */
export function updateLegalDocument(documentId: string, payload: LegalDocumentPayload) {
  return authRequestJSON<LegalDocumentDetail>(`/operational/legal-documents/${documentId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function deleteLegalDocument(documentId: string) {
  return authRequestJSON<{ deleted: boolean }>(`/operational/legal-documents/${documentId}`, { method: "DELETE" });
}

export function generateLegalDocument(documentId: string) {
  return authPostJSON<LegalDocumentDetail, Record<string, never>>(`/operational/legal-documents/${documentId}/generate`, {});
}

export function cancelLegalDocument(documentId: string) {
  return authPostJSON<LegalDocumentDetail, Record<string, never>>(`/operational/legal-documents/${documentId}/cancel`, {});
}

/** Marks the document signed; the scanned signed PDF is optional. */
export function markLegalDocumentSigned(documentId: string, signedAt: string, file?: File | null) {
  const formData = new FormData();
  formData.set("signed_at", signedAt);
  if (file) {
    formData.set("file", file);
  }
  return authRequestJSON<LegalDocumentDetail>(`/operational/legal-documents/${documentId}/signed`, {
    method: "POST",
    body: formData,
  });
}

export function sendLegalDocument(documentId: string, recipient: string) {
  return authPostJSON<LegalSendResponse, { recipient: string }>(`/operational/legal-documents/${documentId}/send`, { recipient });
}

export function downloadLegalPDF(documentId: string, disposition: "inline" | "attachment") {
  return authDownload(`/operational/legal-documents/${documentId}/pdf?disposition=${disposition}`, { method: "GET" });
}

export function downloadLegalDocx(documentId: string) {
  return authDownload(`/operational/legal-documents/${documentId}/docx`, { method: "GET" });
}

export function downloadLegalSignedCopy(documentId: string) {
  return authDownload(`/operational/legal-documents/${documentId}/signed-copy`, { method: "GET" });
}

export function listLegalParties(search = "") {
  const query = search.trim() ? `?search=${encodeURIComponent(search.trim())}` : "";
  return authGetJSON<LegalParty[]>(`/operational/legal-parties${query}`);
}

export function createLegalParty(payload: LegalPartyPayload) {
  return authPostJSON<LegalParty, LegalPartyPayload>("/operational/legal-parties", payload);
}

export function updateLegalParty(partyId: string, payload: LegalPartyPayload) {
  return authRequestJSON<LegalParty>(`/operational/legal-parties/${partyId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function deleteLegalParty(partyId: string) {
  return authRequestJSON<{ deleted: boolean }>(`/operational/legal-parties/${partyId}`, { method: "DELETE" });
}

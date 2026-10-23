import type { EmailDelivery } from "@/types/documents";

export type LegalDocType = "freelance" | "subcontract" | "sow" | "sla" | "bast";
export type LegalDocStatus = "draft" | "generated" | "sent" | "signed" | "cancelled";
export type LegalPartyType = "person" | "company";
export type LegalRenderStatus = "none" | "pending" | "rendering" | "ready" | "failed";

export type LegalFieldType =
  | "text"
  | "textarea"
  | "date"
  | "money"
  | "number"
  | "percent"
  | "email"
  | "select"
  | "bool"
  | "list";

export interface LegalOption {
  value: string;
  label: string;
}

export interface LegalColumn {
  key: string;
  label: string;
  type: LegalFieldType;
  required?: boolean;
  options?: LegalOption[];
  default?: unknown;
}

export interface LegalField {
  key: string;
  label: string;
  type: LegalFieldType;
  section: string;
  required?: boolean;
  default?: unknown;
  placeholder?: string;
  help?: string;
  options?: LegalOption[];
  min?: number;
  max?: number;
  columns?: LegalColumn[];
  show_when?: { field: string; equals: unknown };
}

export interface LegalDocSpec {
  type: LegalDocType;
  code: string;
  label: string;
  short_label: string;
  description: string;
  party_label: string;
  party_types: LegalPartyType[];
  related_types: LegalDocType[] | null;
  fields: LegalField[];
}

export type LegalFieldValues = Record<string, unknown>;

export interface LegalParty {
  id: string;
  party_type: LegalPartyType;
  name: string;
  address: string;
  jurisdiction: string;
  id_number_masked: string;
  tax_id_masked: string;
  representative_name: string;
  representative_title: string;
  contact_name: string;
  email: string;
  phone: string;
  bank_name: string;
  bank_account_masked: string;
  bank_account_holder: string;
  bank_swift: string;
  notes: string;
  documents: number;
  created_at: string;
  updated_at: string;
}

export interface LegalPartyPayload {
  party_type: LegalPartyType;
  name: string;
  address: string;
  jurisdiction: string;
  /** Omitted (undefined) keeps the stored value on an edit; "" clears it. */
  id_number?: string;
  tax_id?: string;
  representative_name: string;
  representative_title: string;
  contact_name: string;
  email: string;
  phone: string;
  bank_name: string;
  bank_account_number?: string;
  bank_account_holder: string;
  bank_swift: string;
  notes: string;
}

export interface LegalDocumentListItem {
  id: string;
  doc_type: LegalDocType;
  status: LegalDocStatus;
  title: string;
  party_id: string;
  party_name: string;
  doc_number: string | null;
  document_date: string | null;
  render_status: LegalRenderStatus;
  last_sent_at: string | null;
  signed_at: string | null;
  updated_at: string;
}

export interface LegalDocumentDetail extends LegalDocumentListItem {
  fields: LegalFieldValues;
  document_city: string | null;
  party: LegalParty;
  related: { id: string; doc_type: LegalDocType; title: string; doc_number: string | null; status: LegalDocStatus } | null;
  render_error: string | null;
  pdf_ready: boolean;
  has_signed_copy: boolean;
  generated_at: string | null;
  created_at: string;
  missing: Record<string, string>;
  company_missing: string[] | null;
  deliveries: EmailDelivery[] | null;
  pdf_available: boolean;
  mail_ready: boolean;
  send_in_flight: boolean;
  default_recipient: string;
}

export interface LegalDocumentPayload {
  doc_type?: LegalDocType;
  title: string;
  party_id: string;
  related_document_id?: string | null;
  fields: LegalFieldValues;
  document_date?: string | null;
  document_city?: string | null;
}

export interface LegalSendResponse {
  sent: boolean;
  error_category?: string;
  error_message?: string;
  delivery: EmailDelivery;
  document: LegalDocumentDetail;
}

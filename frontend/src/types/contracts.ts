// Kontrak Kerja (PKWT + NDA/HKI). Mirrors backend/internal/dto/hris/contract.go
// and backend/internal/model/employment_contract.go.

import type { DocumentRenderStatus, EmailDelivery, EmailDeliveryStatus, EmailRecipientSource } from "@/types/documents";

export type ContractType = "PKWT" | "PKWTT" | "MAGANG";
export type ContractStatus = "draft" | "generated" | "sent" | "signed" | "ended" | "cancelled";
export type ContractWorkMode = "wfo" | "hybrid" | "remote";
export type ContractPart = "pkwt" | "nda";
/** Recipient choice for a send; "default" = login e-mail, or the employee e-mail when unlinked. */
export type ContractRecipientChoice = "default" | "login" | "employee" | "personal";

export interface ContractBenefit {
  name: string;
  value: string;
  notes: string;
}

export interface ContractPriorWork {
  title: string;
  description: string;
  year: string;
}

export interface ContractCompensation {
  base_salary: number;
  fixed_allowance: number;
}

export interface ContractCompanyInfo {
  legal_name: string;
  city: string;
  doc_code: string;
  hr_contact_email: string;
  /** CC addresses must be on this domain ("" = no CC allowed). */
  cc_domain: string;
  payday_day: number;
  annual_leave_days: number;
}

export interface ContractDeliverySummary {
  id: string;
  status: EmailDeliveryStatus;
  recipient: string;
  recipient_source: EmailRecipientSource;
  cc: string[];
  attachment_sha256: string[];
  error?: string;
  created_at: string;
  sent_at?: string;
}

/**
 * One row of the contract list. notice_deadline = end_date - notice_days;
 * notice_alert from notice_days + 14 days before end_date ('Batas
 * pemberitahuan'); expired once end_date has passed for a contract that is
 * not ended or cancelled.
 */
export interface ContractListItem {
  id: string;
  employee_id: string;
  employee_name: string;
  employee_department: string | null;
  contract_type: ContractType;
  is_record_only: boolean;
  status: ContractStatus;
  revision: number;
  start_date: string;
  end_date: string | null;
  job_title: string;
  doc_number: string | null;
  nda_doc_number: string | null;
  render_status: DocumentRenderStatus;
  notice_days: number;
  notice_deadline: string | null;
  notice_alert: boolean;
  expired: boolean;
  previous_contract_id: string | null;
  signed_at: string | null;
  ended_at: string | null;
  last_sent_at: string | null;
  /** Newest delivery of any status (status pill). */
  last_delivery: ContractDeliverySummary | null;
  /** Newest successful delivery (where the documents were last sent). */
  last_sent_delivery: ContractDeliverySummary | null;
  created_at: string;
  updated_at: string;
}

export interface ContractListResponse {
  items: ContractListItem[];
  pdf_available: boolean;
  document_mail_ready: boolean;
}

export interface ContractListFilters {
  employee_id?: string;
  status?: ContractStatus | "";
  type?: ContractType | "";
  search?: string;
  limit?: number;
}

/** One contract with every term; compensation only with hris:salary:view. */
export interface ContractDetail extends ContractListItem {
  department: string | null;
  supervisor_name: string | null;
  work_location: string;
  work_mode: ContractWorkMode;
  work_mode_detail: string | null;
  pkwt_basis: string | null;
  job_description: string;
  work_days: string;
  work_hours: string;
  weekly_hours: number;
  compensation_visible: boolean;
  has_compensation: boolean;
  compensation?: ContractCompensation;
  benefits: ContractBenefit[] | null;
  incident_report_hours: number;
  non_solicit_months: number;
  confidentiality_years: number;
  prior_works: ContractPriorWork[] | null;
  document_date: string | null;
  document_city: string | null;
  seq_no: number | null;
  template_version: string | null;
  render_error: string | null;
  has_pdf: boolean;
  pdf_sha256: string[];
  generated_at: string | null;
  generated_by: string | null;
  end_notes: string | null;
  previous_doc_number: string | null;
  previous_end_date: string | null;
  renewed_by_id: string | null;
  employee_status: string;
  pdf_available: boolean;
  document_mail_ready: boolean;
  allow_docx_send: boolean;
  company: ContractCompanyInfo;
}

/** The terms of the contract form (create and update share them). */
export interface ContractFieldsPayload {
  contract_type: ContractType;
  is_record_only: boolean;
  start_date: string;
  end_date: string | null;
  job_title: string;
  department: string | null;
  supervisor_name: string | null;
  work_location: string;
  work_mode: ContractWorkMode;
  work_mode_detail: string | null;
  pkwt_basis: string | null;
  job_description: string;
  work_days: string | null;
  work_hours: string | null;
  weekly_hours: number;
  notice_days: number;
  /** Needs hris:salary:view; omitted = prefilled from the salary for a caller with salary view (create) or kept (update). */
  compensation?: ContractCompensation;
  benefits: ContractBenefit[];
  incident_report_hours: number;
  non_solicit_months: number;
  confidentiality_years: number;
  prior_works: ContractPriorWork[];
  /** Omitted keeps the stored date; "" clears it while no number is assigned (fixed once numbered). */
  document_date?: string;
  /** Omitted or null keeps the stored city; "" clears it (the company city is used). */
  document_city?: string | null;
}

export interface CreateContractPayload extends ContractFieldsPayload {
  employee_id: string;
}

export interface ContractMissingField {
  scope: "company" | "identity" | "employee" | "contract" | string;
  field: string;
  label: string;
}

export interface ContractWarning {
  code: "employee_probation" | "pkwt_chain_over_5_years" | "duration_not_whole_months" | string;
  message: string;
  action?: "set_employee_active" | string;
}

export interface ContractPreflight {
  contract_id: string;
  /** False for record-only entries (nothing to generate). */
  documents: boolean;
  ready: boolean;
  missing: ContractMissingField[];
  warnings: ContractWarning[];
  chain_pkwt_months: number;
  employee_status: string;
  company: ContractCompanyInfo;
}

export interface ContractRecipient {
  contract_id: string;
  employee_id: string;
  employee_name: string;
  doc_number: string;
  recipient: string;
  recipient_source: EmailRecipientSource;
  linked: boolean;
  personal_available: boolean;
  has_previous: boolean;
  previous_recipient?: string;
  /** 'alamat baru': differs from the last successful delivery to the employee. */
  is_new: boolean;
  cc_domain: string;
}

export interface SendContractPayload {
  recipient_source?: Exclude<ContractRecipientChoice, "default">;
  cc?: string[];
}

export interface ContractSendResponse {
  /** Present only when the caller also holds hris:contract:view. */
  contract?: ContractDetail;
  delivery: EmailDelivery;
  sent: boolean;
  error_category?: string;
  error_message?: string;
}

export interface UpdateContractStatusPayload {
  status: "signed" | "ended" | "cancelled";
  signed_at?: string;
  ended_at?: string;
  end_notes?: string;
}

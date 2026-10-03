// Generated HR documents (Slip Gaji now, Kontrak Kerja later) and their
// e-mail deliveries. Mirrors backend/internal/dto/hris/payslip.go and
// backend/internal/model/{payslip,email_delivery}.go.

export type PayslipStatus = "none" | "draft" | "sent" | "void";
export type DocumentRenderStatus = "none" | "pending" | "rendering" | "ready" | "failed";
export type EmailDeliveryStatus = "queued" | "sending" | "sent" | "failed";
export type EmailRecipientSource = "login" | "employee" | "personal" | "self";
/** Recipient choice for a single send; "default" = login e-mail, or the employee e-mail when unlinked. */
export type PayslipRecipientChoice = "default" | "login" | "employee" | "personal";

export interface PayslipWarning {
  code: string;
  message: string;
  blocking: boolean;
}

export interface PayslipTotals {
  total_gaji: number;
  total_potongan: number;
  total_reimbursement: number;
  total_diterima: number;
}

export interface PayslipLine {
  kind: "base" | "allowance" | "bonus" | "deduction" | "manual" | string;
  label: string;
  keterangan: string;
  amount: number;
  source_id?: string;
}

export interface PayslipReimbursementLine {
  id: string;
  transaction_date: string;
  title: string;
  category: string;
  amount: number;
}

/** A positive amount is an earning, a negative one a deduction. */
export interface PayslipManualLine {
  label: string;
  amount: number;
  keterangan: string;
}

export interface PayslipDeliverySummary {
  id: string;
  status: EmailDeliveryStatus;
  recipient: string;
  recipient_source: EmailRecipientSource;
  error?: string;
  attempts: number;
  created_at: string;
  sent_at?: string;
}

export interface PayslipCompanyInfo {
  legal_name: string;
  doc_code: string;
  payday_day: number;
  hr_contact_email: string;
  has_logo: boolean;
  missing_fields: string[];
}

/** One row of the period table: a stored slip, or a live preview (status "none"). */
export interface PayslipListItem {
  employee_id: string;
  employee_name: string;
  department: string | null;
  employment_type: string;
  employment_status: string;
  date_joined: string;
  job_title: string | null;
  employee_code: string | null;
  payslip_id: string | null;
  doc_number: string | null;
  status: PayslipStatus;
  revision: number;
  render_status: DocumentRenderStatus;
  render_error: string | null;
  pay_date: string | null;
  totals: PayslipTotals;
  warnings: PayslipWarning[];
  blocked: boolean;
  preview: boolean;
  has_pdf: boolean;
  generated_at: string | null;
  last_sent_at: string | null;
  last_delivery: PayslipDeliverySummary | null;
}

export interface PayslipSummary {
  total: number;
  not_generated: number;
  draft: number;
  sent: number;
  failed: number;
  blocked: number;
  in_progress: number;
}

export interface PayslipListResponse {
  year: number;
  month: number;
  period_label: string;
  default_pay_date: string;
  pdf_available: boolean;
  document_mail_ready: boolean;
  allow_docx_send: boolean;
  company: PayslipCompanyInfo;
  summary: PayslipSummary;
  items: PayslipListItem[];
}

export interface PayslipHeader {
  employee_name: string;
  employee_code: string;
  job_title: string;
  department: string;
  status_kerja: string;
  date_joined: string;
  email: string;
  bank: string;
  account_masked: string;
  period: string;
  pay_date_label: string;
  total_in_words: string;
  company_name: string;
  hr_contact_email: string;
}

export interface PayslipDetail {
  id: string;
  employee_id: string;
  period_year: number;
  period_month: number;
  revision: number;
  doc_number: string;
  status: Exclude<PayslipStatus, "none">;
  pay_date: string;
  note: string;
  header: PayslipHeader;
  earnings: PayslipLine[];
  deductions: PayslipLine[];
  reimbursements: PayslipReimbursementLine[];
  manual_lines: PayslipManualLine[];
  totals: PayslipTotals;
  warnings: PayslipWarning[];
  render_status: DocumentRenderStatus;
  render_error: string | null;
  has_pdf: boolean;
  pdf_sha256: string | null;
  template_version: string | null;
  replaces_payslip_id: string | null;
  replaces_doc_number: string | null;
  generated_at: string;
  generated_by: string | null;
  last_sent_at: string | null;
  voided_at: string | null;
  void_reason: string | null;
  last_delivery: PayslipDeliverySummary | null;
  updated_at: string;
}

export interface PayslipHistoryItem {
  id: string;
  period_year: number;
  period_month: number;
  period_label: string;
  doc_number: string;
  revision: number;
  status: Exclude<PayslipStatus, "none">;
  render_status: DocumentRenderStatus;
  total_diterima: number;
  generated_at: string;
  last_sent_at: string | null;
  last_delivery: PayslipDeliverySummary | null;
}

export interface PayslipSkipped {
  employee_id?: string;
  employee_name?: string;
  payslip_id?: string;
  doc_number?: string;
  code: string;
  reason: string;
}

export interface GeneratePayslipsPayload {
  year: number;
  month: number;
  employee_ids: string[];
  pay_date?: string;
}

export interface GeneratePayslipsResponse {
  year: number;
  month: number;
  pay_date: string;
  generated: PayslipListItem[];
  skipped: PayslipSkipped[];
}

export interface UpdatePayslipPayload {
  note: string;
  manual_lines: PayslipManualLine[];
}

export interface PayslipRecipient {
  payslip_id: string;
  employee_id: string;
  employee_name: string;
  doc_number: string;
  status: Exclude<PayslipStatus, "none">;
  recipient: string;
  recipient_source: EmailRecipientSource;
  linked: boolean;
  personal_available: boolean;
  has_previous: boolean;
  previous_recipient?: string;
  /** 'alamat baru': differs from the last successful delivery to the employee. */
  is_new: boolean;
}

export interface PayslipSendPreviewResponse {
  recipients: PayslipRecipient[];
  skipped: PayslipSkipped[];
  already_sent_skipped: number;
  document_mail_ready: boolean;
}

export interface EmailDelivery {
  id: string;
  kind: "payslip" | "contract" | "test";
  reference_type?: string;
  reference_id?: string;
  recipient: string;
  recipient_source: EmailRecipientSource;
  cc: string[] | null;
  subject: string;
  attachment_names: string[] | null;
  attachment_sha256: string[] | null;
  status: EmailDeliveryStatus;
  error?: string;
  attempts: number;
  batch_id?: string;
  requested_by?: string;
  created_at: string;
  updated_at: string;
  sent_at?: string;
}

export interface PayslipSendResponse {
  payslip: PayslipDetail;
  delivery: EmailDelivery;
  sent: boolean;
  error_category?: string;
  error_message?: string;
}

export interface PayslipBatchQueued {
  payslip_id: string;
  doc_number: string;
  employee_name: string;
  delivery_id: string;
  recipient: string;
  recipient_source: EmailRecipientSource;
  is_new: boolean;
}

export interface PayslipBatchResponse {
  batch_id: string;
  queued: PayslipBatchQueued[];
  skipped: PayslipSkipped[];
  already_sent_skipped: number;
}

export interface VoidReissuePayslipResponse {
  voided: PayslipDetail;
  reissue: PayslipDetail;
}

export type DocumentReferenceType = "payslip" | "contract";

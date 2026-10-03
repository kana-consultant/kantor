import type { PaginationMeta } from "@/types/hris";
import type { AuthModuleRole, AuthUser } from "@/types/auth";

export interface AdminRoleSummary {
  id: string;
  name: string;
  slug: string;
  description: string;
  is_system: boolean;
  is_active: boolean;
  hierarchy_level: number;
  permissions_count: number;
  users_count: number;
}

export interface AdminRoleDetail extends AdminRoleSummary {
  permission_ids: string[];
}

export interface PermissionItem {
  id: string;
  resource: string;
  action: string;
  description: string;
  is_sensitive: boolean;
}

export interface PermissionModuleGroup {
  id: string;
  name: string;
  description: string;
  permissions: PermissionItem[];
}

export interface ModuleItem {
  id: string;
  name: string;
  description: string;
  display_order: number;
}

export interface AdminUserSummary {
  user: AuthUser;
  module_roles: Record<string, AuthModuleRole>;
  is_super_admin: boolean;
  has_employee_profile: boolean;
  employee_id?: string | null;
}

export interface AdminUserDetail extends AdminUserSummary {
  effective_permissions: string[];
}

export interface AdminUserFilters {
  page: number;
  perPage: number;
  search: string;
  moduleId?: string;
  roleId?: string;
  superAdmin?: boolean | null;
}

export interface ListAdminUsersResponse {
  items: AdminUserSummary[];
  meta: PaginationMeta;
}

export type AdminUser = AdminUserDetail;
export type ListUsersResponse = ListAdminUsersResponse;

export interface ListRolesFilters {
  search: string;
  isSystem?: boolean | null;
  isActive?: boolean | null;
}

export interface UpsertRolePayload {
  name: string;
  slug: string;
  description: string;
  hierarchy_level: number;
  permission_ids: string[];
}

export interface SetUserModuleRolePayload {
  module_id: string;
  role_id: string | null;
}

export type RoleKeyDTO = SetUserModuleRolePayload;

export interface RoleReference {
  role_id: string | null;
  role_name: string | null;
  role_slug: string | null;
}

export interface AutoCreateEmployeeSetting {
  enabled: boolean;
  default_department_id: string | null;
}

export interface MailDeliverySetting {
  enabled: boolean;
  provider: string;
  sender_name: string;
  sender_email: string;
  reply_to_email: string | null;
  has_api_key: boolean;
  password_reset_enabled: boolean;
  password_reset_expiry_minutes: number;
  notification_enabled: boolean;
}

export interface ReminderChannelsSetting {
  in_app: boolean;
  email: boolean;
  whatsapp: boolean;
}

export interface ReimbursementReminderRuleSetting {
  enabled: boolean;
  cron: string;
  channels: ReminderChannelsSetting;
}

export interface ReimbursementReminderSetting {
  enabled: boolean;
  review: ReimbursementReminderRuleSetting;
  payment: ReimbursementReminderRuleSetting;
}

export interface AdminSettings {
  default_roles: Record<string, RoleReference>;
  auto_create_employee: AutoCreateEmployeeSetting;
  mail_delivery: MailDeliverySetting;
  reimbursement_reminder: ReimbursementReminderSetting;
}

/** Documents-only Gmail settings (payslips, contracts). Never carries the app password. */
export interface DocumentMailSetting {
  enabled: boolean;
  smtp_host: string;
  smtp_username: string;
  smtp_port: 587 | 465;
  sender_name: string;
  has_smtp_password: boolean;
  ready: boolean;
  /** Present only while development mail capture (Mailpit) is active. */
  dev_smtp_addr?: string | null;
  /** Read-only: where document email actually goes (set by the server environment). */
  delivery?: DocumentMailDeliveryStatus;
  /** Read-only: whether generated documents are converted to PDF. */
  pdf?: DocumentPdfStatus;
}

export interface DocumentMailDeliveryStatus {
  mode: "gmail" | "dev_capture";
  capture_addr?: string | null;
}

/** How the server resolved LibreOffice (never a filesystem path). */
export type DocumentPdfSource = "auto" | "env" | "disabled" | "not_found" | "env_invalid";

export interface DocumentPdfStatus {
  enabled: boolean;
  source: DocumentPdfSource;
}

export interface UpdateDocumentMailPayload {
  enabled: boolean;
  smtp_username: string;
  smtp_password: string | null;
  clear_smtp_password: boolean;
  smtp_port: 587 | 465;
  sender_name: string;
}

export type DocumentMailErrorCategory =
  | "config"
  | "message"
  | "connect"
  | "tls"
  | "auth"
  | "recipient"
  | "rejected"
  | "temporary"
  | "timeout";

export interface DocumentMailTestResult {
  sent: boolean;
  delivery_id: string;
  recipient: string;
  error_category?: DocumentMailErrorCategory | null;
  error_message?: string | null;
}

/** Tenant company profile used by generated documents (contracts, payslips). */
export interface CompanyProfile {
  legal_name: string;
  address: string;
  business_type: string;
  city: string;
  signer_name: string;
  signer_title: string;
  hr_contact_email: string;
  /** Prefix of document and employee numbers, e.g. "CTN" -> "CTN-0001". */
  doc_code: string;
  payday_day: number;
  annual_leave_days: number;
  has_logo: boolean;
  logo_updated_at: string | null;
}

export interface UpdateCompanyProfilePayload {
  legal_name: string;
  address: string;
  business_type: string;
  city: string;
  signer_name: string;
  signer_title: string;
  hr_contact_email: string;
  doc_code: string;
  payday_day: number;
  annual_leave_days: number;
}

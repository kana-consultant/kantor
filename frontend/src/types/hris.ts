export type EmploymentStatus = "active" | "probation" | "resigned" | "terminated";

export interface Employee {
  id: string;
  user_id?: string | null;
  full_name: string;
  email: string;
  phone?: string | null;
  position: string;
  department?: string | null;
  date_joined: string;
  employment_status: EmploymentStatus;
  address?: string | null;
  emergency_contact?: string | null;
  avatar_url?: string | null;
  bank_account_number?: string | null;
  bank_name?: string | null;
  linkedin_profile?: string | null;
  ssh_keys?: string | null;
  created_at: string;
  updated_at: string;
}

export interface Department {
  id: string;
  name: string;
  description?: string | null;
  head_id?: string | null;
  head_name?: string | null;
  created_at: string;
}

export interface PaginationMeta {
  page: number;
  per_page: number;
  total: number;
}

export interface EmployeeFilters {
  page: number;
  perPage: number;
  search: string;
  department: string;
  status: string;
}

export interface ListEmployeesResponse {
  items: Employee[];
  meta: PaginationMeta;
}

export interface EmployeeFormValues {
  full_name: string;
  email: string;
  phone: string;
  position: string;
  department: string;
  date_joined: string;
  employment_status: EmploymentStatus;
  address: string;
  emergency_contact: string;
  avatar_url: string;
  bank_account_number: string;
  bank_name: string;
  linkedin_profile: string;
  ssh_keys: string;
}

export interface DepartmentFormValues {
  name: string;
  description: string;
  head_id: string;
}

export interface SalaryRecord {
  id: string;
  employee_id: string;
  base_salary: number;
  allowances: Record<string, number>;
  deductions: Record<string, number>;
  net_salary: number;
  effective_date: string;
  created_by: string;
  created_at: string;
}

export interface BonusRecord {
  id: string;
  employee_id: string;
  amount: number;
  reason: string;
  period_month: number;
  period_year: number;
  approval_status: "pending" | "approved" | "rejected";
  approved_by?: string | null;
  approved_at?: string | null;
  created_by: string;
  created_at: string;
}

export interface SalaryFormValues {
  base_salary: number;
  allowances: string;
  deductions: string;
  effective_date: string;
}

export interface BonusFormValues {
  amount: number;
  reason: string;
  period_month: number;
  period_year: number;
}

export interface Subscription {
  id: string;
  name: string;
  vendor: string;
  description?: string | null;
  cost_amount: number;
  cost_currency: string;
  billing_cycle: "monthly" | "quarterly" | "yearly";
  start_date: string;
  renewal_date: string;
  status: "active" | "cancelled" | "expired";
  pic_employee_id?: string | null;
  pic_employee_name?: string | null;
  category: string;
  login_credentials?: string | null;
  notes?: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface SubscriptionAlert {
  id: string;
  subscription_id: string;
  subscription_name: string;
  alert_type: "30_days" | "7_days" | "1_day";
  is_read: boolean;
  created_at: string;
}

export interface SubscriptionSummary {
  total_monthly_cost: number;
  total_yearly_cost: number;
  active_count: number;
  by_category: Record<string, number>;
}

export interface SubscriptionFormValues {
  name: string;
  vendor: string;
  description: string;
  cost_amount: number;
  cost_currency: string;
  billing_cycle: "monthly" | "quarterly" | "yearly";
  start_date: string;
  renewal_date: string;
  status: "active" | "cancelled" | "expired";
  pic_employee_id: string;
  category: string;
  login_credentials: string;
  notes: string;
}

export interface FinanceCategory {
  id: string;
  name: string;
  type: "income" | "outcome";
  is_default: boolean;
  created_at: string;
}

export interface FinanceRecord {
  id: string;
  category_id: string;
  category_name: string;
  type: "income" | "outcome";
  amount: number;
  description: string;
  record_date: string;
  record_month: number;
  record_year: number;
  approval_status: "draft" | "pending_review" | "approved" | "rejected";
  submitted_by?: string | null;
  reviewed_by?: string | null;
  reviewed_at?: string | null;
  approved_by?: string | null;
  approved_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface FinanceSummaryMonth {
  month: number;
  income: number;
  outcome: number;
}

export interface FinanceSummary {
  year: number;
  monthly: FinanceSummaryMonth[];
  total_income: number;
  total_outcome: number;
  net_profit_this_month: number;
  by_category: Record<string, number>;
}

export interface FinanceRecordFilters {
  page: number;
  perPage: number;
  type: string;
  category: string;
  month: string;
  year: string;
  status: string;
}

export interface ListFinanceRecordsResponse {
  items: FinanceRecord[];
  meta: PaginationMeta;
}

export interface FinanceRecordFormValues {
  category_id: string;
  type: "income" | "outcome";
  amount: number;
  description: string;
  record_date: string;
}

export interface FinanceCategoryFormValues {
  name: string;
  type: "income" | "outcome";
}

export interface Reimbursement {
  id: string;
  employee_id: string;
  employee_name: string;
  title: string;
  category: string;
  amount: number;
  transaction_date: string;
  description: string;
  status: "submitted" | "approved" | "rejected" | "paid";
  attachments: string[];
  submitted_by: string;
  manager_id?: string | null;
  manager_action_at?: string | null;
  manager_notes?: string | null;
  finance_id?: string | null;
  finance_action_at?: string | null;
  finance_notes?: string | null;
  paid_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface ReimbursementSummary {
  month: number;
  year: number;
  counts_by_status: Record<string, number>;
  approved_amount_month: number;
}

export interface ReimbursementFilters {
  page: number;
  perPage: number;
  status: string;
  employee: string;
  month: string;
  year: string;
}

export interface ReimbursementFormValues {
  employee_id: string;
  title: string;
  category: string;
  amount: number;
  transaction_date: string;
  description: string;
}

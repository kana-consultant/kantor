package dto

import "time"

type ListUsersQuery struct {
	Page       int    `validate:"omitempty,min=1"`
	PerPage    int    `validate:"omitempty,min=1,max=100"`
	Search     string `validate:"omitempty,max=150"`
	ModuleID   string `validate:"omitempty,max=50"`
	RoleID     string `validate:"omitempty,max=36"`
	SuperAdmin *bool  `validate:"omitempty"`
}

type ListRolesQuery struct {
	Search   string `validate:"omitempty,max=150"`
	IsSystem *bool  `validate:"omitempty"`
	IsActive *bool  `validate:"omitempty"`
}

type SetUserModuleRoleRequest struct {
	ModuleID string  `json:"module_id" validate:"required,max=50"`
	RoleID   *string `json:"role_id"`
}

type UpdateUserModuleRolesRequest struct {
	ModuleRoles []SetUserModuleRoleRequest `json:"module_roles" validate:"required,min=1,dive"`
}

type ToggleActiveRequest struct {
	Active bool `json:"active"`
}

type ToggleSuperAdminRequest struct {
	Enabled bool `json:"enabled"`
}

type UpsertRoleRequest struct {
	Name           string   `json:"name" validate:"required,min=3,max=100"`
	Slug           string   `json:"slug" validate:"required,min=3,max=50"`
	Description    string   `json:"description" validate:"omitempty,max=500"`
	HierarchyLevel int      `json:"hierarchy_level" validate:"omitempty,min=1,max=100"`
	PermissionIDs  []string `json:"permission_ids" validate:"required"`
}

type UpdateDefaultRolesRequest struct {
	DefaultRoles map[string]*string `json:"default_roles" validate:"required"`
}

type UpdateAutoCreateEmployeeRequest struct {
	Enabled             bool    `json:"enabled"`
	DefaultDepartmentID *string `json:"default_department_id"`
}

type UpdateMailDeliveryRequest struct {
	Enabled                    bool    `json:"enabled"`
	Provider                   string  `json:"provider" validate:"omitempty,oneof=resend"`
	SenderName                 string  `json:"sender_name" validate:"omitempty,max=120"`
	SenderEmail                string  `json:"sender_email" validate:"omitempty,email,max=160"`
	ReplyToEmail               *string `json:"reply_to_email" validate:"omitempty,email,max=160"`
	APIKey                     *string `json:"api_key" validate:"omitempty,max=500"`
	ClearAPIKey                bool    `json:"clear_api_key"`
	PasswordResetEnabled       bool    `json:"password_reset_enabled"`
	PasswordResetExpiryMinutes int     `json:"password_reset_expiry_minutes" validate:"min=5,max=1440"`
	NotificationEnabled        bool    `json:"notification_enabled"`
}

type ReminderChannelsRequest struct {
	InApp    bool `json:"in_app"`
	Email    bool `json:"email"`
	WhatsApp bool `json:"whatsapp"`
}

type ReimbursementReminderRuleRequest struct {
	Enabled  bool                    `json:"enabled"`
	Cron     string                  `json:"cron" validate:"omitempty,max=100"`
	Channels ReminderChannelsRequest `json:"channels"`
}

type UpdateReimbursementReminderRequest struct {
	Enabled bool                             `json:"enabled"`
	Review  ReimbursementReminderRuleRequest `json:"review"`
	Payment ReimbursementReminderRuleRequest `json:"payment"`
}

// UpdateDocumentMailRequest edits the documents-only Gmail settings. The host
// is fixed (smtp.gmail.com); only the account, app password, port and display
// name are configurable. Changing smtp_username or smtp_port while an app
// password is stored requires a new smtp_password in the same request;
// clear_smtp_password alone does not satisfy that rule. A blank smtp_username
// removes the account and drops the stored password.
type UpdateDocumentMailRequest struct {
	Enabled           bool    `json:"enabled"`
	SMTPUsername      string  `json:"smtp_username" validate:"omitempty,email,max=160"`
	SMTPPassword      *string `json:"smtp_password" validate:"omitempty,max=200"`
	ClearSMTPPassword bool    `json:"clear_smtp_password"`
	SMTPPort          int     `json:"smtp_port" validate:"required,oneof=587 465"`
	SenderName        string  `json:"sender_name" validate:"omitempty,max=120"`
}

// DocumentMailSettingResponse never carries the app password, only whether
// one is stored. DevSMTPAddr is present only while development mail capture
// (APP_ENV=development, Mailpit by default) is active. Delivery and PDF are
// read-only server status derived from the environment; the update request
// has no such fields.
type DocumentMailSettingResponse struct {
	Enabled         bool                       `json:"enabled"`
	SMTPHost        string                     `json:"smtp_host"`
	SMTPUsername    string                     `json:"smtp_username"`
	SMTPPort        int                        `json:"smtp_port"`
	SenderName      string                     `json:"sender_name"`
	HasSMTPPassword bool                       `json:"has_smtp_password"`
	Ready           bool                       `json:"ready"`
	DevSMTPAddr     *string                    `json:"dev_smtp_addr,omitempty"`
	Delivery        DocumentMailDeliveryStatus `json:"delivery"`
	PDF             DocumentPDFStatus          `json:"pdf"`
}

// DocumentMailDeliveryStatus says where document email actually goes:
// "gmail" (smtp.gmail.com) or "dev_capture" (a local capture server such as
// Mailpit, only with APP_ENV=development).
type DocumentMailDeliveryStatus struct {
	Mode        string  `json:"mode"`
	CaptureAddr *string `json:"capture_addr,omitempty"`
}

// DocumentPDFStatus says whether generated documents are converted to PDF and
// how the LibreOffice binary was resolved: "auto", "env", "disabled",
// "not_found" or "env_invalid". It never carries a filesystem path.
type DocumentPDFStatus struct {
	Enabled bool   `json:"enabled"`
	Source  string `json:"source"`
}

// DocumentMailTestResponse is the outcome of "Kirim email uji". A delivery
// failure is still a 200: the fixed error category is the useful payload.
type DocumentMailTestResponse struct {
	Sent          bool    `json:"sent"`
	DeliveryID    string  `json:"delivery_id"`
	Recipient     string  `json:"recipient"`
	ErrorCategory *string `json:"error_category,omitempty"`
	ErrorMessage  *string `json:"error_message,omitempty"`
}

// UpdateCompanyProfileRequest replaces the tenant's company profile used by
// generated documents. All fields are sent every time (full replacement).
type UpdateCompanyProfileRequest struct {
	LegalName       string `json:"legal_name" validate:"max=200"`
	Address         string `json:"address" validate:"max=500"`
	BusinessType    string `json:"business_type" validate:"max=200"`
	City            string `json:"city" validate:"max=100"`
	SignerName      string `json:"signer_name" validate:"max=120"`
	SignerTitle     string `json:"signer_title" validate:"max=120"`
	HRContactEmail  string `json:"hr_contact_email" validate:"omitempty,email,max=160"`
	DocCode         string `json:"doc_code" validate:"max=20"`
	PaydayDay       int    `json:"payday_day" validate:"min=1,max=31"`
	AnnualLeaveDays int    `json:"annual_leave_days" validate:"min=0,max=365"`
}

// CompanyProfileResponse is the company profile plus the tenant logo state.
// The logo itself is served by GET /admin/settings/company-profile/logo.
type CompanyProfileResponse struct {
	LegalName       string     `json:"legal_name"`
	Address         string     `json:"address"`
	BusinessType    string     `json:"business_type"`
	City            string     `json:"city"`
	SignerName      string     `json:"signer_name"`
	SignerTitle     string     `json:"signer_title"`
	HRContactEmail  string     `json:"hr_contact_email"`
	DocCode         string     `json:"doc_code"`
	PaydayDay       int        `json:"payday_day"`
	AnnualLeaveDays int        `json:"annual_leave_days"`
	HasLogo         bool       `json:"has_logo"`
	LogoUpdatedAt   *time.Time `json:"logo_updated_at"`
}

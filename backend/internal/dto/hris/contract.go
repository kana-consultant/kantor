package hris

import (
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

// ContractFields are the terms HR enters on the contract form. Create and
// update share them: an update is a full replacement of these fields, except
// that an omitted compensation, document_date or document_city keeps the
// stored value ("" clears document_city, and document_date while no number
// is assigned; the date is fixed once numbered), and omitted numbers fall
// back to their defaults
// (weekly_hours 40, notice_days 30, incident_report_hours 24,
// non_solicit_months 12, confidentiality_years 3).
type ContractFields struct {
	ContractType string `json:"contract_type" validate:"required,oneof=PKWT PKWTT MAGANG"`
	// IsRecordOnly ("Catat saja"): no documents. Forced on for PKWTT and
	// Magang.
	IsRecordOnly bool    `json:"is_record_only"`
	StartDate    string  `json:"start_date" validate:"required,datetime=2006-01-02"`
	EndDate      *string `json:"end_date" validate:"omitempty,datetime=2006-01-02"`

	JobTitle       string  `json:"job_title" validate:"required,max=120"`
	Department     *string `json:"department" validate:"omitempty,max=120"`
	SupervisorName *string `json:"supervisor_name" validate:"omitempty,max=120"`
	WorkLocation   string  `json:"work_location" validate:"max=200"`
	WorkMode       string  `json:"work_mode" validate:"omitempty,oneof=wfo hybrid remote"`
	WorkModeDetail *string `json:"work_mode_detail" validate:"omitempty,max=120"`
	// PKWTBasis (dasar PKWT) and JobDescription (uraian tugas) are single
	// Indonesian sentences: Pasal 12.4 makes the Indonesian text prevail.
	PKWTBasis      *string `json:"pkwt_basis" validate:"omitempty,max=400"`
	JobDescription string  `json:"job_description" validate:"max=600"`

	WorkDays    *string `json:"work_days" validate:"omitempty,max=80"`
	WorkHours   *string `json:"work_hours" validate:"omitempty,max=80"`
	WeeklyHours *int    `json:"weekly_hours" validate:"omitempty,min=1,max=60"`
	NoticeDays  *int    `json:"notice_days" validate:"omitempty,min=0,max=180"`

	// Compensation needs hris:salary:view to be set (and is returned only
	// with it). Omitted on create, it is prefilled from the salary record.
	Compensation *ContractCompensationRequest `json:"compensation" validate:"omitempty"`
	Benefits     []ContractBenefitRequest     `json:"benefits" validate:"max=10,dive"`

	IncidentReportHours  *int                       `json:"incident_report_hours" validate:"omitempty,min=1,max=720"`
	NonSolicitMonths     *int                       `json:"non_solicit_months" validate:"omitempty,min=0,max=60"`
	ConfidentialityYears *int                       `json:"confidentiality_years" validate:"omitempty,min=0,max=30"`
	PriorWorks           []ContractPriorWorkRequest `json:"prior_works" validate:"max=10,dive"`

	// DocumentDate defaults to the day of the first generate (Asia/Jakarta)
	// and is fixed once the numbers are assigned; DocumentCity defaults to
	// the company city. DocumentDate is YYYY-MM-DD or "" (clear); the
	// service checks the format, since a validator datetime tag would refuse
	// "" behind a pointer.
	DocumentDate *string `json:"document_date" validate:"omitempty,max=10"`
	DocumentCity *string `json:"document_city" validate:"omitempty,max=80"`
}

type ContractCompensationRequest struct {
	BaseSalary     int64 `json:"base_salary" validate:"min=0,max=1000000000000"`
	FixedAllowance int64 `json:"fixed_allowance" validate:"min=0,max=1000000000000"`
}

type ContractBenefitRequest struct {
	Name  string `json:"name" validate:"required,max=80"`
	Value string `json:"value" validate:"max=60"`
	Notes string `json:"notes" validate:"max=80"`
}

type ContractPriorWorkRequest struct {
	Title       string `json:"title" validate:"required,max=100"`
	Description string `json:"description" validate:"max=200"`
	Year        string `json:"year" validate:"omitempty,len=4,numeric"`
}

// CreateContractRequest creates a draft (or a record-only entry).
type CreateContractRequest struct {
	EmployeeID string `json:"employee_id" validate:"required,uuid"`
	ContractFields
}

// UpdateContractRequest edits a draft, a generated contract or a sent one
// that is not signed yet (the latter becomes a draft with revision+1 and
// keeps its numbers).
type UpdateContractRequest struct {
	ContractFields
}

// SendContractRequest e-mails the PKWT and the NDA together. recipient_source
// as for payslips (default, login, employee, personal); cc only on the domain
// of the company HR contact e-mail.
type SendContractRequest struct {
	RecipientSource string   `json:"recipient_source" validate:"omitempty,oneof=default login employee personal"`
	Cc              []string `json:"cc" validate:"max=5,dive,required,max=254"`
}

// UpdateContractStatusRequest: signed (signed_at defaults to today), ended
// (ended_at defaults to today) or cancelled.
type UpdateContractStatusRequest struct {
	Status   string  `json:"status" validate:"required,oneof=signed ended cancelled"`
	SignedAt *string `json:"signed_at" validate:"omitempty,datetime=2006-01-02"`
	EndedAt  *string `json:"ended_at" validate:"omitempty,datetime=2006-01-02"`
	EndNotes *string `json:"end_notes" validate:"omitempty,max=500"`
}

// ContractCompanyInfo carries the company profile fields HR needs on the
// contract pages (HRIS-only users never call /admin).
type ContractCompanyInfo struct {
	LegalName       string `json:"legal_name"`
	City            string `json:"city"`
	DocCode         string `json:"doc_code"`
	HRContactEmail  string `json:"hr_contact_email"`
	CcDomain        string `json:"cc_domain"`
	PaydayDay       int    `json:"payday_day"`
	AnnualLeaveDays int    `json:"annual_leave_days"`
}

type ContractCompensationResponse struct {
	BaseSalary     int64 `json:"base_salary"`
	FixedAllowance int64 `json:"fixed_allowance"`
}

// ContractDeliverySummary is the latest delivery of a contract.
type ContractDeliverySummary struct {
	ID               string     `json:"id"`
	Status           string     `json:"status"`
	Recipient        string     `json:"recipient"`
	RecipientSource  string     `json:"recipient_source"`
	Cc               []string   `json:"cc"`
	AttachmentSHA256 []string   `json:"attachment_sha256"`
	Error            *string    `json:"error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	SentAt           *time.Time `json:"sent_at,omitempty"`
}

// ContractListItem is one row of the contract list. notice_deadline is
// end_date - notice_days; notice_alert is set from notice_days + 14 days
// before end_date (the 'Batas pemberitahuan' badge); expired once end_date
// has passed (Asia/Jakarta) for a contract that is not ended or cancelled.
type ContractListItem struct {
	ID                 string     `json:"id"`
	EmployeeID         string     `json:"employee_id"`
	EmployeeName       string     `json:"employee_name"`
	EmployeeDepartment *string    `json:"employee_department"`
	ContractType       string     `json:"contract_type"`
	IsRecordOnly       bool       `json:"is_record_only"`
	Status             string     `json:"status"`
	Revision           int        `json:"revision"`
	StartDate          string     `json:"start_date"`
	EndDate            *string    `json:"end_date"`
	JobTitle           string     `json:"job_title"`
	DocNumber          *string    `json:"doc_number"`
	NDADocNumber       *string    `json:"nda_doc_number"`
	RenderStatus       string     `json:"render_status"`
	NoticeDays         int        `json:"notice_days"`
	NoticeDeadline     *string    `json:"notice_deadline"`
	NoticeAlert        bool       `json:"notice_alert"`
	Expired            bool       `json:"expired"`
	PreviousContractID *string    `json:"previous_contract_id"`
	SignedAt           *string    `json:"signed_at"`
	EndedAt            *string    `json:"ended_at"`
	LastSentAt         *time.Time `json:"last_sent_at"`
	// LastDelivery is the newest delivery of any status (the status pill);
	// LastSentDelivery the newest successful one (where it was last sent).
	LastDelivery     *ContractDeliverySummary `json:"last_delivery"`
	LastSentDelivery *ContractDeliverySummary `json:"last_sent_delivery"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
}

type ContractListResponse struct {
	Items             []ContractListItem `json:"items"`
	PDFAvailable      bool               `json:"pdf_available"`
	DocumentMailReady bool               `json:"document_mail_ready"`
}

// ContractDetailResponse is one contract with every term. Compensation is
// present only for callers with hris:salary:view (compensation_visible).
type ContractDetailResponse struct {
	ContractListItem

	Department     *string `json:"department"`
	SupervisorName *string `json:"supervisor_name"`
	WorkLocation   string  `json:"work_location"`
	WorkMode       string  `json:"work_mode"`
	WorkModeDetail *string `json:"work_mode_detail"`
	PKWTBasis      *string `json:"pkwt_basis"`
	JobDescription string  `json:"job_description"`

	WorkDays    string `json:"work_days"`
	WorkHours   string `json:"work_hours"`
	WeeklyHours int    `json:"weekly_hours"`

	CompensationVisible bool                          `json:"compensation_visible"`
	HasCompensation     bool                          `json:"has_compensation"`
	Compensation        *ContractCompensationResponse `json:"compensation,omitempty"`
	Benefits            []model.ContractBenefit       `json:"benefits"`

	IncidentReportHours  int                       `json:"incident_report_hours"`
	NonSolicitMonths     int                       `json:"non_solicit_months"`
	ConfidentialityYears int                       `json:"confidentiality_years"`
	PriorWorks           []model.ContractPriorWork `json:"prior_works"`

	DocumentDate    *string    `json:"document_date"`
	DocumentCity    *string    `json:"document_city"`
	SeqNo           *int       `json:"seq_no"`
	TemplateVersion *string    `json:"template_version"`
	RenderError     *string    `json:"render_error"`
	HasPDF          bool       `json:"has_pdf"`
	PDFSHA256       []string   `json:"pdf_sha256"`
	GeneratedAt     *time.Time `json:"generated_at"`
	GeneratedBy     *string    `json:"generated_by"`
	EndNotes        *string    `json:"end_notes"`

	PreviousDocNumber *string `json:"previous_doc_number"`
	PreviousEndDate   *string `json:"previous_end_date"`
	RenewedByID       *string `json:"renewed_by_id"`

	EmployeeStatus string `json:"employee_status"`

	PDFAvailable      bool                `json:"pdf_available"`
	DocumentMailReady bool                `json:"document_mail_ready"`
	AllowDocxSend     bool                `json:"allow_docx_send"`
	Company           ContractCompanyInfo `json:"company"`
}

// ContractMissingField is one field a PKWT needs before it can be generated.
// Scope is company (Admin > Settings > Profil Perusahaan), identity (HR
// profile, hris:employee_identity:edit), employee (employee record) or
// contract (this form).
type ContractMissingField struct {
	Scope string `json:"scope"`
	Field string `json:"field"`
	Label string `json:"label"`
}

// ContractWarning is a preflight finding that does not block generation.
// Action names a fix the UI can offer (e.g. set_employee_active).
type ContractWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

type ContractPreflightResponse struct {
	ContractID string `json:"contract_id"`
	// Documents is false for record-only entries (nothing to generate).
	Documents bool                   `json:"documents"`
	Ready     bool                   `json:"ready"`
	Missing   []ContractMissingField `json:"missing"`
	Warnings  []ContractWarning      `json:"warnings"`
	// ChainPKWTMonths is the cumulative PKWT duration over the renewal
	// chain up to and including this contract (the 5-year check).
	ChainPKWTMonths float64             `json:"chain_pkwt_months"`
	EmployeeStatus  string              `json:"employee_status"`
	Company         ContractCompanyInfo `json:"company"`
}

// ContractRecipientResponse is the resolved recipient of a contract e-mail.
// is_new ('alamat baru') is set when it differs from the last successful
// document delivery to the employee.
type ContractRecipientResponse struct {
	ContractID        string `json:"contract_id"`
	EmployeeID        string `json:"employee_id"`
	EmployeeName      string `json:"employee_name"`
	DocNumber         string `json:"doc_number"`
	Recipient         string `json:"recipient"`
	RecipientSource   string `json:"recipient_source"`
	Linked            bool   `json:"linked"`
	PersonalAvailable bool   `json:"personal_available"`
	HasPrevious       bool   `json:"has_previous"`
	PreviousRecipient string `json:"previous_recipient,omitempty"`
	IsNew             bool   `json:"is_new"`
	CcDomain          string `json:"cc_domain"`
}

// ContractSendResponse: contract is present only for callers that also hold
// hris:contract:view.
type ContractSendResponse struct {
	Contract      *ContractDetailResponse `json:"contract,omitempty"`
	Delivery      model.EmailDelivery     `json:"delivery"`
	Sent          bool                    `json:"sent"`
	ErrorCategory *string                 `json:"error_category,omitempty"`
	ErrorMessage  *string                 `json:"error_message,omitempty"`
}

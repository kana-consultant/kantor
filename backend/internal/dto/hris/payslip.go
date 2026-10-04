package hris

import (
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

// GeneratePayslipsRequest builds (or rebuilds) draft slips for a period.
// pay_date defaults to the company payday moved to the previous weekday.
type GeneratePayslipsRequest struct {
	Year        int      `json:"year" validate:"required,min=2000,max=2100"`
	Month       int      `json:"month" validate:"required,min=1,max=12"`
	EmployeeIDs []string `json:"employee_ids" validate:"required,min=1,max=500,dive,uuid"`
	PayDate     *string  `json:"pay_date" validate:"omitempty,datetime=2006-01-02"`
}

// UpdatePayslipRequest edits a draft. Both fields replace the stored value:
// note "" clears it; manual_lines [] removes every adjustment.
type UpdatePayslipRequest struct {
	Note        string                     `json:"note" validate:"max=400"`
	ManualLines []PayslipManualLineRequest `json:"manual_lines" validate:"max=5,dive"`
}

// PayslipManualLineRequest: a positive amount is an earning, a negative one
// a deduction.
type PayslipManualLineRequest struct {
	Label      string `json:"label" validate:"required,max=60"`
	Amount     int64  `json:"amount" validate:"required,min=-1000000000000,max=1000000000000"`
	Keterangan string `json:"keterangan" validate:"max=60"`
}

// SendPayslipRequest: recipient_source is default (login e-mail, or the
// employee e-mail when unlinked), login, employee or personal (HR profile).
// expected_recipient is the address the sender was shown: when set, the send
// is refused unless it resolves to exactly that address (required for calls
// through the MCP tool surface).
type SendPayslipRequest struct {
	RecipientSource   string `json:"recipient_source" validate:"omitempty,oneof=default login employee personal"`
	ExpectedRecipient string `json:"expected_recipient" validate:"omitempty,max=254"`
}

// SendPayslipBatchRequest sends several slips in the background. Slips that
// were already sent are skipped unless include_already_sent.
// expected_recipients maps payslip id -> the address the sender was shown
// (send-preview): when set, the batch is refused unless every slip that would
// be sent is listed with exactly its resolved address (required for calls
// through the MCP tool surface).
type SendPayslipBatchRequest struct {
	IDs                []string          `json:"ids" validate:"required,min=1,max=200,dive,uuid"`
	IncludeAlreadySent bool              `json:"include_already_sent"`
	ExpectedRecipients map[string]string `json:"expected_recipients" validate:"omitempty,max=200,dive,keys,uuid,endkeys,max=254"`
}

// VoidReissuePayslipRequest voids a sent slip and creates its replacement.
type VoidReissuePayslipRequest struct {
	Reason string `json:"reason" validate:"required,min=3,max=200"`
}

type PayslipTotalsResponse = model.PayslipTotals

// PayslipCompanyInfo carries the company profile fields HR needs on the
// payslip page (HRIS-only users never call /admin).
type PayslipCompanyInfo struct {
	LegalName      string   `json:"legal_name"`
	DocCode        string   `json:"doc_code"`
	PaydayDay      int      `json:"payday_day"`
	HRContactEmail string   `json:"hr_contact_email"`
	HasLogo        bool     `json:"has_logo"`
	MissingFields  []string `json:"missing_fields"`
}

// PayslipDeliverySummary is the latest delivery of a slip.
type PayslipDeliverySummary struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	Recipient       string     `json:"recipient"`
	RecipientSource string     `json:"recipient_source"`
	Error           *string    `json:"error,omitempty"`
	Attempts        int        `json:"attempts"`
	CreatedAt       time.Time  `json:"created_at"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
}

// PayslipListItem is one row of the period table: either a stored slip
// (status draft/sent) or a live preview for an employee without one
// (status "none", preview=true; totals computed but nothing stored).
type PayslipListItem struct {
	EmployeeID       string                  `json:"employee_id"`
	EmployeeName     string                  `json:"employee_name"`
	Department       *string                 `json:"department"`
	EmploymentType   string                  `json:"employment_type"`
	EmploymentStatus string                  `json:"employment_status"`
	DateJoined       string                  `json:"date_joined"`
	JobTitle         *string                 `json:"job_title"`
	EmployeeCode     *string                 `json:"employee_code"`
	PayslipID        *string                 `json:"payslip_id"`
	DocNumber        *string                 `json:"doc_number"`
	Status           string                  `json:"status"`
	Revision         int                     `json:"revision"`
	RenderStatus     string                  `json:"render_status"`
	RenderError      *string                 `json:"render_error"`
	PayDate          *string                 `json:"pay_date"`
	Totals           PayslipTotalsResponse   `json:"totals"`
	Warnings         []model.PayslipWarning  `json:"warnings"`
	Blocked          bool                    `json:"blocked"`
	Preview          bool                    `json:"preview"`
	HasPDF           bool                    `json:"has_pdf"`
	GeneratedAt      *time.Time              `json:"generated_at"`
	LastSentAt       *time.Time              `json:"last_sent_at"`
	LastDelivery     *PayslipDeliverySummary `json:"last_delivery"`
}

// PayslipSummary counts the rows of a period for the page tiles. Failed
// counts rows whose last delivery or render failed.
type PayslipSummary struct {
	Total        int `json:"total"`
	NotGenerated int `json:"not_generated"`
	Draft        int `json:"draft"`
	Sent         int `json:"sent"`
	Failed       int `json:"failed"`
	Blocked      int `json:"blocked"`
	InProgress   int `json:"in_progress"`
}

type PayslipListResponse struct {
	Year              int                `json:"year"`
	Month             int                `json:"month"`
	PeriodLabel       string             `json:"period_label"`
	DefaultPayDate    string             `json:"default_pay_date"`
	PDFAvailable      bool               `json:"pdf_available"`
	DocumentMailReady bool               `json:"document_mail_ready"`
	AllowDocxSend     bool               `json:"allow_docx_send"`
	Company           PayslipCompanyInfo `json:"company"`
	Summary           PayslipSummary     `json:"summary"`
	Items             []PayslipListItem  `json:"items"`
}

// PayslipHeader is the employee part printed on the slip (as snapshotted).
type PayslipHeader struct {
	EmployeeName   string `json:"employee_name"`
	EmployeeCode   string `json:"employee_code"`
	JobTitle       string `json:"job_title"`
	Department     string `json:"department"`
	StatusKerja    string `json:"status_kerja"`
	DateJoined     string `json:"date_joined"`
	Email          string `json:"email"`
	Bank           string `json:"bank"`
	AccountMasked  string `json:"account_masked"`
	Period         string `json:"period"`
	PayDateLabel   string `json:"pay_date_label"`
	TotalInWords   string `json:"total_in_words"`
	CompanyName    string `json:"company_name"`
	HRContactEmail string `json:"hr_contact_email"`
}

type PayslipDetailResponse struct {
	ID                string                           `json:"id"`
	EmployeeID        string                           `json:"employee_id"`
	PeriodYear        int                              `json:"period_year"`
	PeriodMonth       int                              `json:"period_month"`
	Revision          int                              `json:"revision"`
	DocNumber         string                           `json:"doc_number"`
	Status            string                           `json:"status"`
	PayDate           string                           `json:"pay_date"`
	Note              string                           `json:"note"`
	Header            PayslipHeader                    `json:"header"`
	Earnings          []model.PayslipLine              `json:"earnings"`
	Deductions        []model.PayslipLine              `json:"deductions"`
	Reimbursements    []model.PayslipReimbursementLine `json:"reimbursements"`
	ManualLines       []model.PayslipManualLine        `json:"manual_lines"`
	Totals            PayslipTotalsResponse            `json:"totals"`
	Warnings          []model.PayslipWarning           `json:"warnings"`
	RenderStatus      string                           `json:"render_status"`
	RenderError       *string                          `json:"render_error"`
	HasPDF            bool                             `json:"has_pdf"`
	PDFSHA256         *string                          `json:"pdf_sha256"`
	TemplateVersion   *string                          `json:"template_version"`
	ReplacesPayslipID *string                          `json:"replaces_payslip_id"`
	ReplacesDocNumber *string                          `json:"replaces_doc_number"`
	GeneratedAt       time.Time                        `json:"generated_at"`
	GeneratedBy       *string                          `json:"generated_by"`
	LastSentAt        *time.Time                       `json:"last_sent_at"`
	VoidedAt          *time.Time                       `json:"voided_at"`
	VoidReason        *string                          `json:"void_reason"`
	LastDelivery      *PayslipDeliverySummary          `json:"last_delivery"`
	UpdatedAt         time.Time                        `json:"updated_at"`
}

// PayslipHistoryItem is one slip in an employee's history (the Slip Gaji
// card on the employee page).
type PayslipHistoryItem struct {
	ID            string                  `json:"id"`
	PeriodYear    int                     `json:"period_year"`
	PeriodMonth   int                     `json:"period_month"`
	PeriodLabel   string                  `json:"period_label"`
	DocNumber     string                  `json:"doc_number"`
	Revision      int                     `json:"revision"`
	Status        string                  `json:"status"`
	RenderStatus  string                  `json:"render_status"`
	TotalDiterima int64                   `json:"total_diterima"`
	GeneratedAt   time.Time               `json:"generated_at"`
	LastSentAt    *time.Time              `json:"last_sent_at"`
	LastDelivery  *PayslipDeliverySummary `json:"last_delivery"`
}

// PayslipSkipped explains why an employee or slip was left out.
type PayslipSkipped struct {
	EmployeeID   string  `json:"employee_id,omitempty"`
	EmployeeName string  `json:"employee_name,omitempty"`
	PayslipID    *string `json:"payslip_id,omitempty"`
	DocNumber    *string `json:"doc_number,omitempty"`
	Code         string  `json:"code"`
	Reason       string  `json:"reason"`
}

type GeneratePayslipsResponse struct {
	Year      int               `json:"year"`
	Month     int               `json:"month"`
	PayDate   string            `json:"pay_date"`
	Generated []PayslipListItem `json:"generated"`
	Skipped   []PayslipSkipped  `json:"skipped"`
}

// PayslipRecipientResponse is the resolved recipient of one slip. is_new
// ('alamat baru') is set when it differs from the last successful delivery
// to the employee.
type PayslipRecipientResponse struct {
	PayslipID         string `json:"payslip_id"`
	EmployeeID        string `json:"employee_id"`
	EmployeeName      string `json:"employee_name"`
	DocNumber         string `json:"doc_number"`
	Status            string `json:"status"`
	Recipient         string `json:"recipient"`
	RecipientSource   string `json:"recipient_source"`
	Linked            bool   `json:"linked"`
	PersonalAvailable bool   `json:"personal_available"`
	HasPrevious       bool   `json:"has_previous"`
	PreviousRecipient string `json:"previous_recipient,omitempty"`
	IsNew             bool   `json:"is_new"`
}

// PayslipSendPreviewRequest previews the recipients of a batch send.
type PayslipSendPreviewRequest struct {
	IDs                []string `json:"ids" validate:"required,min=1,max=200,dive,uuid"`
	IncludeAlreadySent bool     `json:"include_already_sent"`
}

type PayslipSendPreviewResponse struct {
	Recipients         []PayslipRecipientResponse `json:"recipients"`
	Skipped            []PayslipSkipped           `json:"skipped"`
	AlreadySentSkipped int                        `json:"already_sent_skipped"`
	DocumentMailReady  bool                       `json:"document_mail_ready"`
}

type PayslipSendResponse struct {
	Payslip  PayslipDetailResponse `json:"payslip"`
	Delivery model.EmailDelivery   `json:"delivery"`
	Sent     bool                  `json:"sent"`
	// ErrorCategory / ErrorMessage are the fixed SMTP failure category when
	// the attempt failed (the delivery row records the same).
	ErrorCategory *string `json:"error_category,omitempty"`
	ErrorMessage  *string `json:"error_message,omitempty"`
}

type PayslipBatchQueued struct {
	PayslipID       string `json:"payslip_id"`
	DocNumber       string `json:"doc_number"`
	EmployeeName    string `json:"employee_name"`
	DeliveryID      string `json:"delivery_id"`
	Recipient       string `json:"recipient"`
	RecipientSource string `json:"recipient_source"`
	IsNew           bool   `json:"is_new"`
}

type PayslipBatchResponse struct {
	BatchID            string               `json:"batch_id"`
	Queued             []PayslipBatchQueued `json:"queued"`
	Skipped            []PayslipSkipped     `json:"skipped"`
	AlreadySentSkipped int                  `json:"already_sent_skipped"`
}

type VoidReissuePayslipResponse struct {
	Voided  PayslipDetailResponse `json:"voided"`
	Reissue PayslipDetailResponse `json:"reissue"`
}

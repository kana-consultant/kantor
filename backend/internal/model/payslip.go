package model

import "time"

// Payslip lifecycle (payslips.status).
const (
	PayslipStatusDraft = "draft"
	PayslipStatusSent  = "sent"
	PayslipStatusVoid  = "void"
)

// Render lifecycle shared by generated documents (payslips.render_status).
const (
	DocumentRenderNone      = "none"
	DocumentRenderPending   = "pending"
	DocumentRenderRendering = "rendering"
	DocumentRenderReady     = "ready"
	DocumentRenderFailed    = "failed"
)

// Payslip is one payslips row. AmountsEncrypted and PayloadEncrypted are
// security.Encrypter ciphertexts (PayslipAmounts JSON and the docgen payload
// JSON) and are never serialised to clients.
type Payslip struct {
	ID                string
	EmployeeID        string
	PeriodYear        int
	PeriodMonth       int
	Revision          int
	DocNumber         string
	ReplacesPayslipID *string
	SalaryID          *string
	ContractID        *string
	Status            string
	PayDate           time.Time
	AmountsEncrypted  string
	PayloadEncrypted  string
	ReimbursementIDs  []string
	BonusIDs          []string
	Note              *string
	Warnings          []PayslipWarning
	TemplateVersion   *string
	RenderStatus      string
	RenderError       *string
	RenderStartedAt   *time.Time
	PDFPath           *string
	PDFSHA256         *string
	GeneratedBy       *string
	GeneratedAt       time.Time
	LastSentAt        *time.Time
	VoidedBy          *string
	VoidedAt          *time.Time
	VoidReason        *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PayslipWarning is a check result shown next to a slip. Blocking warnings
// (e.g. no salary row) prevent generation.
type PayslipWarning struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

// PayslipLine is one row of the earnings (gaji) or deductions (potongan)
// table. Amount is always positive: deductions are listed in their own table
// and subtracted in the totals.
type PayslipLine struct {
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Keterangan string `json:"keterangan"`
	Amount     int64  `json:"amount"`
	SourceID   string `json:"source_id,omitempty"`
}

// Payslip line kinds.
const (
	PayslipLineBase      = "base"
	PayslipLineAllowance = "allowance"
	PayslipLineBonus     = "bonus"
	PayslipLineDeduction = "deduction"
	PayslipLineManual    = "manual"
)

// PayslipReimbursementLine is one paid reimbursement on the slip.
type PayslipReimbursementLine struct {
	ID              string `json:"id"`
	TransactionDate string `json:"transaction_date"`
	Title           string `json:"title"`
	Category        string `json:"category"`
	Amount          int64  `json:"amount"`
}

// PayslipManualLine is an HR adjustment on a draft. A positive amount is an
// earning, a negative one a deduction (shown as a positive potongan row).
type PayslipManualLine struct {
	Label      string `json:"label"`
	Amount     int64  `json:"amount"`
	Keterangan string `json:"keterangan"`
}

// PayslipTotals: TotalDiterima = TotalGaji - TotalPotongan + TotalReimbursement.
type PayslipTotals struct {
	TotalGaji          int64 `json:"total_gaji"`
	TotalPotongan      int64 `json:"total_potongan"`
	TotalReimbursement int64 `json:"total_reimbursement"`
	TotalDiterima      int64 `json:"total_diterima"`
}

// PayslipAmounts is the decrypted amounts_encrypted snapshot. Earnings and
// Deductions hold the computed rows (salary, bonuses); manual lines are kept
// separately so a draft can be edited and regenerated without losing them.
type PayslipAmounts struct {
	Earnings       []PayslipLine              `json:"earnings"`
	Deductions     []PayslipLine              `json:"deductions"`
	Reimbursements []PayslipReimbursementLine `json:"reimbursements"`
	ManualLines    []PayslipManualLine        `json:"manual_lines"`
	Totals         PayslipTotals              `json:"totals"`
}

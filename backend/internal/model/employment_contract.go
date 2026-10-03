package model

import "time"

// Contract types (employment_contracts.contract_type). Only a PKWT generates
// documents (01 PKWT + 02 NDA/HKI); PKWTT and Magang are record-only.
const (
	ContractTypePKWT   = "PKWT"
	ContractTypePKWTT  = "PKWTT"
	ContractTypeMagang = "MAGANG"
)

// Contract lifecycle (employment_contracts.status).
const (
	ContractStatusDraft     = "draft"
	ContractStatusGenerated = "generated"
	ContractStatusSent      = "sent"
	ContractStatusSigned    = "signed"
	ContractStatusEnded     = "ended"
	ContractStatusCancelled = "cancelled"
)

// Work arrangements (employment_contracts.work_mode).
const (
	ContractWorkModeWFO    = "wfo"
	ContractWorkModeHybrid = "hybrid"
	ContractWorkModeRemote = "remote"
)

// EmploymentContract is one employment_contracts row. CompensationEncrypted
// (ContractCompensation JSON) and PayloadEncrypted (the PKWT and NDA template
// payloads) are security.Encrypter ciphertexts and are never serialised to
// clients. PDFSHA256 holds the PKWT and NDA digests, in that order.
type EmploymentContract struct {
	ID                    string
	EmployeeID            string
	PreviousContractID    *string
	ContractType          string
	IsRecordOnly          bool
	Status                string
	Revision              int
	StartDate             time.Time
	EndDate               *time.Time
	JobTitle              string
	Department            *string
	SupervisorName        *string
	WorkLocation          string
	WorkMode              string
	WorkModeDetail        *string
	PKWTBasis             *string
	JobDescription        string
	WorkDays              string
	WorkHours             string
	WeeklyHours           int
	NoticeDays            int
	CompensationEncrypted *string
	Benefits              []ContractBenefit
	IncidentReportHours   int
	NonSolicitMonths      int
	ConfidentialityYears  int
	PriorWorks            []ContractPriorWork
	DocumentDate          *time.Time
	DocumentCity          *string
	SeqNo                 *int
	DocNumber             *string
	NDADocNumber          *string
	TemplateVersion       *string
	PayloadEncrypted      *string
	RenderStatus          string
	RenderError           *string
	RenderStartedAt       *time.Time
	PKWTPDFPath           *string
	NDAPDFPath            *string
	PDFSHA256             []string
	GeneratedBy           *string
	GeneratedAt           *time.Time
	LastSentAt            *time.Time
	SignedAt              *time.Time
	EndedAt               *time.Time
	EndNotes              *string
	CreatedBy             *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// ContractBenefit is one row of the PKWT benefit table (short bilingual
// labels, e.g. "Laptop kerja / Work laptop").
type ContractBenefit struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Notes string `json:"notes"`
}

// ContractPriorWork is one prior work the employee keeps out of the NDA/HKI
// assignment (Pasal 6, Karya Terdahulu).
type ContractPriorWork struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Year        string `json:"year"`
}

// ContractCompensation is the decrypted compensation_encrypted snapshot
// printed in the PKWT (Pasal 4).
type ContractCompensation struct {
	BaseSalary     int64 `json:"base_salary"`
	FixedAllowance int64 `json:"fixed_allowance"`
}

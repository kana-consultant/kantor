package model

import (
	"encoding/json"
	"time"
)

// Legal document types (legal_documents.doc_type).
const (
	LegalDocFreelance   = "freelance"
	LegalDocSubcontract = "subcontract"
	LegalDocSOW         = "sow"
	LegalDocSLA         = "sla"
	LegalDocBAST        = "bast"
)

// Legal document lifecycle (legal_documents.status).
const (
	LegalDocStatusDraft     = "draft"
	LegalDocStatusGenerated = "generated"
	LegalDocStatusSent      = "sent"
	LegalDocStatusSigned    = "signed"
	LegalDocStatusCancelled = "cancelled"
)

// Party types (legal_parties.party_type).
const (
	LegalPartyPerson  = "person"
	LegalPartyCompany = "company"
)

// Email delivery kind and recipient source of legal documents.
const (
	EmailDeliveryKindLegal     = "legal"
	EmailRecipientSourceParty  = "party"
	LegalDocumentReferenceType = "legal_document"
)

// LegalParty is a freelancer, subcontractor or client. The encrypted columns
// hold the identity number (NIK or company registration), the NPWP and the
// account number.
type LegalParty struct {
	ID                   string    `json:"id"`
	PartyType            string    `json:"party_type"`
	Name                 string    `json:"name"`
	Address              string    `json:"address"`
	Jurisdiction         string    `json:"jurisdiction"`
	IDNumberEncrypted    *string   `json:"-"`
	TaxIDEncrypted       *string   `json:"-"`
	RepresentativeName   string    `json:"representative_name"`
	RepresentativeTitle  string    `json:"representative_title"`
	ContactName          string    `json:"contact_name"`
	Email                string    `json:"email"`
	Phone                string    `json:"phone"`
	BankName             string    `json:"bank_name"`
	BankAccountEncrypted *string   `json:"-"`
	BankAccountHolder    string    `json:"bank_account_holder"`
	BankSwift            string    `json:"bank_swift"`
	Notes                string    `json:"notes"`
	CreatedBy            *string   `json:"created_by"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// LegalDocument is one agreement or report. Fields holds the form values of
// the document type (see service/legal spec).
type LegalDocument struct {
	ID                string          `json:"id"`
	DocType           string          `json:"doc_type"`
	Status            string          `json:"status"`
	Title             string          `json:"title"`
	PartyID           string          `json:"party_id"`
	RelatedDocumentID *string         `json:"related_document_id"`
	Fields            json.RawMessage `json:"fields"`
	DocumentDate      *time.Time      `json:"document_date"`
	DocumentCity      *string         `json:"document_city"`
	SeqNo             *int            `json:"seq_no"`
	DocNumber         *string         `json:"doc_number"`
	TemplateVersion   *string         `json:"template_version"`
	PayloadEncrypted  *string         `json:"-"`
	RenderStatus      string          `json:"render_status"`
	RenderError       *string         `json:"render_error"`
	PDFPath           *string         `json:"-"`
	PDFSHA256         *string         `json:"-"`
	SignedAt          *time.Time      `json:"signed_at"`
	SignedPDFPath     *string         `json:"-"`
	SignedPDFSHA256   *string         `json:"-"`
	GeneratedBy       *string         `json:"generated_by"`
	GeneratedAt       *time.Time      `json:"generated_at"`
	LastSentAt        *time.Time      `json:"last_sent_at"`
	CreatedBy         *string         `json:"created_by"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// Package legal holds the request and response shapes of /operational
// legal-parties and legal-documents.
package legal

import (
	"encoding/json"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

// PartyRequest creates or edits a party. On an edit, id_number, tax_id and
// bank_account_number keep the stored value when omitted (null) and clear it
// when sent as "".
type PartyRequest struct {
	PartyType           string  `json:"party_type" validate:"required,oneof=person company"`
	Name                string  `json:"name" validate:"required,max=200"`
	Address             string  `json:"address" validate:"max=500"`
	Jurisdiction        string  `json:"jurisdiction" validate:"max=200"`
	IDNumber            *string `json:"id_number" validate:"omitempty,max=60"`
	TaxID               *string `json:"tax_id" validate:"omitempty,max=40"`
	RepresentativeName  string  `json:"representative_name" validate:"max=200"`
	RepresentativeTitle string  `json:"representative_title" validate:"max=200"`
	ContactName         string  `json:"contact_name" validate:"max=200"`
	Email               string  `json:"email" validate:"omitempty,email,max=254"`
	Phone               string  `json:"phone" validate:"max=50"`
	BankName            string  `json:"bank_name" validate:"max=100"`
	BankAccountNumber   *string `json:"bank_account_number" validate:"omitempty,max=40"`
	BankAccountHolder   string  `json:"bank_account_holder" validate:"max=200"`
	BankSwift           string  `json:"bank_swift" validate:"max=20"`
	Notes               string  `json:"notes" validate:"max=2000"`
}

// PartyResponse never carries the identity, tax or account numbers in full.
type PartyResponse struct {
	ID                  string    `json:"id"`
	PartyType           string    `json:"party_type"`
	Name                string    `json:"name"`
	Address             string    `json:"address"`
	Jurisdiction        string    `json:"jurisdiction"`
	IDNumberMasked      string    `json:"id_number_masked"`
	TaxIDMasked         string    `json:"tax_id_masked"`
	RepresentativeName  string    `json:"representative_name"`
	RepresentativeTitle string    `json:"representative_title"`
	ContactName         string    `json:"contact_name"`
	Email               string    `json:"email"`
	Phone               string    `json:"phone"`
	BankName            string    `json:"bank_name"`
	BankAccountMasked   string    `json:"bank_account_masked"`
	BankAccountHolder   string    `json:"bank_account_holder"`
	BankSwift           string    `json:"bank_swift"`
	Notes               string    `json:"notes"`
	Documents           int       `json:"documents"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// DocumentRequest creates (doc_type required) or edits a document. fields
// are the form values of the type (see the types endpoint).
type DocumentRequest struct {
	DocType           string          `json:"doc_type" validate:"omitempty,oneof=freelance subcontract sow sla bast"`
	Title             string          `json:"title" validate:"required,max=200"`
	PartyID           string          `json:"party_id" validate:"required,uuid"`
	RelatedDocumentID *string         `json:"related_document_id" validate:"omitempty,uuid"`
	Fields            json.RawMessage `json:"fields"`
	DocumentDate      *string         `json:"document_date" validate:"omitempty,datetime=2006-01-02"`
	DocumentCity      *string         `json:"document_city" validate:"omitempty,max=100"`
}

// SendRequest e-mails the document PDF to recipient (by default the party's
// e-mail, shown to the user before sending).
type SendRequest struct {
	Recipient string `json:"recipient" validate:"required,email,max=254"`
}

type RelatedDocument struct {
	ID        string  `json:"id"`
	DocType   string  `json:"doc_type"`
	Title     string  `json:"title"`
	DocNumber *string `json:"doc_number"`
	Status    string  `json:"status"`
}

type DocumentListItem struct {
	ID           string     `json:"id"`
	DocType      string     `json:"doc_type"`
	Status       string     `json:"status"`
	Title        string     `json:"title"`
	PartyID      string     `json:"party_id"`
	PartyName    string     `json:"party_name"`
	DocNumber    *string    `json:"doc_number"`
	DocumentDate *string    `json:"document_date"`
	RenderStatus string     `json:"render_status"`
	LastSentAt   *time.Time `json:"last_sent_at"`
	SignedAt     *string    `json:"signed_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type DocumentDetail struct {
	DocumentListItem
	Fields           map[string]any        `json:"fields"`
	DocumentCity     *string               `json:"document_city"`
	Party            PartyResponse         `json:"party"`
	Related          *RelatedDocument      `json:"related"`
	RenderError      *string               `json:"render_error"`
	PDFReady         bool                  `json:"pdf_ready"`
	HasSignedCopy    bool                  `json:"has_signed_copy"`
	GeneratedAt      *time.Time            `json:"generated_at"`
	CreatedAt        time.Time             `json:"created_at"`
	Missing          map[string]string     `json:"missing"`
	CompanyMissing   []string              `json:"company_missing"`
	Deliveries       []model.EmailDelivery `json:"deliveries"`
	PDFAvailable     bool                  `json:"pdf_available"`
	MailReady        bool                  `json:"mail_ready"`
	SendInFlight     bool                  `json:"send_in_flight"`
	DefaultRecipient string                `json:"default_recipient"`
}

type SendResponse struct {
	Sent          bool                `json:"sent"`
	ErrorCategory *string             `json:"error_category,omitempty"`
	ErrorMessage  *string             `json:"error_message,omitempty"`
	Delivery      model.EmailDelivery `json:"delivery"`
	Document      DocumentDetail      `json:"document"`
}

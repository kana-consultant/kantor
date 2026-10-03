package model

import "time"

// Email delivery kinds (email_deliveries.kind).
const (
	EmailDeliveryKindContract = "contract"
	EmailDeliveryKindPayslip  = "payslip"
	EmailDeliveryKindTest     = "test"
)

// Where the recipient address came from (email_deliveries.recipient_source).
const (
	EmailRecipientSourceLogin    = "login"
	EmailRecipientSourceEmployee = "employee"
	EmailRecipientSourcePersonal = "personal"
	EmailRecipientSourceSelf     = "self"
)

// Delivery lifecycle (email_deliveries.status).
const (
	EmailDeliveryStatusQueued  = "queued"
	EmailDeliveryStatusSending = "sending"
	EmailDeliveryStatusSent    = "sent"
	EmailDeliveryStatusFailed  = "failed"
)

// EmailDelivery is one attempt to send a document email. It never holds the
// body or attachment bytes, only names and sha256 digests. Error is the fixed
// category message from the mail package, never raw SMTP text.
type EmailDelivery struct {
	ID               string     `json:"id"`
	Kind             string     `json:"kind"`
	ReferenceType    *string    `json:"reference_type,omitempty"`
	ReferenceID      *string    `json:"reference_id,omitempty"`
	Recipient        string     `json:"recipient"`
	RecipientSource  string     `json:"recipient_source"`
	Cc               []string   `json:"cc"`
	Subject          string     `json:"subject"`
	AttachmentNames  []string   `json:"attachment_names"`
	AttachmentSHA256 []string   `json:"attachment_sha256"`
	Status           string     `json:"status"`
	Error            *string    `json:"error,omitempty"`
	Attempts         int        `json:"attempts"`
	BatchID          *string    `json:"batch_id,omitempty"`
	RequestedBy      *string    `json:"requested_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	SentAt           *time.Time `json:"sent_at,omitempty"`
}

package hris

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// DocumentRecipientsRepository resolves where an employee's documents
// (payslips, contracts) are e-mailed and what address was used last.
type DocumentRecipientsRepository struct {
	db repository.DBTX
}

func NewDocumentRecipientsRepository(db repository.DBTX) *DocumentRecipientsRepository {
	return &DocumentRecipientsRepository{db: db}
}

// DocumentRecipient holds the candidate addresses of one employee:
// LoginEmail is the linked user's login (users.email: changed only by the
// user with their password, see auth ChangeEmail; the HR employee form does
// not write it), Email the employees.email field,
// PersonalEmail the HR-profile personal address (set under
// hris:employee_identity:edit).
type DocumentRecipient struct {
	EmployeeID    string
	FullName      string
	UserID        *string
	LoginEmail    *string
	Email         string
	PersonalEmail *string
}

func (r *DocumentRecipientsRepository) GetRecipient(ctx context.Context, employeeID string) (DocumentRecipient, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item DocumentRecipient
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT e.id::text, e.full_name, e.user_id::text, u.email, e.email, p.personal_email
		FROM employees e
		LEFT JOIN users u ON u.id = e.user_id
		LEFT JOIN employee_hr_profiles p ON p.employee_id = e.id
		WHERE e.id = $1::uuid
	`, employeeID).Scan(&item.EmployeeID, &item.FullName, &item.UserID, &item.LoginEmail, &item.Email, &item.PersonalEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentRecipient{}, ErrEmployeeNotFound
	}
	return item, err
}

// LatestSentDeliveryForEmployee returns the most recent successful document
// delivery to the employee, across every document kind that belongs to them
// (payslips and employment contracts).
// ok is false when nothing was ever sent.
func (r *DocumentRecipientsRepository) LatestSentDeliveryForEmployee(ctx context.Context, employeeID string) (model.EmailDelivery, bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.EmailDelivery
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT d.id::text, d.kind, d.recipient, d.recipient_source, d.status, d.created_at, d.sent_at
		FROM email_deliveries d
		WHERE d.status = 'sent'
		  AND (
		        (d.reference_type = 'payslip' AND d.reference_id IN (SELECT s.id FROM payslips s WHERE s.employee_id = $1::uuid))
		     OR (d.reference_type = 'contract' AND d.reference_id IN (SELECT c.id FROM employment_contracts c WHERE c.employee_id = $1::uuid))
		  )
		ORDER BY d.sent_at DESC NULLS LAST, d.created_at DESC
		LIMIT 1
	`, employeeID).Scan(&item.ID, &item.Kind, &item.Recipient, &item.RecipientSource, &item.Status, &item.CreatedAt, &item.SentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmailDelivery{}, false, nil
	}
	if err != nil {
		return model.EmailDelivery{}, false, err
	}
	return item, true, nil
}

// EmployeeIDForDocument returns the employee a document (payslip or
// employment contract) belongs to.
func (r *DocumentRecipientsRepository) EmployeeIDForDocument(ctx context.Context, referenceType string, referenceID string) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var employeeID string
	var err error
	switch referenceType {
	case model.EmailDeliveryKindPayslip:
		err = repository.DB(ctx, r.db).QueryRow(ctx, `SELECT employee_id::text FROM payslips WHERE id = $1::uuid`, referenceID).Scan(&employeeID)
	case model.EmailDeliveryKindContract:
		err = repository.DB(ctx, r.db).QueryRow(ctx, `SELECT employee_id::text FROM employment_contracts WHERE id = $1::uuid`, referenceID).Scan(&employeeID)
	default:
		return "", ErrEmployeeNotFound
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrEmployeeNotFound
	}
	return employeeID, err
}

// LogIdentityAccess records that a personal e-mail address (identity data)
// was shown in full. Callers treat an error as fatal (fail-closed).
func (r *DocumentRecipientsRepository) LogIdentityAccess(ctx context.Context, userID string, employeeID string, action string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, module, resource, resource_id, created_at)
		VALUES (NULLIF($1, '')::uuid, $2, 'hris', 'employee_identity', $3, NOW())
	`, userID, action, employeeID)
	return err
}

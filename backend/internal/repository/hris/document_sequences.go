package hris

import (
	"context"
	"errors"
	"strings"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// Document sequence keys (see migration 20260930120000_document_sequences).
const (
	// DocSequenceEmployment numbers a PKWT + NDA pair; period key 'YYYY-MM'
	// of the document date.
	DocSequenceEmployment = "EMPLOYMENT"
	// DocSequenceEmployee numbers employees (employee_code); period key
	// DocSequencePeriodAll.
	DocSequenceEmployee  = "EMPLOYEE"
	DocSequencePeriodAll = "ALL"
)

var ErrInvalidDocumentSequenceKey = errors.New("invalid document sequence key")

type DocumentSequencesRepository struct {
	db repository.DBTX
}

func NewDocumentSequencesRepository(db repository.DBTX) *DocumentSequencesRepository {
	return &DocumentSequencesRepository{db: db}
}

// Next increments and returns the counter for (docType, periodKey) of the
// current tenant, starting at 1. Call it inside the document's transaction
// (put the tx in ctx with repository.WithConn): the upsert row-locks the
// counter until that transaction ends, so concurrent callers are serialised
// and never receive the same value, and a rolled-back document gives its
// number back.
func (r *DocumentSequencesRepository) Next(ctx context.Context, docType string, periodKey string) (int, error) {
	docType = strings.TrimSpace(docType)
	periodKey = strings.TrimSpace(periodKey)
	if docType == "" || periodKey == "" || len(docType) > 40 || len(periodKey) > 40 {
		return 0, ErrInvalidDocumentSequenceKey
	}

	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var value int
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		INSERT INTO document_sequences (doc_type, period_key, last_value)
		VALUES ($1, $2, 1)
		ON CONFLICT (tenant_id, doc_type, period_key) DO UPDATE
		SET last_value = document_sequences.last_value + 1,
		    updated_at = NOW()
		RETURNING last_value
	`, docType, periodKey).Scan(&value)
	return value, err
}

// Current returns the last issued value (0 when nothing was issued yet).
func (r *DocumentSequencesRepository) Current(ctx context.Context, docType string, periodKey string) (int, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var value int
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COALESCE(MAX(last_value), 0)
		FROM document_sequences
		WHERE doc_type = $1 AND period_key = $2
	`, strings.TrimSpace(docType), strings.TrimSpace(periodKey)).Scan(&value)
	return value, err
}

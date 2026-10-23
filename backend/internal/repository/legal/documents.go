package legal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrDocumentNotFound = errors.New("legal document not found")
	// ErrDocumentStateChanged: the row is no longer in the state the update
	// required (edited, generated, sent, signed or cancelled meanwhile, or an
	// e-mail of it is being sent).
	ErrDocumentStateChanged = errors.New("legal document state changed")
	// ErrDocumentNumberTaken: uq_legal_documents_doc_number.
	ErrDocumentNumberTaken = errors.New("legal document number already exists")
	// ErrDocumentPartyMissing: the party (or related document) does not exist.
	ErrDocumentPartyMissing = errors.New("legal document party missing")
)

// notSending is the edit guard: a document never changes while an e-mail of
// it is queued or sending (checked in the same statement as the update).
const notSending = `NOT EXISTS (
	SELECT 1 FROM email_deliveries d
	WHERE d.reference_type = 'legal_document' AND d.reference_id = legal_documents.id AND d.status IN ('queued', 'sending')
)`

const documentColumns = `
	ld.id::text, ld.doc_type, ld.status, ld.title, ld.party_id::text, ld.related_document_id::text,
	ld.fields, ld.document_date, ld.document_city, ld.seq_no, ld.doc_number, ld.template_version,
	ld.payload_encrypted, ld.render_status, ld.render_error, ld.pdf_path, ld.pdf_sha256,
	ld.signed_at, ld.signed_pdf_path, ld.signed_pdf_sha256,
	ld.generated_by::text, ld.generated_at, ld.last_sent_at, ld.created_by::text, ld.created_at, ld.updated_at`

func scanDocument(row pgx.Row, item *model.LegalDocument, extra ...any) error {
	var fields []byte
	targets := []any{
		&item.ID, &item.DocType, &item.Status, &item.Title, &item.PartyID, &item.RelatedDocumentID,
		&fields, &item.DocumentDate, &item.DocumentCity, &item.SeqNo, &item.DocNumber, &item.TemplateVersion,
		&item.PayloadEncrypted, &item.RenderStatus, &item.RenderError, &item.PDFPath, &item.PDFSHA256,
		&item.SignedAt, &item.SignedPDFPath, &item.SignedPDFSHA256,
		&item.GeneratedBy, &item.GeneratedAt, &item.LastSentAt, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
	}
	if err := row.Scan(append(targets, extra...)...); err != nil {
		return err
	}
	if len(fields) == 0 {
		fields = []byte("{}")
	}
	item.Fields = json.RawMessage(fields)
	return nil
}

type DocumentsRepository struct {
	db repository.DBTX
}

func NewDocumentsRepository(db repository.DBTX) *DocumentsRepository {
	return &DocumentsRepository{db: db}
}

// DocumentFilter narrows the list. Empty values match everything.
type DocumentFilter struct {
	DocType string
	Status  string
	PartyID string
	Search  string
	Limit   int
}

// DocumentListItem is a document with its party's name and type.
type DocumentListItem struct {
	Document  model.LegalDocument
	PartyName string
	PartyType string
}

func (r *DocumentsRepository) List(ctx context.Context, filter DocumentFilter) ([]DocumentListItem, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT `+documentColumns+`, p.name, p.party_type
		FROM legal_documents ld
		JOIN legal_parties p ON p.id = ld.party_id
		WHERE ($1 = '' OR ld.doc_type = $1)
		  AND ($2 = '' OR ld.status = $2)
		  AND ($3 = '' OR ld.party_id::text = $3)
		  AND ($4 = '' OR ld.title ILIKE '%' || $4 || '%' OR p.name ILIKE '%' || $4 || '%' OR COALESCE(ld.doc_number, '') ILIKE '%' || $4 || '%')
		ORDER BY ld.updated_at DESC
		LIMIT $5
	`, strings.TrimSpace(filter.DocType), strings.TrimSpace(filter.Status), strings.TrimSpace(filter.PartyID), strings.TrimSpace(filter.Search), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]DocumentListItem, 0)
	for rows.Next() {
		var item DocumentListItem
		if err := scanDocument(rows, &item.Document, &item.PartyName, &item.PartyType); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *DocumentsRepository) Get(ctx context.Context, id string) (model.LegalDocument, error) {
	return r.get(ctx, id, false)
}

// GetForUpdate locks the row (call inside WithTx).
func (r *DocumentsRepository) GetForUpdate(ctx context.Context, id string) (model.LegalDocument, error) {
	return r.get(ctx, id, true)
}

func (r *DocumentsRepository) get(ctx context.Context, id string, lock bool) (model.LegalDocument, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `SELECT ` + documentColumns + ` FROM legal_documents ld WHERE ld.id = $1::uuid`
	if lock {
		query += ` FOR UPDATE`
	}
	var item model.LegalDocument
	err := scanDocument(repository.DB(ctx, r.db).QueryRow(ctx, query, id), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.LegalDocument{}, ErrDocumentNotFound
	}
	return item, err
}

// DocumentParams are the editable columns.
type DocumentParams struct {
	Title             string
	PartyID           string
	RelatedDocumentID *string
	Fields            json.RawMessage
	DocumentDate      *time.Time
	DocumentCity      *string
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return ErrDocumentPartyMissing
		case "23505":
			return ErrDocumentNumberTaken
		}
	}
	return err
}

func (r *DocumentsRepository) Create(ctx context.Context, docType string, params DocumentParams, createdBy string) (model.LegalDocument, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalDocument
	err := scanDocument(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO legal_documents (doc_type, title, party_id, related_document_id, fields, document_date, document_city, created_by)
			VALUES ($1, $2, $3::uuid, $4::uuid, $5::jsonb, $6, $7, NULLIF($8, '')::uuid)
			RETURNING *
		)
		SELECT `+documentColumns+` FROM inserted ld
	`, docType, params.Title, params.PartyID, params.RelatedDocumentID, []byte(params.Fields), params.DocumentDate, params.DocumentCity, createdBy), &item)
	return item, mapWriteError(err)
}

// Update writes the editable columns of a document that is not signed or
// cancelled and has no e-mail in flight. An issued document goes back to
// draft: its snapshot and PDF are dropped (the number is kept), and the path
// of the dropped PDF is returned so the caller can delete the file.
func (r *DocumentsRepository) Update(ctx context.Context, id string, params DocumentParams) (model.LegalDocument, *string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalDocument
	var oldPDF *string
	err := scanDocument(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH old AS (
			SELECT id, pdf_path FROM legal_documents WHERE id = $1::uuid FOR UPDATE
		), updated AS (
			UPDATE legal_documents SET
				title = $2, party_id = $3::uuid, related_document_id = $4::uuid, fields = $5::jsonb,
				document_date = $6, document_city = $7,
				status = 'draft', payload_encrypted = NULL, render_status = 'none', render_error = NULL,
				render_claim = NULL, pdf_path = NULL, pdf_sha256 = NULL, updated_at = NOW()
			FROM old
			WHERE legal_documents.id = old.id
			  AND legal_documents.status IN ('draft', 'generated', 'sent')
			  AND `+notSending+`
			RETURNING legal_documents.*, old.pdf_path AS old_pdf_path
		)
		SELECT `+documentColumns+`, ld.old_pdf_path FROM updated ld
	`, id, params.Title, params.PartyID, params.RelatedDocumentID, []byte(params.Fields), params.DocumentDate, params.DocumentCity), &item, &oldPDF)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := r.Get(ctx, id); errors.Is(getErr, ErrDocumentNotFound) {
			return model.LegalDocument{}, nil, ErrDocumentNotFound
		}
		return model.LegalDocument{}, nil, ErrDocumentStateChanged
	}
	if err != nil {
		return model.LegalDocument{}, nil, mapWriteError(err)
	}
	return item, oldPDF, nil
}

// SnapshotParams are written by a generate.
type SnapshotParams struct {
	ExpectStatus     string
	SeqNo            int
	DocNumber        string
	DocumentDate     time.Time
	DocumentCity     string
	PayloadEncrypted string
	TemplateVersion  string
	GeneratedBy      string
	RenderStatus     string
}

// RecordSnapshot stores a generate (status 'generated'). It refuses when the
// row moved on since it was locked or an e-mail of it is in flight.
func (r *DocumentsRepository) RecordSnapshot(ctx context.Context, id string, params SnapshotParams) (model.LegalDocument, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalDocument
	err := scanDocument(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE legal_documents SET
				status = 'generated', seq_no = $3, doc_number = $4, document_date = $5, document_city = $6,
				payload_encrypted = $7, template_version = $8, generated_by = NULLIF($9, '')::uuid, generated_at = NOW(),
				render_status = $10, render_error = NULL, render_claim = NULL, render_started_at = NULL,
				updated_at = NOW()
			WHERE id = $1::uuid AND status = $2 AND `+notSending+`
			RETURNING *
		)
		SELECT `+documentColumns+` FROM updated ld
	`, id, params.ExpectStatus, params.SeqNo, params.DocNumber, params.DocumentDate, params.DocumentCity,
		params.PayloadEncrypted, params.TemplateVersion, params.GeneratedBy, params.RenderStatus), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.LegalDocument{}, ErrDocumentStateChanged
	}
	return item, mapWriteError(err)
}

// MarkSent records a successful e-mail.
func (r *DocumentsRepository) MarkSent(ctx context.Context, id string, at time.Time) (model.LegalDocument, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalDocument
	err := scanDocument(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE legal_documents SET
				status = CASE WHEN status = 'generated' THEN 'sent' ELSE status END,
				last_sent_at = $2, updated_at = NOW()
			WHERE id = $1::uuid AND status IN ('generated', 'sent', 'signed')
			RETURNING *
		)
		SELECT `+documentColumns+` FROM updated ld
	`, id, at.UTC()), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.LegalDocument{}, ErrDocumentStateChanged
	}
	return item, err
}

// SetSigned marks an issued document signed and, when a scan is given,
// stores its path; the replaced scan's path is returned for deletion.
func (r *DocumentsRepository) SetSigned(ctx context.Context, id string, signedAt time.Time, scanPath *string, scanSHA256 *string) (model.LegalDocument, *string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalDocument
	var oldScan *string
	err := scanDocument(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH old AS (
			SELECT id, signed_pdf_path FROM legal_documents WHERE id = $1::uuid FOR UPDATE
		), updated AS (
			UPDATE legal_documents SET
				status = 'signed', signed_at = $2,
				signed_pdf_path = COALESCE($3, legal_documents.signed_pdf_path),
				signed_pdf_sha256 = COALESCE($4, legal_documents.signed_pdf_sha256),
				updated_at = NOW()
			FROM old
			WHERE legal_documents.id = old.id AND legal_documents.status IN ('generated', 'sent', 'signed')
			  AND `+notSending+`
			RETURNING legal_documents.*, CASE WHEN $3::text IS NOT NULL THEN old.signed_pdf_path END AS replaced
		)
		SELECT `+documentColumns+`, ld.replaced FROM updated ld
	`, id, signedAt, scanPath, scanSHA256), &item, &oldScan)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.LegalDocument{}, nil, ErrDocumentStateChanged
	}
	return item, oldScan, err
}

// Cancel marks a document that is not signed as cancelled.
func (r *DocumentsRepository) Cancel(ctx context.Context, id string) (model.LegalDocument, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalDocument
	err := scanDocument(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE legal_documents SET status = 'cancelled', updated_at = NOW()
			WHERE id = $1::uuid AND status IN ('draft', 'generated', 'sent') AND `+notSending+`
			RETURNING *
		)
		SELECT `+documentColumns+` FROM updated ld
	`, id), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.LegalDocument{}, ErrDocumentStateChanged
	}
	return item, err
}

// DeleteDraft removes a draft that never received a number.
func (r *DocumentsRepository) DeleteDraft(ctx context.Context, id string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `
		DELETE FROM legal_documents WHERE id = $1::uuid AND status = 'draft' AND doc_number IS NULL
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDocumentStateChanged
	}
	return nil
}

// ---------------------------------------------------------------------------
// Render (document worker)

func (r *DocumentsRepository) ListRenderPending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text
		FROM legal_documents
		WHERE status IN ('generated', 'sent', 'signed') AND payload_encrypted IS NOT NULL
		  AND (render_status = 'pending' OR (render_status = 'rendering' AND (render_started_at IS NULL OR render_started_at < $1)))
		ORDER BY updated_at ASC
		LIMIT 500
	`, staleBefore.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ClaimRender moves a pending (or stale rendering) document to rendering
// with a fresh claim token. ok is false when there is nothing to render.
func (r *DocumentsRepository) ClaimRender(ctx context.Context, id string, staleBefore time.Time) (string, model.LegalDocument, bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var claim string
	var item model.LegalDocument
	err := scanDocument(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE legal_documents
			SET render_status = 'rendering', render_claim = gen_random_uuid(), render_started_at = NOW(), render_error = NULL
			WHERE id = $1::uuid
			  AND status IN ('generated', 'sent', 'signed') AND payload_encrypted IS NOT NULL
			  AND (render_status = 'pending' OR (render_status = 'rendering' AND (render_started_at IS NULL OR render_started_at < $2)))
			RETURNING *
		)
		SELECT `+documentColumns+`, ld.render_claim::text FROM updated ld
	`, id, staleBefore.UTC()), &item, &claim)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", model.LegalDocument{}, false, nil
	}
	if err != nil {
		return "", model.LegalDocument{}, false, err
	}
	return claim, item, true, nil
}

// CompleteRender records the stored PDF if the row still carries claim and
// returns the path of the PDF it replaced.
func (r *DocumentsRepository) CompleteRender(ctx context.Context, id string, claim string, pdfPath string, digest string) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var old *string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE legal_documents d
		SET render_status = 'ready', render_error = NULL, render_claim = NULL,
		    pdf_path = $3, pdf_sha256 = $4, updated_at = NOW()
		FROM (SELECT id, pdf_path AS old_pdf FROM legal_documents WHERE id = $1::uuid FOR UPDATE) old
		WHERE d.id = old.id AND d.render_claim = $2::uuid AND d.render_status = 'rendering'
		RETURNING old.old_pdf
	`, id, claim, pdfPath, digest).Scan(&old)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDocumentStateChanged
	}
	if err != nil {
		return nil, err
	}
	if old != nil && *old != "" && *old != pdfPath {
		return []string{*old}, nil
	}
	return nil, nil
}

// FailRender records a failed render (only for claim, when given).
func (r *DocumentsRepository) FailRender(ctx context.Context, id string, claim string, reason string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var err error
	if claim == "" {
		_, err = repository.DB(ctx, r.db).Exec(ctx, `
			UPDATE legal_documents
			SET render_status = 'failed', render_error = $2, render_claim = NULL, updated_at = NOW()
			WHERE id = $1::uuid AND render_status IN ('pending', 'rendering')
		`, id, reason)
	} else {
		_, err = repository.DB(ctx, r.db).Exec(ctx, `
			UPDATE legal_documents
			SET render_status = 'failed', render_error = $3, render_claim = NULL, updated_at = NOW()
			WHERE id = $1::uuid AND render_claim = $2::uuid AND render_status = 'rendering'
		`, id, claim, reason)
	}
	return err
}

// WithTx runs fn in one transaction; fn receives a ctx bound to it.
func (r *DocumentsRepository) WithTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(context.Background())
		}
	}()
	if err = fn(repository.WithConn(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

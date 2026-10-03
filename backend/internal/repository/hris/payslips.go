package hris

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
	"github.com/kana-consultant/kantor/backend/internal/security"
)

var (
	ErrPayslipNotFound = errors.New("payslip not found")
	// ErrPayslipActiveExists: another active (draft or sent) slip exists for
	// the employee and period (uq_payslips_active).
	ErrPayslipActiveExists = errors.New("an active payslip already exists for this employee and period")
	// ErrPayslipStateChanged: the row is no longer in the state the update
	// required (e.g. a draft that was sent or voided meanwhile).
	ErrPayslipStateChanged = errors.New("payslip state changed")
	// ErrPayslipSendInFlight: the draft has a queued or sending e-mail, so
	// its snapshot is frozen until the delivery finishes.
	ErrPayslipSendInFlight = errors.New("payslip e-mail is being sent")
)

// payslipNotSending is the draft-edit guard: a snapshot is never replaced
// while an e-mail of it is queued or sending (checked in the same statement
// as the update, so a send that took the guard cannot race an edit).
const payslipNotSending = `NOT EXISTS (
	SELECT 1 FROM email_deliveries d
	WHERE d.reference_type = 'payslip' AND d.reference_id = payslips.id AND d.status IN ('queued', 'sending')
)`

type PayslipsRepository struct {
	db repository.DBTX
	// encrypter opens employees.bank_account_encrypted (the slip shows the
	// masked number).
	encrypter *security.Encrypter
}

func NewPayslipsRepository(db repository.DBTX, encrypter *security.Encrypter) *PayslipsRepository {
	return &PayslipsRepository{db: db, encrypter: encrypter}
}

// PayslipEmployee is the employee data a payslip needs, joined with the HR
// profile (job title, code) and the linked user's login e-mail.
type PayslipEmployee struct {
	ID                string
	UserID            *string
	FullName          string
	Email             string
	LoginEmail        *string
	Phone             *string
	Position          string
	Department        *string
	DateJoined        time.Time
	EmploymentStatus  string
	BankAccountNumber *string
	BankName          *string
	CreatedAt         time.Time
	JobTitle          *string
	EmployeeNumber    *int
	EmployeeCode      *string
	PersonalEmail     *string
}

// PayslipReimbursement is a paid or approved reimbursement considered for a
// slip (amount is stored in plain text).
type PayslipReimbursement struct {
	ID              string
	Title           string
	Category        string
	Amount          int64
	TransactionDate time.Time
	Status          string
	PaidAt          *time.Time
}

// PayslipAnchor is the employee's latest sent slip before a period (the D9
// carry-over floor).
type PayslipAnchor struct {
	ID          string
	PeriodYear  int
	PeriodMonth int
	GeneratedAt time.Time
}

// CreatePayslipParams inserts a new draft that waits for rendering.
type CreatePayslipParams struct {
	EmployeeID        string
	PeriodYear        int
	PeriodMonth       int
	Revision          int
	DocNumber         string
	ReplacesPayslipID *string
	SalaryID          *string
	ContractID        *string
	PayDate           time.Time
	AmountsEncrypted  string
	PayloadEncrypted  string
	ReimbursementIDs  []string
	BonusIDs          []string
	Note              *string
	Warnings          []model.PayslipWarning
	TemplateVersion   string
	GeneratedBy       string
	// GeneratedAt is the assembly's own clock (taken before any data was
	// read): the ceiling of its carry-over selection and the floor of the
	// next slip's (D9), so nothing falls between the two windows.
	GeneratedAt time.Time
	// RenderStatus is 'pending' (queued for the PDF worker) or 'none' (no
	// PDF converter on this server).
	RenderStatus string
}

// UpdatePayslipSnapshotParams replaces the snapshot of a draft (regeneration
// or an HR edit) and queues a new render.
type UpdatePayslipSnapshotParams struct {
	SalaryID         *string
	ContractID       *string
	PayDate          time.Time
	AmountsEncrypted string
	PayloadEncrypted string
	ReimbursementIDs []string
	BonusIDs         []string
	Note             *string
	Warnings         []model.PayslipWarning
	TemplateVersion  string
	// GeneratedBy is set on a regeneration (empty keeps the stored value
	// and generated_at, as for an HR edit of the note or manual lines).
	GeneratedBy string
	// GeneratedAt replaces generated_at on a regeneration (see
	// CreatePayslipParams.GeneratedAt).
	GeneratedAt time.Time
	// RenderStatus: 'pending' or 'none' (see CreatePayslipParams).
	RenderStatus string
}

func renderStatusOrPending(value string) string {
	if value == model.DocumentRenderNone {
		return model.DocumentRenderNone
	}
	return model.DocumentRenderPending
}

func timestampOrNil(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

const payslipColumns = `
	id::text, employee_id::text, period_year, period_month, revision, doc_number,
	replaces_payslip_id::text, salary_id::text, contract_id::text, status, pay_date,
	amounts_encrypted, payload_encrypted, reimbursement_ids::text[], bonus_ids::text[],
	note, warnings, template_version, render_status, render_error, render_started_at,
	pdf_path, pdf_sha256, generated_by::text, generated_at, last_sent_at,
	voided_by::text, voided_at, void_reason, created_at, updated_at`

func scanPayslip(row pgx.Row) (model.Payslip, error) {
	var item model.Payslip
	var warnings []byte
	err := row.Scan(
		&item.ID,
		&item.EmployeeID,
		&item.PeriodYear,
		&item.PeriodMonth,
		&item.Revision,
		&item.DocNumber,
		&item.ReplacesPayslipID,
		&item.SalaryID,
		&item.ContractID,
		&item.Status,
		&item.PayDate,
		&item.AmountsEncrypted,
		&item.PayloadEncrypted,
		&item.ReimbursementIDs,
		&item.BonusIDs,
		&item.Note,
		&warnings,
		&item.TemplateVersion,
		&item.RenderStatus,
		&item.RenderError,
		&item.RenderStartedAt,
		&item.PDFPath,
		&item.PDFSHA256,
		&item.GeneratedBy,
		&item.GeneratedAt,
		&item.LastSentAt,
		&item.VoidedBy,
		&item.VoidedAt,
		&item.VoidReason,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return model.Payslip{}, err
	}
	item.Warnings = []model.PayslipWarning{}
	if len(warnings) > 0 {
		if err := json.Unmarshal(warnings, &item.Warnings); err != nil {
			return model.Payslip{}, err
		}
	}
	if item.ReimbursementIDs == nil {
		item.ReimbursementIDs = []string{}
	}
	if item.BonusIDs == nil {
		item.BonusIDs = []string{}
	}
	return item, nil
}

func (r *PayslipsRepository) list(ctx context.Context, clause string, args ...any) ([]model.Payslip, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `SELECT `+payslipColumns+` FROM payslips `+clause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Payslip, 0)
	for rows.Next() {
		item, err := scanPayslip(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PayslipsRepository) GetByID(ctx context.Context, id string) (model.Payslip, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := scanPayslip(repository.DB(ctx, r.db).QueryRow(ctx, `SELECT `+payslipColumns+` FROM payslips WHERE id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Payslip{}, ErrPayslipNotFound
	}
	return item, err
}

// GetByIDs returns the rows of ids that exist (in no particular order).
func (r *PayslipsRepository) GetByIDs(ctx context.Context, ids []string) ([]model.Payslip, error) {
	if len(ids) == 0 {
		return []model.Payslip{}, nil
	}
	return r.list(ctx, `WHERE id = ANY($1::uuid[])`, ids)
}

// ListActiveByPeriod returns every draft and sent slip of a period.
func (r *PayslipsRepository) ListActiveByPeriod(ctx context.Context, year int, month int) ([]model.Payslip, error) {
	return r.list(ctx, `WHERE period_year = $1 AND period_month = $2 AND status <> 'void' ORDER BY doc_number`, year, month)
}

// ListByEmployee returns the employee's slips (voided ones included), newest
// period first.
func (r *PayslipsRepository) ListByEmployee(ctx context.Context, employeeID string, limit int) ([]model.Payslip, error) {
	if limit <= 0 || limit > 120 {
		limit = 120
	}
	return r.list(ctx, `WHERE employee_id = $1::uuid ORDER BY period_year DESC, period_month DESC, revision DESC LIMIT $2`, employeeID, limit)
}

// GetActive returns the draft or sent slip of an employee for a period.
func (r *PayslipsRepository) GetActive(ctx context.Context, employeeID string, year int, month int) (model.Payslip, error) {
	items, err := r.list(ctx, `WHERE employee_id = $1::uuid AND period_year = $2 AND period_month = $3 AND status <> 'void' LIMIT 1`, employeeID, year, month)
	if err != nil {
		return model.Payslip{}, err
	}
	if len(items) == 0 {
		return model.Payslip{}, ErrPayslipNotFound
	}
	return items[0], nil
}

// LatestSentBefore returns the employee's latest sent slip with a period
// before (year, month), or nil.
func (r *PayslipsRepository) LatestSentBefore(ctx context.Context, employeeID string, year int, month int) (*PayslipAnchor, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var anchor PayslipAnchor
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT id::text, period_year, period_month, generated_at
		FROM payslips
		WHERE employee_id = $1::uuid
		  AND status = 'sent'
		  AND (period_year * 12 + period_month) < ($2::int * 12 + $3::int)
		ORDER BY period_year DESC, period_month DESC, generated_at DESC
		LIMIT 1
	`, employeeID, year, month).Scan(&anchor.ID, &anchor.PeriodYear, &anchor.PeriodMonth, &anchor.GeneratedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &anchor, nil
}

// ConsumedItemIDs returns the bonus and reimbursement ids a slip of period
// (year, month) must not take: those already on a sent (non-void) slip of
// the employee, and those held by a draft of an EARLIER period (a draft that
// is still to be sent, e.g. one left 'Gagal'). The second rule keeps one item
// off two drafts; the send guard (OverlappingSlip) catches the rest.
func (r *PayslipsRepository) ConsumedItemIDs(ctx context.Context, employeeID string, year int, month int) (bonusIDs map[string]bool, reimbursementIDs map[string]bool, err error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT bonus_ids::text[], reimbursement_ids::text[]
		FROM payslips
		WHERE employee_id = $1::uuid
		  AND (
		        status = 'sent'
		     OR (status = 'draft' AND (period_year * 12 + period_month) < ($2::int * 12 + $3::int))
		  )
	`, employeeID, year, month)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	bonusIDs = map[string]bool{}
	reimbursementIDs = map[string]bool{}
	for rows.Next() {
		var bonuses, reimbursements []string
		if err := rows.Scan(&bonuses, &reimbursements); err != nil {
			return nil, nil, err
		}
		for _, id := range bonuses {
			bonusIDs[strings.ToLower(id)] = true
		}
		for _, id := range reimbursements {
			reimbursementIDs[strings.ToLower(id)] = true
		}
	}
	return bonusIDs, reimbursementIDs, rows.Err()
}

// PayslipOverlap is another slip of the same employee that holds one of the
// bonuses or reimbursements of a slip about to be sent.
type PayslipOverlap struct {
	ID        string
	DocNumber string
	// Sending is true when the other slip is a draft whose e-mail is queued
	// or sending (false: it is already sent).
	Sending bool
}

// OverlappingSlip returns another slip of the employee of payslipID that is
// sent (non-void), or a draft being e-mailed, and shares a bonus or a
// reimbursement with it; nil when there is none. Sending both would pay the
// item twice.
func (r *PayslipsRepository) OverlappingSlip(ctx context.Context, payslipID string) (*PayslipOverlap, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item PayslipOverlap
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT o.id::text, o.doc_number, o.status <> 'sent'
		FROM payslips p
		JOIN payslips o ON o.employee_id = p.employee_id AND o.id <> p.id
		WHERE p.id = $1::uuid
		  AND (o.bonus_ids && p.bonus_ids OR o.reimbursement_ids && p.reimbursement_ids)
		  AND (
		        o.status = 'sent'
		     OR (o.status = 'draft' AND EXISTS (
		            SELECT 1 FROM email_deliveries d
		            WHERE d.reference_type = 'payslip' AND d.reference_id = o.id AND d.status IN ('queued', 'sending')
		        ))
		  )
		ORDER BY (o.status = 'sent') DESC, o.period_year, o.period_month
		LIMIT 1
	`, payslipID).Scan(&item.ID, &item.DocNumber, &item.Sending)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ListReimbursementsForPayslip returns the employee's paid and approved
// (not yet paid) reimbursements; the service applies the carry-over rule.
func (r *PayslipsRepository) ListReimbursementsForPayslip(ctx context.Context, employeeID string) ([]PayslipReimbursement, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, title, category, amount, transaction_date, status, paid_at
		FROM reimbursements
		WHERE employee_id = $1::uuid AND status IN ('paid', 'approved')
		ORDER BY transaction_date ASC, created_at ASC
	`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PayslipReimbursement, 0)
	for rows.Next() {
		var item PayslipReimbursement
		if err := rows.Scan(&item.ID, &item.Title, &item.Category, &item.Amount, &item.TransactionDate, &item.Status, &item.PaidAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListReimbursementsByIDs returns the employee's reimbursements with the
// given ids, whatever their status (a void & reissue keeps the items of the
// slip it replaces).
func (r *PayslipsRepository) ListReimbursementsByIDs(ctx context.Context, employeeID string, ids []string) ([]PayslipReimbursement, error) {
	items := make([]PayslipReimbursement, 0, len(ids))
	if len(ids) == 0 {
		return items, nil
	}
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, title, category, amount, transaction_date, status, paid_at
		FROM reimbursements
		WHERE employee_id = $1::uuid AND id = ANY($2::uuid[])
		ORDER BY transaction_date ASC, created_at ASC
	`, employeeID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item PayslipReimbursement
		if err := rows.Scan(&item.ID, &item.Title, &item.Category, &item.Amount, &item.TransactionDate, &item.Status, &item.PaidAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const payslipEmployeeSelect = `
	SELECT e.id::text, e.user_id::text, e.full_name, e.email, u.email, e.phone, e.position, e.department,
	       e.date_joined, e.employment_status, e.bank_account_number, e.bank_account_encrypted, e.bank_name, e.created_at,
	       p.job_title, p.employee_number, p.employee_code, p.personal_email
	FROM employees e
	LEFT JOIN users u ON u.id = e.user_id
	LEFT JOIN employee_hr_profiles p ON p.employee_id = e.id`

func (r *PayslipsRepository) scanPayslipEmployee(row pgx.Row) (PayslipEmployee, error) {
	var item PayslipEmployee
	var bankAccountPlain, bankAccountSealed *string
	err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.FullName,
		&item.Email,
		&item.LoginEmail,
		&item.Phone,
		&item.Position,
		&item.Department,
		&item.DateJoined,
		&item.EmploymentStatus,
		&bankAccountPlain,
		&bankAccountSealed,
		&item.BankName,
		&item.CreatedAt,
		&item.JobTitle,
		&item.EmployeeNumber,
		&item.EmployeeCode,
		&item.PersonalEmail,
	)
	if err != nil {
		return PayslipEmployee{}, err
	}
	item.BankAccountNumber, _ = readBankAccount(r.encrypter, item.ID, bankAccountPlain, bankAccountSealed)
	return item, nil
}

// GetEmployee returns one employee with its HR profile and login e-mail.
func (r *PayslipsRepository) GetEmployee(ctx context.Context, employeeID string) (PayslipEmployee, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := r.scanPayslipEmployee(repository.DB(ctx, r.db).QueryRow(ctx, payslipEmployeeSelect+` WHERE e.id = $1::uuid`, employeeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PayslipEmployee{}, ErrEmployeeNotFound
	}
	return item, err
}

// ListPeriodEmployees returns the employees of a payslip period: every
// active or probation employee who joined on or before periodEnd, plus
// anyone who already has an active slip for the period (e.g. resigned after
// it was sent), ordered by name. periodEnd is a calendar date (passed as
// text so the session time zone cannot shift it).
func (r *PayslipsRepository) ListPeriodEmployees(ctx context.Context, year int, month int, periodEnd time.Time) ([]PayslipEmployee, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, payslipEmployeeSelect+`
		WHERE (e.employment_status IN ('active', 'probation') AND e.date_joined <= $3::date)
		   OR EXISTS (
		        SELECT 1 FROM payslips s
		        WHERE s.employee_id = e.id AND s.period_year = $1 AND s.period_month = $2 AND s.status <> 'void'
		   )
		ORDER BY lower(e.full_name), e.id
	`, year, month, periodEnd.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PayslipEmployee, 0)
	for rows.Next() {
		item, err := r.scanPayslipEmployee(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// MaxRevision returns the highest revision of the employee's slips for a
// period (voided ones included), or -1 when there is none.
func (r *PayslipsRepository) MaxRevision(ctx context.Context, employeeID string, year int, month int) (int, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var revision int
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COALESCE(MAX(revision), -1)
		FROM payslips
		WHERE employee_id = $1::uuid AND period_year = $2 AND period_month = $3
	`, employeeID, year, month).Scan(&revision)
	return revision, err
}

func marshalWarnings(warnings []model.PayslipWarning) (string, error) {
	if warnings == nil {
		warnings = []model.PayslipWarning{}
	}
	raw, err := json.Marshal(warnings)
	return string(raw), err
}

func nonNilIDs(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// Create inserts a draft with render_status 'pending'.
func (r *PayslipsRepository) Create(ctx context.Context, params CreatePayslipParams) (model.Payslip, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	warnings, err := marshalWarnings(params.Warnings)
	if err != nil {
		return model.Payslip{}, err
	}

	item, err := scanPayslip(repository.DB(ctx, r.db).QueryRow(ctx, `
		INSERT INTO payslips (
			employee_id, period_year, period_month, revision, doc_number, replaces_payslip_id,
			salary_id, contract_id, status, pay_date, amounts_encrypted, payload_encrypted,
			reimbursement_ids, bonus_ids, note, warnings, template_version, render_status,
			generated_by, generated_at
		)
		VALUES (
			$1::uuid, $2, $3, $4, $5, $6::uuid,
			$7::uuid, $8::uuid, 'draft', $9::date, $10, $11,
			$12::uuid[], $13::uuid[], $14, $15::jsonb, NULLIF($16, ''), $19,
			NULLIF($17, '')::uuid, COALESCE($18::timestamptz, NOW())
		)
		RETURNING `+payslipColumns,
		params.EmployeeID,
		params.PeriodYear,
		params.PeriodMonth,
		params.Revision,
		params.DocNumber,
		params.ReplacesPayslipID,
		params.SalaryID,
		params.ContractID,
		params.PayDate,
		params.AmountsEncrypted,
		params.PayloadEncrypted,
		nonNilIDs(params.ReimbursementIDs),
		nonNilIDs(params.BonusIDs),
		params.Note,
		warnings,
		params.TemplateVersion,
		params.GeneratedBy,
		timestampOrNil(params.GeneratedAt),
		renderStatusOrPending(params.RenderStatus),
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.Payslip{}, ErrPayslipActiveExists
		}
		if isForeignKeyViolation(err) {
			return model.Payslip{}, ErrEmployeeNotFound
		}
		return model.Payslip{}, err
	}
	return item, nil
}

// UpdateDraftSnapshot replaces the snapshot of a draft and queues a render.
// The previous PDF stays recorded until the new render completes (it is not
// served meanwhile: downloads need render_status 'ready'). A draft whose
// e-mail is queued or sending is not touched (ErrPayslipSendInFlight).
func (r *PayslipsRepository) UpdateDraftSnapshot(ctx context.Context, id string, params UpdatePayslipSnapshotParams) (model.Payslip, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	warnings, err := marshalWarnings(params.Warnings)
	if err != nil {
		return model.Payslip{}, err
	}

	item, err := scanPayslip(repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE payslips
		SET salary_id = $2::uuid,
		    contract_id = $3::uuid,
		    pay_date = $4::date,
		    amounts_encrypted = $5,
		    payload_encrypted = $6,
		    reimbursement_ids = $7::uuid[],
		    bonus_ids = $8::uuid[],
		    note = $9,
		    warnings = $10::jsonb,
		    template_version = NULLIF($11, ''),
		    render_status = $14,
		    render_error = NULL,
		    render_claim = NULL,
		    render_started_at = NULL,
		    generated_by = COALESCE(NULLIF($12::text, '')::uuid, generated_by),
		    generated_at = CASE WHEN $12::text = '' THEN generated_at ELSE COALESCE($13::timestamptz, NOW()) END,
		    updated_at = NOW()
		WHERE id = $1::uuid AND status = 'draft' AND `+payslipNotSending+`
		RETURNING `+payslipColumns,
		id,
		params.SalaryID,
		params.ContractID,
		params.PayDate,
		params.AmountsEncrypted,
		params.PayloadEncrypted,
		nonNilIDs(params.ReimbursementIDs),
		nonNilIDs(params.BonusIDs),
		params.Note,
		warnings,
		params.TemplateVersion,
		params.GeneratedBy,
		timestampOrNil(params.GeneratedAt),
		renderStatusOrPending(params.RenderStatus),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Payslip{}, r.draftUpdateRefused(ctx, id)
	}
	return item, err
}

// draftUpdateRefused tells why a draft update matched no row: still a
// draft means its e-mail is in flight.
func (r *PayslipsRepository) draftUpdateRefused(ctx context.Context, id string) error {
	var draft bool
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT status = 'draft' FROM payslips WHERE id = $1::uuid`, id).Scan(&draft)
	if err == nil && draft {
		return ErrPayslipSendInFlight
	}
	return ErrPayslipStateChanged
}

// SentSnapshot identifies what was e-mailed: the snapshot ciphertext and,
// for a PDF attachment, its sha256 (empty for a DOCX attachment).
type SentSnapshot struct {
	PayloadEncrypted string
	PDFSHA256        string
}

// MarkSent records a successful delivery: a draft becomes sent only if it is
// still the snapshot that was e-mailed; a sent slip (re-sent) only gets a
// new last_sent_at. Voided slips are left alone.
func (r *PayslipsRepository) MarkSent(ctx context.Context, id string, at time.Time, sent SentSnapshot) (model.Payslip, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := scanPayslip(repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE payslips
		SET status = 'sent', last_sent_at = $2, updated_at = NOW()
		WHERE id = $1::uuid
		  AND (
		        status = 'sent'
		     OR (status = 'draft' AND payload_encrypted = $3 AND ($4 = '' OR pdf_sha256 = $4))
		  )
		RETURNING `+payslipColumns, id, at.UTC(), sent.PayloadEncrypted, sent.PDFSHA256))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Payslip{}, ErrPayslipStateChanged
	}
	return item, err
}

// Void marks a sent slip void (kept for the record, PDF included).
func (r *PayslipsRepository) Void(ctx context.Context, id string, voidedBy string, reason string) (model.Payslip, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := scanPayslip(repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE payslips
		SET status = 'void', voided_by = NULLIF($2, '')::uuid, voided_at = NOW(), void_reason = $3, updated_at = NOW()
		WHERE id = $1::uuid AND status = 'sent'
		RETURNING `+payslipColumns, id, voidedBy, reason))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Payslip{}, ErrPayslipStateChanged
	}
	return item, err
}

// ---------------------------------------------------------------------------
// Rendering (document worker)

// ListRenderPending returns ids of drafts that wait for a render, or whose
// render was claimed before staleBefore and never finished. Sent slips are
// never re-rendered: their PDF is the one that was e-mailed.
func (r *PayslipsRepository) ListRenderPending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text
		FROM payslips
		WHERE status = 'draft'
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

// ClaimRender moves a pending (or stale rendering) draft to rendering with a
// fresh claim token. ok is false when there is nothing to render.
func (r *PayslipsRepository) ClaimRender(ctx context.Context, id string, staleBefore time.Time) (claim string, item model.Payslip, ok bool, err error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE payslips
		SET render_status = 'rendering', render_claim = gen_random_uuid(), render_started_at = NOW(), render_error = NULL
		WHERE id = $1::uuid
		  AND status = 'draft'
		  AND (render_status = 'pending' OR (render_status = 'rendering' AND (render_started_at IS NULL OR render_started_at < $2)))
		RETURNING render_claim::text, `+payslipColumns, id, staleBefore.UTC())

	var warnings []byte
	err = row.Scan(
		&claim,
		&item.ID, &item.EmployeeID, &item.PeriodYear, &item.PeriodMonth, &item.Revision, &item.DocNumber,
		&item.ReplacesPayslipID, &item.SalaryID, &item.ContractID, &item.Status, &item.PayDate,
		&item.AmountsEncrypted, &item.PayloadEncrypted, &item.ReimbursementIDs, &item.BonusIDs,
		&item.Note, &warnings, &item.TemplateVersion, &item.RenderStatus, &item.RenderError, &item.RenderStartedAt,
		&item.PDFPath, &item.PDFSHA256, &item.GeneratedBy, &item.GeneratedAt, &item.LastSentAt,
		&item.VoidedBy, &item.VoidedAt, &item.VoidReason, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", model.Payslip{}, false, nil
	}
	if err != nil {
		return "", model.Payslip{}, false, err
	}
	return claim, item, true, nil
}

// CompleteRender records the stored PDF if the row is still the draft that
// carries claim and returns the path of the PDF it replaced (empty when
// none). pageWarnings replace the stored warnings whose codes are in
// pageCodes (the page-count check of the rendered PDF).
// ErrPayslipStateChanged when the row moved on.
func (r *PayslipsRepository) CompleteRender(ctx context.Context, id string, claim string, pdfPath string, pdfSHA256 string, templateVersion string, pageCodes []string, pageWarnings []model.PayslipWarning) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	added, err := marshalWarnings(pageWarnings)
	if err != nil {
		return "", err
	}
	var previous *string
	err = repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE payslips p
		SET render_status = 'ready', render_error = NULL, render_claim = NULL,
		    pdf_path = $3, pdf_sha256 = $4, template_version = NULLIF($5, ''),
		    warnings = COALESCE((
		        SELECT jsonb_agg(w) FROM jsonb_array_elements(p.warnings) w
		        WHERE NOT (COALESCE(w->>'code', '') = ANY($6::text[]))
		    ), '[]'::jsonb) || $7::jsonb,
		    updated_at = NOW()
		FROM (SELECT id, pdf_path AS old_path FROM payslips WHERE id = $1::uuid FOR UPDATE) old
		WHERE p.id = old.id AND p.render_claim = $2::uuid AND p.render_status = 'rendering' AND p.status = 'draft'
		RETURNING old.old_path
	`, id, claim, pdfPath, pdfSHA256, templateVersion, nonNilIDs(pageCodes), added).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrPayslipStateChanged
	}
	if err != nil {
		return "", err
	}
	if previous == nil || *previous == pdfPath {
		return "", nil
	}
	return *previous, nil
}

// FailRender records a failed render. With a claim it only applies to that
// claim; without one (the snapshot could not be loaded) to any pending or
// rendering row.
func (r *PayslipsRepository) FailRender(ctx context.Context, id string, claim string, reason string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var err error
	if claim == "" {
		_, err = repository.DB(ctx, r.db).Exec(ctx, `
			UPDATE payslips
			SET render_status = 'failed', render_error = $2, render_claim = NULL, updated_at = NOW()
			WHERE id = $1::uuid AND render_status IN ('pending', 'rendering')
		`, id, reason)
	} else {
		_, err = repository.DB(ctx, r.db).Exec(ctx, `
			UPDATE payslips
			SET render_status = 'failed', render_error = $3, render_claim = NULL, updated_at = NOW()
			WHERE id = $1::uuid AND render_claim = $2::uuid AND render_status = 'rendering'
		`, id, claim, reason)
	}
	return err
}

// ---------------------------------------------------------------------------
// Delivery history

// LatestDeliveries returns the newest email_deliveries row per payslip id.
func (r *PayslipsRepository) LatestDeliveries(ctx context.Context, ids []string) (map[string]model.EmailDelivery, error) {
	result := map[string]model.EmailDelivery{}
	if len(ids) == 0 {
		return result, nil
	}
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT DISTINCT ON (reference_id)
		       id::text, reference_id::text, recipient, recipient_source, status, error, attempts, created_at, sent_at
		FROM email_deliveries
		WHERE reference_type = 'payslip' AND reference_id = ANY($1::uuid[])
		ORDER BY reference_id, created_at DESC
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.EmailDelivery
		var referenceID string
		if err := rows.Scan(&item.ID, &referenceID, &item.Recipient, &item.RecipientSource, &item.Status, &item.Error, &item.Attempts, &item.CreatedAt, &item.SentAt); err != nil {
			return nil, err
		}
		item.Kind = model.EmailDeliveryKindPayslip
		reference := model.EmailDeliveryKindPayslip
		item.ReferenceType = &reference
		item.ReferenceID = &referenceID
		result[referenceID] = item
	}
	return result, rows.Err()
}

// WithTx runs fn in one transaction (a savepoint when ctx already carries
// one); fn receives a ctx bound to it.
func (r *PayslipsRepository) WithTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
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

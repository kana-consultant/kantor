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
	ErrContractNotFound = errors.New("employment contract not found")
	// ErrContractStateChanged: the row is no longer in the state the update
	// required (edited, generated, sent or signed meanwhile).
	ErrContractStateChanged = errors.New("employment contract state changed")
	// ErrContractSendInFlight: an e-mail of the contract is queued or
	// sending, so its terms and snapshot are frozen until it finishes.
	ErrContractSendInFlight = errors.New("employment contract e-mail is being sent")
	// ErrContractNumberTaken: the document number is already used
	// (uq_employment_contracts_doc_number).
	ErrContractNumberTaken = errors.New("employment contract number already exists")
)

// contractNotSending is the edit guard: a contract is never changed while an
// e-mail of it is queued or sending (checked in the same statement as the
// update, so a send that took the guard cannot race an edit).
const contractNotSending = `NOT EXISTS (
	SELECT 1 FROM email_deliveries d
	WHERE d.reference_type = 'contract' AND d.reference_id = employment_contracts.id AND d.status IN ('queued', 'sending')
)`

type ContractsRepository struct {
	db repository.DBTX
	// encrypter opens employees.bank_account_encrypted for the PKWT.
	encrypter *security.Encrypter
}

func NewContractsRepository(db repository.DBTX, encrypter *security.Encrypter) *ContractsRepository {
	return &ContractsRepository{db: db, encrypter: encrypter}
}

// ContractEmployee is the employee data a contract needs: the employee
// record, the HR profile job title and the head of the employee's
// department (the default atasan).
type ContractEmployee struct {
	ID                string
	UserID            *string
	FullName          string
	Email             string
	Phone             *string
	Position          string
	Department        *string
	DateJoined        time.Time
	EmploymentStatus  string
	Address           *string
	BankAccountNumber *string
	BankName          *string
	JobTitle          *string
	DepartmentHead    *string
}

// ContractTerms are the editable columns of a contract (create and edit).
type ContractTerms struct {
	ContractType          string
	IsRecordOnly          bool
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
	Benefits              []model.ContractBenefit
	IncidentReportHours   int
	NonSolicitMonths      int
	ConfidentialityYears  int
	PriorWorks            []model.ContractPriorWork
	DocumentDate          *time.Time
	DocumentCity          *string
}

// CreateContractParams inserts a draft.
type CreateContractParams struct {
	EmployeeID         string
	PreviousContractID *string
	Terms              ContractTerms
	CreatedBy          string
}

// UpdateContractTermsParams replaces the terms of a contract that is still
// in ExpectStatus / ExpectRevision (and not signed), moving it to Status /
// Revision. A contract that leaves 'generated' or 'sent' gets render_status
// 'none': its PDFs no longer match the terms (they stay recorded until the
// next render replaces them, and are never served meanwhile).
type UpdateContractTermsParams struct {
	ExpectStatus   string
	ExpectRevision int
	Status         string
	Revision       int
	Terms          ContractTerms
}

// ContractSnapshotParams records a generation: the numbers (assigned once),
// the encrypted payloads and a new render request.
type ContractSnapshotParams struct {
	ExpectStatus     string
	ExpectRevision   int
	SeqNo            int
	DocNumber        string
	NDADocNumber     string
	DocumentDate     time.Time
	DocumentCity     string
	PayloadEncrypted string
	TemplateVersion  string
	GeneratedBy      string
	// RenderStatus is 'pending' (queued for the PDF worker) or 'none' (no
	// PDF converter on this server).
	RenderStatus string
}

// ContractListFilter narrows the list (all optional).
type ContractListFilter struct {
	EmployeeID   string
	Status       string
	ContractType string
	Search       string
	Limit        int
}

// ContractListRow is a contract with the employee's name and department.
type ContractListRow struct {
	Contract           model.EmploymentContract
	EmployeeName       string
	EmployeeDepartment *string
}

// ContractChainLink is one contract of a renewal chain (5-year check).
type ContractChainLink struct {
	ID                 string
	PreviousContractID *string
	ContractType       string
	Status             string
	StartDate          time.Time
	EndDate            *time.Time
	EndedAt            *time.Time
}

const contractColumns = `
	c.id::text, c.employee_id::text, c.previous_contract_id::text, c.contract_type, c.is_record_only,
	c.status, c.revision, c.start_date, c.end_date,
	c.job_title, c.department, c.supervisor_name, c.work_location, c.work_mode, c.work_mode_detail,
	c.pkwt_basis, c.job_description, c.work_days, c.work_hours, c.weekly_hours, c.notice_days,
	c.compensation_encrypted, c.benefits, c.incident_report_hours, c.non_solicit_months,
	c.confidentiality_years, c.prior_works, c.document_date, c.document_city, c.seq_no,
	c.doc_number, c.nda_doc_number, c.template_version, c.payload_encrypted,
	c.render_status, c.render_error, c.render_started_at, c.pkwt_pdf_path, c.nda_pdf_path, c.pdf_sha256,
	c.generated_by::text, c.generated_at, c.last_sent_at, c.signed_at, c.ended_at, c.end_notes,
	c.created_by::text, c.created_at, c.updated_at`

func contractScanTargets(item *model.EmploymentContract, benefits *[]byte, priorWorks *[]byte) []any {
	return []any{
		&item.ID, &item.EmployeeID, &item.PreviousContractID, &item.ContractType, &item.IsRecordOnly,
		&item.Status, &item.Revision, &item.StartDate, &item.EndDate,
		&item.JobTitle, &item.Department, &item.SupervisorName, &item.WorkLocation, &item.WorkMode, &item.WorkModeDetail,
		&item.PKWTBasis, &item.JobDescription, &item.WorkDays, &item.WorkHours, &item.WeeklyHours, &item.NoticeDays,
		&item.CompensationEncrypted, benefits, &item.IncidentReportHours, &item.NonSolicitMonths,
		&item.ConfidentialityYears, priorWorks, &item.DocumentDate, &item.DocumentCity, &item.SeqNo,
		&item.DocNumber, &item.NDADocNumber, &item.TemplateVersion, &item.PayloadEncrypted,
		&item.RenderStatus, &item.RenderError, &item.RenderStartedAt, &item.PKWTPDFPath, &item.NDAPDFPath, &item.PDFSHA256,
		&item.GeneratedBy, &item.GeneratedAt, &item.LastSentAt, &item.SignedAt, &item.EndedAt, &item.EndNotes,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
	}
}

func decodeContractLists(item *model.EmploymentContract, benefits []byte, priorWorks []byte) error {
	item.Benefits = []model.ContractBenefit{}
	item.PriorWorks = []model.ContractPriorWork{}
	if len(benefits) > 0 {
		if err := json.Unmarshal(benefits, &item.Benefits); err != nil {
			return err
		}
	}
	if len(priorWorks) > 0 {
		if err := json.Unmarshal(priorWorks, &item.PriorWorks); err != nil {
			return err
		}
	}
	if item.Benefits == nil {
		item.Benefits = []model.ContractBenefit{}
	}
	if item.PriorWorks == nil {
		item.PriorWorks = []model.ContractPriorWork{}
	}
	return nil
}

func scanContract(row pgx.Row) (model.EmploymentContract, error) {
	var item model.EmploymentContract
	var benefits, priorWorks []byte
	if err := row.Scan(contractScanTargets(&item, &benefits, &priorWorks)...); err != nil {
		return model.EmploymentContract{}, err
	}
	if err := decodeContractLists(&item, benefits, priorWorks); err != nil {
		return model.EmploymentContract{}, err
	}
	return item, nil
}

// dateText passes a calendar date as text, so the session time zone cannot
// shift it.
func dateText(value time.Time) string {
	return value.Format("2006-01-02")
}

func optionalDateText(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := dateText(*value)
	return &text
}

func marshalJSONList(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func contractTermsArgs(terms ContractTerms) ([]any, error) {
	benefits := terms.Benefits
	if benefits == nil {
		benefits = []model.ContractBenefit{}
	}
	priorWorks := terms.PriorWorks
	if priorWorks == nil {
		priorWorks = []model.ContractPriorWork{}
	}
	benefitsJSON, err := marshalJSONList(benefits)
	if err != nil {
		return nil, err
	}
	priorJSON, err := marshalJSONList(priorWorks)
	if err != nil {
		return nil, err
	}
	return []any{
		terms.ContractType,                   // +0
		terms.IsRecordOnly,                   // +1
		dateText(terms.StartDate),            // +2
		optionalDateText(terms.EndDate),      // +3
		terms.JobTitle,                       // +4
		terms.Department,                     // +5
		terms.SupervisorName,                 // +6
		terms.WorkLocation,                   // +7
		terms.WorkMode,                       // +8
		terms.WorkModeDetail,                 // +9
		terms.PKWTBasis,                      // +10
		terms.JobDescription,                 // +11
		terms.WorkDays,                       // +12
		terms.WorkHours,                      // +13
		terms.WeeklyHours,                    // +14
		terms.NoticeDays,                     // +15
		terms.CompensationEncrypted,          // +16
		benefitsJSON,                         // +17
		terms.IncidentReportHours,            // +18
		terms.NonSolicitMonths,               // +19
		terms.ConfidentialityYears,           // +20
		priorJSON,                            // +21
		optionalDateText(terms.DocumentDate), // +22
		terms.DocumentCity,                   // +23
	}, nil
}

func mapContractWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			if strings.Contains(pgErr.ConstraintName, "employee_id") {
				return ErrEmployeeNotFound
			}
			return ErrContractNotFound
		case "23505":
			return ErrContractNumberTaken
		}
	}
	return err
}

// GetEmployee returns the employee data a contract needs.
func (r *ContractsRepository) GetEmployee(ctx context.Context, employeeID string) (ContractEmployee, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item ContractEmployee
	var bankAccountPlain, bankAccountSealed *string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT e.id::text, e.user_id::text, e.full_name, e.email, e.phone, e.position, e.department,
		       e.date_joined, e.employment_status, e.address, e.bank_account_number, e.bank_account_encrypted, e.bank_name,
		       p.job_title,
		       (SELECT h.full_name FROM departments d JOIN employees h ON h.id = d.head_id
		         WHERE d.name = e.department AND h.id <> e.id LIMIT 1)
		FROM employees e
		LEFT JOIN employee_hr_profiles p ON p.employee_id = e.id
		WHERE e.id = $1::uuid
	`, employeeID).Scan(
		&item.ID, &item.UserID, &item.FullName, &item.Email, &item.Phone, &item.Position, &item.Department,
		&item.DateJoined, &item.EmploymentStatus, &item.Address, &bankAccountPlain, &bankAccountSealed, &item.BankName,
		&item.JobTitle, &item.DepartmentHead,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContractEmployee{}, ErrEmployeeNotFound
	}
	if err != nil {
		return ContractEmployee{}, err
	}
	item.BankAccountNumber, _ = readBankAccount(r.encrypter, item.ID, bankAccountPlain, bankAccountSealed)
	return item, nil
}

func (r *ContractsRepository) GetByID(ctx context.Context, id string) (model.EmploymentContract, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := scanContract(repository.DB(ctx, r.db).QueryRow(ctx, `SELECT `+contractColumns+` FROM employment_contracts c WHERE c.id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmploymentContract{}, ErrContractNotFound
	}
	return item, err
}

// GetForUpdate reads a contract and locks its row until the transaction in
// ctx ends (WithTx): generate, renew and status changes build on the locked
// row, so an edit that lands meanwhile waits and is then refused by its own
// guard instead of being lost from the snapshot.
func (r *ContractsRepository) GetForUpdate(ctx context.Context, id string) (model.EmploymentContract, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := scanContract(repository.DB(ctx, r.db).QueryRow(ctx, `SELECT `+contractColumns+` FROM employment_contracts c WHERE c.id = $1::uuid FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmploymentContract{}, ErrContractNotFound
	}
	return item, err
}

// List returns contracts with the employee's name, newest start first.
func (r *ContractsRepository) List(ctx context.Context, filter ContractListFilter) ([]ContractListRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	search := strings.TrimSpace(filter.Search)
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT `+contractColumns+`, e.full_name, e.department
		FROM employment_contracts c
		JOIN employees e ON e.id = c.employee_id
		WHERE ($1 = '' OR c.employee_id = NULLIF($1, '')::uuid)
		  AND ($2 = '' OR c.status = $2)
		  AND ($3 = '' OR c.contract_type = $3)
		  AND ($4 = '' OR e.full_name ILIKE '%' || $4 || '%'
		       OR c.job_title ILIKE '%' || $4 || '%'
		       OR COALESCE(c.doc_number, '') ILIKE '%' || $4 || '%'
		       OR COALESCE(c.nda_doc_number, '') ILIKE '%' || $4 || '%')
		ORDER BY c.start_date DESC, c.created_at DESC
		LIMIT $5
	`, strings.TrimSpace(filter.EmployeeID), strings.TrimSpace(filter.Status), strings.TrimSpace(filter.ContractType), escapeLike(search), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ContractListRow, 0)
	for rows.Next() {
		var row ContractListRow
		var benefits, priorWorks []byte
		targets := append(contractScanTargets(&row.Contract, &benefits, &priorWorks), &row.EmployeeName, &row.EmployeeDepartment)
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		if err := decodeContractLists(&row.Contract, benefits, priorWorks); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

// escapeLike escapes the ILIKE wildcards of a search term.
func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// Create inserts a draft.
func (r *ContractsRepository) Create(ctx context.Context, params CreateContractParams) (model.EmploymentContract, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	terms, err := contractTermsArgs(params.Terms)
	if err != nil {
		return model.EmploymentContract{}, err
	}
	args := append([]any{params.EmployeeID, params.PreviousContractID, params.CreatedBy}, terms...)
	item, err := scanContract(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO employment_contracts (
				employee_id, previous_contract_id, created_by,
				contract_type, is_record_only, start_date, end_date,
				job_title, department, supervisor_name, work_location, work_mode, work_mode_detail,
				pkwt_basis, job_description, work_days, work_hours, weekly_hours, notice_days,
				compensation_encrypted, benefits, incident_report_hours, non_solicit_months,
				confidentiality_years, prior_works, document_date, document_city
			)
			VALUES (
				$1::uuid, $2::uuid, NULLIF($3, '')::uuid,
				$4, $5, $6::date, $7::date,
				$8, $9, $10, $11, $12, $13,
				$14, $15, $16, $17, $18, $19,
				$20, $21::jsonb, $22, $23,
				$24, $25::jsonb, $26::date, $27
			)
			RETURNING *
		)
		SELECT `+contractColumns+` FROM inserted c
	`, args...))
	if err != nil {
		return model.EmploymentContract{}, mapContractWriteError(err)
	}
	return item, nil
}

// UpdateTerms replaces the terms (see UpdateContractTermsParams). A
// contract whose e-mail is queued or sending is not touched
// (ErrContractSendInFlight); one that changed meanwhile gives
// ErrContractStateChanged.
func (r *ContractsRepository) UpdateTerms(ctx context.Context, id string, params UpdateContractTermsParams) (model.EmploymentContract, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	terms, err := contractTermsArgs(params.Terms)
	if err != nil {
		return model.EmploymentContract{}, err
	}
	args := append([]any{id, params.ExpectStatus, params.ExpectRevision, params.Status, params.Revision}, terms...)
	item, err := scanContract(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE employment_contracts
			SET status = $4,
			    revision = $5,
			    contract_type = $6,
			    is_record_only = $7,
			    start_date = $8::date,
			    end_date = $9::date,
			    job_title = $10,
			    department = $11,
			    supervisor_name = $12,
			    work_location = $13,
			    work_mode = $14,
			    work_mode_detail = $15,
			    pkwt_basis = $16,
			    job_description = $17,
			    work_days = $18,
			    work_hours = $19,
			    weekly_hours = $20,
			    notice_days = $21,
			    compensation_encrypted = $22,
			    benefits = $23::jsonb,
			    incident_report_hours = $24,
			    non_solicit_months = $25,
			    confidentiality_years = $26,
			    prior_works = $27::jsonb,
			    document_date = $28::date,
			    document_city = $29,
			    render_status = CASE WHEN status IN ('generated', 'sent') OR render_status IN ('pending', 'rendering') THEN 'none' ELSE render_status END,
			    render_error = NULL,
			    render_claim = NULL,
			    render_started_at = NULL,
			    updated_at = NOW()
			WHERE id = $1::uuid AND status = $2 AND revision = $3 AND signed_at IS NULL AND `+contractNotSending+`
			RETURNING *
		)
		SELECT `+contractColumns+` FROM updated c
	`, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmploymentContract{}, r.refusal(ctx, id, params.ExpectStatus, params.ExpectRevision)
	}
	if err != nil {
		return model.EmploymentContract{}, mapContractWriteError(err)
	}
	return item, nil
}

// refusal tells why a guarded update matched no row: still in the expected
// state means its e-mail is in flight.
func (r *ContractsRepository) refusal(ctx context.Context, id string, status string, revision int) error {
	var same bool
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT status = $2 AND revision = $3 FROM employment_contracts WHERE id = $1::uuid
	`, id, status, revision).Scan(&same)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrContractNotFound
	}
	if err == nil && same {
		return ErrContractSendInFlight
	}
	return ErrContractStateChanged
}

// RecordSnapshot stores a generation (numbers, payloads) and queues the
// render. The numbers are written only when none are assigned yet; a
// contract that already carries numbers keeps them (revisions).
func (r *ContractsRepository) RecordSnapshot(ctx context.Context, id string, params ContractSnapshotParams) (model.EmploymentContract, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := scanContract(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE employment_contracts
			SET status = 'generated',
			    seq_no = COALESCE(seq_no, $4),
			    doc_number = COALESCE(doc_number, $5),
			    nda_doc_number = COALESCE(nda_doc_number, $6),
			    document_date = $7::date,
			    document_city = $8,
			    payload_encrypted = $9,
			    template_version = NULLIF($10, ''),
			    generated_by = NULLIF($11, '')::uuid,
			    generated_at = NOW(),
			    render_status = $12,
			    render_error = NULL,
			    render_claim = NULL,
			    render_started_at = NULL,
			    updated_at = NOW()
			WHERE id = $1::uuid AND status = $2 AND revision = $3 AND NOT is_record_only AND `+contractNotSending+`
			RETURNING *
		)
		SELECT `+contractColumns+` FROM updated c
	`, id, params.ExpectStatus, params.ExpectRevision, params.SeqNo, params.DocNumber, params.NDADocNumber,
		dateText(params.DocumentDate), params.DocumentCity, params.PayloadEncrypted, params.TemplateVersion,
		params.GeneratedBy, renderStatusOrPending(params.RenderStatus)))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmploymentContract{}, r.refusal(ctx, id, params.ExpectStatus, params.ExpectRevision)
	}
	if err != nil {
		return model.EmploymentContract{}, mapContractWriteError(err)
	}
	return item, nil
}

// SetStatus moves a contract from one of fromStatuses to status (signed,
// ended or cancelled) and records the dates and notes given.
func (r *ContractsRepository) SetStatus(ctx context.Context, id string, fromStatuses []string, status string, signedAt *time.Time, endedAt *time.Time, endNotes *string) (model.EmploymentContract, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := scanContract(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE employment_contracts
			SET status = $3,
			    signed_at = COALESCE($4::date, signed_at),
			    ended_at = COALESCE($5::date, ended_at),
			    end_notes = COALESCE($6, end_notes),
			    updated_at = NOW()
			WHERE id = $1::uuid AND status = ANY($2::text[]) AND `+contractNotSending+`
			RETURNING *
		)
		SELECT `+contractColumns+` FROM updated c
	`, id, fromStatuses, status, optionalDateText(signedAt), optionalDateText(endedAt), endNotes))
	if errors.Is(err, pgx.ErrNoRows) {
		var current string
		lookupErr := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT status FROM employment_contracts WHERE id = $1::uuid`, id).Scan(&current)
		switch {
		case errors.Is(lookupErr, pgx.ErrNoRows):
			return model.EmploymentContract{}, ErrContractNotFound
		case lookupErr == nil && containsString(fromStatuses, current):
			return model.EmploymentContract{}, ErrContractSendInFlight
		default:
			return model.EmploymentContract{}, ErrContractStateChanged
		}
	}
	if err != nil {
		return model.EmploymentContract{}, mapContractWriteError(err)
	}
	return item, nil
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

// ContractSentSnapshot identifies what was e-mailed: the payload ciphertext,
// the revision and (for PDF attachments) the PDF digests.
type ContractSentSnapshot struct {
	Revision         int
	PayloadEncrypted string
	PDFSHA256        []string
}

// MarkSent records a successful delivery: the contract becomes 'sent'
// unless it is already signed or ended (a copy re-sent), only if it is still
// the snapshot that was e-mailed; last_sent_at is set either way.
func (r *ContractsRepository) MarkSent(ctx context.Context, id string, at time.Time, sent ContractSentSnapshot) (model.EmploymentContract, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	digests := sent.PDFSHA256
	if digests == nil {
		digests = []string{}
	}
	item, err := scanContract(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE employment_contracts
			SET status = CASE WHEN status IN ('signed', 'ended') THEN status ELSE 'sent' END,
			    last_sent_at = $2,
			    updated_at = NOW()
			WHERE id = $1::uuid
			  AND status IN ('generated', 'sent', 'signed', 'ended')
			  AND revision = $3
			  AND payload_encrypted = $4
			  AND (cardinality($5::text[]) = 0 OR pdf_sha256 = $5::text[])
			RETURNING *
		)
		SELECT `+contractColumns+` FROM updated c
	`, id, at.UTC(), sent.Revision, sent.PayloadEncrypted, digests))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmploymentContract{}, ErrContractStateChanged
	}
	return item, err
}

// ChainLinks returns every contract of the employee (any type and status,
// at most 200, oldest start first): the service follows the Perpanjang links
// and the contiguous PKWT entries from them (the 5-year check).
func (r *ContractsRepository) ChainLinks(ctx context.Context, employeeID string) ([]ContractChainLink, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, previous_contract_id::text, contract_type, status, start_date, end_date, ended_at
		FROM employment_contracts
		WHERE employee_id = $1::uuid
		ORDER BY start_date, created_at
		LIMIT 200
	`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := make([]ContractChainLink, 0)
	for rows.Next() {
		var link ContractChainLink
		if err := rows.Scan(&link.ID, &link.PreviousContractID, &link.ContractType, &link.Status, &link.StartDate, &link.EndDate, &link.EndedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// RenewalOf returns the id of the (not cancelled) contract that renews id,
// or "" when there is none.
func (r *ContractsRepository) RenewalOf(ctx context.Context, id string) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var renewal string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT id::text FROM employment_contracts
		WHERE previous_contract_id = $1::uuid AND status <> 'cancelled'
		ORDER BY created_at DESC LIMIT 1
	`, id).Scan(&renewal)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return renewal, err
}

// ActiveForPeriod returns the employee's contract that is in force during
// [periodStart, periodEnd]: status sent or signed, or ended (Akhiri) on or
// after the period start, started by the period end and not past its end
// date before the period start. The latest start wins. An ended contract
// still gives the final slip of its last month its status kerja.
func (r *ContractsRepository) ActiveForPeriod(ctx context.Context, employeeID string, periodStart time.Time, periodEnd time.Time) (model.EmploymentContract, bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := scanContract(repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT `+contractColumns+`
		FROM employment_contracts c
		WHERE c.employee_id = $1::uuid
		  AND c.status IN ('sent', 'signed', 'ended')
		  AND c.start_date <= $3::date
		  AND (c.end_date IS NULL OR c.end_date >= $2::date)
		  AND (c.status <> 'ended' OR c.ended_at >= $2::date)
		ORDER BY c.start_date DESC, c.created_at DESC
		LIMIT 1
	`, employeeID, dateText(periodStart), dateText(periodEnd)))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmploymentContract{}, false, nil
	}
	if err != nil {
		return model.EmploymentContract{}, false, err
	}
	return item, true, nil
}

// LatestDeliveries returns the newest email_deliveries row per contract id
// (onlySent: the newest successful one).
func (r *ContractsRepository) LatestDeliveries(ctx context.Context, ids []string, onlySent bool) (map[string]model.EmailDelivery, error) {
	result := map[string]model.EmailDelivery{}
	if len(ids) == 0 {
		return result, nil
	}
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT DISTINCT ON (reference_id)
		       id::text, reference_id::text, recipient, recipient_source, cc, attachment_names, attachment_sha256,
		       status, error, attempts, created_at, sent_at
		FROM email_deliveries
		WHERE reference_type = 'contract' AND reference_id = ANY($1::uuid[])
		  AND (NOT $2::boolean OR status = 'sent')
		ORDER BY reference_id, created_at DESC
	`, ids, onlySent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item model.EmailDelivery
		var referenceID string
		if err := rows.Scan(&item.ID, &referenceID, &item.Recipient, &item.RecipientSource, &item.Cc, &item.AttachmentNames, &item.AttachmentSHA256,
			&item.Status, &item.Error, &item.Attempts, &item.CreatedAt, &item.SentAt); err != nil {
			return nil, err
		}
		item.Kind = model.EmailDeliveryKindContract
		reference := model.EmailDeliveryKindContract
		item.ReferenceType = &reference
		item.ReferenceID = &referenceID
		result[referenceID] = item
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Rendering (document worker)

// ListRenderPending returns ids of generated contracts that wait for a
// render, or whose render was claimed before staleBefore and never finished.
func (r *ContractsRepository) ListRenderPending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text
		FROM employment_contracts
		WHERE status = 'generated' AND NOT is_record_only AND payload_encrypted IS NOT NULL
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

// ClaimRender moves a pending (or stale rendering) generated contract to
// rendering with a fresh claim token. ok is false when there is nothing to
// render.
func (r *ContractsRepository) ClaimRender(ctx context.Context, id string, staleBefore time.Time) (claim string, item model.EmploymentContract, ok bool, err error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var benefits, priorWorks []byte
	targets := append([]any{&claim}, contractScanTargets(&item, &benefits, &priorWorks)...)
	err = repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE employment_contracts
			SET render_status = 'rendering', render_claim = gen_random_uuid(), render_started_at = NOW(), render_error = NULL
			WHERE id = $1::uuid
			  AND status = 'generated' AND NOT is_record_only AND payload_encrypted IS NOT NULL
			  AND (render_status = 'pending' OR (render_status = 'rendering' AND (render_started_at IS NULL OR render_started_at < $2)))
			RETURNING *
		)
		SELECT c.render_claim::text, `+contractColumns+` FROM updated c
	`, id, staleBefore.UTC()).Scan(targets...)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", model.EmploymentContract{}, false, nil
	}
	if err != nil {
		return "", model.EmploymentContract{}, false, err
	}
	if err := decodeContractLists(&item, benefits, priorWorks); err != nil {
		return "", model.EmploymentContract{}, false, err
	}
	return claim, item, true, nil
}

// CompleteRender records the stored PDFs if the row still carries claim and
// returns the paths of the PDFs it replaced. ErrContractStateChanged when
// the row moved on.
func (r *ContractsRepository) CompleteRender(ctx context.Context, id string, claim string, pkwtPath string, ndaPath string, digests []string, templateVersion string) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var oldPKWT, oldNDA *string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE employment_contracts c
		SET render_status = 'ready', render_error = NULL, render_claim = NULL,
		    pkwt_pdf_path = $3, nda_pdf_path = $4, pdf_sha256 = $5::text[],
		    template_version = COALESCE(NULLIF($6, ''), c.template_version),
		    updated_at = NOW()
		FROM (SELECT id, pkwt_pdf_path AS old_pkwt, nda_pdf_path AS old_nda FROM employment_contracts WHERE id = $1::uuid FOR UPDATE) old
		WHERE c.id = old.id AND c.render_claim = $2::uuid AND c.render_status = 'rendering' AND c.status = 'generated'
		RETURNING old.old_pkwt, old.old_nda
	`, id, claim, pkwtPath, ndaPath, digests, templateVersion).Scan(&oldPKWT, &oldNDA)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrContractStateChanged
	}
	if err != nil {
		return nil, err
	}
	superseded := []string{}
	for _, previous := range []*string{oldPKWT, oldNDA} {
		if previous != nil && *previous != "" && *previous != pkwtPath && *previous != ndaPath {
			superseded = append(superseded, *previous)
		}
	}
	return superseded, nil
}

// FailRender records a failed render. With a claim it only applies to that
// claim; without one (the snapshot could not be loaded) to any pending or
// rendering row.
func (r *ContractsRepository) FailRender(ctx context.Context, id string, claim string, reason string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var err error
	if claim == "" {
		_, err = repository.DB(ctx, r.db).Exec(ctx, `
			UPDATE employment_contracts
			SET render_status = 'failed', render_error = $2, render_claim = NULL, updated_at = NOW()
			WHERE id = $1::uuid AND render_status IN ('pending', 'rendering')
		`, id, reason)
	} else {
		_, err = repository.DB(ctx, r.db).Exec(ctx, `
			UPDATE employment_contracts
			SET render_status = 'failed', render_error = $3, render_claim = NULL, updated_at = NOW()
			WHERE id = $1::uuid AND render_claim = $2::uuid AND render_status = 'rendering'
		`, id, claim, reason)
	}
	return err
}

// WithTx runs fn in one transaction (a savepoint when ctx already carries
// one); fn receives a ctx bound to it.
func (r *ContractsRepository) WithTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
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

package hris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/mail"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// Kontrak kerja. A contract holds the terms of one employment period. For a
// PKWT that is not record-only, Generate assigns the shared monthly
// EMPLOYMENT number once, snapshots both template payloads (01 PKWT and 02
// NDA/HKI, encrypted) and queues the PDF render on the document worker;
// Send e-mails both PDFs in ONE message through the shared DocumentMailer.
// Editing a sent contract that is not signed returns it to draft with
// revision+1 and the same numbers. Record-only entries (PKWTT, Magang, paper
// history) never produce documents but feed payslips (PayslipContractSource)
// and the PKWT 5-year check.

const (
	ContractDocumentKind = "contract"
	ContractPartPKWT     = "pkwt"
	ContractPartNDA      = "nda"

	// contractExpectedPages: both templates render to five A4 pages with the
	// author's example data.
	contractExpectedPages = 5

	// Access-log actions (fail-closed). Salary rows: the PKWT prints the
	// compensation; identity rows: it prints the full NIK and account number.
	contractAccessView     = "contract_view"
	contractAccessDocument = "contract_document"
	contractAccessSend     = "contract_send"
)

var (
	ErrContractNotFound              = errors.New("kontrak kerja tidak ditemukan")
	ErrContractNotEditable           = errors.New("kontrak hanya dapat diubah selama berstatus draf, sudah di-generate, atau terkirim dan belum ditandatangani")
	ErrContractRecordOnly            = errors.New("kontrak ini hanya catatan (tanpa dokumen); tidak ada dokumen yang dapat di-generate atau dikirim")
	ErrContractNotGeneratable        = errors.New("kontrak terkirim, ditandatangani, berakhir atau dibatalkan tidak dapat di-generate ulang; gunakan Edit / Revisi untuk kontrak terkirim yang belum ditandatangani")
	ErrContractNotSendable           = errors.New("kontrak belum di-generate atau sudah dibatalkan; generate kontrak sebelum mengirim")
	ErrContractPDFNotReady           = errors.New("PDF kontrak belum siap; tunggu proses pembuatan PDF selesai")
	ErrContractEndDateRequired       = errors.New("PKWT wajib memiliki tanggal berakhir")
	ErrContractDatesInvalid          = errors.New("tanggal berakhir tidak boleh sebelum tanggal mulai")
	ErrContractJobTitleRequired      = errors.New("jabatan wajib diisi")
	ErrContractTypeLocked            = errors.New("nomor dokumen sudah ditetapkan: jenis kontrak tidak dapat diubah dan kontrak tidak dapat dijadikan catatan saja")
	ErrContractDocumentDateLocked    = errors.New("nomor dokumen sudah ditetapkan: tanggal dokumen tidak dapat diubah")
	ErrContractDocumentDateInvalid   = errors.New("tanggal dokumen tidak valid (format YYYY-MM-DD)")
	ErrContractCompensationForbidden = errors.New("anda tidak memiliki izin melihat gaji, sehingga tidak dapat mengubah kompensasi kontrak")
	ErrContractStatusTransition      = errors.New("perubahan status kontrak tidak diizinkan dari status saat ini")
	ErrContractStatusDateInvalid     = errors.New("tanggal tanda tangan atau tanggal berakhir tidak valid: tidak boleh di masa depan, dan tanggal berakhir tidak boleh sebelum tanggal tanda tangan (atau tanggal mulai)")
	ErrContractNotRenewable          = errors.New("hanya kontrak dengan tanggal berakhir yang sudah ditandatangani atau berakhir yang dapat diperpanjang")
	ErrContractSignBeforePDF         = errors.New("PDF kontrak belum siap; tunggu PDF selesai dibuat (atau Generate ulang bila gagal) sebelum menandai ditandatangani")
	ErrContractDocumentNotCurrent    = errors.New("dokumen belum di-generate untuk isi kontrak saat ini; generate (ulang) terlebih dahulu")
	ErrContractAlreadyRenewed        = errors.New("kontrak ini sudah memiliki perpanjangan")
	ErrContractStateChanged          = errors.New("kontrak berubah; muat ulang halaman lalu coba lagi")
	ErrContractPartInvalid           = errors.New("dokumen harus pkwt atau nda")
	ErrContractNoDocument            = errors.New("kontrak belum pernah di-generate")
	ErrContractCcInvalid             = errors.New("alamat CC tidak valid (maksimal 5 alamat)")
	ErrContractCcDomain              = errors.New("CC hanya boleh ke alamat dengan domain email HR perusahaan")
	ErrContractNumberTaken           = errors.New("nomor dokumen bentrok dengan kontrak lain; coba generate ulang")
	ErrContractTypeInvalid           = errors.New("jenis kontrak harus PKWT, PKWTT, atau MAGANG")
)

// ContractIncompleteError refuses a generation while required data is
// missing; Missing is what the preflight lists.
type ContractIncompleteError struct {
	Missing []hrisdto.ContractMissingField
}

func (e *ContractIncompleteError) Error() string {
	labels := make([]string, 0, len(e.Missing))
	for _, item := range e.Missing {
		labels = append(labels, item.Label)
	}
	return "data kontrak belum lengkap: " + strings.Join(labels, ", ")
}

type contractsRepository interface {
	GetEmployee(ctx context.Context, employeeID string) (hrisrepo.ContractEmployee, error)
	GetByID(ctx context.Context, id string) (model.EmploymentContract, error)
	GetForUpdate(ctx context.Context, id string) (model.EmploymentContract, error)
	List(ctx context.Context, filter hrisrepo.ContractListFilter) ([]hrisrepo.ContractListRow, error)
	Create(ctx context.Context, params hrisrepo.CreateContractParams) (model.EmploymentContract, error)
	UpdateTerms(ctx context.Context, id string, params hrisrepo.UpdateContractTermsParams) (model.EmploymentContract, error)
	RecordSnapshot(ctx context.Context, id string, params hrisrepo.ContractSnapshotParams) (model.EmploymentContract, error)
	SetStatus(ctx context.Context, id string, fromStatuses []string, status string, signedAt *time.Time, endedAt *time.Time, endNotes *string) (model.EmploymentContract, error)
	MarkSent(ctx context.Context, id string, at time.Time, sent hrisrepo.ContractSentSnapshot) (model.EmploymentContract, error)
	ChainLinks(ctx context.Context, employeeID string) ([]hrisrepo.ContractChainLink, error)
	RenewalOf(ctx context.Context, id string) (string, error)
	ActiveForPeriod(ctx context.Context, employeeID string, periodStart time.Time, periodEnd time.Time) (model.EmploymentContract, bool, error)
	LatestDeliveries(ctx context.Context, ids []string, onlySent bool) (map[string]model.EmailDelivery, error)
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type contractSequences interface {
	Next(ctx context.Context, docType string, periodKey string) (int, error)
}

type contractCompensationSource interface {
	SalaryAsOf(ctx context.Context, employeeID string, date time.Time) (model.SalaryRecord, error)
	LogSalaryAccess(ctx context.Context, actorID string, resourceID string, action string) error
}

type contractIdentitySource interface {
	IdentityWithAccess(ctx context.Context, employeeID string, actorID string, action string) (model.EmployeeIdentity, bool, error)
	LogIdentityAccess(ctx context.Context, actorID string, employeeID string, action string) error
	EnsureEmployeeCode(ctx context.Context, employeeID string, docCode string) (int, string, error)
}

type contractCompany interface {
	Profile(ctx context.Context) (authrepo.CompanyProfileRecord, error)
}

// ContractViewer is the caller of a contract endpoint: compensation is shown,
// prefilled and changed only with hris:salary:view; personal e-mail
// addresses are shown in full only with hris:employee_identity:view; a
// send returns the contract detail only with hris:contract:view.
type ContractViewer struct {
	DocumentViewer
	CanViewSalary   bool
	CanViewContract bool
}

type ContractsService struct {
	repo         contractsRepository
	sequences    contractSequences
	compensation contractCompensationSource
	identity     contractIdentitySource
	company      contractCompany
	queue        payslipRenderQueue
	store        payslipPDFStore
	mailer       *DocumentMailer
	encrypter    *security.Encrypter
	now          func() time.Time
}

func NewContractsService(
	repo contractsRepository,
	sequences contractSequences,
	compensation contractCompensationSource,
	identity contractIdentitySource,
	company contractCompany,
	queue payslipRenderQueue,
	store payslipPDFStore,
	mailer *DocumentMailer,
	encrypter *security.Encrypter,
) *ContractsService {
	return &ContractsService{
		repo:         repo,
		sequences:    sequences,
		compensation: compensation,
		identity:     identity,
		company:      company,
		queue:        queue,
		store:        store,
		mailer:       mailer,
		encrypter:    encrypter,
		now:          time.Now,
	}
}

// ContractAuditEntry is the metadata the handler audits (never NIK, account
// number or amounts).
type ContractAuditEntry struct {
	ContractID string
	Values     map[string]any
}

func contractAuditValues(contract model.EmploymentContract) map[string]any {
	values := map[string]any{
		"employee_id":    contract.EmployeeID,
		"contract_type":  contract.ContractType,
		"status":         contract.Status,
		"revision":       contract.Revision,
		"is_record_only": contract.IsRecordOnly,
	}
	if contract.DocNumber != nil {
		values["doc_number"] = *contract.DocNumber
	}
	if contract.NDADocNumber != nil {
		values["nda_doc_number"] = *contract.NDADocNumber
	}
	return values
}

// ---------------------------------------------------------------------------
// Snapshots

// contractPayloads is the decrypted payload_encrypted snapshot.
type contractPayloads struct {
	PKWT docgen.Payload `json:"pkwt"`
	NDA  docgen.Payload `json:"nda"`
}

func decryptContractPayloads(encrypter *security.Encrypter, contract model.EmploymentContract) (contractPayloads, error) {
	if encrypter == nil {
		return contractPayloads{}, errors.New("contract encrypter is not configured")
	}
	if contract.PayloadEncrypted == nil || strings.TrimSpace(*contract.PayloadEncrypted) == "" {
		return contractPayloads{}, ErrContractNoDocument
	}
	plain, err := encrypter.DecryptString(*contract.PayloadEncrypted)
	if err != nil {
		return contractPayloads{}, fmt.Errorf("decrypt contract payload: %w", err)
	}
	var payloads contractPayloads
	if err := json.Unmarshal([]byte(plain), &payloads); err != nil {
		return contractPayloads{}, fmt.Errorf("decode contract payload: %w", err)
	}
	if payloads.PKWT == nil || payloads.NDA == nil {
		return contractPayloads{}, errors.New("contract payload is incomplete")
	}
	return payloads, nil
}

func (s *ContractsService) encryptJSON(value any) (string, error) {
	if s.encrypter == nil {
		return "", errors.New("contract encrypter is not configured")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return s.encrypter.EncryptString(string(raw))
}

func (s *ContractsService) decryptCompensation(contract model.EmploymentContract) (model.ContractCompensation, bool, error) {
	if contract.CompensationEncrypted == nil || strings.TrimSpace(*contract.CompensationEncrypted) == "" {
		return model.ContractCompensation{}, false, nil
	}
	if s.encrypter == nil {
		return model.ContractCompensation{}, false, errors.New("contract encrypter is not configured")
	}
	plain, err := s.encrypter.DecryptString(*contract.CompensationEncrypted)
	if err != nil {
		return model.ContractCompensation{}, false, fmt.Errorf("decrypt contract compensation: %w", err)
	}
	var compensation model.ContractCompensation
	if err := json.Unmarshal([]byte(plain), &compensation); err != nil {
		return model.ContractCompensation{}, false, fmt.Errorf("decode contract compensation: %w", err)
	}
	return compensation, true, nil
}

// contractTemplateVersion records both template digests.
func contractTemplateVersion() string {
	parts := []string{}
	for _, id := range []docgen.TemplateID{docgen.TemplatePKWT, docgen.TemplateNDA} {
		tpl, err := docgen.Load(id)
		if err != nil {
			return ""
		}
		parts = append(parts, tpl.Version)
	}
	return "pkwt:" + parts[0] + ",nda:" + parts[1]
}

// ---------------------------------------------------------------------------
// Terms

func parseContractDate(raw string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, ErrContractDatesInvalid
	}
	return parsed, nil
}

func optionalSingleLine(value *string) *string {
	if value == nil {
		return nil
	}
	text := docgen.SingleLine(*value)
	if text == "" {
		return nil
	}
	return &text
}

func intOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

// normalizeContractTerms turns the form into stored terms. PKWTT and Magang
// are always record-only; a PKWT needs an end date. Compensation and the
// document date/city are handled by the caller (they keep stored values).
func normalizeContractTerms(input hrisdto.ContractFields) (hrisrepo.ContractTerms, error) {
	terms := hrisrepo.ContractTerms{
		ContractType:         strings.ToUpper(strings.TrimSpace(input.ContractType)),
		IsRecordOnly:         input.IsRecordOnly,
		JobTitle:             docgen.SingleLine(input.JobTitle),
		Department:           optionalSingleLine(input.Department),
		SupervisorName:       optionalSingleLine(input.SupervisorName),
		WorkLocation:         docgen.SingleLine(input.WorkLocation),
		WorkMode:             strings.ToLower(strings.TrimSpace(input.WorkMode)),
		WorkModeDetail:       optionalSingleLine(input.WorkModeDetail),
		PKWTBasis:            optionalSingleLine(input.PKWTBasis),
		JobDescription:       docgen.SingleLine(input.JobDescription),
		WorkDays:             contractDefaultWorkDays,
		WorkHours:            contractDefaultWorkHours,
		WeeklyHours:          intOr(input.WeeklyHours, contractDefaultWeekly),
		NoticeDays:           intOr(input.NoticeDays, contractDefaultNotice),
		IncidentReportHours:  intOr(input.IncidentReportHours, contractDefaultIncident),
		NonSolicitMonths:     intOr(input.NonSolicitMonths, contractDefaultSolicit),
		ConfidentialityYears: intOr(input.ConfidentialityYears, contractDefaultSecrecy),
		DocumentCity:         optionalSingleLine(input.DocumentCity),
	}
	switch terms.ContractType {
	case model.ContractTypePKWT:
	case model.ContractTypePKWTT, model.ContractTypeMagang:
		terms.IsRecordOnly = true
	default:
		return hrisrepo.ContractTerms{}, ErrContractTypeInvalid
	}
	if terms.JobTitle == "" {
		return hrisrepo.ContractTerms{}, ErrContractJobTitleRequired
	}
	if terms.WorkMode == "" {
		terms.WorkMode = model.ContractWorkModeWFO
	}
	if value := optionalSingleLine(input.WorkDays); value != nil {
		terms.WorkDays = *value
	}
	if value := optionalSingleLine(input.WorkHours); value != nil {
		terms.WorkHours = *value
	}

	start, err := parseContractDate(input.StartDate)
	if err != nil {
		return hrisrepo.ContractTerms{}, err
	}
	terms.StartDate = start
	if input.EndDate != nil && strings.TrimSpace(*input.EndDate) != "" {
		end, err := parseContractDate(*input.EndDate)
		if err != nil {
			return hrisrepo.ContractTerms{}, err
		}
		if end.Before(start) {
			return hrisrepo.ContractTerms{}, ErrContractDatesInvalid
		}
		terms.EndDate = &end
	}
	if terms.ContractType == model.ContractTypePKWT && terms.EndDate == nil {
		return hrisrepo.ContractTerms{}, ErrContractEndDateRequired
	}
	if input.DocumentDate != nil && strings.TrimSpace(*input.DocumentDate) != "" {
		documentDate, err := parseContractDate(*input.DocumentDate)
		if err != nil {
			return hrisrepo.ContractTerms{}, ErrContractDocumentDateInvalid
		}
		terms.DocumentDate = &documentDate
	}

	terms.Benefits = make([]model.ContractBenefit, 0, len(input.Benefits))
	for _, item := range input.Benefits {
		name := docgen.SingleLine(item.Name)
		if name == "" {
			continue
		}
		terms.Benefits = append(terms.Benefits, model.ContractBenefit{Name: name, Value: docgen.SingleLine(item.Value), Notes: docgen.SingleLine(item.Notes)})
	}
	terms.PriorWorks = make([]model.ContractPriorWork, 0, len(input.PriorWorks))
	for _, item := range input.PriorWorks {
		title := docgen.SingleLine(item.Title)
		if title == "" {
			continue
		}
		terms.PriorWorks = append(terms.PriorWorks, model.ContractPriorWork{Title: title, Description: docgen.SingleLine(item.Description), Year: strings.TrimSpace(item.Year)})
	}
	return terms, nil
}

func termsOf(contract model.EmploymentContract) hrisrepo.ContractTerms {
	return hrisrepo.ContractTerms{
		ContractType:          contract.ContractType,
		IsRecordOnly:          contract.IsRecordOnly,
		StartDate:             contract.StartDate,
		EndDate:               contract.EndDate,
		JobTitle:              contract.JobTitle,
		Department:            contract.Department,
		SupervisorName:        contract.SupervisorName,
		WorkLocation:          contract.WorkLocation,
		WorkMode:              contract.WorkMode,
		WorkModeDetail:        contract.WorkModeDetail,
		PKWTBasis:             contract.PKWTBasis,
		JobDescription:        contract.JobDescription,
		WorkDays:              contract.WorkDays,
		WorkHours:             contract.WorkHours,
		WeeklyHours:           contract.WeeklyHours,
		NoticeDays:            contract.NoticeDays,
		CompensationEncrypted: contract.CompensationEncrypted,
		Benefits:              contract.Benefits,
		IncidentReportHours:   contract.IncidentReportHours,
		NonSolicitMonths:      contract.NonSolicitMonths,
		ConfidentialityYears:  contract.ConfidentialityYears,
		PriorWorks:            contract.PriorWorks,
		DocumentDate:          contract.DocumentDate,
		DocumentCity:          contract.DocumentCity,
	}
}

func sameDate(a *time.Time, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return calendarDate(*a).Equal(calendarDate(*b))
}

// diffContractTerms lists the changed field names (for the audit; never
// values). Compensation is compared on the decrypted amounts by the caller.
func diffContractTerms(old hrisrepo.ContractTerms, next hrisrepo.ContractTerms) []string {
	changed := []string{}
	add := func(name string, different bool) {
		if different {
			changed = append(changed, name)
		}
	}
	add("contract_type", old.ContractType != next.ContractType)
	add("is_record_only", old.IsRecordOnly != next.IsRecordOnly)
	add("start_date", !calendarDate(old.StartDate).Equal(calendarDate(next.StartDate)))
	add("end_date", !sameDate(old.EndDate, next.EndDate))
	add("job_title", old.JobTitle != next.JobTitle)
	add("department", optionalText(old.Department) != optionalText(next.Department))
	add("supervisor_name", optionalText(old.SupervisorName) != optionalText(next.SupervisorName))
	add("work_location", old.WorkLocation != next.WorkLocation)
	add("work_mode", old.WorkMode != next.WorkMode)
	add("work_mode_detail", optionalText(old.WorkModeDetail) != optionalText(next.WorkModeDetail))
	add("pkwt_basis", optionalText(old.PKWTBasis) != optionalText(next.PKWTBasis))
	add("job_description", old.JobDescription != next.JobDescription)
	add("work_days", old.WorkDays != next.WorkDays)
	add("work_hours", old.WorkHours != next.WorkHours)
	add("weekly_hours", old.WeeklyHours != next.WeeklyHours)
	add("notice_days", old.NoticeDays != next.NoticeDays)
	oldBenefits, _ := json.Marshal(old.Benefits)
	nextBenefits, _ := json.Marshal(next.Benefits)
	add("benefits", string(oldBenefits) != string(nextBenefits))
	add("incident_report_hours", old.IncidentReportHours != next.IncidentReportHours)
	add("non_solicit_months", old.NonSolicitMonths != next.NonSolicitMonths)
	add("confidentiality_years", old.ConfidentialityYears != next.ConfidentialityYears)
	oldWorks, _ := json.Marshal(old.PriorWorks)
	nextWorks, _ := json.Marshal(next.PriorWorks)
	add("prior_works", string(oldWorks) != string(nextWorks))
	add("document_date", !sameDate(old.DocumentDate, next.DocumentDate))
	add("document_city", optionalText(old.DocumentCity) != optionalText(next.DocumentCity))
	return changed
}

// compensationPrefill reads the salary in force at the contract start (or
// the latest one when the start predates every salary row).
func (s *ContractsService) compensationPrefill(ctx context.Context, employeeID string, start time.Time) (*model.ContractCompensation, error) {
	salary, err := s.compensation.SalaryAsOf(ctx, employeeID, start)
	if errors.Is(err, ErrSalaryNotFound) {
		salary, err = s.compensation.SalaryAsOf(ctx, employeeID, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	}
	if errors.Is(err, ErrSalaryNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	allowance := int64(0)
	for _, value := range salary.Allowances {
		allowance += value
	}
	return &model.ContractCompensation{BaseSalary: salary.BaseSalary, FixedAllowance: allowance}, nil
}

func (s *ContractsService) encryptCompensation(value *model.ContractCompensation) (*string, error) {
	if value == nil {
		return nil, nil
	}
	cipher, err := s.encryptJSON(value)
	if err != nil {
		return nil, err
	}
	return &cipher, nil
}

// ---------------------------------------------------------------------------
// Loading

func (s *ContractsService) getContract(ctx context.Context, id string) (model.EmploymentContract, error) {
	if _, err := uuid.Parse(id); err != nil {
		return model.EmploymentContract{}, ErrContractNotFound
	}
	contract, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, hrisrepo.ErrContractNotFound) {
		return model.EmploymentContract{}, ErrContractNotFound
	}
	return contract, err
}

func (s *ContractsService) getEmployee(ctx context.Context, employeeID string) (hrisrepo.ContractEmployee, error) {
	if _, err := uuid.Parse(employeeID); err != nil {
		return hrisrepo.ContractEmployee{}, ErrEmployeeNotFound
	}
	employee, err := s.repo.GetEmployee(ctx, employeeID)
	if errors.Is(err, hrisrepo.ErrEmployeeNotFound) {
		return hrisrepo.ContractEmployee{}, ErrEmployeeNotFound
	}
	return employee, err
}

func mapContractRepoError(err error) error {
	switch {
	case errors.Is(err, hrisrepo.ErrContractNotFound):
		return ErrContractNotFound
	case errors.Is(err, hrisrepo.ErrContractStateChanged):
		return ErrContractStateChanged
	case errors.Is(err, hrisrepo.ErrContractSendInFlight):
		return ErrDocumentSendInFlight
	case errors.Is(err, hrisrepo.ErrContractNumberTaken):
		return ErrContractNumberTaken
	case errors.Is(err, hrisrepo.ErrEmployeeNotFound):
		return ErrEmployeeNotFound
	}
	return err
}

func (s *ContractsService) companyInfo(ctx context.Context) (authrepo.CompanyProfileRecord, hrisdto.ContractCompanyInfo, error) {
	record, err := s.company.Profile(ctx)
	if err != nil {
		return authrepo.CompanyProfileRecord{}, hrisdto.ContractCompanyInfo{}, err
	}
	return record, hrisdto.ContractCompanyInfo{
		LegalName:       record.LegalName,
		City:            record.City,
		DocCode:         record.DocCode,
		HRContactEmail:  record.HRContactEmail,
		CcDomain:        contractCcDomain(record.HRContactEmail),
		PaydayDay:       record.PaydayDay,
		AnnualLeaveDays: record.AnnualLeaveDays,
	}, nil
}

func (s *ContractsService) inFlight(ctx context.Context, id string) (bool, error) {
	if s.mailer == nil {
		return false, nil
	}
	return s.mailer.InFlight(ctx, model.EmailDeliveryKindContract, id)
}

func (s *ContractsService) pdfAvailable() bool {
	return s.queue != nil && s.queue.PDFAvailable()
}

// ---------------------------------------------------------------------------
// List / detail

func dateString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.Format("2006-01-02")
	return &text
}

func contractDelivery(delivery *model.EmailDelivery, reveal *personalReveal, employeeID string) *hrisdto.ContractDeliverySummary {
	if delivery == nil {
		return nil
	}
	cc := delivery.Cc
	if cc == nil {
		cc = []string{}
	}
	digests := delivery.AttachmentSHA256
	if digests == nil {
		digests = []string{}
	}
	return &hrisdto.ContractDeliverySummary{
		ID:               delivery.ID,
		Status:           delivery.Status,
		Recipient:        reveal.address(employeeID, delivery.Recipient, delivery.RecipientSource),
		RecipientSource:  delivery.RecipientSource,
		Cc:               cc,
		AttachmentSHA256: digests,
		Error:            delivery.Error,
		CreatedAt:        delivery.CreatedAt,
		SentAt:           delivery.SentAt,
	}
}

func (s *ContractsService) listItem(contract model.EmploymentContract, employeeName string, department *string, delivery *model.EmailDelivery, sentDelivery *model.EmailDelivery, reveal *personalReveal) hrisdto.ContractListItem {
	deadline, alert, expired := contractNotice(contract, jakartaToday(s.now()))
	return hrisdto.ContractListItem{
		ID:                 contract.ID,
		EmployeeID:         contract.EmployeeID,
		EmployeeName:       employeeName,
		EmployeeDepartment: department,
		ContractType:       contract.ContractType,
		IsRecordOnly:       contract.IsRecordOnly,
		Status:             contract.Status,
		Revision:           contract.Revision,
		StartDate:          contract.StartDate.Format("2006-01-02"),
		EndDate:            dateString(contract.EndDate),
		JobTitle:           contract.JobTitle,
		DocNumber:          contract.DocNumber,
		NDADocNumber:       contract.NDADocNumber,
		RenderStatus:       contract.RenderStatus,
		NoticeDays:         contract.NoticeDays,
		NoticeDeadline:     dateString(deadline),
		NoticeAlert:        alert,
		Expired:            expired,
		PreviousContractID: contract.PreviousContractID,
		SignedAt:           dateString(contract.SignedAt),
		EndedAt:            dateString(contract.EndedAt),
		LastSentAt:         contract.LastSentAt,
		LastDelivery:       contractDelivery(delivery, reveal, contract.EmployeeID),
		LastSentDelivery:   contractDelivery(sentDelivery, reveal, contract.EmployeeID),
		CreatedAt:          contract.CreatedAt,
		UpdatedAt:          contract.UpdatedAt,
	}
}

// List returns the contracts matching filter (no amounts; compensation is
// only on the detail). An employee_id that is not a UUID matches nothing.
func (s *ContractsService) List(ctx context.Context, viewer ContractViewer, filter hrisrepo.ContractListFilter) (hrisdto.ContractListResponse, error) {
	result := hrisdto.ContractListResponse{
		Items:        []hrisdto.ContractListItem{},
		PDFAvailable: s.pdfAvailable(),
	}
	if s.mailer != nil {
		ready, err := s.mailer.Ready(ctx)
		if err != nil {
			return hrisdto.ContractListResponse{}, err
		}
		result.DocumentMailReady = ready
	}
	if filter.EmployeeID != "" {
		if _, err := uuid.Parse(filter.EmployeeID); err != nil {
			return result, nil
		}
	}
	filter.ContractType = strings.ToUpper(strings.TrimSpace(filter.ContractType))
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	rows, err := s.repo.List(ctx, filter)
	if err != nil {
		return hrisdto.ContractListResponse{}, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Contract.ID)
	}
	deliveries, err := s.repo.LatestDeliveries(ctx, ids, false)
	if err != nil {
		return hrisdto.ContractListResponse{}, err
	}
	sentDeliveries, err := s.repo.LatestDeliveries(ctx, ids, true)
	if err != nil {
		return hrisdto.ContractListResponse{}, err
	}
	reveal := newPersonalReveal(viewer.DocumentViewer)
	for _, row := range rows {
		result.Items = append(result.Items, s.listItem(row.Contract, row.EmployeeName, row.EmployeeDepartment,
			deliveryOf(deliveries, row.Contract.ID), deliveryOf(sentDeliveries, row.Contract.ID), reveal))
	}
	if s.mailer != nil {
		if err := s.mailer.logReveal(ctx, reveal); err != nil {
			return hrisdto.ContractListResponse{}, err
		}
	}
	return result, nil
}

func deliveryOf(deliveries map[string]model.EmailDelivery, id string) *model.EmailDelivery {
	if found, ok := deliveries[id]; ok {
		return &found
	}
	return nil
}

func (s *ContractsService) toDetail(ctx context.Context, viewer ContractViewer, contract model.EmploymentContract, reveal *personalReveal) (hrisdto.ContractDetailResponse, error) {
	employee, err := s.getEmployee(ctx, contract.EmployeeID)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, err
	}
	_, companyInfo, err := s.companyInfo(ctx)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, err
	}
	deliveries, err := s.repo.LatestDeliveries(ctx, []string{contract.ID}, false)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, err
	}
	sentDeliveries, err := s.repo.LatestDeliveries(ctx, []string{contract.ID}, true)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, err
	}

	digests := contract.PDFSHA256
	if digests == nil {
		digests = []string{}
	}
	detail := hrisdto.ContractDetailResponse{
		ContractListItem:     s.listItem(contract, employee.FullName, employee.Department, deliveryOf(deliveries, contract.ID), deliveryOf(sentDeliveries, contract.ID), reveal),
		Department:           contract.Department,
		SupervisorName:       contract.SupervisorName,
		WorkLocation:         contract.WorkLocation,
		WorkMode:             contract.WorkMode,
		WorkModeDetail:       contract.WorkModeDetail,
		PKWTBasis:            contract.PKWTBasis,
		JobDescription:       contract.JobDescription,
		WorkDays:             contract.WorkDays,
		WorkHours:            contract.WorkHours,
		WeeklyHours:          contract.WeeklyHours,
		CompensationVisible:  viewer.CanViewSalary,
		HasCompensation:      contract.CompensationEncrypted != nil && *contract.CompensationEncrypted != "",
		Benefits:             contract.Benefits,
		IncidentReportHours:  contract.IncidentReportHours,
		NonSolicitMonths:     contract.NonSolicitMonths,
		ConfidentialityYears: contract.ConfidentialityYears,
		PriorWorks:           contract.PriorWorks,
		DocumentDate:         dateString(contract.DocumentDate),
		DocumentCity:         contract.DocumentCity,
		SeqNo:                contract.SeqNo,
		TemplateVersion:      contract.TemplateVersion,
		RenderError:          contract.RenderError,
		HasPDF:               contractPDFReady(contract),
		PDFSHA256:            digests,
		GeneratedAt:          contract.GeneratedAt,
		GeneratedBy:          contract.GeneratedBy,
		EndNotes:             contract.EndNotes,
		EmployeeStatus:       employee.EmploymentStatus,
		PDFAvailable:         s.pdfAvailable(),
		AllowDocxSend:        s.mailer != nil && s.mailer.AllowDocxSend(),
		Company:              companyInfo,
	}
	if s.mailer != nil {
		if detail.DocumentMailReady, err = s.mailer.Ready(ctx); err != nil {
			return hrisdto.ContractDetailResponse{}, err
		}
	}
	if viewer.CanViewSalary && detail.HasCompensation {
		compensation, _, err := s.decryptCompensation(contract)
		if err != nil {
			return hrisdto.ContractDetailResponse{}, err
		}
		detail.Compensation = &hrisdto.ContractCompensationResponse{BaseSalary: compensation.BaseSalary, FixedAllowance: compensation.FixedAllowance}
	}
	if contract.PreviousContractID != nil {
		previous, err := s.repo.GetByID(ctx, *contract.PreviousContractID)
		switch {
		case err == nil:
			detail.PreviousDocNumber = previous.DocNumber
			detail.PreviousEndDate = dateString(previous.EndDate)
		case !errors.Is(err, hrisrepo.ErrContractNotFound):
			return hrisdto.ContractDetailResponse{}, err
		}
	}
	renewal, err := s.repo.RenewalOf(ctx, contract.ID)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, err
	}
	if renewal != "" {
		detail.RenewedByID = &renewal
	}
	return detail, nil
}

// detailFor builds the detail for viewer: the compensation read is logged
// (fail-closed) before it is returned, and so is a personal address shown
// in full.
func (s *ContractsService) detailFor(ctx context.Context, viewer ContractViewer, contract model.EmploymentContract) (hrisdto.ContractDetailResponse, error) {
	if viewer.CanViewSalary && contract.CompensationEncrypted != nil && *contract.CompensationEncrypted != "" {
		if err := s.compensation.LogSalaryAccess(ctx, viewer.ActorID, contract.EmployeeID, contractAccessView); err != nil {
			return hrisdto.ContractDetailResponse{}, fmt.Errorf("log salary access: %w", err)
		}
	}
	reveal := newPersonalReveal(viewer.DocumentViewer)
	detail, err := s.toDetail(ctx, viewer, contract, reveal)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, err
	}
	if s.mailer != nil {
		if err := s.mailer.logReveal(ctx, reveal); err != nil {
			return hrisdto.ContractDetailResponse{}, err
		}
	}
	return detail, nil
}

// Get returns one contract with every term (compensation only with
// hris:salary:view).
func (s *ContractsService) Get(ctx context.Context, viewer ContractViewer, id string) (hrisdto.ContractDetailResponse, error) {
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, err
	}
	return s.detailFor(ctx, viewer, contract)
}

// ---------------------------------------------------------------------------
// Create / update

// Create stores a draft (or a record-only entry). The department and atasan
// default to the employee's department and its head (the company signer when
// the employee heads the department or it has none). A caller with
// hris:salary:view gets the compensation prefilled from the salary in force
// at the start date unless they give one; without it the compensation stays
// empty (the PKWT would otherwise print a salary the caller may not read).
func (s *ContractsService) Create(ctx context.Context, viewer ContractViewer, input hrisdto.CreateContractRequest) (hrisdto.ContractDetailResponse, ContractAuditEntry, error) {
	employee, err := s.getEmployee(ctx, strings.ToLower(strings.TrimSpace(input.EmployeeID)))
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	terms, err := normalizeContractTerms(input.ContractFields)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	if terms.Department == nil {
		terms.Department = optionalSingleLine(employee.Department)
	}
	if terms.SupervisorName == nil {
		terms.SupervisorName = optionalSingleLine(employee.DepartmentHead)
	}
	if terms.SupervisorName == nil {
		company, err := s.company.Profile(ctx)
		if err != nil {
			return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
		}
		if signer := docgen.SingleLine(company.SignerName); signer != "" && !strings.EqualFold(signer, docgen.SingleLine(employee.FullName)) {
			terms.SupervisorName = &signer
		}
	}

	var compensation *model.ContractCompensation
	switch {
	case input.Compensation != nil:
		if !viewer.CanViewSalary {
			return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, ErrContractCompensationForbidden
		}
		compensation = &model.ContractCompensation{BaseSalary: input.Compensation.BaseSalary, FixedAllowance: input.Compensation.FixedAllowance}
	case viewer.CanViewSalary:
		if compensation, err = s.compensationPrefill(ctx, employee.ID, terms.StartDate); err != nil {
			return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
		}
	}
	if terms.CompensationEncrypted, err = s.encryptCompensation(compensation); err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}

	created, err := s.repo.Create(ctx, hrisrepo.CreateContractParams{EmployeeID: employee.ID, Terms: terms, CreatedBy: viewer.ActorID})
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, mapContractRepoError(err)
	}
	detail, err := s.detailFor(ctx, viewer, created)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	values := contractAuditValues(created)
	values["compensation_prefilled"] = input.Compensation == nil && compensation != nil
	return detail, ContractAuditEntry{ContractID: created.ID, Values: values}, nil
}

// contractEditTarget is the status and revision an edit moves a contract
// to: a draft or generated contract becomes (or stays) a draft; a sent one
// that is not signed becomes a draft with revision+1 (it keeps its
// numbers). Anything else cannot be edited.
func contractEditTarget(contract model.EmploymentContract) (string, int, error) {
	switch contract.Status {
	case model.ContractStatusDraft, model.ContractStatusGenerated:
		return model.ContractStatusDraft, contract.Revision, nil
	case model.ContractStatusSent:
		if contract.SignedAt == nil {
			return model.ContractStatusDraft, contract.Revision + 1, nil
		}
	}
	return "", 0, ErrContractNotEditable
}

// Update replaces the terms (Edit / Revisi). It returns the changed field
// names for the audit (never values).
func (s *ContractsService) Update(ctx context.Context, viewer ContractViewer, id string, input hrisdto.UpdateContractRequest) (hrisdto.ContractDetailResponse, []string, map[string]any, error) {
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, nil, nil, err
	}
	status, revision, err := contractEditTarget(contract)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, nil, nil, err
	}
	terms, err := normalizeContractTerms(input.ContractFields)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, nil, nil, err
	}
	numbered := contract.SeqNo != nil
	if numbered && (terms.ContractType != model.ContractTypePKWT || terms.IsRecordOnly) {
		return hrisdto.ContractDetailResponse{}, nil, nil, ErrContractTypeLocked
	}
	// An omitted document date / city keeps the stored one; "" clears it
	// (Generate then uses its day and the company city). The date is fixed
	// once the numbers carry its month ("" keeps it then).
	switch {
	case input.DocumentDate == nil, numbered && strings.TrimSpace(*input.DocumentDate) == "":
		terms.DocumentDate = contract.DocumentDate
	case numbered && !sameDate(terms.DocumentDate, contract.DocumentDate):
		return hrisdto.ContractDetailResponse{}, nil, nil, ErrContractDocumentDateLocked
	}
	if input.DocumentCity == nil {
		terms.DocumentCity = contract.DocumentCity
	}

	old := termsOf(contract)
	changed := diffContractTerms(old, terms)
	terms.CompensationEncrypted = contract.CompensationEncrypted
	if input.Compensation != nil {
		if !viewer.CanViewSalary {
			return hrisdto.ContractDetailResponse{}, nil, nil, ErrContractCompensationForbidden
		}
		current, has, err := s.decryptCompensation(contract)
		if err != nil {
			return hrisdto.ContractDetailResponse{}, nil, nil, err
		}
		next := model.ContractCompensation{BaseSalary: input.Compensation.BaseSalary, FixedAllowance: input.Compensation.FixedAllowance}
		if !has || current != next {
			if terms.CompensationEncrypted, err = s.encryptCompensation(&next); err != nil {
				return hrisdto.ContractDetailResponse{}, nil, nil, err
			}
			changed = append(changed, "compensation")
		}
	}
	if len(changed) == 0 {
		detail, err := s.detailFor(ctx, viewer, contract)
		return detail, changed, nil, err
	}
	busy, err := s.inFlight(ctx, contract.ID)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, nil, nil, err
	}
	if busy {
		return hrisdto.ContractDetailResponse{}, nil, nil, ErrDocumentSendInFlight
	}

	updated, err := s.repo.UpdateTerms(ctx, contract.ID, hrisrepo.UpdateContractTermsParams{
		ExpectStatus:   contract.Status,
		ExpectRevision: contract.Revision,
		Status:         status,
		Revision:       revision,
		Terms:          terms,
	})
	if err != nil {
		return hrisdto.ContractDetailResponse{}, nil, nil, mapContractRepoError(err)
	}
	detail, err := s.detailFor(ctx, viewer, updated)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, nil, nil, err
	}
	audit := contractAuditValues(updated)
	audit["changed_fields"] = changed
	audit["previous_status"] = contract.Status
	if updated.Revision != contract.Revision {
		audit["revised"] = true
	}
	return detail, changed, audit, nil
}

// ---------------------------------------------------------------------------
// Preflight

// Preflight lists what is missing before the PKWT + NDA can be generated
// (company profile, identity, employee record, contract terms) and the
// warnings: an employee still on probation, a renewal chain above five
// years, a duration that is not whole months. The identity is decrypted only
// to check presence (no value is returned); the read is logged first.
func (s *ContractsService) Preflight(ctx context.Context, viewer ContractViewer, id string) (hrisdto.ContractPreflightResponse, error) {
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return hrisdto.ContractPreflightResponse{}, err
	}
	employee, err := s.getEmployee(ctx, contract.EmployeeID)
	if err != nil {
		return hrisdto.ContractPreflightResponse{}, err
	}
	company, companyInfo, err := s.companyInfo(ctx)
	if err != nil {
		return hrisdto.ContractPreflightResponse{}, err
	}
	chainMonths, err := s.chainMonths(ctx, contract)
	if err != nil {
		return hrisdto.ContractPreflightResponse{}, err
	}

	response := hrisdto.ContractPreflightResponse{
		ContractID:      contract.ID,
		Documents:       contractHasDocuments(contract),
		Missing:         []hrisdto.ContractMissingField{},
		Warnings:        contractWarnings(contract, employee.EmploymentStatus, chainMonths),
		ChainPKWTMonths: chainMonths,
		EmployeeStatus:  employee.EmploymentStatus,
		Company:         companyInfo,
	}
	if response.Documents {
		identity, _, err := s.identity.IdentityWithAccess(ctx, contract.EmployeeID, viewer.ActorID, IdentityAccessPreflight)
		if err != nil {
			return hrisdto.ContractPreflightResponse{}, mapEmployeeError(err)
		}
		response.Missing = contractMissingFields(contractCompleteness{
			Contract:        contract,
			Employee:        employee,
			Identity:        identity,
			Company:         company,
			HasCompensation: contract.CompensationEncrypted != nil && *contract.CompensationEncrypted != "",
		})
	}
	response.Ready = response.Documents && len(response.Missing) == 0
	return response, nil
}

// chainMonths is the cumulative PKWT duration of contract's chain: its
// Perpanjang links and the contiguous PKWT entries before them (record-only
// history included).
func (s *ContractsService) chainMonths(ctx context.Context, contract model.EmploymentContract) (float64, error) {
	links, err := s.repo.ChainLinks(ctx, contract.EmployeeID)
	if err != nil {
		return 0, err
	}
	return chainPKWTMonths(contractChain(contract.ID, links)), nil
}

func contractHasDocuments(contract model.EmploymentContract) bool {
	return contract.ContractType == model.ContractTypePKWT && !contract.IsRecordOnly
}

// ---------------------------------------------------------------------------
// Generate

func (s *ContractsService) newRenderStatus() string {
	if s.pdfAvailable() {
		return model.DocumentRenderPending
	}
	return model.DocumentRenderNone
}

func (s *ContractsService) enqueue(ctx context.Context, contract model.EmploymentContract) {
	if s.queue == nil || contract.RenderStatus != model.DocumentRenderPending {
		return
	}
	info, ok := tenant.FromContext(ctx)
	if !ok {
		// The sweep picks the row up (render_status 'pending').
		return
	}
	s.queue.Enqueue(DocumentJob{Tenant: info, Kind: ContractDocumentKind, ID: contract.ID})
}

// Generate checks the contract is complete, assigns the shared EMPLOYMENT
// number once (kept by every revision), snapshots both payloads (the PKWT
// with the full NIK and account number) and queues the PDF render. The
// payloads are built from the contract row locked in the generate
// transaction, so an edit racing the generate either lands first (and is in
// the snapshot) or waits and is refused (CONTRACT_STATE_CHANGED).
func (s *ContractsService) Generate(ctx context.Context, viewer ContractViewer, id string) (hrisdto.ContractDetailResponse, ContractAuditEntry, error) {
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	if err := contractGeneratable(contract); err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	busy, err := s.inFlight(ctx, contract.ID)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	if busy {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, ErrDocumentSendInFlight
	}

	employee, err := s.getEmployee(ctx, contract.EmployeeID)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	company, err := s.company.Profile(ctx)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	identity, _, err := s.identity.IdentityWithAccess(ctx, contract.EmployeeID, viewer.ActorID, IdentityAccessDocument)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, mapEmployeeError(err)
	}
	if err := s.checkComplete(contract, employee, identity, company); err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	// The employee code is assigned lazily, at the first slip or contract.
	if _, _, err := s.identity.EnsureEmployeeCode(ctx, contract.EmployeeID, company.DocCode); err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, mapEmployeeError(err)
	}

	firstNumbering := false
	var generated model.EmploymentContract
	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		locked, err := s.repo.GetForUpdate(txCtx, contract.ID)
		if err != nil {
			return err
		}
		if err := contractGeneratable(locked); err != nil {
			return err
		}
		if err := s.checkComplete(locked, employee, identity, company); err != nil {
			return err
		}
		compensation, _, err := s.decryptCompensation(locked)
		if err != nil {
			return err
		}
		city := optionalText(locked.DocumentCity)
		if city == "" {
			city = docgen.SingleLine(company.City)
		}
		firstNumbering = locked.SeqNo == nil
		seq, docNumber, ndaNumber := 0, optionalText(locked.DocNumber), optionalText(locked.NDADocNumber)
		documentDate := jakartaToday(s.now())
		if locked.DocumentDate != nil {
			documentDate = calendarDate(*locked.DocumentDate)
		}
		if firstNumbering {
			next, err := s.sequences.Next(txCtx, hrisrepo.DocSequenceEmployment, contractPeriodKey(documentDate))
			if err != nil {
				return fmt.Errorf("next contract number: %w", err)
			}
			seq = next
			docNumber, ndaNumber = FormatContractNumbers(seq, company.DocCode, documentDate)
		} else {
			seq = *locked.SeqNo
		}
		pkwt, nda := buildContractPayloads(contractPayloadInput{
			Contract:     locked,
			Employee:     employee,
			Identity:     identity,
			Company:      company,
			Compensation: compensation,
			DocNumber:    docNumber,
			NDADocNumber: ndaNumber,
			DocumentDate: documentDate,
			DocumentCity: city,
		})
		payloadCipher, err := s.encryptJSON(contractPayloads{PKWT: pkwt, NDA: nda})
		if err != nil {
			return err
		}
		generated, err = s.repo.RecordSnapshot(txCtx, locked.ID, hrisrepo.ContractSnapshotParams{
			ExpectStatus:     locked.Status,
			ExpectRevision:   locked.Revision,
			SeqNo:            seq,
			DocNumber:        docNumber,
			NDADocNumber:     ndaNumber,
			DocumentDate:     documentDate,
			DocumentCity:     city,
			PayloadEncrypted: payloadCipher,
			TemplateVersion:  contractTemplateVersion(),
			GeneratedBy:      viewer.ActorID,
			RenderStatus:     s.newRenderStatus(),
		})
		return err
	})
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, mapContractRepoError(err)
	}
	s.enqueue(ctx, generated)

	detail, err := s.detailFor(ctx, viewer, generated)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	values := contractAuditValues(generated)
	values["numbers_assigned"] = firstNumbering
	values["render_status"] = generated.RenderStatus
	if generated.DocumentDate != nil {
		values["document_date"] = generated.DocumentDate.Format("2006-01-02")
	}
	return detail, ContractAuditEntry{ContractID: generated.ID, Values: values}, nil
}

// contractGeneratable: only a PKWT with documents, still a draft or
// generated, can be (re)generated.
func contractGeneratable(contract model.EmploymentContract) error {
	if !contractHasDocuments(contract) {
		return ErrContractRecordOnly
	}
	if contract.Status != model.ContractStatusDraft && contract.Status != model.ContractStatusGenerated {
		return ErrContractNotGeneratable
	}
	return nil
}

// checkComplete refuses a generation while required data is missing.
func (s *ContractsService) checkComplete(contract model.EmploymentContract, employee hrisrepo.ContractEmployee, identity model.EmployeeIdentity, company authrepo.CompanyProfileRecord) error {
	if missing := contractMissingFields(contractCompleteness{
		Contract:        contract,
		Employee:        employee,
		Identity:        identity,
		Company:         company,
		HasCompensation: contract.CompensationEncrypted != nil && strings.TrimSpace(*contract.CompensationEncrypted) != "",
	}); len(missing) > 0 {
		return &ContractIncompleteError{Missing: missing}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Renew (Perpanjang)

// Renew clones a signed or ended contract into a linked draft that starts
// the day after it ends, with the same terms and duration. Numbers, dates of
// signing and document date are not copied; the renewal of a record-only
// PKWT is a PKWT with documents. The predecessor row is locked while the
// renewal is created, so two concurrent renewals cannot fork the chain. The
// 5-year check is in the preflight (and in the audit entry).
func (s *ContractsService) Renew(ctx context.Context, viewer ContractViewer, id string) (hrisdto.ContractDetailResponse, ContractAuditEntry, error) {
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	if err := contractRenewable(contract); err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}

	var created model.EmploymentContract
	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		locked, err := s.repo.GetForUpdate(txCtx, contract.ID)
		if err != nil {
			return err
		}
		if err := contractRenewable(locked); err != nil {
			return err
		}
		existing, err := s.repo.RenewalOf(txCtx, locked.ID)
		if err != nil {
			return err
		}
		if existing != "" {
			return ErrContractAlreadyRenewed
		}
		contract = locked
		terms := termsOf(locked)
		start, end := renewalDates(locked.StartDate, *locked.EndDate)
		terms.StartDate, terms.EndDate = start, &end
		terms.DocumentDate, terms.DocumentCity = nil, nil
		if terms.ContractType == model.ContractTypePKWT {
			terms.IsRecordOnly = false
		}
		previous := locked.ID
		created, err = s.repo.Create(txCtx, hrisrepo.CreateContractParams{
			EmployeeID:         locked.EmployeeID,
			PreviousContractID: &previous,
			Terms:              terms,
			CreatedBy:          viewer.ActorID,
		})
		return err
	})
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, mapContractRepoError(err)
	}
	months, err := s.chainMonths(ctx, created)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	detail, err := s.detailFor(ctx, viewer, created)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	values := contractAuditValues(created)
	values["renews"] = contract.ID
	if contract.DocNumber != nil {
		values["renews_doc_number"] = *contract.DocNumber
	}
	values["chain_pkwt_months"] = months
	if months > contractChainLimitMonths {
		values["chain_over_5_years"] = true
	}
	return detail, ContractAuditEntry{ContractID: created.ID, Values: values}, nil
}

// contractRenewable: a contract with an end date that is signed or ended (a
// sent contract is still unsigned and editable: its dates may move).
func contractRenewable(contract model.EmploymentContract) error {
	switch contract.Status {
	case model.ContractStatusSigned, model.ContractStatusEnded:
	default:
		return ErrContractNotRenewable
	}
	if contract.EndDate == nil {
		return ErrContractNotRenewable
	}
	return nil
}

// ---------------------------------------------------------------------------
// Status (Tandai Ditandatangani, Akhiri, Batalkan)

// contractStatusSources lists the statuses a contract may move to target
// from. Documents are signed after they were generated or sent; a
// record-only entry can be marked signed or ended directly.
func contractStatusSources(contract model.EmploymentContract, target string) []string {
	recordOnly := contract.IsRecordOnly
	switch target {
	case model.ContractStatusSigned:
		if recordOnly {
			return []string{model.ContractStatusDraft}
		}
		return []string{model.ContractStatusGenerated, model.ContractStatusSent}
	case model.ContractStatusEnded:
		if recordOnly {
			return []string{model.ContractStatusDraft, model.ContractStatusSigned}
		}
		return []string{model.ContractStatusSigned}
	case model.ContractStatusCancelled:
		if recordOnly {
			return []string{model.ContractStatusDraft}
		}
		return []string{model.ContractStatusDraft, model.ContractStatusGenerated, model.ContractStatusSent}
	}
	return nil
}

func (s *ContractsService) statusDate(raw *string) (*time.Time, error) {
	today := jakartaToday(s.now())
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return &today, nil
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*raw))
	if err != nil || parsed.After(today) || parsed.Year() < 2000 {
		return nil, ErrContractStatusDateInvalid
	}
	return &parsed, nil
}

// SetStatus records signed (signed_at, default today), ended (ended_at,
// default today, not before signed_at or, unsigned, start_date; optional end
// notes) or cancelled. A generated contract is marked signed only once its
// documents exist (PDFs ready, or DOCX-only on a server without converter):
// the PDF worker never renders a signed contract. The checks run on the row
// locked for the update, so a regenerate cannot slip in between.
func (s *ContractsService) SetStatus(ctx context.Context, viewer ContractViewer, id string, input hrisdto.UpdateContractStatusRequest) (hrisdto.ContractDetailResponse, ContractAuditEntry, error) {
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	target := strings.ToLower(strings.TrimSpace(input.Status))
	if err := s.statusAllowed(contract, target); err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	var signedAt, endedAt, inputSignedAt *time.Time
	var endNotes *string
	switch target {
	case model.ContractStatusSigned:
		if signedAt, err = s.statusDate(input.SignedAt); err != nil {
			return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
		}
	case model.ContractStatusEnded:
		if endedAt, err = s.statusDate(input.EndedAt); err != nil {
			return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
		}
		if contract.IsRecordOnly && contract.SignedAt == nil && input.SignedAt != nil {
			if inputSignedAt, err = s.statusDate(input.SignedAt); err != nil {
				return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
			}
		}
	}
	if target != model.ContractStatusSigned && input.EndNotes != nil {
		if notes := strings.TrimSpace(*input.EndNotes); notes != "" {
			endNotes = &notes
		}
	}
	busy, err := s.inFlight(ctx, contract.ID)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	if busy {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, ErrDocumentSendInFlight
	}

	var updated model.EmploymentContract
	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		locked, err := s.repo.GetForUpdate(txCtx, contract.ID)
		if err != nil {
			return err
		}
		if err := s.statusAllowed(locked, target); err != nil {
			return err
		}
		contract = locked
		if target == model.ContractStatusEnded {
			// A record-only entry may get its signing date with Akhiri.
			if locked.IsRecordOnly && locked.SignedAt == nil {
				signedAt = inputSignedAt
			}
			floor := calendarDate(locked.StartDate)
			switch {
			case locked.SignedAt != nil:
				floor = calendarDate(*locked.SignedAt)
			case signedAt != nil:
				floor = *signedAt
			}
			if endedAt.Before(floor) {
				return ErrContractStatusDateInvalid
			}
		}
		updated, err = s.repo.SetStatus(txCtx, locked.ID, contractStatusSources(locked, target), target, signedAt, endedAt, endNotes)
		return err
	})
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, mapContractRepoError(err)
	}
	detail, err := s.detailFor(ctx, viewer, updated)
	if err != nil {
		return hrisdto.ContractDetailResponse{}, ContractAuditEntry{}, err
	}
	values := contractAuditValues(updated)
	values["previous_status"] = contract.Status
	if signedAt != nil {
		values["signed_at"] = signedAt.Format("2006-01-02")
	}
	if endedAt != nil {
		values["ended_at"] = endedAt.Format("2006-01-02")
	}
	if endNotes != nil {
		values["end_notes"] = true
	}
	return detail, ContractAuditEntry{ContractID: updated.ID, Values: values}, nil
}

// statusAllowed checks contract may move to target now.
func (s *ContractsService) statusAllowed(contract model.EmploymentContract, target string) error {
	sources := contractStatusSources(contract, target)
	if len(sources) == 0 || !containsStatus(sources, contract.Status) {
		return ErrContractStatusTransition
	}
	if target == model.ContractStatusSigned && contract.Status == model.ContractStatusGenerated && !s.documentsIssued(contract) {
		return ErrContractSignBeforePDF
	}
	return nil
}

// documentsIssued: the generated documents exist (both PDFs ready, or the
// DOCX snapshot on a server without a PDF converter).
func (s *ContractsService) documentsIssued(contract model.EmploymentContract) bool {
	if !contractHasDocuments(contract) {
		return true
	}
	if contractPDFReady(contract) {
		return true
	}
	return !s.pdfAvailable() && contract.RenderStatus == model.DocumentRenderNone && contract.PayloadEncrypted != nil
}

func containsStatus(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Files

// ContractFile is a document streamed to the client.
type ContractFile struct {
	Data        []byte
	Filename    string
	ContentType string
	Contract    model.EmploymentContract
	Part        string
}

func contractPDFReady(contract model.EmploymentContract) bool {
	return contract.RenderStatus == model.DocumentRenderReady && contract.PKWTPDFPath != nil && contract.NDAPDFPath != nil && len(contract.PDFSHA256) == 2
}

// asciiName keeps letters and digits, joining the rest with '_' (safe in
// Content-Disposition and MIME parameters).
func asciiName(value string, max int) string {
	var name strings.Builder
	lastUnderscore := false
	for _, char := range docgen.SingleLine(value) {
		switch {
		case (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9'):
			name.WriteRune(char)
			lastUnderscore = false
		default:
			if !lastUnderscore && name.Len() > 0 {
				name.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	cleaned := strings.Trim(name.String(), "_")
	if len(cleaned) > max {
		cleaned = strings.Trim(cleaned[:max], "_")
	}
	return cleaned
}

// contractFilename: "PKWT_001-PKWT-CTN-X-2026_Budi_Santoso" or
// "NDA-HKI_001-NDA-HKI-CTN-X-2026_Budi_Santoso" (+ "_Revisi1").
func contractFilename(contract model.EmploymentContract, part string, employeeName string) string {
	prefix, number := "PKWT", optionalText(contract.DocNumber)
	if part == ContractPartNDA {
		prefix, number = "NDA-HKI", optionalText(contract.NDADocNumber)
	}
	filename := prefix
	if cleaned := strings.ReplaceAll(asciiName(strings.ReplaceAll(number, "/", " "), 60), "_", "-"); cleaned != "" {
		filename += "_" + cleaned
	}
	if name := asciiName(employeeName, 60); name != "" {
		filename += "_" + name
	}
	if contract.Revision > 0 {
		filename += fmt.Sprintf("_Revisi%d", contract.Revision)
	}
	return filename
}

func contractPartIndex(part string) (int, docgen.TemplateID, error) {
	switch strings.ToLower(strings.TrimSpace(part)) {
	case ContractPartPKWT:
		return 0, docgen.TemplatePKWT, nil
	case ContractPartNDA:
		return 1, docgen.TemplateNDA, nil
	}
	return 0, "", ErrContractPartInvalid
}

func (s *ContractsService) pdfReady(contract model.EmploymentContract) error {
	if contractPDFReady(contract) {
		return nil
	}
	if !contractHasDocuments(contract) {
		return ErrContractRecordOnly
	}
	if !s.pdfAvailable() {
		return ErrDocumentPDFConverterMissing
	}
	return ErrContractPDFNotReady
}

func (s *ContractsService) readPDF(ctx context.Context, contract model.EmploymentContract, index int) ([]byte, error) {
	if err := s.pdfReady(contract); err != nil {
		return nil, err
	}
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return nil, errors.New("tenant is missing from context")
	}
	path := *contract.PKWTPDFPath
	if index == 1 {
		path = *contract.NDAPDFPath
	}
	return s.store.Read(info.ID, path, contract.PDFSHA256[index])
}

// logDocumentAccess writes the access rows of a contract document read
// (fail-closed, before any byte leaves): identity (the PKWT carries the full
// NIK and account number, the NDA the masked NIK) and, when the PKWT is
// included, salary (it prints the compensation).
func (s *ContractsService) logDocumentAccess(ctx context.Context, actorID string, contract model.EmploymentContract, action string, includesPKWT bool) error {
	if err := s.identity.LogIdentityAccess(ctx, actorID, contract.EmployeeID, action); err != nil {
		return fmt.Errorf("log identity access: %w", err)
	}
	if !includesPKWT {
		return nil
	}
	if err := s.compensation.LogSalaryAccess(ctx, actorID, contract.EmployeeID, action); err != nil {
		return fmt.Errorf("log salary access: %w", err)
	}
	return nil
}

func (s *ContractsService) employeeName(ctx context.Context, contract model.EmploymentContract) string {
	if employee, err := s.repo.GetEmployee(ctx, contract.EmployeeID); err == nil {
		return employee.FullName
	}
	return ""
}

// PDF returns one stored PDF (part pkwt or nda). The access rows are
// written before the file is read.
func (s *ContractsService) PDF(ctx context.Context, actorID string, id string, part string, disposition string) (ContractFile, error) {
	index, _, err := contractPartIndex(part)
	if err != nil {
		return ContractFile{}, err
	}
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return ContractFile{}, err
	}
	if err := s.pdfReady(contract); err != nil {
		return ContractFile{}, err
	}
	part = []string{ContractPartPKWT, ContractPartNDA}[index]
	if err := s.logDocumentAccess(ctx, actorID, contract, fmt.Sprintf("%s_%s_pdf_%s", contractAccessDocument, part, disposition), part == ContractPartPKWT); err != nil {
		return ContractFile{}, err
	}
	data, err := s.readPDF(ctx, contract, index)
	if err != nil {
		return ContractFile{}, err
	}
	return ContractFile{
		Data:        data,
		Filename:    contractFilename(contract, part, s.employeeName(ctx, contract)) + ".pdf",
		ContentType: contentTypePDF,
		Contract:    contract,
		Part:        part,
	}, nil
}

func renderContractDocx(encrypter *security.Encrypter, contract model.EmploymentContract, index int) ([]byte, error) {
	payloads, err := decryptContractPayloads(encrypter, contract)
	if err != nil {
		return nil, err
	}
	_, templateID, _ := contractPartIndex([]string{ContractPartPKWT, ContractPartNDA}[index])
	tpl, err := docgen.Load(templateID)
	if err != nil {
		return nil, err
	}
	payload := payloads.PKWT
	if index == 1 {
		payload = payloads.NDA
	}
	return docgen.Render(tpl, payload)
}

// Docx re-renders one document from the encrypted snapshot (HR download; no
// soffice involved). Only a generated, sent, signed or ended contract has a
// snapshot of its current terms: a draft (edited after its last generate) or
// a cancelled one is refused. Access rows first.
func (s *ContractsService) Docx(ctx context.Context, actorID string, id string, part string) (ContractFile, error) {
	index, _, err := contractPartIndex(part)
	if err != nil {
		return ContractFile{}, err
	}
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return ContractFile{}, err
	}
	if !contractHasDocuments(contract) {
		return ContractFile{}, ErrContractRecordOnly
	}
	switch contract.Status {
	case model.ContractStatusGenerated, model.ContractStatusSent, model.ContractStatusSigned, model.ContractStatusEnded:
	default:
		return ContractFile{}, ErrContractDocumentNotCurrent
	}
	if contract.PayloadEncrypted == nil {
		return ContractFile{}, ErrContractNoDocument
	}
	part = []string{ContractPartPKWT, ContractPartNDA}[index]
	if err := s.logDocumentAccess(ctx, actorID, contract, fmt.Sprintf("%s_%s_docx", contractAccessDocument, part), part == ContractPartPKWT); err != nil {
		return ContractFile{}, err
	}
	data, err := renderContractDocx(s.encrypter, contract, index)
	if err != nil {
		return ContractFile{}, err
	}
	return ContractFile{
		Data:        data,
		Filename:    contractFilename(contract, part, s.employeeName(ctx, contract)) + ".docx",
		ContentType: contentTypeDocx,
		Contract:    contract,
		Part:        part,
	}, nil
}

// ---------------------------------------------------------------------------
// Sending

func contractSendable(contract model.EmploymentContract) error {
	if !contractHasDocuments(contract) {
		return ErrContractRecordOnly
	}
	switch contract.Status {
	case model.ContractStatusGenerated, model.ContractStatusSent, model.ContractStatusSigned, model.ContractStatusEnded:
	default:
		return ErrContractNotSendable
	}
	if contract.PayloadEncrypted == nil || contract.DocNumber == nil {
		return ErrContractNotSendable
	}
	return nil
}

// attachments returns both files e-mailed with a contract: the stored PDFs,
// or — only with DOCUMENTS_ALLOW_DOCX_SEND and no PDF converter — the
// DOCX files.
func (s *ContractsService) attachments(ctx context.Context, contract model.EmploymentContract, employeeName string) ([]mail.Attachment, string, error) {
	parts := []string{ContractPartPKWT, ContractPartNDA}
	if contractPDFReady(contract) {
		out := make([]mail.Attachment, 0, 2)
		for index, part := range parts {
			data, err := s.readPDF(ctx, contract, index)
			if err != nil {
				return nil, "", err
			}
			out = append(out, mail.Attachment{Filename: contractFilename(contract, part, employeeName) + ".pdf", ContentType: contentTypePDF, Data: data})
		}
		return out, "pdf", nil
	}
	if !s.pdfAvailable() {
		if s.mailer != nil && s.mailer.AllowDocxSend() {
			out := make([]mail.Attachment, 0, 2)
			for index, part := range parts {
				data, err := renderContractDocx(s.encrypter, contract, index)
				if err != nil {
					return nil, "", err
				}
				out = append(out, mail.Attachment{Filename: contractFilename(contract, part, employeeName) + ".docx", ContentType: contentTypeDocx, Data: data})
			}
			return out, "docx", nil
		}
		return nil, "", ErrDocumentPDFConverterMissing
	}
	return nil, "", ErrContractPDFNotReady
}

// contractSubject: "Kontrak Kerja (PKWT) & NDA/HKI — 001/PKWT/CTN/X/2026",
// plus " (Revisi n)" for a revised contract.
func contractSubject(contract model.EmploymentContract) string {
	subject := fmt.Sprintf("Kontrak Kerja (PKWT) & NDA/HKI — %s", optionalText(contract.DocNumber))
	if contract.Revision > 0 {
		subject += fmt.Sprintf(" (Revisi %d)", contract.Revision)
	}
	return subject
}

func (s *ContractsService) documentMail(ctx context.Context, contract model.EmploymentContract, recipient ResolvedRecipient, cc []string, attachments []mail.Attachment, actorID string) (DocumentMail, error) {
	pkwt, nda := optionalText(contract.DocNumber), optionalText(contract.NDADocNumber)
	paragraphsID := []string{
		fmt.Sprintf("Terlampir Perjanjian Kerja Waktu Tertentu (PKWT) No. %s beserta Perjanjian Kerahasiaan dan Pengalihan Hak Kekayaan Intelektual No. %s untuk Anda pelajari.", pkwt, nda),
		"Mohon periksa kedua dokumen. Penandatanganan dilakukan sesuai arahan HR.",
	}
	paragraphsEN := []string{
		fmt.Sprintf("Please find attached your Fixed-Term Employment Agreement (PKWT) No. %s together with the Confidentiality and Intellectual Property Assignment Agreement No. %s for your review.", pkwt, nda),
		"Please review both documents. Signing follows HR's instructions.",
	}
	if contract.Revision > 0 {
		paragraphsID = append(paragraphsID, fmt.Sprintf("Dokumen ini adalah revisi ke-%d dan menggantikan versi yang dikirim sebelumnya; nomor dokumen tetap sama.", contract.Revision))
		paragraphsEN = append(paragraphsEN, fmt.Sprintf("These documents are revision %d and replace the version sent earlier; the document numbers stay the same.", contract.Revision))
	}
	company, err := s.mailer.HRContact(ctx)
	if err != nil {
		return DocumentMail{}, err
	}
	if contact := strings.TrimSpace(company.HRContactEmail); contact != "" {
		paragraphsID = append(paragraphsID, fmt.Sprintf("Dokumen ini bersifat rahasia dan hanya ditujukan kepada Anda. Bila ada pertanyaan, silakan hubungi HR di %s.", contact))
		paragraphsEN = append(paragraphsEN, fmt.Sprintf("These documents are confidential and intended for you only. If you have any questions, please contact HR at %s.", contact))
	} else {
		paragraphsID = append(paragraphsID, "Dokumen ini bersifat rahasia dan hanya ditujukan kepada Anda. Bila ada pertanyaan, silakan hubungi bagian HR.")
		paragraphsEN = append(paragraphsEN, "These documents are confidential and intended for you only. If you have any questions, please contact HR.")
	}
	return DocumentMail{
		Kind:          model.EmailDeliveryKindContract,
		ReferenceType: model.EmailDeliveryKindContract,
		ReferenceID:   contract.ID,
		Recipient:     recipient,
		Cc:            cc,
		Subject:       contractSubject(contract),
		ParagraphsID:  paragraphsID,
		ParagraphsEN:  paragraphsEN,
		Attachments:   attachments,
		RequestedBy:   actorID,
	}, nil
}

// Recipient previews where a contract would be sent for source (the
// recipient picker), with the 'alamat baru' flag and the CC domain.
func (s *ContractsService) Recipient(ctx context.Context, viewer ContractViewer, id string, source string) (hrisdto.ContractRecipientResponse, error) {
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return hrisdto.ContractRecipientResponse{}, err
	}
	recipient, err := s.mailer.ResolveRecipient(ctx, contract.EmployeeID, source)
	if err != nil {
		return hrisdto.ContractRecipientResponse{}, err
	}
	company, err := s.company.Profile(ctx)
	if err != nil {
		return hrisdto.ContractRecipientResponse{}, err
	}
	reveal := newPersonalReveal(viewer.DocumentViewer)
	response := hrisdto.ContractRecipientResponse{
		ContractID:        contract.ID,
		EmployeeID:        contract.EmployeeID,
		EmployeeName:      recipient.EmployeeName,
		DocNumber:         optionalText(contract.DocNumber),
		Recipient:         reveal.address(contract.EmployeeID, recipient.Address, recipient.Source),
		RecipientSource:   recipient.Source,
		Linked:            recipient.Linked,
		PersonalAvailable: recipient.PersonalAvailable,
		HasPrevious:       recipient.HasPrevious,
		PreviousRecipient: reveal.address(contract.EmployeeID, recipient.PreviousAddress, recipient.PreviousSource),
		IsNew:             recipient.IsNew,
		CcDomain:          contractCcDomain(company.HRContactEmail),
	}
	if err := s.mailer.logReveal(ctx, reveal); err != nil {
		return hrisdto.ContractRecipientResponse{}, err
	}
	return response, nil
}

// ContractSendResult is a synchronous send outcome plus its audit metadata.
type ContractSendResult struct {
	Response hrisdto.ContractSendResponse
	Audit    map[string]any
}

func contractSentSnapshot(contract model.EmploymentContract, format string) hrisrepo.ContractSentSnapshot {
	sent := hrisrepo.ContractSentSnapshot{Revision: contract.Revision, PayloadEncrypted: optionalText(contract.PayloadEncrypted)}
	if format == "pdf" {
		sent.PDFSHA256 = contract.PDFSHA256
	}
	return sent
}

// verifyQueued re-reads the contract once its queued delivery exists (from
// then on it cannot be edited or regenerated): it must still be the
// snapshot the attachments were built from.
func (s *ContractsService) verifyQueued(ctx context.Context, contract model.EmploymentContract, format string) error {
	current, err := s.repo.GetByID(ctx, contract.ID)
	if errors.Is(err, hrisrepo.ErrContractNotFound) {
		return ErrContractStateChanged
	}
	if err != nil {
		return err
	}
	if contractSendable(current) != nil || current.Revision != contract.Revision ||
		optionalText(current.PayloadEncrypted) != optionalText(contract.PayloadEncrypted) {
		return ErrContractStateChanged
	}
	if format == "pdf" && strings.Join(current.PDFSHA256, ",") != strings.Join(contract.PDFSHA256, ",") {
		return ErrContractStateChanged
	}
	return nil
}

// Send e-mails the PKWT and the NDA/HKI together in ONE message (Kirim /
// Kirim ulang), synchronously. CC addresses must be on the domain of the
// company HR contact e-mail (never a public webmail domain); the handler
// allows CC only to callers who may read the documents themselves. The queued delivery row is created first (the
// double-send guard, which also freezes the contract); a revised contract's
// subject carries '(Revisi n)'. On success the contract becomes 'sent'
// unless it is already signed.
func (s *ContractsService) Send(ctx context.Context, viewer ContractViewer, id string, input hrisdto.SendContractRequest) (ContractSendResult, error) {
	contract, err := s.getContract(ctx, id)
	if err != nil {
		return ContractSendResult{}, err
	}
	if err := contractSendable(contract); err != nil {
		return ContractSendResult{}, err
	}
	if err := s.mailer.requireReady(ctx); err != nil {
		return ContractSendResult{}, err
	}
	recipient, err := s.mailer.ResolveRecipient(ctx, contract.EmployeeID, input.RecipientSource)
	if err != nil {
		return ContractSendResult{}, err
	}
	if err := viewer.guardSend(recipient, input.ExpectedRecipient); err != nil {
		return ContractSendResult{}, err
	}
	// A cc mailbox gets the full NIK, account number and compensation, and
	// the caller picks it: not something an AI client may add.
	if viewer.ViaMCP && len(input.Cc) > 0 {
		return ContractSendResult{}, ErrDocumentCcRestricted
	}
	company, err := s.company.Profile(ctx)
	if err != nil {
		return ContractSendResult{}, err
	}
	cc, err := normalizeContractCc(input.Cc, contractCcDomain(company.HRContactEmail), recipient.Address)
	if err != nil {
		return ContractSendResult{}, err
	}
	attachments, format, err := s.attachments(ctx, contract, recipient.EmployeeName)
	if err != nil {
		return ContractSendResult{}, err
	}
	// The PKWT carries the full NIK, account number and compensation.
	if err := s.logDocumentAccess(ctx, viewer.ActorID, contract, contractAccessSend, true); err != nil {
		return ContractSendResult{}, err
	}
	doc, err := s.documentMail(ctx, contract, recipient, cc, attachments, viewer.ActorID)
	if err != nil {
		return ContractSendResult{}, err
	}

	queued, err := s.mailer.Enqueue(ctx, doc)
	if err != nil {
		return ContractSendResult{}, err
	}
	if err := s.verifyQueued(ctx, contract, format); err != nil {
		s.mailer.CancelQueued(ctx, queued.Delivery.ID, err.Error())
		return ContractSendResult{}, err
	}

	delivery, sendErr := s.mailer.DeliverQueued(ctx, queued)
	audit := viewer.auditValues(contractAuditValues(contract))
	audit["format"] = format
	audit["recipient"] = MaskEmail(recipient.Address)
	audit["recipient_source"] = recipient.Source
	audit["attachments"] = len(attachments)
	if recipient.IsNew {
		audit["new_address"] = true
	}
	if len(cc) > 0 {
		masked := make([]string, 0, len(cc))
		for _, address := range cc {
			masked = append(masked, MaskEmail(address))
		}
		audit["cc"] = masked
	}
	result := ContractSendResult{Audit: audit}
	if sendErr != nil {
		classified, ok := mail.AsSendError(sendErr)
		if !ok || delivery.ID == "" {
			return ContractSendResult{}, sendErr
		}
		category, message := string(classified.Category), classified.Message()
		result.Response.ErrorCategory = &category
		result.Response.ErrorMessage = &message
		audit["status"] = model.EmailDeliveryStatusFailed
		audit["error_category"] = category
	} else {
		result.Response.Sent = true
		audit["status"] = model.EmailDeliveryStatusSent
		updated, err := s.repo.MarkSent(ctx, contract.ID, s.now(), contractSentSnapshot(contract, format))
		switch {
		case err == nil:
			contract = updated
		case errors.Is(err, hrisrepo.ErrContractStateChanged):
			// Impossible short of a bug (the queued row froze the
			// contract); logged, never silently ignored.
			slog.ErrorContext(ctx, "contract e-mailed but the contract no longer matches the sent snapshot", "contract_id", contract.ID)
		default:
			return ContractSendResult{}, err
		}
	}
	audit["delivery_id"] = delivery.ID

	reveal := newPersonalReveal(viewer.DocumentViewer)
	delivery.Recipient = reveal.address(contract.EmployeeID, delivery.Recipient, delivery.RecipientSource)
	result.Response.Delivery = delivery
	// The detail only for a caller who may read it (hris:contract:view).
	if viewer.CanViewContract {
		detail, err := s.toDetail(ctx, viewer, contract, reveal)
		if err != nil {
			return ContractSendResult{}, err
		}
		result.Response.Contract = &detail
	}
	if err := s.mailer.logReveal(ctx, reveal); err != nil {
		return ContractSendResult{}, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Payslip hook

// ActiveContractForPayslip implements PayslipContractSource: the contract in
// force during the month that ends on periodEnd (status sent or signed)
// gives the slip its status kerja ("PKWT", "PKWTT", "Magang") and jabatan,
// and becomes payslips.contract_id.
func (s *ContractsService) ActiveContractForPayslip(ctx context.Context, employeeID string, periodEnd time.Time) (*PayslipContractInfo, error) {
	end := calendarDate(periodEnd)
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	contract, ok, err := s.repo.ActiveForPeriod(ctx, employeeID, start, end)
	if err != nil || !ok {
		return nil, err
	}
	return &PayslipContractInfo{
		ContractID:  contract.ID,
		JobTitle:    contract.JobTitle,
		StatusKerja: contractStatusKerja(contract.ContractType),
	}, nil
}

package hris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/dto"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/mail"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// Slip gaji. Generation builds an encrypted snapshot per employee and period
// synchronously and queues the PDF render on the document worker; sending
// goes through the shared DocumentMailer. Sent slips are never edited: they
// are voided and reissued with a '-R<n>' number.

const (
	PayslipDocumentKind = "payslip"
	payslipDocumentPart = "slip"

	// Salary-access log actions (fail-closed, before any amount is returned).
	payslipAccessList    = "payslip_list_view"
	payslipAccessView    = "payslip_view"
	payslipAccessHistory = "payslip_history_view"
	payslipAccessPDF     = "payslip_pdf_"
	payslipAccessDocx    = "payslip_docx_download"
	// Endpoints that change or send a slip also return its amounts.
	payslipAccessGenerate = "payslip_generate"
	payslipAccessUpdate   = "payslip_update"
	payslipAccessSend     = "payslip_send"
	payslipAccessReissue  = "payslip_void_reissue"

	payslipVoidReasonMinRunes = 3
	payslipVoidReasonMaxRunes = 200
)

var (
	ErrPayslipNotFound          = errors.New("slip gaji tidak ditemukan")
	ErrPayslipNotDraft          = errors.New("hanya slip berstatus draf yang dapat diubah; slip terkirim dibatalkan dan diterbitkan ulang")
	ErrPayslipNotSent           = errors.New("hanya slip yang sudah terkirim yang dapat dibatalkan dan diterbitkan ulang")
	ErrPayslipVoided            = errors.New("slip ini sudah dibatalkan")
	ErrPayslipNoteInvalid       = errors.New("catatan slip maksimal 120 karakter dalam satu baris")
	ErrPayslipManualLineInvalid = errors.New("baris penyesuaian tidak valid: label wajib diisi (maks. 60 karakter), nominal tidak boleh 0, keterangan maks. 60 karakter, maksimal 5 baris")
	ErrPayslipPayDateInvalid    = errors.New("tanggal bayar harus berada di bulan periode atau bulan berikutnya")
	ErrPayslipPeriodInvalid     = errors.New("periode slip gaji tidak valid")
	ErrPayslipPDFNotReady       = errors.New("PDF slip belum siap; tunggu proses pembuatan PDF selesai")
	ErrPayslipBlocked           = errors.New("slip tidak dapat dibuat: data gaji karyawan belum tersedia")
	ErrPayslipVoidReasonInvalid = errors.New("alasan pembatalan wajib diisi (3-200 karakter)")
	ErrPayslipStateChanged      = errors.New("status slip berubah; muat ulang halaman lalu coba lagi")
	// ErrPayslipItemsTaken: another slip of the employee that is sent (or
	// being sent) already holds one of this draft's bonuses or
	// reimbursements; sending both would pay it twice.
	ErrPayslipItemsTaken = errors.New("bonus atau reimbursement di slip ini sudah tercantum di slip lain yang terkirim atau sedang dikirim; Generate ulang slip ini sebelum mengirim")
	// ErrPayslipItemsChanged: a bonus or reimbursement of the draft was
	// rejected, unpaid, deleted or changed after the snapshot was built.
	ErrPayslipItemsChanged = errors.New("bonus atau reimbursement di slip ini berubah setelah slip dibuat; Generate ulang slip ini sebelum mengirim")
)

type payslipsRepository interface {
	GetByID(ctx context.Context, id string) (model.Payslip, error)
	GetByIDs(ctx context.Context, ids []string) ([]model.Payslip, error)
	ListActiveByPeriod(ctx context.Context, year int, month int) ([]model.Payslip, error)
	ListByEmployee(ctx context.Context, employeeID string, limit int) ([]model.Payslip, error)
	GetActive(ctx context.Context, employeeID string, year int, month int) (model.Payslip, error)
	LatestSentBefore(ctx context.Context, employeeID string, year int, month int) (*hrisrepo.PayslipAnchor, error)
	ConsumedItemIDs(ctx context.Context, employeeID string, year int, month int) (map[string]bool, map[string]bool, error)
	OverlappingSlip(ctx context.Context, payslipID string) (*hrisrepo.PayslipOverlap, error)
	ListReimbursementsForPayslip(ctx context.Context, employeeID string) ([]hrisrepo.PayslipReimbursement, error)
	ListReimbursementsByIDs(ctx context.Context, employeeID string, ids []string) ([]hrisrepo.PayslipReimbursement, error)
	GetEmployee(ctx context.Context, employeeID string) (hrisrepo.PayslipEmployee, error)
	ListPeriodEmployees(ctx context.Context, year int, month int, periodEnd time.Time) ([]hrisrepo.PayslipEmployee, error)
	MaxRevision(ctx context.Context, employeeID string, year int, month int) (int, error)
	Create(ctx context.Context, params hrisrepo.CreatePayslipParams) (model.Payslip, error)
	UpdateDraftSnapshot(ctx context.Context, id string, params hrisrepo.UpdatePayslipSnapshotParams) (model.Payslip, error)
	MarkSent(ctx context.Context, id string, at time.Time, sent hrisrepo.SentSnapshot) (model.Payslip, error)
	Void(ctx context.Context, id string, voidedBy string, reason string) (model.Payslip, error)
	LatestDeliveries(ctx context.Context, ids []string) (map[string]model.EmailDelivery, error)
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type payslipCompensation interface {
	SalaryAsOf(ctx context.Context, employeeID string, date time.Time) (model.SalaryRecord, error)
	BonusRecords(ctx context.Context, employeeID string) ([]model.BonusRecord, error)
	LogSalaryAccess(ctx context.Context, actorID string, resourceID string, action string) error
}

type payslipEmployeeCodes interface {
	EnsureEmployeeCode(ctx context.Context, employeeID string, docCode string) (int, string, error)
}

type payslipCompany interface {
	Profile(ctx context.Context) (authrepo.CompanyProfileRecord, error)
	Get(ctx context.Context) (dto.CompanyProfileResponse, error)
	LogoPNG(ctx context.Context) ([]byte, error)
}

type payslipRenderQueue interface {
	Enqueue(job DocumentJob) bool
	PDFAvailable() bool
}

type payslipPDFStore interface {
	Read(tenantID string, rel string, wantSHA256 string) ([]byte, error)
}

// PayslipContractSource lets the contracts phase put the active contract's
// job title and type on the slip without touching the assembly: when set,
// its answer wins over the HR profile job title and the employment type
// mapping. nil info means no active contract.
type PayslipContractSource interface {
	ActiveContractForPayslip(ctx context.Context, employeeID string, periodEnd time.Time) (*PayslipContractInfo, error)
}

type PayslipsService struct {
	repo         payslipsRepository
	compensation payslipCompensation
	codes        payslipEmployeeCodes
	company      payslipCompany
	queue        payslipRenderQueue
	store        payslipPDFStore
	mailer       *DocumentMailer
	contracts    PayslipContractSource
	encrypter    *security.Encrypter
	now          func() time.Time
}

func NewPayslipsService(
	repo payslipsRepository,
	compensation payslipCompensation,
	codes payslipEmployeeCodes,
	company payslipCompany,
	queue payslipRenderQueue,
	store payslipPDFStore,
	mailer *DocumentMailer,
	encrypter *security.Encrypter,
) *PayslipsService {
	return &PayslipsService{
		repo:         repo,
		compensation: compensation,
		codes:        codes,
		company:      company,
		queue:        queue,
		store:        store,
		mailer:       mailer,
		encrypter:    encrypter,
		now:          time.Now,
	}
}

// SetContractSource plugs in the active-contract lookup (contracts phase).
func (s *PayslipsService) SetContractSource(source PayslipContractSource) {
	s.contracts = source
}

func validPeriod(year int, month int) bool {
	return year >= 2000 && year <= 2100 && month >= 1 && month <= 12
}

func periodKey(year int, month int) string {
	return fmt.Sprintf("%04d-%02d", year, month)
}

// ---------------------------------------------------------------------------
// Snapshot encryption

func (s *PayslipsService) encryptJSON(value any) (string, error) {
	if s.encrypter == nil {
		return "", errors.New("payslip encrypter is not configured")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return s.encrypter.EncryptString(string(raw))
}

func (s *PayslipsService) decryptAmounts(slip model.Payslip) (model.PayslipAmounts, error) {
	var amounts model.PayslipAmounts
	if s.encrypter == nil {
		return amounts, errors.New("payslip encrypter is not configured")
	}
	plain, err := s.encrypter.DecryptString(slip.AmountsEncrypted)
	if err != nil {
		return amounts, fmt.Errorf("decrypt payslip amounts: %w", err)
	}
	if err := json.Unmarshal([]byte(plain), &amounts); err != nil {
		return amounts, fmt.Errorf("decode payslip amounts: %w", err)
	}
	if amounts.Earnings == nil {
		amounts.Earnings = []model.PayslipLine{}
	}
	if amounts.Deductions == nil {
		amounts.Deductions = []model.PayslipLine{}
	}
	if amounts.Reimbursements == nil {
		amounts.Reimbursements = []model.PayslipReimbursementLine{}
	}
	if amounts.ManualLines == nil {
		amounts.ManualLines = []model.PayslipManualLine{}
	}
	return amounts, nil
}

func decryptPayslipPayload(encrypter *security.Encrypter, slip model.Payslip) (docgen.Payload, error) {
	if encrypter == nil {
		return nil, errors.New("payslip encrypter is not configured")
	}
	plain, err := encrypter.DecryptString(slip.PayloadEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt payslip payload: %w", err)
	}
	payload := docgen.Payload{}
	if err := json.Unmarshal([]byte(plain), &payload); err != nil {
		return nil, fmt.Errorf("decode payslip payload: %w", err)
	}
	return payload, nil
}

func payloadString(payload docgen.Payload, key string) string {
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}

// ---------------------------------------------------------------------------
// Assembly (loads the data, then the pure assemblePayslip)

// assembleFor loads the data of one slip and assembles it. replaced is the
// voided slip a reissue replaces (nil for a normal slip): the reissue then
// keeps exactly its bonuses and reimbursements and its generated_at (D9).
func (s *PayslipsService) assembleFor(ctx context.Context, employee hrisrepo.PayslipEmployee, year int, month int, manual []model.PayslipManualLine, note string, company authrepo.CompanyProfileRecord, replaced *model.Payslip) (payslipAssembly, error) {
	_, periodEnd := payslipPeriodBounds(year, month)
	// The clock is read before any data: it is the ceiling of this
	// selection and, stored as generated_at, the floor of the next slip's.
	in := payslipAssemblyInput{
		Year:        year,
		Month:       month,
		Now:         s.now(),
		Employee:    employee,
		ManualLines: manual,
		Note:        note,
		Company:     company,
	}

	salary, err := s.compensation.SalaryAsOf(ctx, employee.ID, periodEnd)
	switch {
	case err == nil:
		in.Salary = &salary
	case errors.Is(err, ErrSalaryNotFound):
	default:
		return payslipAssembly{}, err
	}
	if in.Bonuses, err = s.compensation.BonusRecords(ctx, employee.ID); err != nil {
		return payslipAssembly{}, err
	}
	if in.Reimbursements, err = s.repo.ListReimbursementsForPayslip(ctx, employee.ID); err != nil {
		return payslipAssembly{}, err
	}
	if replaced != nil {
		previous, err := s.decryptAmounts(*replaced)
		if err != nil {
			return payslipAssembly{}, err
		}
		reissue := &payslipReissue{
			BonusIDs:         replaced.BonusIDs,
			ReimbursementIDs: replaced.ReimbursementIDs,
			Previous:         previous,
		}
		if reissue.Reimbursements, err = s.repo.ListReimbursementsByIDs(ctx, employee.ID, replaced.ReimbursementIDs); err != nil {
			return payslipAssembly{}, err
		}
		in.Reissue = reissue
		in.Now = replaced.GeneratedAt
	} else {
		if in.Anchor, err = s.repo.LatestSentBefore(ctx, employee.ID, year, month); err != nil {
			return payslipAssembly{}, err
		}
		if in.ConsumedBonus, in.ConsumedReimburs, err = s.repo.ConsumedItemIDs(ctx, employee.ID, year, month); err != nil {
			return payslipAssembly{}, err
		}
	}
	if s.contracts != nil {
		if in.Contract, err = s.contracts.ActiveContractForPayslip(ctx, employee.ID, periodEnd); err != nil {
			return payslipAssembly{}, err
		}
	}
	return assemblePayslip(in), nil
}

// replacedSlip returns the voided slip a reissue draft replaces (nil for a
// normal draft).
func (s *PayslipsService) replacedSlip(ctx context.Context, slip *model.Payslip) (*model.Payslip, error) {
	if slip == nil || slip.ReplacesPayslipID == nil {
		return nil, nil
	}
	replaced, err := s.repo.GetByID(ctx, *slip.ReplacesPayslipID)
	if err != nil {
		return nil, err
	}
	return &replaced, nil
}

func blockingReason(warnings []model.PayslipWarning) string {
	parts := []string{}
	for _, item := range warnings {
		if item.Blocking {
			parts = append(parts, item.Message)
		}
	}
	return strings.Join(parts, "; ")
}

// ---------------------------------------------------------------------------
// List / detail / history

func (s *PayslipsService) companyInfo(ctx context.Context) (authrepo.CompanyProfileRecord, hrisdto.PayslipCompanyInfo, error) {
	record, err := s.company.Profile(ctx)
	if err != nil {
		return authrepo.CompanyProfileRecord{}, hrisdto.PayslipCompanyInfo{}, err
	}
	view, err := s.company.Get(ctx)
	if err != nil {
		return authrepo.CompanyProfileRecord{}, hrisdto.PayslipCompanyInfo{}, err
	}
	missing := []string{}
	for field, value := range map[string]string{
		"legal_name":       record.LegalName,
		"address":          record.Address,
		"hr_contact_email": record.HRContactEmail,
		"doc_code":         record.DocCode,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, field)
		}
	}
	sort.Strings(missing)
	return record, hrisdto.PayslipCompanyInfo{
		LegalName:      record.LegalName,
		DocCode:        record.DocCode,
		PaydayDay:      record.PaydayDay,
		HRContactEmail: record.HRContactEmail,
		HasLogo:        view.HasLogo,
		MissingFields:  missing,
	}, nil
}

func deliverySummary(delivery *model.EmailDelivery, reveal *personalReveal, employeeID string) *hrisdto.PayslipDeliverySummary {
	if delivery == nil {
		return nil
	}
	return &hrisdto.PayslipDeliverySummary{
		ID:              delivery.ID,
		Status:          delivery.Status,
		Recipient:       reveal.address(employeeID, delivery.Recipient, delivery.RecipientSource),
		RecipientSource: delivery.RecipientSource,
		Error:           delivery.Error,
		Attempts:        delivery.Attempts,
		CreatedAt:       delivery.CreatedAt,
		SentAt:          delivery.SentAt,
	}
}

func nonEmptyPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func (s *PayslipsService) itemFromSlip(employee hrisrepo.PayslipEmployee, slip model.Payslip, delivery *model.EmailDelivery, reveal *personalReveal) (hrisdto.PayslipListItem, error) {
	amounts, err := s.decryptAmounts(slip)
	if err != nil {
		return hrisdto.PayslipListItem{}, err
	}
	payload, err := decryptPayslipPayload(s.encrypter, slip)
	if err != nil {
		return hrisdto.PayslipListItem{}, err
	}
	id, number := slip.ID, slip.DocNumber
	payDate := slip.PayDate.Format("2006-01-02")
	generatedAt := slip.GeneratedAt
	jobTitle := payloadString(payload, "karyawan_jabatan")
	if jobTitle == "-" {
		jobTitle = ""
	}
	code := payloadString(payload, "karyawan_id")
	if code == "-" {
		code = ""
	}
	item := hrisdto.PayslipListItem{
		EmployeeID:       employee.ID,
		EmployeeName:     employee.FullName,
		Department:       employee.Department,
		EmploymentType:   employee.Position,
		EmploymentStatus: employee.EmploymentStatus,
		DateJoined:       employee.DateJoined.Format("2006-01-02"),
		JobTitle:         nonEmptyPtr(jobTitle),
		EmployeeCode:     nonEmptyPtr(code),
		PayslipID:        &id,
		DocNumber:        &number,
		Status:           slip.Status,
		Revision:         slip.Revision,
		RenderStatus:     slip.RenderStatus,
		RenderError:      slip.RenderError,
		PayDate:          &payDate,
		Totals:           amounts.Totals,
		Warnings:         slip.Warnings,
		HasPDF:           slip.RenderStatus == model.DocumentRenderReady && slip.PDFPath != nil,
		GeneratedAt:      &generatedAt,
		LastSentAt:       slip.LastSentAt,
		LastDelivery:     deliverySummary(delivery, reveal, slip.EmployeeID),
	}
	for _, w := range slip.Warnings {
		if w.Blocking {
			item.Blocked = true
		}
	}
	return item, nil
}

func previewItem(employee hrisrepo.PayslipEmployee, asm payslipAssembly) hrisdto.PayslipListItem {
	return hrisdto.PayslipListItem{
		EmployeeID:       employee.ID,
		EmployeeName:     employee.FullName,
		Department:       employee.Department,
		EmploymentType:   employee.Position,
		EmploymentStatus: employee.EmploymentStatus,
		DateJoined:       employee.DateJoined.Format("2006-01-02"),
		JobTitle:         employee.JobTitle,
		EmployeeCode:     employee.EmployeeCode,
		Status:           "none",
		RenderStatus:     model.DocumentRenderNone,
		Totals:           asm.Amounts.Totals,
		Warnings:         asm.Warnings,
		Blocked:          asm.Blocked,
		Preview:          true,
	}
}

// List returns one row per employee of the period: the stored slip, or a
// live preview (totals and warnings, nothing stored) for employees without
// one. The access is logged before anything is computed.
func (s *PayslipsService) List(ctx context.Context, viewer DocumentViewer, year int, month int) (hrisdto.PayslipListResponse, error) {
	if !validPeriod(year, month) {
		return hrisdto.PayslipListResponse{}, ErrPayslipPeriodInvalid
	}
	if err := s.compensation.LogSalaryAccess(ctx, viewer.ActorID, periodKey(year, month), payslipAccessList); err != nil {
		return hrisdto.PayslipListResponse{}, fmt.Errorf("log salary access: %w", err)
	}

	company, companyInfo, err := s.companyInfo(ctx)
	if err != nil {
		return hrisdto.PayslipListResponse{}, err
	}
	periodStart, periodEnd := payslipPeriodBounds(year, month)
	employees, err := s.repo.ListPeriodEmployees(ctx, year, month, periodEnd)
	if err != nil {
		return hrisdto.PayslipListResponse{}, err
	}
	slips, err := s.repo.ListActiveByPeriod(ctx, year, month)
	if err != nil {
		return hrisdto.PayslipListResponse{}, err
	}
	byEmployee := make(map[string]model.Payslip, len(slips))
	ids := make([]string, 0, len(slips))
	for _, slip := range slips {
		byEmployee[slip.EmployeeID] = slip
		ids = append(ids, slip.ID)
	}
	deliveries, err := s.repo.LatestDeliveries(ctx, ids)
	if err != nil {
		return hrisdto.PayslipListResponse{}, err
	}

	result := hrisdto.PayslipListResponse{
		Year:           year,
		Month:          month,
		PeriodLabel:    docgen.PeriodID(periodStart),
		DefaultPayDate: DefaultPayslipPayDate(year, month, company.PaydayDay).Format("2006-01-02"),
		PDFAvailable:   s.queue != nil && s.queue.PDFAvailable(),
		AllowDocxSend:  s.mailer != nil && s.mailer.AllowDocxSend(),
		Company:        companyInfo,
		Items:          make([]hrisdto.PayslipListItem, 0, len(employees)),
	}
	if s.mailer != nil {
		if result.DocumentMailReady, err = s.mailer.Ready(ctx); err != nil {
			return hrisdto.PayslipListResponse{}, err
		}
	}

	reveal := newPersonalReveal(viewer)
	for _, employee := range employees {
		var item hrisdto.PayslipListItem
		if slip, ok := byEmployee[employee.ID]; ok {
			var delivery *model.EmailDelivery
			if found, ok := deliveries[slip.ID]; ok {
				delivery = &found
			}
			if item, err = s.itemFromSlip(employee, slip, delivery, reveal); err != nil {
				return hrisdto.PayslipListResponse{}, err
			}
		} else {
			asm, err := s.assembleFor(ctx, employee, year, month, nil, "", company, nil)
			if err != nil {
				return hrisdto.PayslipListResponse{}, err
			}
			item = previewItem(employee, asm)
		}
		countPayslipSummary(&result.Summary, item)
		result.Items = append(result.Items, item)
	}
	if s.mailer != nil {
		if err := s.mailer.logReveal(ctx, reveal); err != nil {
			return hrisdto.PayslipListResponse{}, err
		}
	}
	return result, nil
}

func countPayslipSummary(summary *hrisdto.PayslipSummary, item hrisdto.PayslipListItem) {
	summary.Total++
	switch item.Status {
	case "none":
		summary.NotGenerated++
		if item.Blocked {
			summary.Blocked++
		}
	case model.PayslipStatusDraft:
		summary.Draft++
	case model.PayslipStatusSent:
		summary.Sent++
	}
	// A failed render, or a failed latest delivery (a draft that was never
	// sent, or a 'Kirim ulang' of a sent slip).
	if item.RenderStatus == model.DocumentRenderFailed ||
		(item.LastDelivery != nil && item.LastDelivery.Status == model.EmailDeliveryStatusFailed) {
		summary.Failed++
	}
	if item.RenderStatus == model.DocumentRenderPending || item.RenderStatus == model.DocumentRenderRendering ||
		(item.LastDelivery != nil && (item.LastDelivery.Status == model.EmailDeliveryStatusQueued || item.LastDelivery.Status == model.EmailDeliveryStatusSending)) {
		summary.InProgress++
	}
}

func (s *PayslipsService) getSlip(ctx context.Context, id string) (model.Payslip, error) {
	if _, err := uuid.Parse(id); err != nil {
		return model.Payslip{}, ErrPayslipNotFound
	}
	slip, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, hrisrepo.ErrPayslipNotFound) {
		return model.Payslip{}, ErrPayslipNotFound
	}
	return slip, err
}

func (s *PayslipsService) toDetail(ctx context.Context, slip model.Payslip, reveal *personalReveal) (hrisdto.PayslipDetailResponse, error) {
	amounts, err := s.decryptAmounts(slip)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, err
	}
	payload, err := decryptPayslipPayload(s.encrypter, slip)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, err
	}
	earnings, deductions := payslipRows(amounts)
	detail := hrisdto.PayslipDetailResponse{
		ID:          slip.ID,
		EmployeeID:  slip.EmployeeID,
		PeriodYear:  slip.PeriodYear,
		PeriodMonth: slip.PeriodMonth,
		Revision:    slip.Revision,
		DocNumber:   slip.DocNumber,
		Status:      slip.Status,
		PayDate:     slip.PayDate.Format("2006-01-02"),
		Note:        optionalText(slip.Note),
		Header: hrisdto.PayslipHeader{
			EmployeeName:   payloadString(payload, "karyawan_nama"),
			EmployeeCode:   payloadString(payload, "karyawan_id"),
			JobTitle:       payloadString(payload, "karyawan_jabatan"),
			Department:     payloadString(payload, "karyawan_departemen"),
			StatusKerja:    payloadString(payload, "karyawan_status_kerja"),
			DateJoined:     payloadString(payload, "karyawan_tanggal_bergabung"),
			Email:          payloadString(payload, "karyawan_email"),
			Bank:           payloadString(payload, "karyawan_bank"),
			AccountMasked:  payloadString(payload, "karyawan_norek"),
			Period:         payloadString(payload, "periode"),
			PayDateLabel:   payloadString(payload, "tanggal_bayar"),
			TotalInWords:   payloadString(payload, "total_diterima_terbilang"),
			CompanyName:    payloadString(payload, "perusahaan_nama"),
			HRContactEmail: payloadString(payload, "kontak_hr"),
		},
		Earnings:          earnings,
		Deductions:        deductions,
		Reimbursements:    amounts.Reimbursements,
		ManualLines:       amounts.ManualLines,
		Totals:            amounts.Totals,
		Warnings:          slip.Warnings,
		RenderStatus:      slip.RenderStatus,
		RenderError:       slip.RenderError,
		HasPDF:            slip.RenderStatus == model.DocumentRenderReady && slip.PDFPath != nil,
		PDFSHA256:         slip.PDFSHA256,
		TemplateVersion:   slip.TemplateVersion,
		ReplacesPayslipID: slip.ReplacesPayslipID,
		GeneratedAt:       slip.GeneratedAt,
		GeneratedBy:       slip.GeneratedBy,
		LastSentAt:        slip.LastSentAt,
		VoidedAt:          slip.VoidedAt,
		VoidReason:        slip.VoidReason,
		UpdatedAt:         slip.UpdatedAt,
	}
	if slip.ReplacesPayslipID != nil {
		if replaced, err := s.repo.GetByID(ctx, *slip.ReplacesPayslipID); err == nil {
			number := replaced.DocNumber
			detail.ReplacesDocNumber = &number
		} else if !errors.Is(err, hrisrepo.ErrPayslipNotFound) {
			return hrisdto.PayslipDetailResponse{}, err
		}
	}
	deliveries, err := s.repo.LatestDeliveries(ctx, []string{slip.ID})
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, err
	}
	if delivery, ok := deliveries[slip.ID]; ok {
		detail.LastDelivery = deliverySummary(&delivery, reveal, slip.EmployeeID)
	}
	return detail, nil
}

// detailFor builds the detail of slip for viewer and logs a personal
// address shown in full (fail-closed).
func (s *PayslipsService) detailFor(ctx context.Context, viewer DocumentViewer, slip model.Payslip) (hrisdto.PayslipDetailResponse, error) {
	reveal := newPersonalReveal(viewer)
	detail, err := s.toDetail(ctx, slip, reveal)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, err
	}
	if s.mailer != nil {
		if err := s.mailer.logReveal(ctx, reveal); err != nil {
			return hrisdto.PayslipDetailResponse{}, err
		}
	}
	return detail, nil
}

// logAmountsAccess writes the salary access row (fail-closed) of an
// endpoint that returns a slip's amounts.
func (s *PayslipsService) logAmountsAccess(ctx context.Context, actorID string, resourceID string, action string) error {
	if err := s.compensation.LogSalaryAccess(ctx, actorID, resourceID, action); err != nil {
		return fmt.Errorf("log salary access: %w", err)
	}
	return nil
}

// Get returns one slip with its rows; the access is logged first.
func (s *PayslipsService) Get(ctx context.Context, viewer DocumentViewer, id string) (hrisdto.PayslipDetailResponse, error) {
	slip, err := s.getSlip(ctx, id)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, err
	}
	if err := s.logAmountsAccess(ctx, viewer.ActorID, slip.EmployeeID, payslipAccessView); err != nil {
		return hrisdto.PayslipDetailResponse{}, err
	}
	return s.detailFor(ctx, viewer, slip)
}

// History lists an employee's slips (newest first, voided included) for the
// employee page.
func (s *PayslipsService) History(ctx context.Context, viewer DocumentViewer, employeeID string, limit int) ([]hrisdto.PayslipHistoryItem, error) {
	if _, err := uuid.Parse(employeeID); err != nil {
		return nil, ErrEmployeeNotFound
	}
	if _, err := s.repo.GetEmployee(ctx, employeeID); err != nil {
		if errors.Is(err, hrisrepo.ErrEmployeeNotFound) {
			return nil, ErrEmployeeNotFound
		}
		return nil, err
	}
	if err := s.logAmountsAccess(ctx, viewer.ActorID, employeeID, payslipAccessHistory); err != nil {
		return nil, err
	}
	slips, err := s.repo.ListByEmployee(ctx, employeeID, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(slips))
	for _, slip := range slips {
		ids = append(ids, slip.ID)
	}
	deliveries, err := s.repo.LatestDeliveries(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]hrisdto.PayslipHistoryItem, 0, len(slips))
	reveal := newPersonalReveal(viewer)
	for _, slip := range slips {
		amounts, err := s.decryptAmounts(slip)
		if err != nil {
			return nil, err
		}
		periodStart, _ := payslipPeriodBounds(slip.PeriodYear, slip.PeriodMonth)
		item := hrisdto.PayslipHistoryItem{
			ID:            slip.ID,
			PeriodYear:    slip.PeriodYear,
			PeriodMonth:   slip.PeriodMonth,
			PeriodLabel:   docgen.PeriodID(periodStart),
			DocNumber:     slip.DocNumber,
			Revision:      slip.Revision,
			Status:        slip.Status,
			RenderStatus:  slip.RenderStatus,
			TotalDiterima: amounts.Totals.TotalDiterima,
			GeneratedAt:   slip.GeneratedAt,
			LastSentAt:    slip.LastSentAt,
		}
		if delivery, ok := deliveries[slip.ID]; ok {
			item.LastDelivery = deliverySummary(&delivery, reveal, slip.EmployeeID)
		}
		result = append(result, item)
	}
	if s.mailer != nil {
		if err := s.mailer.logReveal(ctx, reveal); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Generation

// PayslipAuditEntry is the metadata the handler audits per generated or
// changed slip (never amounts).
type PayslipAuditEntry struct {
	PayslipID string
	Values    map[string]any
}

func payslipAuditValues(slip model.Payslip) map[string]any {
	return map[string]any{
		"doc_number":  slip.DocNumber,
		"employee_id": slip.EmployeeID,
		"period":      periodKey(slip.PeriodYear, slip.PeriodMonth),
		"revision":    slip.Revision,
		"status":      slip.Status,
	}
}

func (s *PayslipsService) parsePayDate(year int, month int, raw *string, paydayDay int) (time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return DefaultPayslipPayDate(year, month, paydayDay), nil
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*raw))
	if err != nil {
		return time.Time{}, ErrPayslipPayDateInvalid
	}
	first, last := PayslipPayDateBounds(year, month)
	if parsed.Before(first) || parsed.After(last) {
		return time.Time{}, ErrPayslipPayDateInvalid
	}
	return parsed, nil
}

// newRenderStatus is the render_status of a new or rebuilt snapshot:
// 'pending' (queued for the PDF worker), or 'none' when this server has no
// PDF converter: such a draft is not a failed render, it is checked as DOCX
// (and sent as DOCX with DOCUMENTS_ALLOW_DOCX_SEND).
func (s *PayslipsService) newRenderStatus() string {
	if s.queue != nil && s.queue.PDFAvailable() {
		return model.DocumentRenderPending
	}
	return model.DocumentRenderNone
}

// enqueue queues the PDF renders of slips. Generation calls it once with
// every slip it built, so the worker converts them in one LibreOffice
// batch.
func (s *PayslipsService) enqueue(ctx context.Context, slips ...model.Payslip) {
	if s.queue == nil {
		return
	}
	info, ok := tenant.FromContext(ctx)
	if !ok {
		// The sweep picks the rows up (render_status 'pending').
		return
	}
	for _, slip := range slips {
		if slip.RenderStatus != model.DocumentRenderPending {
			continue
		}
		s.queue.Enqueue(DocumentJob{Tenant: info, Kind: PayslipDocumentKind, ID: slip.ID})
	}
}

func templateVersion() string {
	tpl, err := docgen.Load(docgen.TemplateSlip)
	if err != nil {
		return ""
	}
	return tpl.Version
}

// Generate builds draft snapshots synchronously for the given employees and
// queues their PDF renders. A draft that already exists is rebuilt with the
// current data but keeps its note and manual lines; sent slips, blocked
// employees (no salary row) and employees outside the period are skipped.
func (s *PayslipsService) Generate(ctx context.Context, actorID string, input hrisdto.GeneratePayslipsRequest) (hrisdto.GeneratePayslipsResponse, []PayslipAuditEntry, error) {
	if !validPeriod(input.Year, input.Month) {
		return hrisdto.GeneratePayslipsResponse{}, nil, ErrPayslipPeriodInvalid
	}
	company, err := s.company.Profile(ctx)
	if err != nil {
		return hrisdto.GeneratePayslipsResponse{}, nil, err
	}
	payDate, err := s.parsePayDate(input.Year, input.Month, input.PayDate, company.PaydayDay)
	if err != nil {
		return hrisdto.GeneratePayslipsResponse{}, nil, err
	}
	// The response carries the totals of every generated slip.
	if err := s.logAmountsAccess(ctx, actorID, periodKey(input.Year, input.Month), payslipAccessGenerate); err != nil {
		return hrisdto.GeneratePayslipsResponse{}, nil, err
	}

	response := hrisdto.GeneratePayslipsResponse{
		Year:      input.Year,
		Month:     input.Month,
		PayDate:   payDate.Format("2006-01-02"),
		Generated: []hrisdto.PayslipListItem{},
		Skipped:   []hrisdto.PayslipSkipped{},
	}
	audits := []PayslipAuditEntry{}
	built := []model.Payslip{}
	// Renders are queued once every snapshot is stored, so the worker
	// converts the whole selection in one LibreOffice batch.
	defer func() { s.enqueue(ctx, built...) }()
	seen := map[string]bool{}
	for _, raw := range input.EmployeeIDs {
		employeeID := strings.ToLower(strings.TrimSpace(raw))
		if seen[employeeID] {
			continue
		}
		seen[employeeID] = true

		slip, employee, skipped, err := s.generateOne(ctx, actorID, company, input.Year, input.Month, payDate, employeeID)
		if err != nil {
			return hrisdto.GeneratePayslipsResponse{}, nil, err
		}
		if skipped != nil {
			response.Skipped = append(response.Skipped, *skipped)
			continue
		}
		built = append(built, slip)
		item, err := s.itemFromSlip(employee, slip, nil, nil)
		if err != nil {
			return hrisdto.GeneratePayslipsResponse{}, nil, err
		}
		response.Generated = append(response.Generated, item)
		values := payslipAuditValues(slip)
		values["pay_date"] = response.PayDate
		audits = append(audits, PayslipAuditEntry{PayslipID: slip.ID, Values: values})
	}
	return response, audits, nil
}

func skip(employee hrisrepo.PayslipEmployee, employeeID string, code string, reason string) *hrisdto.PayslipSkipped {
	return &hrisdto.PayslipSkipped{EmployeeID: employeeID, EmployeeName: employee.FullName, Code: code, Reason: reason}
}

func (s *PayslipsService) generateOne(ctx context.Context, actorID string, company authrepo.CompanyProfileRecord, year int, month int, payDate time.Time, employeeID string) (model.Payslip, hrisrepo.PayslipEmployee, *hrisdto.PayslipSkipped, error) {
	none := hrisrepo.PayslipEmployee{}
	employee, err := s.repo.GetEmployee(ctx, employeeID)
	if errors.Is(err, hrisrepo.ErrEmployeeNotFound) {
		return model.Payslip{}, none, skip(none, employeeID, "employee_not_found", "Karyawan tidak ditemukan"), nil
	}
	if err != nil {
		return model.Payslip{}, none, nil, err
	}

	var existing *model.Payslip
	if found, err := s.repo.GetActive(ctx, employeeID, year, month); err == nil {
		existing = &found
	} else if !errors.Is(err, hrisrepo.ErrPayslipNotFound) {
		return model.Payslip{}, none, nil, err
	}
	if existing != nil && existing.Status == model.PayslipStatusSent {
		return model.Payslip{}, none, skip(employee, employeeID, "already_sent", "Slip sudah terkirim; gunakan Batalkan & Terbitkan Ulang untuk mengoreksi"), nil
	}

	_, periodEnd := payslipPeriodBounds(year, month)
	periodEndDate := time.Date(periodEnd.Year(), periodEnd.Month(), periodEnd.Day(), 0, 0, 0, 0, time.UTC)
	eligible := (employee.EmploymentStatus == "active" || employee.EmploymentStatus == "probation") && !employee.DateJoined.After(periodEndDate)
	if !eligible && existing == nil {
		return model.Payslip{}, none, skip(employee, employeeID, "not_eligible", "Karyawan tidak aktif atau belum bergabung pada periode ini"), nil
	}

	var manual []model.PayslipManualLine
	var note *string
	var replaced *model.Payslip
	if existing != nil {
		if s.mailer != nil {
			busy, err := s.mailer.InFlight(ctx, model.EmailDeliveryKindPayslip, existing.ID)
			if err != nil {
				return model.Payslip{}, none, nil, err
			}
			if busy {
				return model.Payslip{}, none, skip(employee, employeeID, "sending", "Slip sedang dikirim; coba lagi setelah selesai"), nil
			}
		}
		amounts, err := s.decryptAmounts(*existing)
		if err != nil {
			return model.Payslip{}, none, nil, err
		}
		manual = amounts.ManualLines
		note = existing.Note
		// A reissue draft stays pinned to the voided slip's items.
		if replaced, err = s.replacedSlip(ctx, existing); err != nil {
			return model.Payslip{}, none, nil, err
		}
	}

	asm, err := s.assembleFor(ctx, employee, year, month, manual, optionalText(note), company, replaced)
	if err != nil {
		return model.Payslip{}, none, nil, err
	}
	if asm.Blocked {
		return model.Payslip{}, none, skip(employee, employeeID, "blocked", blockingReason(asm.Warnings)), nil
	}

	// The employee code is assigned lazily, at the first slip or contract.
	number, code, err := s.codes.EnsureEmployeeCode(ctx, employeeID, company.DocCode)
	if err != nil {
		return model.Payslip{}, none, nil, err
	}
	employee.EmployeeNumber = &number
	employee.EmployeeCode = &code

	docNumber, revision := "", 0
	if existing != nil {
		docNumber, revision = existing.DocNumber, existing.Revision
	} else {
		maxRevision, err := s.repo.MaxRevision(ctx, employeeID, year, month)
		if err != nil {
			return model.Payslip{}, none, nil, err
		}
		revision = maxRevision + 1
		docNumber = FormatPayslipNumber(year, month, number, revision)
	}

	slip, err := s.storeSnapshot(ctx, storeSnapshotInput{
		ActorID:   actorID,
		Existing:  existing,
		Employee:  employee,
		Year:      year,
		Month:     month,
		Revision:  revision,
		DocNumber: docNumber,
		PayDate:   payDate,
		Note:      note,
		Assembly:  asm,
		Company:   company,
	})
	if errors.Is(err, hrisrepo.ErrPayslipSendInFlight) {
		return model.Payslip{}, none, skip(employee, employeeID, "sending", "Slip sedang dikirim; coba lagi setelah selesai"), nil
	}
	if errors.Is(err, hrisrepo.ErrPayslipActiveExists) || errors.Is(err, hrisrepo.ErrPayslipStateChanged) {
		return model.Payslip{}, none, skip(employee, employeeID, "conflict", "Slip periode ini baru saja dibuat atau diubah oleh proses lain; muat ulang halaman"), nil
	}
	if err != nil {
		return model.Payslip{}, none, nil, err
	}
	return slip, employee, nil, nil
}

type storeSnapshotInput struct {
	ActorID   string
	Existing  *model.Payslip
	Replaces  *string
	Employee  hrisrepo.PayslipEmployee
	Year      int
	Month     int
	Revision  int
	DocNumber string
	PayDate   time.Time
	Note      *string
	Assembly  payslipAssembly
	Company   authrepo.CompanyProfileRecord
}

// storeSnapshot encrypts the amounts and the template payload and writes a
// new draft (or rebuilds the existing one) with render_status 'pending'.
func (s *PayslipsService) storeSnapshot(ctx context.Context, in storeSnapshotInput) (model.Payslip, error) {
	header := payslipHeader{
		DocNumber:   in.DocNumber,
		PayDate:     in.PayDate,
		Year:        in.Year,
		Month:       in.Month,
		Employee:    in.Employee,
		JobTitle:    in.Assembly.JobTitle,
		StatusKerja: in.Assembly.StatusKerja,
		Company:     in.Company,
	}
	payload := buildPayslipPayload(header, in.Assembly.Amounts, optionalText(in.Note))
	amountsCipher, err := s.encryptJSON(in.Assembly.Amounts)
	if err != nil {
		return model.Payslip{}, err
	}
	payloadCipher, err := s.encryptJSON(payload)
	if err != nil {
		return model.Payslip{}, err
	}
	version := templateVersion()

	if in.Existing != nil {
		return s.repo.UpdateDraftSnapshot(ctx, in.Existing.ID, hrisrepo.UpdatePayslipSnapshotParams{
			SalaryID:         in.Assembly.SalaryID,
			ContractID:       in.Assembly.ContractID,
			PayDate:          in.PayDate,
			AmountsEncrypted: amountsCipher,
			PayloadEncrypted: payloadCipher,
			ReimbursementIDs: in.Assembly.ReimbursementIDs,
			BonusIDs:         in.Assembly.BonusIDs,
			Note:             in.Note,
			Warnings:         in.Assembly.Warnings,
			TemplateVersion:  version,
			GeneratedBy:      in.ActorID,
			GeneratedAt:      in.Assembly.GeneratedAt,
			RenderStatus:     s.newRenderStatus(),
		})
	}
	return s.repo.Create(ctx, hrisrepo.CreatePayslipParams{
		EmployeeID:        in.Employee.ID,
		PeriodYear:        in.Year,
		PeriodMonth:       in.Month,
		Revision:          in.Revision,
		DocNumber:         in.DocNumber,
		ReplacesPayslipID: in.Replaces,
		SalaryID:          in.Assembly.SalaryID,
		ContractID:        in.Assembly.ContractID,
		PayDate:           in.PayDate,
		AmountsEncrypted:  amountsCipher,
		PayloadEncrypted:  payloadCipher,
		ReimbursementIDs:  in.Assembly.ReimbursementIDs,
		BonusIDs:          in.Assembly.BonusIDs,
		Note:              in.Note,
		Warnings:          in.Assembly.Warnings,
		TemplateVersion:   version,
		GeneratedBy:       in.ActorID,
		GeneratedAt:       in.Assembly.GeneratedAt,
		RenderStatus:      s.newRenderStatus(),
	})
}

// ---------------------------------------------------------------------------
// Draft edit

func normalizeManualLines(input []hrisdto.PayslipManualLineRequest) ([]model.PayslipManualLine, error) {
	if len(input) > payslipManualLinesMax {
		return nil, ErrPayslipManualLineInvalid
	}
	lines := make([]model.PayslipManualLine, 0, len(input))
	for _, item := range input {
		label := docgen.SingleLine(item.Label)
		keterangan := docgen.SingleLine(item.Keterangan)
		if label == "" || utf8.RuneCountInString(label) > 60 || utf8.RuneCountInString(keterangan) > payslipKeteranganMaxRunes || item.Amount == 0 {
			return nil, ErrPayslipManualLineInvalid
		}
		lines = append(lines, model.PayslipManualLine{Label: label, Amount: item.Amount, Keterangan: keterangan})
	}
	return lines, nil
}

func equalManualLines(a []model.PayslipManualLine, b []model.PayslipManualLine) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

// Update edits the note and manual lines of a draft and re-renders it. It
// returns the changed field names (for the audit; never values).
func (s *PayslipsService) Update(ctx context.Context, viewer DocumentViewer, id string, input hrisdto.UpdatePayslipRequest) (hrisdto.PayslipDetailResponse, []string, error) {
	slip, err := s.getSlip(ctx, id)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, nil, err
	}
	if slip.Status != model.PayslipStatusDraft {
		return hrisdto.PayslipDetailResponse{}, nil, ErrPayslipNotDraft
	}
	note, ok := normalizePayslipNote(input.Note)
	if !ok {
		return hrisdto.PayslipDetailResponse{}, nil, ErrPayslipNoteInvalid
	}
	manual, err := normalizeManualLines(input.ManualLines)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, nil, err
	}
	if s.mailer != nil {
		busy, err := s.mailer.InFlight(ctx, model.EmailDeliveryKindPayslip, slip.ID)
		if err != nil {
			return hrisdto.PayslipDetailResponse{}, nil, err
		}
		if busy {
			return hrisdto.PayslipDetailResponse{}, nil, ErrDocumentSendInFlight
		}
	}
	if err := s.logAmountsAccess(ctx, viewer.ActorID, slip.EmployeeID, payslipAccessUpdate); err != nil {
		return hrisdto.PayslipDetailResponse{}, nil, err
	}

	amounts, err := s.decryptAmounts(slip)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, nil, err
	}
	changed := []string{}
	if note != optionalText(slip.Note) {
		changed = append(changed, "note")
	}
	if !equalManualLines(amounts.ManualLines, manual) {
		changed = append(changed, "manual_lines")
	}
	if len(changed) == 0 {
		detail, err := s.detailFor(ctx, viewer, slip)
		return detail, changed, err
	}

	payload, err := decryptPayslipPayload(s.encrypter, slip)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, nil, err
	}
	amounts.ManualLines = manual
	amounts.Totals = computePayslipTotals(amounts)
	applyAmountsToPayload(payload, amounts, note)

	amountsCipher, err := s.encryptJSON(amounts)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, nil, err
	}
	payloadCipher, err := s.encryptJSON(payload)
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, nil, err
	}
	version := ""
	if slip.TemplateVersion != nil {
		version = *slip.TemplateVersion
	}
	updated, err := s.repo.UpdateDraftSnapshot(ctx, slip.ID, hrisrepo.UpdatePayslipSnapshotParams{
		SalaryID:         slip.SalaryID,
		ContractID:       slip.ContractID,
		PayDate:          slip.PayDate,
		AmountsEncrypted: amountsCipher,
		PayloadEncrypted: payloadCipher,
		ReimbursementIDs: slip.ReimbursementIDs,
		BonusIDs:         slip.BonusIDs,
		Note:             nonEmptyPtr(note),
		Warnings:         replaceRowWarnings(slip.Warnings, amounts, note),
		TemplateVersion:  version,
		RenderStatus:     s.newRenderStatus(),
	})
	if errors.Is(err, hrisrepo.ErrPayslipSendInFlight) {
		return hrisdto.PayslipDetailResponse{}, nil, ErrDocumentSendInFlight
	}
	if errors.Is(err, hrisrepo.ErrPayslipStateChanged) {
		return hrisdto.PayslipDetailResponse{}, nil, ErrPayslipNotDraft
	}
	if err != nil {
		return hrisdto.PayslipDetailResponse{}, nil, err
	}
	s.enqueue(ctx, updated)
	detail, err := s.detailFor(ctx, viewer, updated)
	return detail, changed, err
}

// ---------------------------------------------------------------------------
// Files

// PayslipFile is a document streamed to the client.
type PayslipFile struct {
	Data        []byte
	Filename    string
	ContentType string
	Slip        model.Payslip
}

const (
	contentTypePDF  = "application/pdf"
	contentTypeDocx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

// payslipFilename: "Slip_Gaji_2026-09_Budi_Santoso" (+ "_R1" for a reissue),
// ASCII only so it is safe in Content-Disposition and MIME parameters.
func payslipFilename(slip model.Payslip, employeeName string) string {
	var name strings.Builder
	lastUnderscore := false
	for _, char := range docgen.SingleLine(employeeName) {
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
	if len(cleaned) > 60 {
		cleaned = strings.Trim(cleaned[:60], "_")
	}
	filename := fmt.Sprintf("Slip_Gaji_%04d-%02d", slip.PeriodYear, slip.PeriodMonth)
	if cleaned != "" {
		filename += "_" + cleaned
	}
	if slip.Revision > 0 {
		filename += fmt.Sprintf("_R%d", slip.Revision)
	}
	return filename
}

func (s *PayslipsService) employeeName(ctx context.Context, slip model.Payslip) string {
	if payload, err := decryptPayslipPayload(s.encrypter, slip); err == nil {
		if name := payloadString(payload, "karyawan_nama"); name != "" {
			return name
		}
	}
	if employee, err := s.repo.GetEmployee(ctx, slip.EmployeeID); err == nil {
		return employee.FullName
	}
	return ""
}

// pdfReady reports why a slip has no PDF to serve yet.
func (s *PayslipsService) pdfReady(slip model.Payslip) error {
	if slip.RenderStatus == model.DocumentRenderReady && slip.PDFPath != nil && slip.PDFSHA256 != nil {
		return nil
	}
	if s.queue != nil && !s.queue.PDFAvailable() {
		return ErrDocumentPDFConverterMissing
	}
	return ErrPayslipPDFNotReady
}

func (s *PayslipsService) readPDF(ctx context.Context, slip model.Payslip) ([]byte, error) {
	if err := s.pdfReady(slip); err != nil {
		return nil, err
	}
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return nil, errors.New("tenant is missing from context")
	}
	return s.store.Read(info.ID, *slip.PDFPath, *slip.PDFSHA256)
}

// PDF returns the stored PDF. The access row is written (fail-closed)
// before the file is read.
func (s *PayslipsService) PDF(ctx context.Context, actorID string, id string, disposition string) (PayslipFile, error) {
	slip, err := s.getSlip(ctx, id)
	if err != nil {
		return PayslipFile{}, err
	}
	if err := s.pdfReady(slip); err != nil {
		return PayslipFile{}, err
	}
	if err := s.compensation.LogSalaryAccess(ctx, actorID, slip.EmployeeID, payslipAccessPDF+disposition); err != nil {
		return PayslipFile{}, fmt.Errorf("log salary access: %w", err)
	}
	data, err := s.readPDF(ctx, slip)
	if err != nil {
		return PayslipFile{}, err
	}
	return PayslipFile{Data: data, Filename: payslipFilename(slip, s.employeeName(ctx, slip)) + ".pdf", ContentType: contentTypePDF, Slip: slip}, nil
}

// renderDocx re-renders the DOCX from the encrypted snapshot (milliseconds,
// no soffice), with the stored processed tenant logo.
func (s *PayslipsService) renderDocx(ctx context.Context, slip model.Payslip) ([]byte, error) {
	payload, err := decryptPayslipPayload(s.encrypter, slip)
	if err != nil {
		return nil, err
	}
	tpl, err := docgen.Load(docgen.TemplateSlip)
	if err != nil {
		return nil, err
	}
	logo, err := tenantLogo(ctx, s.company)
	if err != nil {
		return nil, err
	}
	return docgen.Render(tpl, payload, docgen.WithLogo(logo))
}

// tenantLogo returns the stored processed logo, or nil when none was
// uploaded (the slot stays empty).
func tenantLogo(ctx context.Context, company interface {
	LogoPNG(ctx context.Context) ([]byte, error)
}) ([]byte, error) {
	if company == nil {
		return nil, nil
	}
	logo, err := company.LogoPNG(ctx)
	if errors.Is(err, authservice.ErrCompanyLogoNotFound) {
		return nil, nil
	}
	return logo, err
}

// Docx renders the DOCX on demand (HR download). Fail-closed access row
// first.
func (s *PayslipsService) Docx(ctx context.Context, actorID string, id string) (PayslipFile, error) {
	slip, err := s.getSlip(ctx, id)
	if err != nil {
		return PayslipFile{}, err
	}
	if err := s.compensation.LogSalaryAccess(ctx, actorID, slip.EmployeeID, payslipAccessDocx); err != nil {
		return PayslipFile{}, fmt.Errorf("log salary access: %w", err)
	}
	data, err := s.renderDocx(ctx, slip)
	if err != nil {
		return PayslipFile{}, err
	}
	return PayslipFile{Data: data, Filename: payslipFilename(slip, s.employeeName(ctx, slip)) + ".docx", ContentType: contentTypeDocx, Slip: slip}, nil
}

// ---------------------------------------------------------------------------
// Sending

// attachment returns the file e-mailed with a slip: the stored PDF, or —
// only with DOCUMENTS_ALLOW_DOCX_SEND and no PDF converter — the DOCX.
func (s *PayslipsService) attachment(ctx context.Context, slip model.Payslip, employeeName string) (mail.Attachment, string, error) {
	base := payslipFilename(slip, employeeName)
	if slip.RenderStatus == model.DocumentRenderReady && slip.PDFPath != nil {
		data, err := s.readPDF(ctx, slip)
		if err != nil {
			return mail.Attachment{}, "", err
		}
		return mail.Attachment{Filename: base + ".pdf", ContentType: contentTypePDF, Data: data}, "pdf", nil
	}
	pdfAvailable := s.queue != nil && s.queue.PDFAvailable()
	if !pdfAvailable {
		if s.mailer != nil && s.mailer.AllowDocxSend() {
			data, err := s.renderDocx(ctx, slip)
			if err != nil {
				return mail.Attachment{}, "", err
			}
			return mail.Attachment{Filename: base + ".docx", ContentType: contentTypeDocx, Data: data}, "docx", nil
		}
		return mail.Attachment{}, "", ErrDocumentPDFConverterMissing
	}
	return mail.Attachment{}, "", ErrPayslipPDFNotReady
}

func (s *PayslipsService) documentMail(ctx context.Context, slip model.Payslip, recipient ResolvedRecipient, attachment mail.Attachment, actorID string, batchID string) (DocumentMail, error) {
	periodStart, _ := payslipPeriodBounds(slip.PeriodYear, slip.PeriodMonth)
	periodID, periodEN := docgen.PeriodID(periodStart), docgen.PeriodEN(periodStart)
	subject := fmt.Sprintf("Slip Gaji — %s", periodID)
	paragraphsID := []string{fmt.Sprintf("Terlampir slip gaji Anda untuk periode %s dengan nomor %s.", periodID, slip.DocNumber)}
	paragraphsEN := []string{fmt.Sprintf("Please find attached your payslip for %s, number %s.", periodEN, slip.DocNumber)}

	if slip.ReplacesPayslipID != nil {
		replaced := payslipBaseNumber(slip.DocNumber)
		if previous, err := s.repo.GetByID(ctx, *slip.ReplacesPayslipID); err == nil {
			replaced = previous.DocNumber
		} else if !errors.Is(err, hrisrepo.ErrPayslipNotFound) {
			return DocumentMail{}, err
		}
		subject = fmt.Sprintf("%s (Revisi %d)", subject, slip.Revision)
		paragraphsID = append(paragraphsID, fmt.Sprintf("Slip ini menggantikan slip No. %s yang telah dibatalkan.", replaced))
		paragraphsEN = append(paragraphsEN, fmt.Sprintf("This payslip replaces payslip No. %s, which has been cancelled.", replaced))
	}

	company, err := s.mailer.HRContact(ctx)
	if err != nil {
		return DocumentMail{}, err
	}
	if contact := strings.TrimSpace(company.HRContactEmail); contact != "" {
		paragraphsID = append(paragraphsID, fmt.Sprintf("Dokumen ini bersifat rahasia dan hanya ditujukan kepada Anda. Bila ada pertanyaan, silakan hubungi HR di %s.", contact))
		paragraphsEN = append(paragraphsEN, fmt.Sprintf("This document is confidential and intended for you only. If you have any questions, please contact HR at %s.", contact))
	} else {
		paragraphsID = append(paragraphsID, "Dokumen ini bersifat rahasia dan hanya ditujukan kepada Anda. Bila ada pertanyaan, silakan hubungi bagian HR.")
		paragraphsEN = append(paragraphsEN, "This document is confidential and intended for you only. If you have any questions, please contact HR.")
	}

	return DocumentMail{
		Kind:          model.EmailDeliveryKindPayslip,
		ReferenceType: model.EmailDeliveryKindPayslip,
		ReferenceID:   slip.ID,
		Recipient:     recipient,
		Subject:       subject,
		ParagraphsID:  paragraphsID,
		ParagraphsEN:  paragraphsEN,
		Attachments:   []mail.Attachment{attachment},
		BatchID:       batchID,
		RequestedBy:   actorID,
	}, nil
}

func (s *PayslipsService) recipientResponse(slip model.Payslip, recipient ResolvedRecipient, reveal *personalReveal) hrisdto.PayslipRecipientResponse {
	return hrisdto.PayslipRecipientResponse{
		PayslipID:         slip.ID,
		EmployeeID:        slip.EmployeeID,
		EmployeeName:      recipient.EmployeeName,
		DocNumber:         slip.DocNumber,
		Status:            slip.Status,
		Recipient:         reveal.address(slip.EmployeeID, recipient.Address, recipient.Source),
		RecipientSource:   recipient.Source,
		Linked:            recipient.Linked,
		PersonalAvailable: recipient.PersonalAvailable,
		HasPrevious:       recipient.HasPrevious,
		PreviousRecipient: reveal.address(slip.EmployeeID, recipient.PreviousAddress, recipient.PreviousSource),
		IsNew:             recipient.IsNew,
	}
}

// Recipient previews where a slip would be sent for source. A personal
// address is masked unless the viewer may see identity data (then the read
// is access-logged).
func (s *PayslipsService) Recipient(ctx context.Context, viewer DocumentViewer, id string, source string) (hrisdto.PayslipRecipientResponse, error) {
	slip, err := s.getSlip(ctx, id)
	if err != nil {
		return hrisdto.PayslipRecipientResponse{}, err
	}
	recipient, err := s.mailer.ResolveRecipient(ctx, slip.EmployeeID, source)
	if err != nil {
		return hrisdto.PayslipRecipientResponse{}, err
	}
	reveal := newPersonalReveal(viewer)
	response := s.recipientResponse(slip, recipient, reveal)
	if err := s.mailer.logReveal(ctx, reveal); err != nil {
		return hrisdto.PayslipRecipientResponse{}, err
	}
	return response, nil
}

// PayslipSendAudit is the metadata audited for one send (masked recipient,
// never amounts).
func payslipSendAudit(slip model.Payslip, recipient ResolvedRecipient, format string) map[string]any {
	values := payslipAuditValues(slip)
	values["format"] = format
	values["recipient"] = MaskEmail(recipient.Address)
	values["recipient_source"] = recipient.Source
	if recipient.IsNew {
		values["new_address"] = true
	}
	return values
}

// PayslipSendResult is a synchronous send outcome.
type PayslipSendResult struct {
	Response hrisdto.PayslipSendResponse
	Audit    map[string]any
}

// overlapConflict refuses a draft that shares a bonus or reimbursement with
// another slip of the employee that is sent or being sent.
func (s *PayslipsService) overlapConflict(ctx context.Context, slip model.Payslip) error {
	if len(slip.BonusIDs) == 0 && len(slip.ReimbursementIDs) == 0 {
		return nil
	}
	overlap, err := s.repo.OverlappingSlip(ctx, slip.ID)
	if err != nil {
		return err
	}
	if overlap != nil {
		return fmt.Errorf("%w (slip %s)", ErrPayslipItemsTaken, overlap.DocNumber)
	}
	return nil
}

// itemsChanged refuses a draft whose bonuses or reimbursements no longer
// match the records: rejected or unapproved bonuses, reimbursements no
// longer paid, deleted records or changed amounts. A reissue draft keeps the
// voided slip's items whatever their status (it was warned about); only an
// amount that differs from a record that still exists refuses it.
func (s *PayslipsService) itemsChanged(ctx context.Context, slip model.Payslip) error {
	if len(slip.BonusIDs) == 0 && len(slip.ReimbursementIDs) == 0 {
		return nil
	}
	amounts, err := s.decryptAmounts(slip)
	if err != nil {
		return err
	}
	reissue := slip.ReplacesPayslipID != nil

	if len(slip.BonusIDs) > 0 {
		records, err := s.compensation.BonusRecords(ctx, slip.EmployeeID)
		if err != nil {
			return err
		}
		byID := make(map[string]model.BonusRecord, len(records))
		for _, record := range records {
			byID[strings.ToLower(record.ID)] = record
		}
		for _, line := range amounts.Earnings {
			if line.Kind != model.PayslipLineBonus || line.SourceID == "" {
				continue
			}
			record, ok := byID[strings.ToLower(line.SourceID)]
			switch {
			case !ok && reissue:
			case !ok, !reissue && record.ApprovalStatus != "approved", record.Amount != line.Amount:
				return ErrPayslipItemsChanged
			}
		}
	}
	if len(slip.ReimbursementIDs) > 0 {
		records, err := s.repo.ListReimbursementsByIDs(ctx, slip.EmployeeID, slip.ReimbursementIDs)
		if err != nil {
			return err
		}
		byID := make(map[string]hrisrepo.PayslipReimbursement, len(records))
		for _, record := range records {
			byID[strings.ToLower(record.ID)] = record
		}
		for _, line := range amounts.Reimbursements {
			record, ok := byID[strings.ToLower(line.ID)]
			switch {
			case !ok && reissue:
			case !ok, !reissue && record.Status != "paid", record.Amount != line.Amount:
				return ErrPayslipItemsChanged
			}
		}
	}
	return nil
}

// sendConflict reports why a draft must not be sent as it is (nil: it may).
// Sent slips are immutable and may always be re-sent.
func (s *PayslipsService) sendConflict(ctx context.Context, slip model.Payslip) error {
	if slip.Status != model.PayslipStatusDraft {
		return nil
	}
	if err := s.overlapConflict(ctx, slip); err != nil {
		return err
	}
	return s.itemsChanged(ctx, slip)
}

func isSendConflict(err error) bool {
	return errors.Is(err, ErrPayslipItemsTaken) || errors.Is(err, ErrPayslipItemsChanged)
}

func sendConflictCode(err error) string {
	if errors.Is(err, ErrPayslipItemsTaken) {
		return "items_taken"
	}
	return "items_changed"
}

// verifyQueued re-reads a slip once its queued delivery exists (from then
// on a draft cannot be edited or regenerated): it must still be the
// snapshot the attachment was built from, and a draft must still be
// sendable.
func (s *PayslipsService) verifyQueued(ctx context.Context, slip model.Payslip, format string) error {
	current, err := s.repo.GetByID(ctx, slip.ID)
	if errors.Is(err, hrisrepo.ErrPayslipNotFound) {
		return ErrPayslipStateChanged
	}
	if err != nil {
		return err
	}
	if current.Status != slip.Status || current.PayloadEncrypted != slip.PayloadEncrypted {
		return ErrPayslipStateChanged
	}
	if format == "pdf" && optionalText(current.PDFSHA256) != optionalText(slip.PDFSHA256) {
		return ErrPayslipStateChanged
	}
	return s.sendConflict(ctx, current)
}

// sentSnapshot is what MarkSent checks the row against.
func sentSnapshot(slip model.Payslip, format string) hrisrepo.SentSnapshot {
	sent := hrisrepo.SentSnapshot{PayloadEncrypted: slip.PayloadEncrypted}
	if format == "pdf" {
		sent.PDFSHA256 = optionalText(slip.PDFSHA256)
	}
	return sent
}

// markSent records a delivered slip. The guard makes a mismatch impossible
// short of a bug; it is logged, never silently ignored.
func (s *PayslipsService) markSent(ctx context.Context, slip model.Payslip, format string) (model.Payslip, error) {
	updated, err := s.repo.MarkSent(ctx, slip.ID, s.now(), sentSnapshot(slip, format))
	if errors.Is(err, hrisrepo.ErrPayslipStateChanged) {
		slog.ErrorContext(ctx, "payslip e-mailed but the slip no longer matches the sent snapshot", "payslip_id", slip.ID, "doc_number", slip.DocNumber)
		return slip, nil
	}
	return updated, err
}

// Send e-mails one slip synchronously (Kirim / Kirim ulang). The queued
// delivery row is created first (the double-send guard, which also freezes
// the draft); the slip is then re-checked against the attachment before
// anything is sent. A successful send marks a draft sent; an SMTP failure is
// returned in the response (sent=false with the fixed category) together
// with the failed delivery.
func (s *PayslipsService) Send(ctx context.Context, viewer DocumentViewer, id string, input hrisdto.SendPayslipRequest) (PayslipSendResult, error) {
	slip, err := s.getSlip(ctx, id)
	if err != nil {
		return PayslipSendResult{}, err
	}
	if slip.Status == model.PayslipStatusVoid {
		return PayslipSendResult{}, ErrPayslipVoided
	}
	if err := s.mailer.requireReady(ctx); err != nil {
		return PayslipSendResult{}, err
	}
	recipient, err := s.mailer.ResolveRecipient(ctx, slip.EmployeeID, input.RecipientSource)
	if err != nil {
		return PayslipSendResult{}, err
	}
	if err := viewer.guardSend(recipient, input.ExpectedRecipient); err != nil {
		return PayslipSendResult{}, err
	}
	// The response carries the slip's amounts.
	if err := s.logAmountsAccess(ctx, viewer.ActorID, slip.EmployeeID, payslipAccessSend); err != nil {
		return PayslipSendResult{}, err
	}
	if err := s.sendConflict(ctx, slip); err != nil {
		return PayslipSendResult{}, err
	}
	attachment, format, err := s.attachment(ctx, slip, recipient.EmployeeName)
	if err != nil {
		return PayslipSendResult{}, err
	}
	doc, err := s.documentMail(ctx, slip, recipient, attachment, viewer.ActorID, "")
	if err != nil {
		return PayslipSendResult{}, err
	}

	queued, err := s.mailer.Enqueue(ctx, doc)
	if err != nil {
		return PayslipSendResult{}, err
	}
	if err := s.verifyQueued(ctx, slip, format); err != nil {
		s.mailer.CancelQueued(ctx, queued.Delivery.ID, err.Error())
		return PayslipSendResult{}, err
	}

	delivery, sendErr := s.mailer.DeliverQueued(ctx, queued)
	audit := viewer.auditValues(payslipSendAudit(slip, recipient, format))
	result := PayslipSendResult{Audit: audit}
	if sendErr != nil {
		classified, ok := mail.AsSendError(sendErr)
		if !ok || delivery.ID == "" {
			return PayslipSendResult{}, sendErr
		}
		category, message := string(classified.Category), classified.Message()
		result.Response.ErrorCategory = &category
		result.Response.ErrorMessage = &message
		audit["status"] = model.EmailDeliveryStatusFailed
		audit["error_category"] = category
	} else {
		result.Response.Sent = true
		audit["status"] = model.EmailDeliveryStatusSent
		if slip, err = s.markSent(ctx, slip, format); err != nil {
			return PayslipSendResult{}, err
		}
	}
	audit["delivery_id"] = delivery.ID

	reveal := newPersonalReveal(viewer)
	delivery.Recipient = reveal.address(slip.EmployeeID, delivery.Recipient, delivery.RecipientSource)
	result.Response.Delivery = delivery
	if result.Response.Payslip, err = s.toDetail(ctx, slip, reveal); err != nil {
		return PayslipSendResult{}, err
	}
	if err := s.mailer.logReveal(ctx, reveal); err != nil {
		return PayslipSendResult{}, err
	}
	return result, nil
}

type payslipSendCandidate struct {
	slip       model.Payslip
	recipient  ResolvedRecipient
	attachment mail.Attachment
	format     string
}

// selectForSend loads the requested slips in request order and splits them
// into sendable candidates and skipped ones. withAttachments loads the
// files (batch send); the preview only checks they exist.
func (s *PayslipsService) selectForSend(ctx context.Context, viewer DocumentViewer, ids []string, includeSent bool, withAttachments bool) ([]payslipSendCandidate, []hrisdto.PayslipSkipped, int, error) {
	unique := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, raw := range ids {
		id := strings.ToLower(strings.TrimSpace(raw))
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	slips, err := s.repo.GetByIDs(ctx, unique)
	if err != nil {
		return nil, nil, 0, err
	}
	byID := make(map[string]model.Payslip, len(slips))
	for _, slip := range slips {
		byID[strings.ToLower(slip.ID)] = slip
	}

	candidates := []payslipSendCandidate{}
	skipped := []hrisdto.PayslipSkipped{}
	alreadySent := 0
	for _, id := range unique {
		slip, ok := byID[id]
		if !ok {
			payslipID := id
			skipped = append(skipped, hrisdto.PayslipSkipped{PayslipID: &payslipID, Code: "not_found", Reason: "Slip tidak ditemukan"})
			continue
		}
		slipID, number := slip.ID, slip.DocNumber
		skipSlip := func(code string, reason string, name string) {
			skipped = append(skipped, hrisdto.PayslipSkipped{PayslipID: &slipID, DocNumber: &number, EmployeeID: slip.EmployeeID, EmployeeName: name, Code: code, Reason: reason})
		}
		recipient, err := s.mailer.ResolveRecipient(ctx, slip.EmployeeID, DocumentRecipientDefault)
		name := recipient.EmployeeName
		if err != nil {
			if errors.Is(err, ErrDocumentRecipientUnavailable) {
				skipSlip("no_recipient", "Alamat email penerima tidak tersedia", s.employeeName(ctx, slip))
				continue
			}
			return nil, nil, 0, err
		}
		switch {
		case slip.Status == model.PayslipStatusVoid:
			skipSlip("void", "Slip sudah dibatalkan", name)
			continue
		case slip.Status == model.PayslipStatusSent && !includeSent:
			alreadySent++
			skipSlip("already_sent", "Sudah terkirim, dilewati", name)
			continue
		case viewer.ViaMCP && !mcpRecipientAllowed(recipient):
			skipSlip("mcp_recipient_not_login", ErrDocumentRecipientRestricted.Error(), name)
			continue
		}
		busy, err := s.mailer.InFlight(ctx, model.EmailDeliveryKindPayslip, slip.ID)
		if err != nil {
			return nil, nil, 0, err
		}
		if busy {
			skipSlip("sending", "Pengiriman sebelumnya masih berjalan", name)
			continue
		}
		if err := s.sendConflict(ctx, slip); err != nil {
			if isSendConflict(err) {
				skipSlip(sendConflictCode(err), err.Error(), name)
				continue
			}
			return nil, nil, 0, err
		}
		candidate := payslipSendCandidate{slip: slip, recipient: recipient}
		if withAttachments {
			attachment, format, err := s.attachment(ctx, slip, name)
			if err != nil {
				if errors.Is(err, ErrPayslipPDFNotReady) || errors.Is(err, ErrDocumentPDFConverterMissing) || errors.Is(err, ErrDocumentNotFound) || errors.Is(err, ErrDocumentHashMismatch) {
					skipSlip("pdf_not_ready", err.Error(), name)
					continue
				}
				return nil, nil, 0, err
			}
			candidate.attachment, candidate.format = attachment, format
		} else if slip.RenderStatus != model.DocumentRenderReady || slip.PDFPath == nil {
			if s.queue != nil && s.queue.PDFAvailable() {
				skipSlip("pdf_not_ready", ErrPayslipPDFNotReady.Error(), name)
				continue
			}
			if s.mailer == nil || !s.mailer.AllowDocxSend() {
				skipSlip("pdf_not_ready", ErrDocumentPDFConverterMissing.Error(), name)
				continue
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates, skipped, alreadySent, nil
}

// guardBatchRecipients checks a batch against the addresses the human was
// shown (send-preview): once expected is given, every slip that would be
// sent must be listed with exactly its resolved address. Through MCP the map
// is required. Nothing is queued when it fails.
func guardBatchRecipients(viewer DocumentViewer, candidates []payslipSendCandidate, expected map[string]string) error {
	if len(expected) == 0 {
		if viewer.ViaMCP && len(candidates) > 0 {
			return ErrDocumentExpectedRecipientsRequired
		}
		return nil
	}
	byID := make(map[string]string, len(expected))
	for id, address := range expected {
		byID[strings.ToLower(strings.TrimSpace(id))] = address
	}
	for _, candidate := range candidates {
		// The message names the slip, so the caller knows which entry to fix.
		address, listed := byID[strings.ToLower(candidate.slip.ID)]
		if !listed || strings.TrimSpace(address) == "" {
			return fmt.Errorf("%w (slip %s tidak ada di expected_recipients)", ErrDocumentRecipientMismatch, candidate.slip.ID)
		}
		if err := viewer.guardSend(candidate.recipient, address); err != nil {
			return fmt.Errorf("%w (slip %s)", err, candidate.slip.ID)
		}
	}
	return nil
}

// SendPreview lists the recipients a batch send would use (the confirm
// dialog): every address, 'alamat baru' flags, and the skipped slips.
func (s *PayslipsService) SendPreview(ctx context.Context, viewer DocumentViewer, input hrisdto.PayslipSendPreviewRequest) (hrisdto.PayslipSendPreviewResponse, error) {
	candidates, skipped, alreadySent, err := s.selectForSend(ctx, viewer, input.IDs, input.IncludeAlreadySent, false)
	if err != nil {
		return hrisdto.PayslipSendPreviewResponse{}, err
	}
	response := hrisdto.PayslipSendPreviewResponse{
		Recipients:         make([]hrisdto.PayslipRecipientResponse, 0, len(candidates)),
		Skipped:            skipped,
		AlreadySentSkipped: alreadySent,
	}
	reveal := newPersonalReveal(viewer)
	for _, candidate := range candidates {
		response.Recipients = append(response.Recipients, s.recipientResponse(candidate.slip, candidate.recipient, reveal))
	}
	if response.DocumentMailReady, err = s.mailer.Ready(ctx); err != nil {
		return hrisdto.PayslipSendPreviewResponse{}, err
	}
	if err := s.mailer.logReveal(ctx, reveal); err != nil {
		return hrisdto.PayslipSendPreviewResponse{}, err
	}
	return response, nil
}

// PreparedPayslipBatch is a batch whose queued delivery rows exist; the
// handler audits Requested synchronously and then calls StartBatch.
type PreparedPayslipBatch struct {
	Response  hrisdto.PayslipBatchResponse
	Requested []PayslipAuditEntry
	jobs      []DocumentBatchJob
}

// PrepareBatch validates the selection and records one queued delivery per
// slip (which takes the double-send guard and freezes the draft). Nothing
// is sent yet; each job re-checks its slip right before sending. If the
// batch cannot be prepared, the rows already queued are cancelled.
func (s *PayslipsService) PrepareBatch(ctx context.Context, viewer DocumentViewer, input hrisdto.SendPayslipBatchRequest) (prepared PreparedPayslipBatch, err error) {
	if err := s.mailer.requireReady(ctx); err != nil {
		return PreparedPayslipBatch{}, err
	}
	candidates, skipped, alreadySent, err := s.selectForSend(ctx, viewer, input.IDs, input.IncludeAlreadySent, true)
	if err != nil {
		return PreparedPayslipBatch{}, err
	}
	if err := guardBatchRecipients(viewer, candidates, input.ExpectedRecipients); err != nil {
		return PreparedPayslipBatch{}, err
	}

	batchID := NewBatchID()
	prepared = PreparedPayslipBatch{
		Response: hrisdto.PayslipBatchResponse{
			BatchID:            batchID,
			Queued:             []hrisdto.PayslipBatchQueued{},
			Skipped:            skipped,
			AlreadySentSkipped: alreadySent,
		},
		Requested: []PayslipAuditEntry{},
	}
	defer func() {
		if err == nil {
			return
		}
		for _, job := range prepared.jobs {
			s.mailer.CancelQueued(ctx, job.Queued.Delivery.ID, "pengiriman massal tidak dapat dimulai")
		}
		prepared = PreparedPayslipBatch{}
	}()

	reveal := newPersonalReveal(viewer)
	for _, candidate := range candidates {
		slip := candidate.slip
		slipID, number := slip.ID, slip.DocNumber
		skipSlip := func(code string, reason string) {
			prepared.Response.Skipped = append(prepared.Response.Skipped, hrisdto.PayslipSkipped{PayslipID: &slipID, DocNumber: &number, EmployeeID: slip.EmployeeID, EmployeeName: candidate.recipient.EmployeeName, Code: code, Reason: reason})
		}
		// An earlier slip of this batch may now be queued with the same
		// items (e.g. an October and a November draft of one employee).
		if slip.Status == model.PayslipStatusDraft {
			if err := s.overlapConflict(ctx, slip); err != nil {
				if !isSendConflict(err) {
					return prepared, err
				}
				skipSlip(sendConflictCode(err), err.Error())
				continue
			}
		}
		doc, err := s.documentMail(ctx, slip, candidate.recipient, candidate.attachment, viewer.ActorID, batchID)
		if err != nil {
			return prepared, err
		}
		queued, err := s.mailer.Enqueue(ctx, doc)
		if errors.Is(err, ErrDocumentSendInFlight) {
			skipSlip("sending", "Pengiriman sebelumnya masih berjalan")
			continue
		}
		if err != nil {
			return prepared, err
		}

		// Marked here, before the copy below, so the outcome row written by
		// the background sender carries it too.
		audit := viewer.auditValues(payslipSendAudit(slip, candidate.recipient, candidate.format))
		audit["delivery_id"] = queued.Delivery.ID
		audit["batch_id"] = batchID
		prepared.Requested = append(prepared.Requested, PayslipAuditEntry{PayslipID: slip.ID, Values: audit})
		prepared.Response.Queued = append(prepared.Response.Queued, hrisdto.PayslipBatchQueued{
			PayslipID:       slip.ID,
			DocNumber:       slip.DocNumber,
			EmployeeName:    candidate.recipient.EmployeeName,
			DeliveryID:      queued.Delivery.ID,
			Recipient:       reveal.address(slip.EmployeeID, candidate.recipient.Address, candidate.recipient.Source),
			RecipientSource: candidate.recipient.Source,
			IsNew:           candidate.recipient.IsNew,
		})

		outcome := map[string]any{}
		for key, value := range audit {
			outcome[key] = value
		}
		format := candidate.format
		prepared.jobs = append(prepared.jobs, DocumentBatchJob{
			Queued:     queued,
			Resource:   PayslipDocumentKind,
			ResourceID: slipID,
			AuditValue: outcome,
			Precheck: func(ctx context.Context) error {
				return s.verifyQueued(ctx, slip, format)
			},
			OnSent: func(ctx context.Context, _ model.EmailDelivery) error {
				_, err := s.markSent(ctx, slip, format)
				return err
			},
		})
	}
	if err := s.mailer.logReveal(ctx, reveal); err != nil {
		return prepared, err
	}
	return prepared, nil
}

// StartBatch sends a prepared batch in the background.
func (s *PayslipsService) StartBatch(ctx context.Context, actorID string, prepared PreparedPayslipBatch) {
	s.mailer.StartBatch(ctx, actorID, prepared.jobs)
}

// ---------------------------------------------------------------------------
// Void & reissue

// VoidReissue voids a sent slip (kept, with its PDF) and creates the draft
// that replaces it: revision+1, number '<doc>-R<n>', the same pay date, note
// and manual lines. Salary and header data are rebuilt from the current
// records, but the bonuses and reimbursements are exactly those of the
// voided slip (reloaded, so a corrected amount is picked up) and the draft
// keeps the voided slip's generated_at: items paid or approved since belong
// to the next slip (D9), not to the reissue.
func (s *PayslipsService) VoidReissue(ctx context.Context, viewer DocumentViewer, id string, reason string) (hrisdto.VoidReissuePayslipResponse, []PayslipAuditEntry, error) {
	actorID := viewer.ActorID
	slip, err := s.getSlip(ctx, id)
	if err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}
	switch slip.Status {
	case model.PayslipStatusVoid:
		return hrisdto.VoidReissuePayslipResponse{}, nil, ErrPayslipVoided
	case model.PayslipStatusSent:
	default:
		return hrisdto.VoidReissuePayslipResponse{}, nil, ErrPayslipNotSent
	}
	reason = docgen.SingleLine(reason)
	if count := utf8.RuneCountInString(reason); count < payslipVoidReasonMinRunes || count > payslipVoidReasonMaxRunes {
		return hrisdto.VoidReissuePayslipResponse{}, nil, ErrPayslipVoidReasonInvalid
	}
	busy, err := s.mailer.InFlight(ctx, model.EmailDeliveryKindPayslip, slip.ID)
	if err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}
	if busy {
		return hrisdto.VoidReissuePayslipResponse{}, nil, ErrDocumentSendInFlight
	}
	if err := s.logAmountsAccess(ctx, actorID, slip.EmployeeID, payslipAccessReissue); err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}
	company, err := s.company.Profile(ctx)
	if err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}
	oldAmounts, err := s.decryptAmounts(slip)
	if err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}

	var voided, reissue model.Payslip
	err = s.repo.WithTx(ctx, func(txCtx context.Context) error {
		var err error
		if voided, err = s.repo.Void(txCtx, slip.ID, actorID, reason); err != nil {
			if errors.Is(err, hrisrepo.ErrPayslipStateChanged) {
				return ErrPayslipStateChanged
			}
			return err
		}
		employee, err := s.repo.GetEmployee(txCtx, slip.EmployeeID)
		if err != nil {
			return err
		}
		asm, err := s.assembleFor(txCtx, employee, slip.PeriodYear, slip.PeriodMonth, oldAmounts.ManualLines, optionalText(slip.Note), company, &slip)
		if err != nil {
			return err
		}
		if asm.Blocked {
			return ErrPayslipBlocked
		}
		number, code, err := s.codes.EnsureEmployeeCode(txCtx, slip.EmployeeID, company.DocCode)
		if err != nil {
			return err
		}
		employee.EmployeeNumber = &number
		employee.EmployeeCode = &code
		maxRevision, err := s.repo.MaxRevision(txCtx, slip.EmployeeID, slip.PeriodYear, slip.PeriodMonth)
		if err != nil {
			return err
		}
		revision := maxRevision + 1
		replaces := slip.ID
		reissue, err = s.storeSnapshot(txCtx, storeSnapshotInput{
			ActorID:   actorID,
			Replaces:  &replaces,
			Employee:  employee,
			Year:      slip.PeriodYear,
			Month:     slip.PeriodMonth,
			Revision:  revision,
			DocNumber: fmt.Sprintf("%s-R%d", payslipBaseNumber(slip.DocNumber), revision),
			PayDate:   slip.PayDate,
			Note:      slip.Note,
			Assembly:  asm,
			Company:   company,
		})
		if errors.Is(err, hrisrepo.ErrPayslipActiveExists) {
			return ErrPayslipStateChanged
		}
		return err
	})
	if err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}
	s.enqueue(ctx, reissue)

	reveal := newPersonalReveal(viewer)
	voidedDetail, err := s.toDetail(ctx, voided, reveal)
	if err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}
	reissueDetail, err := s.toDetail(ctx, reissue, reveal)
	if err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}
	if err := s.mailer.logReveal(ctx, reveal); err != nil {
		return hrisdto.VoidReissuePayslipResponse{}, nil, err
	}
	voidAudit := payslipAuditValues(voided)
	voidAudit["reason"] = reason
	voidAudit["replaced_by"] = reissue.ID
	reissueAudit := payslipAuditValues(reissue)
	reissueAudit["replaces"] = voided.DocNumber
	return hrisdto.VoidReissuePayslipResponse{Voided: voidedDetail, Reissue: reissueDetail},
		[]PayslipAuditEntry{{PayslipID: voided.ID, Values: voidAudit}, {PayslipID: reissue.ID, Values: reissueAudit}}, nil
}

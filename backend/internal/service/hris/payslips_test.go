package hris

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// In-memory fakes for the payslip service: enough of the repository,
// compensation, HR profile, company, queue and mailer to run generation,
// editing, sending and void & reissue end to end without a database.

type fakePayslipRepo struct {
	mu             sync.Mutex
	slips          map[string]model.Payslip
	employees      map[string]hrisrepo.PayslipEmployee
	reimbursements map[string][]hrisrepo.PayslipReimbursement
	generatedAt    time.Time
	// inFlight reports a queued/sending e-mail of a slip (the mailer fake).
	inFlight func(id string) bool
}

func newFakePayslipRepo() *fakePayslipRepo {
	return &fakePayslipRepo{
		slips:          map[string]model.Payslip{},
		employees:      map[string]hrisrepo.PayslipEmployee{},
		reimbursements: map[string][]hrisrepo.PayslipReimbursement{},
	}
}

func (r *fakePayslipRepo) GetByID(_ context.Context, id string) (model.Payslip, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slip, ok := r.slips[id]
	if !ok {
		return model.Payslip{}, hrisrepo.ErrPayslipNotFound
	}
	return slip, nil
}

func (r *fakePayslipRepo) GetByIDs(ctx context.Context, ids []string) ([]model.Payslip, error) {
	result := []model.Payslip{}
	for _, id := range ids {
		if slip, err := r.GetByID(ctx, id); err == nil {
			result = append(result, slip)
		}
	}
	return result, nil
}

func (r *fakePayslipRepo) all() []model.Payslip {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]model.Payslip, 0, len(r.slips))
	for _, slip := range r.slips {
		result = append(result, slip)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (r *fakePayslipRepo) ListActiveByPeriod(_ context.Context, year int, month int) ([]model.Payslip, error) {
	result := []model.Payslip{}
	for _, slip := range r.all() {
		if slip.PeriodYear == year && slip.PeriodMonth == month && slip.Status != model.PayslipStatusVoid {
			result = append(result, slip)
		}
	}
	return result, nil
}

func (r *fakePayslipRepo) ListByEmployee(_ context.Context, employeeID string, _ int) ([]model.Payslip, error) {
	result := []model.Payslip{}
	for _, slip := range r.all() {
		if slip.EmployeeID == employeeID {
			result = append(result, slip)
		}
	}
	return result, nil
}

func (r *fakePayslipRepo) GetActive(_ context.Context, employeeID string, year int, month int) (model.Payslip, error) {
	for _, slip := range r.all() {
		if slip.EmployeeID == employeeID && slip.PeriodYear == year && slip.PeriodMonth == month && slip.Status != model.PayslipStatusVoid {
			return slip, nil
		}
	}
	return model.Payslip{}, hrisrepo.ErrPayslipNotFound
}

func (r *fakePayslipRepo) LatestSentBefore(_ context.Context, employeeID string, year int, month int) (*hrisrepo.PayslipAnchor, error) {
	var best *hrisrepo.PayslipAnchor
	for _, slip := range r.all() {
		if slip.EmployeeID != employeeID || slip.Status != model.PayslipStatusSent || periodIndex(slip.PeriodYear, slip.PeriodMonth) >= periodIndex(year, month) {
			continue
		}
		if best == nil || periodIndex(slip.PeriodYear, slip.PeriodMonth) > periodIndex(best.PeriodYear, best.PeriodMonth) {
			best = &hrisrepo.PayslipAnchor{ID: slip.ID, PeriodYear: slip.PeriodYear, PeriodMonth: slip.PeriodMonth, GeneratedAt: slip.GeneratedAt}
		}
	}
	return best, nil
}

func (r *fakePayslipRepo) ConsumedItemIDs(_ context.Context, employeeID string, year int, month int) (map[string]bool, map[string]bool, error) {
	bonuses, reimbursements := map[string]bool{}, map[string]bool{}
	for _, slip := range r.all() {
		earlierDraft := slip.Status == model.PayslipStatusDraft && periodIndex(slip.PeriodYear, slip.PeriodMonth) < periodIndex(year, month)
		if slip.EmployeeID != employeeID || (slip.Status != model.PayslipStatusSent && !earlierDraft) {
			continue
		}
		for _, id := range slip.BonusIDs {
			bonuses[strings.ToLower(id)] = true
		}
		for _, id := range slip.ReimbursementIDs {
			reimbursements[strings.ToLower(id)] = true
		}
	}
	return bonuses, reimbursements, nil
}

func (r *fakePayslipRepo) ListReimbursementsForPayslip(_ context.Context, employeeID string) ([]hrisrepo.PayslipReimbursement, error) {
	result := []hrisrepo.PayslipReimbursement{}
	for _, item := range r.reimbursements[employeeID] {
		if item.Status == "paid" || item.Status == "approved" {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakePayslipRepo) ListReimbursementsByIDs(_ context.Context, employeeID string, ids []string) ([]hrisrepo.PayslipReimbursement, error) {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[strings.ToLower(id)] = true
	}
	result := []hrisrepo.PayslipReimbursement{}
	for _, item := range r.reimbursements[employeeID] {
		if wanted[strings.ToLower(item.ID)] {
			result = append(result, item)
		}
	}
	return result, nil
}

func sharesItem(a model.Payslip, b model.Payslip) bool {
	ids := map[string]bool{}
	for _, id := range append(append([]string{}, a.BonusIDs...), a.ReimbursementIDs...) {
		ids[id] = true
	}
	for _, id := range append(append([]string{}, b.BonusIDs...), b.ReimbursementIDs...) {
		if ids[id] {
			return true
		}
	}
	return false
}

func (r *fakePayslipRepo) OverlappingSlip(ctx context.Context, payslipID string) (*hrisrepo.PayslipOverlap, error) {
	slip, err := r.GetByID(ctx, payslipID)
	if err != nil {
		return nil, nil
	}
	for _, other := range r.all() {
		if other.ID == slip.ID || other.EmployeeID != slip.EmployeeID || !sharesItem(slip, other) {
			continue
		}
		sending := other.Status == model.PayslipStatusDraft && r.inFlight != nil && r.inFlight(other.ID)
		if other.Status == model.PayslipStatusSent || sending {
			return &hrisrepo.PayslipOverlap{ID: other.ID, DocNumber: other.DocNumber, Sending: sending}, nil
		}
	}
	return nil, nil
}

func (r *fakePayslipRepo) GetEmployee(_ context.Context, employeeID string) (hrisrepo.PayslipEmployee, error) {
	employee, ok := r.employees[employeeID]
	if !ok {
		return hrisrepo.PayslipEmployee{}, hrisrepo.ErrEmployeeNotFound
	}
	return employee, nil
}

func (r *fakePayslipRepo) ListPeriodEmployees(_ context.Context, _ int, _ int, _ time.Time) ([]hrisrepo.PayslipEmployee, error) {
	result := []hrisrepo.PayslipEmployee{}
	for _, employee := range r.employees {
		result = append(result, employee)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].FullName < result[j].FullName })
	return result, nil
}

func (r *fakePayslipRepo) MaxRevision(_ context.Context, employeeID string, year int, month int) (int, error) {
	max := -1
	for _, slip := range r.all() {
		if slip.EmployeeID == employeeID && slip.PeriodYear == year && slip.PeriodMonth == month && slip.Revision > max {
			max = slip.Revision
		}
	}
	return max, nil
}

func (r *fakePayslipRepo) now() time.Time {
	if r.generatedAt.IsZero() {
		return time.Now()
	}
	return r.generatedAt
}

func (r *fakePayslipRepo) Create(ctx context.Context, params hrisrepo.CreatePayslipParams) (model.Payslip, error) {
	if _, err := r.GetActive(ctx, params.EmployeeID, params.PeriodYear, params.PeriodMonth); err == nil {
		return model.Payslip{}, hrisrepo.ErrPayslipActiveExists
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	version := params.TemplateVersion
	generatedBy := params.GeneratedBy
	slip := model.Payslip{
		ID:                uuid.NewString(),
		EmployeeID:        params.EmployeeID,
		PeriodYear:        params.PeriodYear,
		PeriodMonth:       params.PeriodMonth,
		Revision:          params.Revision,
		DocNumber:         params.DocNumber,
		ReplacesPayslipID: params.ReplacesPayslipID,
		SalaryID:          params.SalaryID,
		ContractID:        params.ContractID,
		Status:            model.PayslipStatusDraft,
		PayDate:           params.PayDate,
		AmountsEncrypted:  params.AmountsEncrypted,
		PayloadEncrypted:  params.PayloadEncrypted,
		ReimbursementIDs:  append([]string{}, params.ReimbursementIDs...),
		BonusIDs:          append([]string{}, params.BonusIDs...),
		Note:              params.Note,
		Warnings:          params.Warnings,
		TemplateVersion:   &version,
		RenderStatus:      params.RenderStatus,
		GeneratedBy:       &generatedBy,
		GeneratedAt:       r.now(),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	if !params.GeneratedAt.IsZero() {
		slip.GeneratedAt = params.GeneratedAt
	}
	r.slips[slip.ID] = slip
	return slip, nil
}

func (r *fakePayslipRepo) UpdateDraftSnapshot(_ context.Context, id string, params hrisrepo.UpdatePayslipSnapshotParams) (model.Payslip, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slip, ok := r.slips[id]
	if !ok || slip.Status != model.PayslipStatusDraft {
		return model.Payslip{}, hrisrepo.ErrPayslipStateChanged
	}
	if r.inFlight != nil && r.inFlight(id) {
		return model.Payslip{}, hrisrepo.ErrPayslipSendInFlight
	}
	slip.SalaryID = params.SalaryID
	slip.ContractID = params.ContractID
	slip.PayDate = params.PayDate
	slip.AmountsEncrypted = params.AmountsEncrypted
	slip.PayloadEncrypted = params.PayloadEncrypted
	slip.ReimbursementIDs = append([]string{}, params.ReimbursementIDs...)
	slip.BonusIDs = append([]string{}, params.BonusIDs...)
	slip.Note = params.Note
	slip.Warnings = params.Warnings
	slip.RenderStatus = params.RenderStatus
	if params.GeneratedBy != "" {
		slip.GeneratedBy = &params.GeneratedBy
		slip.GeneratedAt = r.now()
		if !params.GeneratedAt.IsZero() {
			slip.GeneratedAt = params.GeneratedAt
		}
	}
	slip.UpdatedAt = time.Now()
	r.slips[id] = slip
	return slip, nil
}

func (r *fakePayslipRepo) MarkSent(_ context.Context, id string, at time.Time, sent hrisrepo.SentSnapshot) (model.Payslip, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slip, ok := r.slips[id]
	if !ok || slip.Status == model.PayslipStatusVoid {
		return model.Payslip{}, hrisrepo.ErrPayslipStateChanged
	}
	if slip.Status == model.PayslipStatusDraft && (slip.PayloadEncrypted != sent.PayloadEncrypted || (sent.PDFSHA256 != "" && optionalText(slip.PDFSHA256) != sent.PDFSHA256)) {
		return model.Payslip{}, hrisrepo.ErrPayslipStateChanged
	}
	slip.Status = model.PayslipStatusSent
	slip.LastSentAt = &at
	r.slips[id] = slip
	return slip, nil
}

func (r *fakePayslipRepo) Void(_ context.Context, id string, voidedBy string, reason string) (model.Payslip, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slip, ok := r.slips[id]
	if !ok || slip.Status != model.PayslipStatusSent {
		return model.Payslip{}, hrisrepo.ErrPayslipStateChanged
	}
	now := time.Now()
	slip.Status = model.PayslipStatusVoid
	slip.VoidedAt = &now
	slip.VoidedBy = &voidedBy
	slip.VoidReason = &reason
	r.slips[id] = slip
	return slip, nil
}

func (r *fakePayslipRepo) LatestDeliveries(context.Context, []string) (map[string]model.EmailDelivery, error) {
	return map[string]model.EmailDelivery{}, nil
}

func (r *fakePayslipRepo) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	r.mu.Lock()
	snapshot := make(map[string]model.Payslip, len(r.slips))
	for id, slip := range r.slips {
		snapshot[id] = slip
	}
	r.mu.Unlock()
	if err := fn(ctx); err != nil {
		r.mu.Lock()
		r.slips = snapshot
		r.mu.Unlock()
		return err
	}
	return nil
}

func (r *fakePayslipRepo) setRenderReady(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slip := r.slips[id]
	slip.RenderStatus = model.DocumentRenderReady
	path, sum := "documents/x/payslip/"+id+".pdf.enc", "abc"
	slip.PDFPath, slip.PDFSHA256 = &path, &sum
	r.slips[id] = slip
}

type fakePayslipCompensation struct {
	salaries  map[string]model.SalaryRecord
	bonuses   map[string][]model.BonusRecord
	accessLog []string
	failLog   bool
}

func (c *fakePayslipCompensation) SalaryAsOf(_ context.Context, employeeID string, _ time.Time) (model.SalaryRecord, error) {
	salary, ok := c.salaries[employeeID]
	if !ok {
		return model.SalaryRecord{}, ErrSalaryNotFound
	}
	return salary, nil
}

func (c *fakePayslipCompensation) BonusRecords(_ context.Context, employeeID string) ([]model.BonusRecord, error) {
	return c.bonuses[employeeID], nil
}

func (c *fakePayslipCompensation) LogSalaryAccess(_ context.Context, _ string, resourceID string, action string) error {
	if c.failLog {
		return errors.New("audit insert failed")
	}
	c.accessLog = append(c.accessLog, action+":"+resourceID)
	return nil
}

type fakePayslipCodes struct{ numbers map[string]int }

func (c *fakePayslipCodes) EnsureEmployeeCode(_ context.Context, employeeID string, docCode string) (int, string, error) {
	number, ok := c.numbers[employeeID]
	if !ok {
		number = len(c.numbers) + 1
		c.numbers[employeeID] = number
	}
	return number, FormatEmployeeCode(docCode, number), nil
}

type fakePayslipCompany struct{ record authrepo.CompanyProfileRecord }

func (c fakePayslipCompany) Profile(context.Context) (authrepo.CompanyProfileRecord, error) {
	return c.record, nil
}

func (c fakePayslipCompany) Get(context.Context) (dto.CompanyProfileResponse, error) {
	return dto.CompanyProfileResponse{LegalName: c.record.LegalName}, nil
}

func (c fakePayslipCompany) LogoPNG(context.Context) ([]byte, error) {
	return nil, authservice.ErrCompanyLogoNotFound
}

type fakePayslipQueue struct {
	jobs  []DocumentJob
	noPDF bool
}

func (q *fakePayslipQueue) Enqueue(job DocumentJob) bool { q.jobs = append(q.jobs, job); return true }
func (q *fakePayslipQueue) PDFAvailable() bool           { return !q.noPDF }

type fakePayslipStore struct{ reads int }

func (s *fakePayslipStore) Read(string, string, string) ([]byte, error) {
	s.reads++
	return []byte("%PDF-1.7 fake"), nil
}

type fakeMailSender struct {
	mu         sync.Mutex
	delivered  []authservice.DocumentMailRequest
	queued     []authservice.DocumentMailRequest
	deliveries *fakeMailDeliveries
	// onEnqueue runs right after a queued row is taken (e.g. to simulate an
	// edit that landed just before the guard).
	onEnqueue func()
}

func (f *fakeMailSender) Ready(context.Context) (bool, error) { return true, nil }
func (f *fakeMailSender) GetSettings(context.Context) (dto.DocumentMailSettingResponse, error) {
	return dto.DocumentMailSettingResponse{SenderName: "HR Kantor", Enabled: true}, nil
}
func (f *fakeMailSender) Deliver(_ context.Context, req authservice.DocumentMailRequest) (model.EmailDelivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered = append(f.delivered, req)
	if f.deliveries != nil && req.ReferenceID != nil {
		f.deliveries.setInFlight(*req.ReferenceID, false)
	}
	now := time.Now()
	return model.EmailDelivery{ID: uuid.NewString(), Kind: req.Kind, Recipient: req.Recipient, RecipientSource: req.RecipientSource, Status: model.EmailDeliveryStatusSent, SentAt: &now}, nil
}
func (f *fakeMailSender) Enqueue(_ context.Context, req authservice.DocumentMailRequest) (model.EmailDelivery, error) {
	f.mu.Lock()
	f.queued = append(f.queued, req)
	delivery := model.EmailDelivery{ID: uuid.NewString(), Kind: req.Kind, Recipient: req.Recipient, Status: model.EmailDeliveryStatusQueued}
	if f.deliveries != nil && req.ReferenceID != nil {
		if f.deliveries.isInFlight(*req.ReferenceID) {
			f.mu.Unlock()
			return model.EmailDelivery{}, authservice.ErrDocumentMailInFlight
		}
		f.deliveries.queue(delivery.ID, *req.ReferenceID)
	}
	hook := f.onEnqueue
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return delivery, nil
}

type fakeMailDeliveries struct {
	mu         sync.Mutex
	inFlight   map[string]bool
	cancelled  map[string]string
	references map[string]string
	personal   bool
	staleRuns  int
}

func (f *fakeMailDeliveries) HasInFlight(_ context.Context, _ string, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inFlight[id], nil
}

func (f *fakeMailDeliveries) isInFlight(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inFlight[id]
}

func (f *fakeMailDeliveries) setInFlight(id string, value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight[id] = value
}

func (f *fakeMailDeliveries) queue(deliveryID string, referenceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.references == nil {
		f.references = map[string]string{}
	}
	f.references[deliveryID] = referenceID
	f.inFlight[referenceID] = true
}

func (f *fakeMailDeliveries) FailStaleInFlight(context.Context, string, string, time.Duration, string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.staleRuns++
	return 0, nil
}

func (f *fakeMailDeliveries) FailStaleDocuments(context.Context, time.Duration, string) (int64, error) {
	return 0, nil
}
func (f *fakeMailDeliveries) ListByReference(context.Context, string, string) ([]model.EmailDelivery, error) {
	rows := []model.EmailDelivery{
		{ID: "d1", Kind: model.EmailDeliveryKindPayslip, Status: model.EmailDeliveryStatusSent, Recipient: "staff.ops@kantor.local", RecipientSource: model.EmailRecipientSourceLogin},
		{ID: "t1", Kind: model.EmailDeliveryKindTest, Status: model.EmailDeliveryStatusSent},
	}
	if f.personal {
		rows = append(rows, model.EmailDelivery{ID: "d2", Kind: model.EmailDeliveryKindPayslip, Status: model.EmailDeliveryStatusSent, Recipient: "budi.pribadi@example.com", RecipientSource: model.EmailRecipientSourcePersonal})
	}
	return rows, nil
}
func (f *fakeMailDeliveries) MarkFailed(_ context.Context, id string, message string) (model.EmailDelivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancelled == nil {
		f.cancelled = map[string]string{}
	}
	f.cancelled[id] = message
	if reference, ok := f.references[id]; ok {
		f.inFlight[reference] = false
	}
	return model.EmailDelivery{ID: id, Status: model.EmailDeliveryStatusFailed}, nil
}

type fakeMailRecipients struct {
	repo        *fakePayslipRepo
	last        map[string]string
	lastSource  map[string]string
	identityLog []string
	failLog     bool
}

func (f *fakeMailRecipients) EmployeeIDForDocument(ctx context.Context, _ string, id string) (string, error) {
	slip, err := f.repo.GetByID(ctx, id)
	if err != nil {
		return "e-budi", nil
	}
	return slip.EmployeeID, nil
}

func (f *fakeMailRecipients) LogIdentityAccess(_ context.Context, _ string, employeeID string, action string) error {
	if f.failLog {
		return errors.New("audit insert failed")
	}
	f.identityLog = append(f.identityLog, action+":"+employeeID)
	return nil
}

func (f *fakeMailRecipients) GetRecipient(_ context.Context, employeeID string) (hrisrepo.DocumentRecipient, error) {
	employee, ok := f.repo.employees[employeeID]
	if !ok {
		return hrisrepo.DocumentRecipient{}, hrisrepo.ErrEmployeeNotFound
	}
	return hrisrepo.DocumentRecipient{EmployeeID: employee.ID, FullName: employee.FullName, UserID: employee.UserID, LoginEmail: employee.LoginEmail, Email: employee.Email, PersonalEmail: employee.PersonalEmail}, nil
}

func (f *fakeMailRecipients) LatestSentDeliveryForEmployee(_ context.Context, employeeID string) (model.EmailDelivery, bool, error) {
	if address, ok := f.last[employeeID]; ok {
		return model.EmailDelivery{Recipient: address, RecipientSource: f.lastSource[employeeID]}, true, nil
	}
	return model.EmailDelivery{}, false, nil
}

type payslipFixture struct {
	service      *PayslipsService
	repo         *fakePayslipRepo
	compensation *fakePayslipCompensation
	queue        *fakePayslipQueue
	sender       *fakeMailSender
	recipients   *fakeMailRecipients
	deliveries   *fakeMailDeliveries
	ctx          context.Context
}

const payslipTestActor = "22222222-2222-2222-2222-222222222222"

// payslipTestViewer may see identity data (personal addresses in full).
var payslipTestViewer = DocumentViewer{ActorID: payslipTestActor, CanViewIdentity: true}

func newPayslipFixture(t *testing.T) *payslipFixture {
	t.Helper()
	encrypter, err := security.NewEncrypter("payslip-test-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakePayslipRepo()
	budi := demoEmployee()
	budi.ID = "e-budi"
	budi.JobTitle = nil
	repo.employees[budi.ID] = budi
	gita := demoEmployee()
	gita.ID, gita.FullName, gita.UserID, gita.LoginEmail = "e-gita", "Gita Permatasari", nil, nil
	gita.Email = "gita.permatasari@kantor.local"
	repo.employees[gita.ID] = gita
	repo.reimbursements[budi.ID] = []hrisrepo.PayslipReimbursement{
		reimb("r1", 185_000, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))),
		reimb("r2", 350_000, time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))),
	}
	compensation := &fakePayslipCompensation{
		salaries: map[string]model.SalaryRecord{budi.ID: *budiSalary()},
		bonuses: map[string][]model.BonusRecord{budi.ID: {
			bonus("thr", 11_000_000, 3, 2026, "approved", timePtr(wib(2026, 3, 20, 10))),
			bonus("kinerja", 3_000_000, 9, 2026, "approved", timePtr(wib(2026, 9, 28, 10))),
		}},
	}
	queue := &fakePayslipQueue{}
	deliveries := &fakeMailDeliveries{inFlight: map[string]bool{}}
	sender := &fakeMailSender{deliveries: deliveries}
	recipients := &fakeMailRecipients{repo: repo, last: map[string]string{}, lastSource: map[string]string{}}
	repo.inFlight = deliveries.isInFlight
	company := fakePayslipCompany{record: authrepo.CompanyProfileRecord{LegalName: "PT Contoh Teknologi Nusantara", DocCode: "CTN", HRContactEmail: "hr@contoh.co.id", PaydayDay: 25}}
	mailer := NewDocumentMailer(sender, deliveries, recipients, company, nil, false)
	mailer.spacing = 0
	mailer.runAsync = func(fn func()) { fn() }
	service := NewPayslipsService(repo, compensation, &fakePayslipCodes{numbers: map[string]int{}}, company, queue, &fakePayslipStore{}, mailer, encrypter)
	service.now = func() time.Time { return wib(2026, 10, 2, 12) }
	ctx := tenant.WithInfo(context.Background(), tenant.Info{ID: "00000000-0000-0000-0000-000000000001", Slug: "default"})
	return &payslipFixture{service: service, repo: repo, compensation: compensation, queue: queue, sender: sender, recipients: recipients, deliveries: deliveries, ctx: ctx}
}

func (f *payslipFixture) generate(t *testing.T, year int, month int, ids ...string) hrisdto.GeneratePayslipsResponse {
	t.Helper()
	result, _, err := f.service.Generate(f.ctx, payslipTestActor, hrisdto.GeneratePayslipsRequest{Year: year, Month: month, EmployeeIDs: ids})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return result
}

func TestPayslipGenerateEditRegenerate(t *testing.T) {
	f := newPayslipFixture(t)
	result := f.generate(t, 2026, 9, "e-budi", "e-gita", "e-budi")
	if len(result.Generated) != 1 || len(result.Skipped) != 1 {
		t.Fatalf("generated %d skipped %d: %+v", len(result.Generated), len(result.Skipped), result.Skipped)
	}
	if result.Skipped[0].Code != "blocked" || !strings.Contains(strings.ToLower(result.Skipped[0].Reason), "tanpa data gaji") {
		t.Fatalf("gita skip = %+v", result.Skipped[0])
	}
	if result.PayDate != "2026-09-25" {
		t.Fatalf("pay date = %s", result.PayDate)
	}
	item := result.Generated[0]
	if *item.DocNumber != "PAY/2026/09/0001" || item.Totals.TotalDiterima != 17_190_000 || item.Status != model.PayslipStatusDraft || item.RenderStatus != model.DocumentRenderPending {
		t.Fatalf("generated item = %+v", item)
	}
	if item.EmployeeCode == nil || *item.EmployeeCode != "CTN-0001" {
		t.Fatalf("employee code = %v", item.EmployeeCode)
	}
	if len(f.queue.jobs) != 1 || f.queue.jobs[0].Kind != PayslipDocumentKind {
		t.Fatalf("render jobs = %+v", f.queue.jobs)
	}

	id := *item.PayslipID
	detail, changed, err := f.service.Update(f.ctx, payslipTestViewer, id, hrisdto.UpdatePayslipRequest{
		Note:        "Penyesuaian pro-rata September",
		ManualLines: []hrisdto.PayslipManualLineRequest{{Label: "Penyesuaian", Amount: -250_000, Keterangan: "koreksi"}},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if strings.Join(changed, ",") != "note,manual_lines" {
		t.Fatalf("changed = %v", changed)
	}
	if detail.Totals.TotalPotongan != 1_245_000 || detail.Totals.TotalDiterima != 16_940_000 {
		t.Fatalf("totals after edit = %+v", detail.Totals)
	}
	last := detail.Deductions[len(detail.Deductions)-1]
	if last.Label != "Penyesuaian" || last.Amount != 250_000 {
		t.Fatalf("manual deduction row = %+v", last)
	}
	if len(f.queue.jobs) != 2 {
		t.Fatalf("edit must queue a re-render, jobs = %d", len(f.queue.jobs))
	}

	// Regenerating the draft keeps the note and the manual lines.
	again := f.generate(t, 2026, 9, "e-budi")
	if len(again.Generated) != 1 || *again.Generated[0].PayslipID != id || *again.Generated[0].DocNumber != "PAY/2026/09/0001" {
		t.Fatalf("regenerated = %+v", again)
	}
	regenerated, err := f.service.Get(f.ctx, payslipTestViewer, id)
	if err != nil {
		t.Fatal(err)
	}
	if regenerated.Note != "Penyesuaian pro-rata September" || len(regenerated.ManualLines) != 1 || regenerated.Totals.TotalDiterima != 16_940_000 {
		t.Fatalf("regeneration lost edits: note=%q manual=%v totals=%+v", regenerated.Note, regenerated.ManualLines, regenerated.Totals)
	}

	// Invalid edits are refused.
	if _, _, err := f.service.Update(f.ctx, payslipTestViewer, id, hrisdto.UpdatePayslipRequest{Note: strings.Repeat("x", 121)}); !errors.Is(err, ErrPayslipNoteInvalid) {
		t.Fatalf("long note err = %v", err)
	}
	if _, _, err := f.service.Update(f.ctx, payslipTestViewer, id, hrisdto.UpdatePayslipRequest{ManualLines: []hrisdto.PayslipManualLineRequest{{Label: " ", Amount: 5}}}); !errors.Is(err, ErrPayslipManualLineInvalid) {
		t.Fatalf("blank label err = %v", err)
	}
}

func TestPayslipSendVoidReissueKeepsItems(t *testing.T) {
	f := newPayslipFixture(t)
	f.repo.generatedAt = wib(2026, 9, 29, 9)
	generated := f.generate(t, 2026, 9, "e-budi")
	id := *generated.Generated[0].PayslipID

	// No PDF yet: sending is refused.
	if _, err := f.service.Send(f.ctx, payslipTestViewer, id, ""); !errors.Is(err, ErrPayslipPDFNotReady) {
		t.Fatalf("send without PDF err = %v", err)
	}
	f.repo.setRenderReady(id)

	sent, err := f.service.Send(f.ctx, payslipTestViewer, id, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !sent.Response.Sent || sent.Response.Payslip.Status != model.PayslipStatusSent {
		t.Fatalf("send result = %+v", sent.Response)
	}
	req := f.sender.delivered[0]
	if req.Recipient != "staff.ops@kantor.local" || req.RecipientSource != model.EmailRecipientSourceLogin {
		t.Fatalf("recipient = %s (%s)", req.Recipient, req.RecipientSource)
	}
	if len(req.Attachments) != 1 || req.Attachments[0].Filename != "Slip_Gaji_2026-09_Budi_Santoso.pdf" || req.Attachments[0].ContentType != "application/pdf" {
		t.Fatalf("attachments = %+v", req.Attachments)
	}
	if strings.Contains(req.Text, "17.190.000") || strings.Contains(req.HTML, "17.190.000") {
		t.Fatal("the e-mail body must not carry amounts")
	}
	if sent.Audit["recipient"] != "st***@kantor.local" {
		t.Fatalf("audit recipient = %v", sent.Audit["recipient"])
	}

	// A sent slip is not regenerated.
	again := f.generate(t, 2026, 9, "e-budi")
	if len(again.Skipped) != 1 || again.Skipped[0].Code != "already_sent" {
		t.Fatalf("regenerating a sent slip = %+v", again)
	}

	original, _ := f.repo.GetByID(f.ctx, id)
	result, audits, err := f.service.VoidReissue(f.ctx, payslipTestViewer, id, "Salah nominal tunjangan")
	if err != nil {
		t.Fatalf("void-reissue: %v", err)
	}
	if result.Voided.Status != model.PayslipStatusVoid || result.Reissue.Status != model.PayslipStatusDraft {
		t.Fatalf("statuses = %s / %s", result.Voided.Status, result.Reissue.Status)
	}
	if result.Reissue.DocNumber != "PAY/2026/09/0001-R1" || result.Reissue.Revision != 1 || result.Reissue.ReplacesDocNumber == nil || *result.Reissue.ReplacesDocNumber != "PAY/2026/09/0001" {
		t.Fatalf("reissue = %+v", result.Reissue)
	}
	reissue, _ := f.repo.GetByID(f.ctx, result.Reissue.ID)
	if strings.Join(reissue.BonusIDs, ",") != strings.Join(original.BonusIDs, ",") || strings.Join(reissue.ReimbursementIDs, ",") != strings.Join(original.ReimbursementIDs, ",") {
		t.Fatalf("reissue items %v/%v differ from original %v/%v", reissue.BonusIDs, reissue.ReimbursementIDs, original.BonusIDs, original.ReimbursementIDs)
	}
	if len(audits) != 2 || audits[0].Values["reason"] != "Salah nominal tunjangan" {
		t.Fatalf("audits = %+v", audits)
	}

	// The reissue's e-mail says which slip it replaces.
	f.repo.setRenderReady(reissue.ID)
	if _, err := f.service.Send(f.ctx, payslipTestViewer, reissue.ID, ""); err != nil {
		t.Fatalf("send reissue: %v", err)
	}
	reissueMail := f.sender.delivered[len(f.sender.delivered)-1]
	if !strings.Contains(reissueMail.Text, "menggantikan slip No. PAY/2026/09/0001") || !strings.Contains(reissueMail.Subject, "(Revisi 1)") {
		t.Fatalf("reissue mail subject=%q text=%q", reissueMail.Subject, reissueMail.Text)
	}
	if reissueMail.Attachments[0].Filename != "Slip_Gaji_2026-09_Budi_Santoso_R1.pdf" {
		t.Fatalf("reissue filename = %s", reissueMail.Attachments[0].Filename)
	}

	// Only sent slips can be voided.
	if _, _, err := f.service.VoidReissue(f.ctx, payslipTestViewer, id, "lagi"); !errors.Is(err, ErrPayslipVoided) {
		t.Fatalf("void a void slip err = %v", err)
	}
}

// D9 over two months: the September slip is generated on 24 Sep; a bonus of
// September approved after that and reimbursements paid after that land on
// the October slip; items already on the sent September slip do not come
// back.
func TestPayslipLateApprovalRollsToNextSlip(t *testing.T) {
	f := newPayslipFixture(t)
	at := func(now time.Time) {
		f.repo.generatedAt = now
		f.service.now = func() time.Time { return now }
	}
	at(wib(2026, 9, 24, 9))
	f.compensation.bonuses["e-budi"] = []model.BonusRecord{
		bonus("kinerja", 3_000_000, 9, 2026, "approved", timePtr(wib(2026, 9, 20, 10))),
	}
	f.repo.reimbursements["e-budi"] = append(f.repo.reimbursements["e-budi"],
		reimb("r-early", 40_000, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 10, 10))))
	sep := f.generate(t, 2026, 9, "e-budi")
	sepID := *sep.Generated[0].PayslipID
	sepSlip, _ := f.repo.GetByID(f.ctx, sepID)
	equalIDs(t, sepSlip.BonusIDs, "kinerja")
	equalIDs(t, sepSlip.ReimbursementIDs, "r-early") // r1/r2 are paid on 25 Sep, after generation
	f.repo.setRenderReady(sepID)
	if _, err := f.service.Send(f.ctx, payslipTestViewer, sepID, ""); err != nil {
		t.Fatal(err)
	}

	// After the September slip: a late approval and a late payment.
	f.compensation.bonuses["e-budi"] = append(f.compensation.bonuses["e-budi"],
		bonus("sep-late", 2_000_000, 9, 2026, "approved", timePtr(wib(2026, 9, 26, 10))))
	f.repo.reimbursements["e-budi"] = append(f.repo.reimbursements["e-budi"],
		reimb("r-late", 99_000, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 30, 10))))

	at(wib(2026, 10, 20, 12))
	oct := f.generate(t, 2026, 10, "e-budi")
	if len(oct.Generated) != 1 {
		t.Fatalf("october = %+v", oct)
	}
	slip, _ := f.repo.GetByID(f.ctx, *oct.Generated[0].PayslipID)
	equalIDs(t, slip.BonusIDs, "sep-late")
	equalIDs(t, slip.ReimbursementIDs, "r1", "r2", "r-late")
	if oct.PayDate != "2026-10-23" {
		t.Fatalf("october pay date = %s", oct.PayDate)
	}
}

func TestPayslipBatchSendSkipsSentAndUsesRecipients(t *testing.T) {
	f := newPayslipFixture(t)
	gita := f.repo.employees["e-gita"]
	f.compensation.salaries[gita.ID] = model.SalaryRecord{ID: "s-gita", BaseSalary: 5_000_000}
	generated := f.generate(t, 2026, 9, "e-budi", "e-gita")
	if len(generated.Generated) != 2 {
		t.Fatalf("generated = %+v", generated)
	}
	ids := []string{}
	for _, item := range generated.Generated {
		ids = append(ids, *item.PayslipID)
		f.repo.setRenderReady(*item.PayslipID)
	}
	// Budi was mailed elsewhere before: 'alamat baru'.
	f.recipients.last["e-budi"] = "budi.lama@example.com"

	preview, err := f.service.SendPreview(f.ctx, payslipTestViewer, hrisdto.PayslipSendPreviewRequest{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Recipients) != 2 {
		t.Fatalf("preview = %+v", preview)
	}
	byName := map[string]hrisdto.PayslipRecipientResponse{}
	for _, item := range preview.Recipients {
		byName[item.EmployeeName] = item
	}
	if r := byName["Budi Santoso"]; r.Recipient != "staff.ops@kantor.local" || !r.IsNew || r.RecipientSource != "login" {
		t.Fatalf("budi recipient = %+v", r)
	}
	if r := byName["Gita Permatasari"]; r.Recipient != "gita.permatasari@kantor.local" || r.RecipientSource != "employee" || r.IsNew {
		t.Fatalf("gita recipient = %+v", r)
	}

	prepared, err := f.service.PrepareBatch(f.ctx, payslipTestViewer, hrisdto.SendPayslipBatchRequest{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Response.Queued) != 2 || len(prepared.Requested) != 2 || len(f.sender.queued) != 2 {
		t.Fatalf("prepared = %+v", prepared.Response)
	}
	f.service.StartBatch(f.ctx, payslipTestActor, prepared)
	// runAsync is inline in tests, but the batch needs a tenant pool for the
	// scoped connection: without one nothing is delivered and no slip moves.
	for _, id := range ids {
		slip, _ := f.repo.GetByID(f.ctx, id)
		if slip.Status != model.PayslipStatusDraft {
			t.Fatalf("slip %s status %s without a delivered mail", id, slip.Status)
		}
	}

	// Deliver directly (what the background worker does per job).
	for _, job := range prepared.jobs {
		f.service.mailer.deliverBatchJob(f.ctx, payslipTestActor, job)
	}
	for _, id := range ids {
		slip, _ := f.repo.GetByID(f.ctx, id)
		if slip.Status != model.PayslipStatusSent {
			t.Fatalf("slip %s status %s after batch", id, slip.Status)
		}
	}

	second, err := f.service.PrepareBatch(f.ctx, payslipTestViewer, hrisdto.SendPayslipBatchRequest{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Response.Queued) != 0 || second.Response.AlreadySentSkipped != 2 {
		t.Fatalf("second batch = %+v", second.Response)
	}

	// In-flight guard.
	f.deliveries.inFlight[ids[0]] = true
	third, err := f.service.PrepareBatch(f.ctx, payslipTestViewer, hrisdto.SendPayslipBatchRequest{IDs: ids, IncludeAlreadySent: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Response.Queued) != 1 || third.Response.Skipped[0].Code != "sending" {
		t.Fatalf("third batch = %+v", third.Response)
	}
}

func TestPayslipPDFAccessLoggedFailClosed(t *testing.T) {
	f := newPayslipFixture(t)
	generated := f.generate(t, 2026, 9, "e-budi")
	id := *generated.Generated[0].PayslipID
	if _, err := f.service.PDF(f.ctx, payslipTestActor, id, "inline"); !errors.Is(err, ErrPayslipPDFNotReady) {
		t.Fatalf("pdf before render err = %v", err)
	}
	f.repo.setRenderReady(id)
	store := f.service.store.(*fakePayslipStore)

	f.compensation.failLog = true
	if _, err := f.service.PDF(f.ctx, payslipTestActor, id, "inline"); err == nil {
		t.Fatal("PDF must fail when the access row cannot be written")
	}
	if store.reads != 0 {
		t.Fatal("the PDF was read although the access was not logged")
	}

	f.compensation.failLog = false
	file, err := f.service.PDF(f.ctx, payslipTestActor, id, "attachment")
	if err != nil {
		t.Fatal(err)
	}
	if file.Filename != "Slip_Gaji_2026-09_Budi_Santoso.pdf" || file.ContentType != "application/pdf" {
		t.Fatalf("file = %s %s", file.Filename, file.ContentType)
	}
	if got := f.compensation.accessLog[len(f.compensation.accessLog)-1]; got != "payslip_pdf_attachment:e-budi" {
		t.Fatalf("access log = %v", f.compensation.accessLog)
	}

	docx, err := f.service.Docx(f.ctx, payslipTestActor, id)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	if !strings.HasSuffix(docx.Filename, ".docx") || len(docx.Data) < 1000 {
		t.Fatalf("docx = %s (%d bytes)", docx.Filename, len(docx.Data))
	}
}

func TestPayslipListPreviewsAndLogs(t *testing.T) {
	f := newPayslipFixture(t)
	list, err := f.service.List(f.ctx, payslipTestViewer, 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if list.Summary.Total != 2 || list.Summary.NotGenerated != 2 || list.Summary.Blocked != 1 {
		t.Fatalf("summary = %+v", list.Summary)
	}
	if list.DefaultPayDate != "2026-09-25" || !list.DocumentMailReady || !list.PDFAvailable {
		t.Fatalf("list flags = %+v", list)
	}
	if f.compensation.accessLog[0] != "payslip_list_view:2026-09" {
		t.Fatalf("access log = %v", f.compensation.accessLog)
	}
	for _, item := range list.Items {
		if item.EmployeeName == "Budi Santoso" && (item.Totals.TotalDiterima != 17_190_000 || !item.Preview) {
			t.Fatalf("budi preview = %+v", item)
		}
	}

	f.compensation.failLog = true
	if _, err := f.service.List(f.ctx, payslipTestViewer, 2026, 9); err == nil {
		t.Fatal("list must fail closed when the access cannot be logged")
	}
}

func TestDocumentDeliveriesPermissionByKind(t *testing.T) {
	f := newPayslipFixture(t)
	id := uuid.NewString()
	rows, err := f.service.mailer.ListDeliveries(f.ctx, "payslip", id, func(kind string) bool { return kind == "payslip" }, payslipTestViewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Kind != model.EmailDeliveryKindPayslip {
		t.Fatalf("test rows must be hidden: %+v", rows)
	}
	if _, err := f.service.mailer.ListDeliveries(f.ctx, "payslip", id, func(string) bool { return false }, payslipTestViewer); !errors.Is(err, ErrDocumentDeliveryForbidden) {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.service.mailer.ListDeliveries(f.ctx, "test", id, func(string) bool { return true }, payslipTestViewer); !errors.Is(err, ErrDocumentDeliveryReferenceType) {
		t.Fatalf("err = %v", err)
	}
}

func TestDocumentRecipientSources(t *testing.T) {
	f := newPayslipFixture(t)
	budi := f.repo.employees["e-budi"]
	budi.PersonalEmail = strPtr("budi.pribadi@example.com")
	f.repo.employees["e-budi"] = budi

	cases := map[string]string{
		"":         "staff.ops@kantor.local",
		"login":    "staff.ops@kantor.local",
		"employee": "budi@example.com",
		"personal": "budi.pribadi@example.com",
	}
	for source, want := range cases {
		got, err := f.service.mailer.ResolveRecipient(f.ctx, "e-budi", source)
		if err != nil || got.Address != want {
			t.Errorf("source %q -> %q, %v (want %q)", source, got.Address, err, want)
		}
	}
	if _, err := f.service.mailer.ResolveRecipient(f.ctx, "e-gita", "personal"); !errors.Is(err, ErrDocumentRecipientUnavailable) {
		t.Fatalf("missing personal e-mail err = %v", err)
	}
	if _, err := f.service.mailer.ResolveRecipient(f.ctx, "e-gita", "login"); !errors.Is(err, ErrDocumentRecipientUnavailable) {
		t.Fatalf("unlinked login err = %v", err)
	}
	if _, err := f.service.mailer.ResolveRecipient(f.ctx, "e-budi", "bogus"); !errors.Is(err, ErrDocumentRecipientSourceBad) {
		t.Fatalf("bad source err = %v", err)
	}
}

// sendSeptember generates, renders and sends Budi's September slip at
// 29 Sep 2026 09:00 (bonus kinerja, reimbursements r1 + r2).
func (f *payslipFixture) sendSeptember(t *testing.T) model.Payslip {
	t.Helper()
	f.at(wib(2026, 9, 29, 9))
	generated := f.generate(t, 2026, 9, "e-budi")
	id := *generated.Generated[0].PayslipID
	f.repo.setRenderReady(id)
	if _, err := f.service.Send(f.ctx, payslipTestViewer, id, ""); err != nil {
		t.Fatalf("send september: %v", err)
	}
	slip, _ := f.repo.GetByID(f.ctx, id)
	return slip
}

func (f *payslipFixture) at(now time.Time) {
	f.repo.generatedAt = now
	f.service.now = func() time.Time { return now }
}

// The same bonus or reimbursement is never on two sent slips: a later
// draft does not take items held by an earlier draft, and a draft whose
// items are already on another sent slip cannot be sent (single or batch)
// until it is regenerated.
func TestPayslipItemsNeverPaidTwice(t *testing.T) {
	f := newPayslipFixture(t)
	f.sendSeptember(t)
	f.compensation.bonuses["e-budi"] = append(f.compensation.bonuses["e-budi"],
		bonus("oct-b", 1_000_000, 10, 2026, "approved", timePtr(wib(2026, 10, 5, 10))))
	f.repo.reimbursements["e-budi"] = append(f.repo.reimbursements["e-budi"],
		reimb("r-oct", 210_000, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 10, 10, 10))))

	// October is generated (and left as a draft, e.g. 'Gagal'), then
	// November: November does not take October's items.
	f.at(wib(2026, 10, 20, 12))
	oct := f.generate(t, 2026, 10, "e-budi")
	octSlip, _ := f.repo.GetByID(f.ctx, *oct.Generated[0].PayslipID)
	equalIDs(t, octSlip.BonusIDs, "oct-b")
	equalIDs(t, octSlip.ReimbursementIDs, "r-oct")
	f.at(wib(2026, 11, 20, 12))
	nov := f.generate(t, 2026, 11, "e-budi")
	novSlip, _ := f.repo.GetByID(f.ctx, *nov.Generated[0].PayslipID)
	if len(novSlip.BonusIDs) != 0 || len(novSlip.ReimbursementIDs) != 0 {
		t.Fatalf("november took october's items: %v / %v", novSlip.BonusIDs, novSlip.ReimbursementIDs)
	}

	// A draft that holds them anyway (e.g. generated before the earlier
	// one existed) cannot be sent once the earlier one is sent.
	novSlip.BonusIDs, novSlip.ReimbursementIDs = octSlip.BonusIDs, octSlip.ReimbursementIDs
	f.repo.slips[novSlip.ID] = novSlip
	f.repo.setRenderReady(octSlip.ID)
	f.repo.setRenderReady(novSlip.ID)
	if _, err := f.service.Send(f.ctx, payslipTestViewer, octSlip.ID, ""); err != nil {
		t.Fatalf("send october: %v", err)
	}
	delivered := len(f.sender.delivered)
	if _, err := f.service.Send(f.ctx, payslipTestViewer, novSlip.ID, ""); !errors.Is(err, ErrPayslipItemsTaken) || !strings.Contains(err.Error(), octSlip.DocNumber) {
		t.Fatalf("send november err = %v", err)
	}
	batch, err := f.service.PrepareBatch(f.ctx, payslipTestViewer, hrisdto.SendPayslipBatchRequest{IDs: []string{novSlip.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Response.Queued) != 0 || len(batch.Response.Skipped) != 1 || batch.Response.Skipped[0].Code != "items_taken" {
		t.Fatalf("batch = %+v", batch.Response)
	}
	if len(f.sender.delivered) != delivered {
		t.Fatal("a conflicting slip was e-mailed")
	}

	// Regenerating drops the items that are already paid; then it sends.
	f.generate(t, 2026, 11, "e-budi")
	regenerated, _ := f.repo.GetByID(f.ctx, novSlip.ID)
	if len(regenerated.BonusIDs) != 0 || len(regenerated.ReimbursementIDs) != 0 {
		t.Fatalf("regenerated november = %v / %v", regenerated.BonusIDs, regenerated.ReimbursementIDs)
	}
	f.repo.setRenderReady(novSlip.ID)
	if _, err := f.service.Send(f.ctx, payslipTestViewer, novSlip.ID, ""); err != nil {
		t.Fatalf("send regenerated november: %v", err)
	}
}

// Two drafts of one employee in one batch that share items: only the first
// is queued.
func TestPayslipBatchSkipsSecondSlipWithSameItems(t *testing.T) {
	f := newPayslipFixture(t)
	f.at(wib(2026, 10, 20, 12))
	oct := f.generate(t, 2026, 10, "e-budi")
	octID := *oct.Generated[0].PayslipID
	octSlip, _ := f.repo.GetByID(f.ctx, octID)
	nov := f.generate(t, 2026, 11, "e-budi")
	novSlip, _ := f.repo.GetByID(f.ctx, *nov.Generated[0].PayslipID)
	novSlip.ReimbursementIDs = []string{"shared"}
	octSlip.ReimbursementIDs = []string{"shared"}
	f.repo.slips[novSlip.ID], f.repo.slips[octID] = novSlip, octSlip
	f.repo.setRenderReady(octID)
	f.repo.setRenderReady(novSlip.ID)
	// The snapshot amounts do not list "shared": only the overlap matters.
	batch, err := f.service.PrepareBatch(f.ctx, payslipTestViewer, hrisdto.SendPayslipBatchRequest{IDs: []string{octID, novSlip.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Response.Queued) != 1 || batch.Response.Queued[0].PayslipID != octID {
		t.Fatalf("queued = %+v skipped = %+v", batch.Response.Queued, batch.Response.Skipped)
	}
	if last := batch.Response.Skipped[len(batch.Response.Skipped)-1]; last.Code != "items_taken" {
		t.Fatalf("skipped = %+v", batch.Response.Skipped)
	}
}

// A draft whose bonus was rejected (or whose reimbursement amount changed)
// after the snapshot is refused until it is regenerated.
func TestPayslipSendRefusesChangedItems(t *testing.T) {
	f := newPayslipFixture(t)
	generated := f.generate(t, 2026, 9, "e-budi")
	id := *generated.Generated[0].PayslipID
	f.repo.setRenderReady(id)

	f.compensation.bonuses["e-budi"][1].ApprovalStatus = "rejected"
	if _, err := f.service.Send(f.ctx, payslipTestViewer, id, ""); !errors.Is(err, ErrPayslipItemsChanged) {
		t.Fatalf("rejected bonus err = %v", err)
	}
	f.compensation.bonuses["e-budi"][1].ApprovalStatus = "approved"
	f.repo.reimbursements["e-budi"][0].Amount = 999
	preview, err := f.service.SendPreview(f.ctx, payslipTestViewer, hrisdto.PayslipSendPreviewRequest{IDs: []string{id}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Recipients) != 0 || len(preview.Skipped) != 1 || preview.Skipped[0].Code != "items_changed" {
		t.Fatalf("preview = %+v", preview)
	}
	if len(f.sender.delivered) != 0 {
		t.Fatal("a stale slip was e-mailed")
	}
}

// D9: a void & reissue keeps exactly the voided slip's items. A
// reimbursement paid after the original was generated (already on the next
// month's draft) stays off the reissue, and the reissue keeps the
// original's generated_at, so the next slip's window is unchanged.
func TestPayslipVoidReissueIgnoresLaterItems(t *testing.T) {
	f := newPayslipFixture(t)
	sep := f.sendSeptember(t)
	equalIDs(t, sep.ReimbursementIDs, "r1", "r2")

	f.repo.reimbursements["e-budi"] = append(f.repo.reimbursements["e-budi"],
		reimb("r-new", 80_000, time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 10, 1, 10))))
	f.at(wib(2026, 10, 20, 12))
	oct := f.generate(t, 2026, 10, "e-budi")
	octSlip, _ := f.repo.GetByID(f.ctx, *oct.Generated[0].PayslipID)
	equalIDs(t, octSlip.ReimbursementIDs, "r-new")

	result, _, err := f.service.VoidReissue(f.ctx, payslipTestViewer, sep.ID, "Salah data tunjangan")
	if err != nil {
		t.Fatal(err)
	}
	reissue, _ := f.repo.GetByID(f.ctx, result.Reissue.ID)
	equalIDs(t, reissue.ReimbursementIDs, "r1", "r2")
	equalIDs(t, reissue.BonusIDs, sep.BonusIDs...)
	if !reissue.GeneratedAt.Equal(sep.GeneratedAt) {
		t.Fatalf("reissue generated_at %v, original %v", reissue.GeneratedAt, sep.GeneratedAt)
	}
	if result.Reissue.Totals != result.Voided.Totals {
		t.Fatalf("reissue totals %+v differ from the voided %+v", result.Reissue.Totals, result.Voided.Totals)
	}

	// Regenerating the reissue draft keeps it pinned to those items.
	f.generate(t, 2026, 9, "e-budi")
	reissue, _ = f.repo.GetByID(f.ctx, result.Reissue.ID)
	equalIDs(t, reissue.ReimbursementIDs, "r1", "r2")

	// Both can be sent: they share nothing.
	f.repo.setRenderReady(reissue.ID)
	f.repo.setRenderReady(octSlip.ID)
	if _, err := f.service.Send(f.ctx, payslipTestViewer, reissue.ID, ""); err != nil {
		t.Fatalf("send reissue: %v", err)
	}
	if _, err := f.service.Send(f.ctx, payslipTestViewer, octSlip.ID, ""); err != nil {
		t.Fatalf("send october: %v", err)
	}
}

// generated_at is the assembly's own clock, read before the data: it is
// both the ceiling of this slip's selection and the next slip's floor.
func TestPayslipGeneratedAtIsAssemblyClock(t *testing.T) {
	f := newPayslipFixture(t)
	f.repo.generatedAt = time.Time{}
	clock := wib(2026, 9, 29, 9)
	f.service.now = func() time.Time { return clock }
	generated := f.generate(t, 2026, 9, "e-budi")
	slip, _ := f.repo.GetByID(f.ctx, *generated.Generated[0].PayslipID)
	if !slip.GeneratedAt.Equal(clock) {
		t.Fatalf("generated_at = %v, want %v", slip.GeneratedAt, clock)
	}
}

// The draft is frozen once its queued delivery exists; an edit that landed
// between reading the attachment and taking the guard cancels the send.
func TestPayslipSendDetectsConcurrentEdit(t *testing.T) {
	f := newPayslipFixture(t)
	generated := f.generate(t, 2026, 9, "e-budi")
	id := *generated.Generated[0].PayslipID
	f.repo.setRenderReady(id)

	f.sender.onEnqueue = func() {
		f.repo.mu.Lock()
		slip := f.repo.slips[id]
		slip.PayloadEncrypted = "edited"
		f.repo.slips[id] = slip
		f.repo.mu.Unlock()
	}
	if _, err := f.service.Send(f.ctx, payslipTestViewer, id, ""); !errors.Is(err, ErrPayslipStateChanged) {
		t.Fatalf("send after a concurrent edit err = %v", err)
	}
	if len(f.sender.delivered) != 0 || len(f.deliveries.cancelled) != 1 {
		t.Fatalf("delivered %d cancelled %v", len(f.sender.delivered), f.deliveries.cancelled)
	}
	f.sender.onEnqueue = nil

	// While a delivery is in flight the draft cannot be edited.
	f.deliveries.setInFlight(id, true)
	if _, _, err := f.service.Update(f.ctx, payslipTestViewer, id, hrisdto.UpdatePayslipRequest{Note: "x"}); !errors.Is(err, ErrDocumentSendInFlight) {
		t.Fatalf("edit while sending err = %v", err)
	}
	if f.deliveries.staleRuns == 0 {
		t.Fatal("the in-flight check must release stale rows first")
	}
}

// Send, update, void & reissue and generate return amounts: each writes a
// salary access row first, fail-closed.
func TestPayslipAmountEndpointsLogAccess(t *testing.T) {
	f := newPayslipFixture(t)
	generated := f.generate(t, 2026, 9, "e-budi")
	if f.compensation.accessLog[0] != "payslip_generate:2026-09" {
		t.Fatalf("access log = %v", f.compensation.accessLog)
	}
	id := *generated.Generated[0].PayslipID
	f.repo.setRenderReady(id)

	f.compensation.failLog = true
	if _, err := f.service.Send(f.ctx, payslipTestViewer, id, ""); err == nil {
		t.Fatal("send must fail when the access cannot be logged")
	}
	if len(f.sender.delivered) != 0 {
		t.Fatal("e-mailed although the access was not logged")
	}
	if _, _, err := f.service.Update(f.ctx, payslipTestViewer, id, hrisdto.UpdatePayslipRequest{Note: "x"}); err == nil {
		t.Fatal("update must fail when the access cannot be logged")
	}
	f.compensation.failLog = false
	if _, err := f.service.Send(f.ctx, payslipTestViewer, id, ""); err != nil {
		t.Fatal(err)
	}
	if got := f.compensation.accessLog[len(f.compensation.accessLog)-1]; got != "payslip_send:e-budi" {
		t.Fatalf("access log = %v", f.compensation.accessLog)
	}
}

// A personal address is identity data: masked for callers without
// hris:employee_identity:view, shown (and access-logged) for the others.
func TestPayslipPersonalAddressNeedsIdentityView(t *testing.T) {
	f := newPayslipFixture(t)
	budi := f.repo.employees["e-budi"]
	budi.PersonalEmail = strPtr("budi.pribadi@example.com")
	f.repo.employees["e-budi"] = budi
	generated := f.generate(t, 2026, 9, "e-budi")
	id := *generated.Generated[0].PayslipID

	sender := DocumentViewer{ActorID: payslipTestActor}
	masked, err := f.service.Recipient(f.ctx, sender, id, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if masked.Recipient != "bu***@example.com" || len(f.recipients.identityLog) != 0 {
		t.Fatalf("masked = %q, identity log %v", masked.Recipient, f.recipients.identityLog)
	}
	full, err := f.service.Recipient(f.ctx, payslipTestViewer, id, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if full.Recipient != "budi.pribadi@example.com" || len(f.recipients.identityLog) != 1 || f.recipients.identityLog[0] != "identity_view:e-budi" {
		t.Fatalf("full = %q, identity log %v", full.Recipient, f.recipients.identityLog)
	}
	f.recipients.failLog = true
	if _, err := f.service.Recipient(f.ctx, payslipTestViewer, id, "personal"); err == nil {
		t.Fatal("the full address must not be returned when the access cannot be logged")
	}
	f.recipients.failLog = false

	// The previous address of a personal delivery is masked too.
	f.recipients.last["e-budi"], f.recipients.lastSource["e-budi"] = "budi.pribadi@example.com", model.EmailRecipientSourcePersonal
	login, err := f.service.Recipient(f.ctx, sender, id, "")
	if err != nil {
		t.Fatal(err)
	}
	if login.Recipient != "staff.ops@kantor.local" || login.PreviousRecipient != "bu***@example.com" || !login.IsNew {
		t.Fatalf("login preview = %+v", login)
	}

	// Delivery history.
	f.deliveries.personal = true
	rows, err := f.service.mailer.ListDeliveries(f.ctx, "payslip", id, func(string) bool { return true }, sender)
	if err != nil {
		t.Fatal(err)
	}
	if rows[len(rows)-1].Recipient != "bu***@example.com" || rows[0].Recipient != "staff.ops@kantor.local" {
		t.Fatalf("history = %+v", rows)
	}
}

// Without a PDF converter a draft is not a failed render: it stays 'none'
// (checked and, with DOCUMENTS_ALLOW_DOCX_SEND, sent as DOCX) and nothing
// is queued.
func TestPayslipWithoutConverterIsNotFailed(t *testing.T) {
	f := newPayslipFixture(t)
	f.queue.noPDF = true
	generated := f.generate(t, 2026, 9, "e-budi")
	item := generated.Generated[0]
	if item.RenderStatus != model.DocumentRenderNone || len(f.queue.jobs) != 0 {
		t.Fatalf("render status %s, jobs %d", item.RenderStatus, len(f.queue.jobs))
	}
	list, err := f.service.List(f.ctx, payslipTestViewer, 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if list.Summary.Failed != 0 || list.PDFAvailable {
		t.Fatalf("summary = %+v", list.Summary)
	}
}

// A batch job re-checks its slip right before sending: a slip that changed
// after it was queued is cancelled, not e-mailed or marked sent.
func TestPayslipBatchJobPrecheck(t *testing.T) {
	f := newPayslipFixture(t)
	generated := f.generate(t, 2026, 9, "e-budi")
	id := *generated.Generated[0].PayslipID
	f.repo.setRenderReady(id)
	prepared, err := f.service.PrepareBatch(f.ctx, payslipTestViewer, hrisdto.SendPayslipBatchRequest{IDs: []string{id}})
	if err != nil || len(prepared.jobs) != 1 {
		t.Fatalf("prepare = %+v, %v", prepared.Response, err)
	}
	f.repo.mu.Lock()
	slip := f.repo.slips[id]
	slip.PayloadEncrypted = "edited"
	f.repo.slips[id] = slip
	f.repo.mu.Unlock()

	f.service.mailer.deliverBatchJob(f.ctx, payslipTestActor, prepared.jobs[0])
	after, _ := f.repo.GetByID(f.ctx, id)
	if len(f.sender.delivered) != 0 || after.Status != model.PayslipStatusDraft || f.deliveries.cancelled[prepared.jobs[0].Queued.Delivery.ID] == "" {
		t.Fatalf("delivered %d status %s cancelled %v", len(f.sender.delivered), after.Status, f.deliveries.cancelled)
	}
}

package hris

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/mail"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	auditservice "github.com/kana-consultant/kantor/backend/internal/service/audit"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
)

// Document mailer: the sending half shared by payslips and contracts. It
// resolves the recipient, enforces the preconditions (Email Dokumen ready,
// one document in flight at a time), builds the neutral bilingual message
// (never any amounts) and hands it to the Phase 1 DocumentMailService, which
// records every attempt in email_deliveries.
//
//   - Single send: synchronous (Enqueue + DeliverQueued), bounded by
//     DocumentMailService's 20 s budget.
//   - Batch: the caller enqueues a 'queued' delivery row per document (which
//     already takes the double-send guard) and writes its 'send_requested'
//     audit rows synchronously; StartBatch then sends sequentially in a
//     goroutine (about 1 s apart) with a detached tenant context, a scoped
//     tenant connection per document and the injected audit service for the
//     outcome rows (platformmiddleware.AuditLog is a no-op outside the
//     request).

const (
	// documentBatchSpacing spaces batch sends (Gmail rate friendliness).
	documentBatchSpacing = time.Second

	// Recipient sources accepted from HR. "default" picks the login e-mail
	// for linked employees and employees.email for unlinked ones.
	DocumentRecipientDefault  = "default"
	DocumentRecipientLogin    = model.EmailRecipientSourceLogin
	DocumentRecipientEmployee = model.EmailRecipientSourceEmployee
	DocumentRecipientPersonal = model.EmailRecipientSourcePersonal
)

var (
	ErrDocumentMailNotReady          = errors.New("Email Dokumen belum aktif atau belum lengkap. Minta Admin mengaktifkan dan mengisi akun Gmail di Admin > Settings > Email Dokumen.")
	ErrDocumentSendInFlight          = errors.New("pengiriman dokumen ini masih berjalan; tunggu hingga selesai")
	ErrDocumentPDFConverterMissing   = errors.New(DocumentRenderFailedUnavailable)
	ErrDocumentRecipientUnavailable  = errors.New("alamat email penerima tidak tersedia untuk sumber yang dipilih")
	ErrDocumentRecipientSourceBad    = errors.New("sumber penerima harus default, login, employee, atau personal")
	ErrDocumentDeliveryForbidden     = errors.New("anda tidak memiliki izin melihat riwayat pengiriman dokumen ini")
	ErrDocumentDeliveryReferenceType = errors.New("reference_type harus payslip atau contract")
)

type documentMailSender interface {
	Ready(ctx context.Context) (bool, error)
	GetSettings(ctx context.Context) (dto.DocumentMailSettingResponse, error)
	Deliver(ctx context.Context, req authservice.DocumentMailRequest) (model.EmailDelivery, error)
	Enqueue(ctx context.Context, req authservice.DocumentMailRequest) (model.EmailDelivery, error)
}

type documentMailerDeliveries interface {
	HasInFlight(ctx context.Context, referenceType string, referenceID string) (bool, error)
	ListByReference(ctx context.Context, referenceType string, referenceID string) ([]model.EmailDelivery, error)
	MarkFailed(ctx context.Context, id string, message string) (model.EmailDelivery, error)
	FailStaleInFlight(ctx context.Context, referenceType string, referenceID string, olderThan time.Duration, message string) (int64, error)
	FailStaleDocuments(ctx context.Context, olderThan time.Duration, message string) (int64, error)
}

type documentMailerRecipients interface {
	GetRecipient(ctx context.Context, employeeID string) (hrisrepo.DocumentRecipient, error)
	LatestSentDeliveryForEmployee(ctx context.Context, employeeID string) (model.EmailDelivery, bool, error)
	EmployeeIDForDocument(ctx context.Context, referenceType string, referenceID string) (string, error)
	LogIdentityAccess(ctx context.Context, userID string, employeeID string, action string) error
}

type documentMailerCompany interface {
	Profile(ctx context.Context) (authrepo.CompanyProfileRecord, error)
}

type DocumentMailer struct {
	sender        documentMailSender
	deliveries    documentMailerDeliveries
	recipients    documentMailerRecipients
	company       documentMailerCompany
	audit         func(ctx context.Context, entry auditservice.Entry)
	allowDocxSend bool
	spacing       time.Duration
	now           func() time.Time
	// runAsync starts the batch goroutine (tests run it inline).
	runAsync func(fn func())
}

// NewDocumentMailer wires the shared sender. allowDocxSend mirrors
// DOCUMENTS_ALLOW_DOCX_SEND.
func NewDocumentMailer(sender documentMailSender, deliveries documentMailerDeliveries, recipients documentMailerRecipients, company documentMailerCompany, audit *auditservice.Service, allowDocxSend bool) *DocumentMailer {
	mailer := &DocumentMailer{
		sender:        sender,
		deliveries:    deliveries,
		recipients:    recipients,
		company:       company,
		allowDocxSend: allowDocxSend,
		spacing:       documentBatchSpacing,
		now:           time.Now,
		runAsync:      func(fn func()) { go fn() },
	}
	if audit != nil {
		mailer.audit = audit.Log
	}
	return mailer
}

// AllowDocxSend reports DOCUMENTS_ALLOW_DOCX_SEND.
func (m *DocumentMailer) AllowDocxSend() bool {
	return m.allowDocxSend
}

// Ready reports whether Email Dokumen is enabled and complete.
func (m *DocumentMailer) Ready(ctx context.Context) (bool, error) {
	return m.sender.Ready(ctx)
}

func (m *DocumentMailer) requireReady(ctx context.Context) error {
	ready, err := m.sender.Ready(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return ErrDocumentMailNotReady
	}
	return nil
}

// ResolvedRecipient is where a document will be sent. IsNew ("alamat baru")
// is set when the employee already received a document at a different
// address.
type ResolvedRecipient struct {
	EmployeeID        string
	EmployeeName      string
	Address           string
	Source            string
	Linked            bool
	PersonalAvailable bool
	HasPrevious       bool
	PreviousAddress   string
	PreviousSource    string
	IsNew             bool
}

// DocumentViewer is the caller of a document endpoint. A personal e-mail
// address is identity data (Phase 2): it is shown in full only with
// hris:employee_identity:view, and each such response is access-logged.
type DocumentViewer struct {
	ActorID         string
	CanViewIdentity bool
}

// personalReveal masks personal addresses for viewers without identity
// view and remembers whose personal address was shown in full, so the
// identity access can be logged before the response leaves.
type personalReveal struct {
	viewer    DocumentViewer
	employees map[string]bool
}

func newPersonalReveal(viewer DocumentViewer) *personalReveal {
	return &personalReveal{viewer: viewer, employees: map[string]bool{}}
}

// address returns address as the viewer may see it.
func (p *personalReveal) address(employeeID string, address string, source string) string {
	if source != model.EmailRecipientSourcePersonal || strings.TrimSpace(address) == "" {
		return address
	}
	if !p.viewer.CanViewIdentity {
		return MaskEmail(address)
	}
	if employeeID != "" {
		p.employees[employeeID] = true
	}
	return address
}

// logReveal writes one identity access row per employee whose personal
// address is about to be returned in full (fail-closed: an error means the
// response must not be sent).
func (m *DocumentMailer) logReveal(ctx context.Context, reveal *personalReveal) error {
	for employeeID := range reveal.employees {
		if err := m.recipients.LogIdentityAccess(ctx, reveal.viewer.ActorID, employeeID, IdentityAccessView); err != nil {
			return fmt.Errorf("log identity access: %w", err)
		}
	}
	return nil
}

// ResolveRecipient picks the address for source: "" / "default" -> the
// linked user's login e-mail (only the user can change it, with their
// password), employees.email for unlinked employees; "personal" -> the HR
// profile personal e-mail, only when HR picks it explicitly.
func (m *DocumentMailer) ResolveRecipient(ctx context.Context, employeeID string, source string) (ResolvedRecipient, error) {
	info, err := m.recipients.GetRecipient(ctx, employeeID)
	if err != nil {
		if errors.Is(err, hrisrepo.ErrEmployeeNotFound) {
			return ResolvedRecipient{}, ErrEmployeeNotFound
		}
		return ResolvedRecipient{}, err
	}
	login := strings.ToLower(strings.TrimSpace(optionalText(info.LoginEmail)))
	linked := info.UserID != nil && login != ""
	employeeEmail := strings.ToLower(strings.TrimSpace(info.Email))
	personal := strings.ToLower(strings.TrimSpace(optionalText(info.PersonalEmail)))

	result := ResolvedRecipient{
		EmployeeID:        info.EmployeeID,
		EmployeeName:      info.FullName,
		Linked:            linked,
		PersonalAvailable: personal != "",
	}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", DocumentRecipientDefault:
		if linked {
			result.Address, result.Source = login, model.EmailRecipientSourceLogin
		} else {
			result.Address, result.Source = employeeEmail, model.EmailRecipientSourceEmployee
		}
	case DocumentRecipientLogin:
		if !linked {
			return ResolvedRecipient{}, ErrDocumentRecipientUnavailable
		}
		result.Address, result.Source = login, model.EmailRecipientSourceLogin
	case DocumentRecipientEmployee:
		result.Address, result.Source = employeeEmail, model.EmailRecipientSourceEmployee
	case DocumentRecipientPersonal:
		result.Address, result.Source = personal, model.EmailRecipientSourcePersonal
	default:
		return ResolvedRecipient{}, ErrDocumentRecipientSourceBad
	}
	if result.Address == "" || mail.ValidateAddress(result.Address) != nil {
		return ResolvedRecipient{}, ErrDocumentRecipientUnavailable
	}

	previous, ok, err := m.recipients.LatestSentDeliveryForEmployee(ctx, employeeID)
	if err != nil {
		return ResolvedRecipient{}, err
	}
	if ok {
		result.HasPrevious = true
		result.PreviousAddress = strings.ToLower(strings.TrimSpace(previous.Recipient))
		result.PreviousSource = previous.RecipientSource
		result.IsNew = result.PreviousAddress != result.Address
	}
	return result, nil
}

// InFlight reports whether a queued or sending delivery exists for the
// document (the double-send guard). Rows that have not moved for the stale
// window (a batch cut off by a restart) are failed first, so they cannot
// block the document forever.
func (m *DocumentMailer) InFlight(ctx context.Context, referenceType string, referenceID string) (bool, error) {
	if _, err := m.deliveries.FailStaleInFlight(ctx, referenceType, referenceID, authservice.DocumentMailStaleAfter, authservice.DocumentMailStaleMessage); err != nil {
		slog.WarnContext(ctx, "document mailer: releasing stale deliveries failed", "reference_type", referenceType, "error", err)
	}
	return m.deliveries.HasInFlight(ctx, referenceType, referenceID)
}

// SweepStale fails the queued/sending document deliveries of the tenant in
// ctx that have not moved for the stale window. A batch runs in memory, so
// rows queued by a process that stopped are released here (per-tenant
// ticker) rather than only when someone touches the document again.
func (m *DocumentMailer) SweepStale(ctx context.Context) error {
	released, err := m.deliveries.FailStaleDocuments(ctx, authservice.DocumentMailStaleAfter, authservice.DocumentMailStaleMessage)
	if err != nil {
		return err
	}
	if released > 0 {
		slog.WarnContext(ctx, "document mailer: released stale deliveries", "count", released)
	}
	return nil
}

// DocumentMail is one document e-mail before it is handed to the sender.
// Paragraphs must not contain amounts. A payslip carries one attachment; a
// contract carries the PKWT and the NDA/HKI in the same message. Cc must
// already be checked by the caller (contracts: company domain only).
type DocumentMail struct {
	Kind          string
	ReferenceType string
	ReferenceID   string
	Recipient     ResolvedRecipient
	Cc            []string
	Subject       string
	ParagraphsID  []string
	ParagraphsEN  []string
	Attachments   []mail.Attachment
	BatchID       string
	RequestedBy   string
}

// HRContact returns the company's HR contact e-mail (Reply-To) and legal
// name for message texts.
func (m *DocumentMailer) HRContact(ctx context.Context) (authrepo.CompanyProfileRecord, error) {
	if m.company == nil {
		return authrepo.DefaultCompanyProfileRecord(), nil
	}
	return m.company.Profile(ctx)
}

func (m *DocumentMailer) buildRequest(ctx context.Context, doc DocumentMail) (authservice.DocumentMailRequest, error) {
	settings, err := m.sender.GetSettings(ctx)
	if err != nil {
		return authservice.DocumentMailRequest{}, err
	}
	company, err := m.HRContact(ctx)
	if err != nil {
		return authservice.DocumentMailRequest{}, err
	}
	signature := strings.TrimSpace(settings.SenderName)
	if signature == "" {
		signature = strings.TrimSpace(company.LegalName)
	}
	text, html, err := mail.RenderDocumentEmail(mail.DocumentEmail{
		RecipientName: doc.Recipient.EmployeeName,
		ParagraphsID:  doc.ParagraphsID,
		ParagraphsEN:  doc.ParagraphsEN,
		Signature:     signature,
	})
	if err != nil {
		return authservice.DocumentMailRequest{}, err
	}
	referenceType, referenceID := doc.ReferenceType, doc.ReferenceID
	req := authservice.DocumentMailRequest{
		Kind:            doc.Kind,
		ReferenceType:   &referenceType,
		ReferenceID:     &referenceID,
		Recipient:       doc.Recipient.Address,
		RecipientSource: doc.Recipient.Source,
		Subject:         doc.Subject,
		Text:            text,
		HTML:            html,
		Cc:              doc.Cc,
		Attachments:     doc.Attachments,
		RequestedBy:     doc.RequestedBy,
	}
	if replyTo := strings.TrimSpace(company.HRContactEmail); replyTo != "" && mail.ValidateAddress(replyTo) == nil {
		req.ReplyTo = replyTo
	}
	if doc.BatchID != "" {
		batchID := doc.BatchID
		req.BatchID = &batchID
	}
	return req, nil
}

func mapSenderError(err error) error {
	switch {
	case errors.Is(err, authservice.ErrDocumentMailNotReady):
		return ErrDocumentMailNotReady
	case errors.Is(err, authservice.ErrDocumentMailInFlight):
		return ErrDocumentSendInFlight
	default:
		return err
	}
}

// QueuedDocumentMail is a document whose 'queued' delivery row exists. Every
// send (single and batch) goes Enqueue -> checks -> DeliverQueued: the
// queued row takes the double-send guard first, which also freezes a
// payslip draft (its snapshot update refuses while a delivery is in
// flight), so the document can be re-checked against what is attached.
type QueuedDocumentMail struct {
	Request  authservice.DocumentMailRequest
	Delivery model.EmailDelivery
}

// Enqueue records a queued delivery for a batch item (taking the
// double-send guard).
func (m *DocumentMailer) Enqueue(ctx context.Context, doc DocumentMail) (QueuedDocumentMail, error) {
	req, err := m.buildRequest(ctx, doc)
	if err != nil {
		return QueuedDocumentMail{}, err
	}
	delivery, err := m.sender.Enqueue(ctx, req)
	if err != nil {
		return QueuedDocumentMail{}, mapSenderError(err)
	}
	req.DeliveryID = delivery.ID
	return QueuedDocumentMail{Request: req, Delivery: delivery}, nil
}

// CancelQueued fails a queued delivery that will not be sent (the document
// changed, or the batch could not be started), so it does not hold the
// double-send guard until the stale timeout.
func (m *DocumentMailer) CancelQueued(ctx context.Context, deliveryID string, reason string) {
	if _, err := m.deliveries.MarkFailed(ctx, deliveryID, "Pengiriman dibatalkan: "+reason); err != nil {
		slog.ErrorContext(ctx, "document mailer: close queued delivery", "delivery_id", deliveryID, "error", err)
	}
}

// DeliverQueued sends a document whose queued row exists (taken with
// Enqueue). On an SMTP failure the returned delivery is the failed row and
// the error a *mail.SendError; when nothing was attempted the queued row is
// closed so it does not block the document.
func (m *DocumentMailer) DeliverQueued(ctx context.Context, queued QueuedDocumentMail) (model.EmailDelivery, error) {
	delivery, err := m.sender.Deliver(ctx, queued.Request)
	if err == nil {
		return delivery, nil
	}
	if _, isSendErr := mail.AsSendError(err); isSendErr {
		return delivery, err
	}
	// An in-flight answer means another sender already moved the row to
	// 'sending'; that sender records the outcome.
	if !errors.Is(err, authservice.ErrDocumentMailInFlight) {
		reason := mapSenderError(err).Error()
		if errors.Is(err, authservice.ErrDocumentMailInvalidRequest) {
			reason = "data pengiriman berubah"
		}
		m.CancelQueued(ctx, queued.Delivery.ID, reason)
	}
	return delivery, mapSenderError(err)
}

// DocumentBatchJob is one document of a background batch. Precheck runs
// with the scoped tenant connection right before the send (the queued row,
// and so the double-send guard, already exists): an error cancels this
// document with that reason. OnSent runs after a successful delivery (e.g.
// mark the payslip sent); AuditValue is the metadata of the outcome audit
// row.
type DocumentBatchJob struct {
	Queued     QueuedDocumentMail
	Resource   string
	ResourceID string
	AuditValue map[string]any
	Precheck   func(ctx context.Context) error
	OnSent     func(ctx context.Context, delivery model.EmailDelivery) error
}

// NewBatchID returns an id for email_deliveries.batch_id.
func NewBatchID() string {
	return uuid.NewString()
}

// StartBatch sends the queued jobs sequentially in the background. ctx is
// the request context: only the tenant and the tenant pool are kept
// (DetachTenantContext), and each job acquires its own scoped tenant
// connection, so no request connection is held after the handler returns.
func (m *DocumentMailer) StartBatch(ctx context.Context, requestedBy string, jobs []DocumentBatchJob) {
	if len(jobs) == 0 {
		return
	}
	detached := platformmiddleware.DetachTenantContext(ctx)
	m.runAsync(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(detached, "document batch send panicked", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		m.runBatch(detached, requestedBy, jobs)
	})
}

func (m *DocumentMailer) runBatch(ctx context.Context, requestedBy string, jobs []DocumentBatchJob) {
	for index, job := range jobs {
		if index > 0 && m.spacing > 0 {
			timer := time.NewTimer(m.spacing)
			<-timer.C
		}
		if _, err := platformmiddleware.WithScopedTenantConn(ctx, func(scoped context.Context) (struct{}, error) {
			m.deliverBatchJob(scoped, requestedBy, job)
			return struct{}{}, nil
		}); err != nil {
			// The queued row cannot be closed without a connection either;
			// the stale sweep (SweepStale) fails it after the stale window.
			slog.ErrorContext(ctx, "document batch: acquire tenant connection", "resource", job.Resource, "resource_id", job.ResourceID, "delivery_id", job.Queued.Delivery.ID, "error", err)
		}
	}
}

func (m *DocumentMailer) deliverBatchJob(ctx context.Context, requestedBy string, job DocumentBatchJob) {
	value := map[string]any{}
	for key, item := range job.AuditValue {
		value[key] = item
	}
	value["delivery_id"] = job.Queued.Delivery.ID
	value["batch"] = true

	var delivery model.EmailDelivery
	var err error
	if job.Precheck != nil {
		if checkErr := job.Precheck(ctx); checkErr != nil {
			m.CancelQueued(ctx, job.Queued.Delivery.ID, checkErr.Error())
			slog.WarnContext(ctx, "document batch: document cancelled before sending", "resource", job.Resource, "resource_id", job.ResourceID, "error", checkErr)
			err = checkErr
			value["error_category"] = "precheck"
		}
	}
	if err == nil {
		delivery, err = m.DeliverQueued(ctx, job.Queued)
		if err != nil {
			if sendErr, ok := mail.AsSendError(err); ok {
				value["error_category"] = string(sendErr.Category)
			} else {
				slog.WarnContext(ctx, "document batch: delivery not attempted", "resource", job.Resource, "resource_id", job.ResourceID, "error", err)
				value["error_category"] = "not_attempted"
			}
		} else if job.OnSent != nil {
			if onSentErr := job.OnSent(ctx, delivery); onSentErr != nil {
				slog.ErrorContext(ctx, "document batch: record sent document", "resource", job.Resource, "resource_id", job.ResourceID, "error", onSentErr)
			}
		}
	}

	status := model.EmailDeliveryStatusSent
	action := "send"
	if err != nil {
		status = model.EmailDeliveryStatusFailed
		action = "send_failed"
	}
	value["status"] = status
	if m.audit != nil {
		m.audit(ctx, auditservice.Entry{
			UserID:     requestedBy,
			Action:     action,
			Module:     "hris",
			Resource:   job.Resource,
			ResourceID: job.ResourceID,
			NewValue:   value,
		})
	}
}

// ListDeliveries returns the delivery history of one document, newest
// first. The caller's permission is checked against each stored row's kind
// (canView); test rows are never listed. Personal addresses are masked
// unless the viewer may see identity data (then the read is access-logged).
func (m *DocumentMailer) ListDeliveries(ctx context.Context, referenceType string, referenceID string, canView func(kind string) bool, viewer DocumentViewer) ([]model.EmailDelivery, error) {
	switch referenceType {
	case model.EmailDeliveryKindPayslip, model.EmailDeliveryKindContract:
	default:
		return nil, ErrDocumentDeliveryReferenceType
	}
	// The requested type alone already needs the permission, so an empty
	// history does not reveal anything to a caller without it.
	if !canView(referenceType) {
		return nil, ErrDocumentDeliveryForbidden
	}
	if _, err := uuid.Parse(referenceID); err != nil {
		return []model.EmailDelivery{}, nil
	}
	rows, err := m.deliveries.ListByReference(ctx, referenceType, referenceID)
	if err != nil {
		return nil, err
	}
	result := make([]model.EmailDelivery, 0, len(rows))
	personal := false
	for _, row := range rows {
		if row.Kind == model.EmailDeliveryKindTest {
			continue
		}
		if !canView(row.Kind) {
			return nil, ErrDocumentDeliveryForbidden
		}
		if row.RecipientSource == model.EmailRecipientSourcePersonal {
			personal = true
		}
		result = append(result, row)
	}
	if !personal {
		return result, nil
	}

	reveal := newPersonalReveal(viewer)
	employeeID := ""
	if viewer.CanViewIdentity {
		if employeeID, err = m.recipients.EmployeeIDForDocument(ctx, referenceType, referenceID); err != nil {
			return nil, err
		}
	}
	for index := range result {
		result[index].Recipient = reveal.address(employeeID, result[index].Recipient, result[index].RecipientSource)
	}
	if err := m.logReveal(ctx, reveal); err != nil {
		return nil, err
	}
	return result, nil
}

// MaskEmail hides most of the local part for audit metadata:
// "budi.santoso@kantor.local" -> "bu***@kantor.local".
func MaskEmail(address string) string {
	address = strings.TrimSpace(address)
	local, domain, ok := strings.Cut(address, "@")
	if !ok || local == "" {
		return "***"
	}
	runes := []rune(local)
	keep := 2
	if len(runes) <= 2 {
		keep = 1
	}
	return fmt.Sprintf("%s***@%s", string(runes[:keep]), domain)
}

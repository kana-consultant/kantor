package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/mail"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

const (
	// documentMailSendTimeout bounds one synchronous send (dial + TLS + AUTH
	// + DATA). It stays below the server's 30 s WriteTimeout.
	documentMailSendTimeout = 20 * time.Second
	// documentMailStaleAfter releases the double-send guard for rows left in
	// queued/sending by a crashed process.
	documentMailStaleAfter    = 10 * time.Minute
	documentMailStaleMessage  = "Pengiriman terputus sebelum selesai"
	documentMailAdminNotice   = "admin:settings:manage"
	documentMailNoticeType    = "admin.document_mail.updated"
	documentMailNoticeRefType = "system_setting"
)

// DocumentMailStaleAfter / DocumentMailStaleMessage are shared with the HRIS
// document mailer, which releases the same guard before its own in-flight
// checks and sweeps rows of interrupted batches.
const (
	DocumentMailStaleAfter   = documentMailStaleAfter
	DocumentMailStaleMessage = documentMailStaleMessage
)

var (
	ErrDocumentMailNotReady         = errors.New("Email Dokumen belum aktif atau belum lengkap. Aktifkan dan isi alamat Gmail serta app password di Admin > Settings.")
	ErrDocumentMailUsernameRequired = errors.New("alamat Gmail wajib diisi saat Email Dokumen diaktifkan")
	ErrDocumentMailUsernameInvalid  = errors.New("alamat Gmail tidak valid: isi satu alamat email biasa, misalnya slip@perusahaan.co.id")
	ErrDocumentMailPasswordRequired = errors.New("alamat Gmail atau port berubah: isi app password baru untuk akun tersebut")
	ErrDocumentMailInFlight         = errors.New("pengiriman untuk dokumen ini masih berjalan")
	ErrDocumentMailInvalidRequest   = errors.New("permintaan pengiriman email dokumen tidak valid")
)

type documentMailSettingsRepository interface {
	GetDocumentMailRecord(ctx context.Context) (authrepo.DocumentMailSettingRecord, error)
	UpdateDocumentMail(ctx context.Context, updatedBy string, setting authrepo.DocumentMailSettingRecord) error
	GetUserByID(ctx context.Context, userID string) (model.User, error)
	ListUserIDsByPermission(ctx context.Context, permissionID string) ([]string, error)
	ResolveUserFullName(ctx context.Context, userID string) (string, error)
}

type documentMailDeliveriesRepository interface {
	Create(ctx context.Context, params notificationsrepo.CreateEmailDeliveryParams) (model.EmailDelivery, error)
	GetByID(ctx context.Context, id string) (model.EmailDelivery, error)
	MarkSending(ctx context.Context, id string) (model.EmailDelivery, error)
	MarkSent(ctx context.Context, id string, sentAt time.Time) (model.EmailDelivery, error)
	MarkFailed(ctx context.Context, id string, message string) (model.EmailDelivery, error)
	FailStaleInFlight(ctx context.Context, referenceType string, referenceID string, olderThan time.Duration, message string) (int64, error)
	FailStaleUnreferenced(ctx context.Context, kind string, olderThan time.Duration, message string) (int64, error)
}

type documentMailNotifier interface {
	CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error
}

// DocumentMailTransport is implemented by *mail.GmailSender.
type DocumentMailTransport interface {
	Send(ctx context.Context, creds mail.Credentials, msg mail.Message) error
	DevSMTPAddr() string
}

// DocumentMailService owns the tenant's 'document_mail' setting and is the
// single entry point for sending document email. Payslip and contract
// senders call Deliver (or Enqueue + Deliver with DeliveryID for batches);
// every attempt is recorded in email_deliveries.
type DocumentMailService struct {
	repo       documentMailSettingsRepository
	deliveries documentMailDeliveriesRepository
	notifier   documentMailNotifier
	transport  DocumentMailTransport
	encrypter  *security.Encrypter
	now        func() time.Time
	// pdf is the read-only PDF conversion status shown next to the
	// settings (SetPDFStatus); it never holds a filesystem path.
	pdf dto.DocumentPDFStatus
}

// Document mail delivery modes reported in DocumentMailSettingResponse.
const (
	DocumentMailModeGmail      = "gmail"
	DocumentMailModeDevCapture = "dev_capture"
)

// DocumentMailRequest is one outgoing document email.
type DocumentMailRequest struct {
	Kind            string
	ReferenceType   *string
	ReferenceID     *string
	Recipient       string
	RecipientSource string
	Cc              []string
	ReplyTo         string
	Subject         string
	Text            string
	HTML            string
	Attachments     []mail.Attachment
	BatchID         *string
	RequestedBy     string
	// DeliveryID continues a queued row created earlier by Enqueue (batch
	// sends). The request must describe exactly what that row recorded
	// (kind, reference, recipient + source, cc, subject, attachments), so the
	// log always shows what was actually sent. Empty creates a new row;
	// a retry ("Kirim ulang") of a failed row is a new row too, so every
	// attempt keeps its own recipient and digests.
	DeliveryID string
}

func NewDocumentMailService(
	repo documentMailSettingsRepository,
	deliveries documentMailDeliveriesRepository,
	notifier documentMailNotifier,
	transport DocumentMailTransport,
	encrypter *security.Encrypter,
) *DocumentMailService {
	return &DocumentMailService{
		repo:       repo,
		deliveries: deliveries,
		notifier:   notifier,
		transport:  transport,
		encrypter:  encrypter,
		now:        time.Now,
		pdf:        dto.DocumentPDFStatus{Enabled: false, Source: "not_found"},
	}
}

// SetPDFStatus records whether the document engine converts to PDF and how
// the LibreOffice binary was resolved (config.SofficeSource*). It is shown
// read-only on the settings page.
func (s *DocumentMailService) SetPDFStatus(enabled bool, source string) {
	s.pdf = dto.DocumentPDFStatus{Enabled: enabled, Source: source}
}

func (s *DocumentMailService) devSMTPAddr() string {
	if s.transport == nil {
		return ""
	}
	return s.transport.DevSMTPAddr()
}

func (s *DocumentMailService) toResponse(setting authrepo.DocumentMailSettingRecord) dto.DocumentMailSettingResponse {
	response := dto.DocumentMailSettingResponse{
		Enabled:         setting.Enabled,
		SMTPHost:        mail.GmailHost,
		SMTPUsername:    setting.SMTPUsername,
		SMTPPort:        setting.SMTPPort,
		SenderName:      setting.SenderName,
		HasSMTPPassword: setting.HasPassword(),
		Ready:           s.isReady(setting),
	}
	response.Delivery = dto.DocumentMailDeliveryStatus{Mode: DocumentMailModeGmail}
	if addr := s.devSMTPAddr(); addr != "" {
		devAddr, captureAddr := addr, addr
		response.DevSMTPAddr = &devAddr
		response.Delivery = dto.DocumentMailDeliveryStatus{Mode: DocumentMailModeDevCapture, CaptureAddr: &captureAddr}
	}
	response.PDF = s.pdf
	return response
}

// isReady: enabled with a username, and an app password — which may only be
// missing while the development SMTP override is active.
func (s *DocumentMailService) isReady(setting authrepo.DocumentMailSettingRecord) bool {
	return setting.Enabled &&
		setting.SMTPUsername != "" &&
		mail.ValidateAddress(setting.SMTPUsername) == nil &&
		(setting.HasPassword() || s.devSMTPAddr() != "")
}

// GetSettings returns the public view (has_smtp_password only).
func (s *DocumentMailService) GetSettings(ctx context.Context) (dto.DocumentMailSettingResponse, error) {
	setting, err := s.repo.GetDocumentMailRecord(ctx)
	if err != nil {
		return dto.DocumentMailSettingResponse{}, err
	}
	return s.toResponse(setting), nil
}

// Ready reports whether document email can be sent right now.
func (s *DocumentMailService) Ready(ctx context.Context) (bool, error) {
	setting, err := s.repo.GetDocumentMailRecord(ctx)
	if err != nil {
		return false, err
	}
	return s.isReady(setting), nil
}

// UpdateSettings applies the admin's edit and returns the new public view plus
// the names of the fields that changed (for the audit entry and the admin
// notice). The app password is cleared whenever the username or port changes,
// and such a change is refused unless a new password comes in the same
// request; clear_smtp_password does not waive that rule. Removing the account
// (blank username) is the exception: there is no account left to need a
// password, so the stored one is simply dropped.
func (s *DocumentMailService) UpdateSettings(ctx context.Context, actorID string, input dto.UpdateDocumentMailRequest) (dto.DocumentMailSettingResponse, []string, error) {
	existing, err := s.repo.GetDocumentMailRecord(ctx)
	if err != nil {
		return dto.DocumentMailSettingResponse{}, nil, err
	}

	updated := existing
	updated.Enabled = input.Enabled
	updated.SMTPUsername = strings.ToLower(strings.TrimSpace(input.SMTPUsername))
	updated.SMTPPort = input.SMTPPort
	updated.SenderName = sanitizeSenderName(input.SenderName)

	if updated.Enabled && updated.SMTPUsername == "" {
		return dto.DocumentMailSettingResponse{}, nil, ErrDocumentMailUsernameRequired
	}
	// The DTO's email tag accepts RFC quoted local parts ("a b"@x) that the
	// sender refuses; store only what can actually be sent from.
	if updated.SMTPUsername != "" && mail.ValidateAddress(updated.SMTPUsername) != nil {
		return dto.DocumentMailSettingResponse{}, nil, ErrDocumentMailUsernameInvalid
	}

	newPassword := ""
	if input.SMTPPassword != nil {
		// App passwords are shown by Google as "abcd efgh ijkl mnop"; the
		// spaces are not part of the secret.
		newPassword = strings.Join(strings.Fields(*input.SMTPPassword), "")
	}

	clearPassword := input.ClearSMTPPassword
	accountChanged := updated.SMTPUsername != existing.SMTPUsername || updated.SMTPPort != existing.SMTPPort
	switch {
	case updated.SMTPUsername == "":
		// No account: an app password (stored or new) has nothing to belong
		// to, so it is never kept.
		clearPassword = true
		newPassword = ""
	case accountChanged && existing.HasPassword():
		if newPassword == "" {
			return dto.DocumentMailSettingResponse{}, nil, ErrDocumentMailPasswordRequired
		}
		// Never reuse a stored app password for a different account or port.
		// The new password replaces it, so an explicit clear is redundant.
		updated.SMTPPasswordEncrypted = ""
		clearPassword = false
	}

	switch {
	case clearPassword:
		updated.SMTPPasswordEncrypted = ""
	case newPassword != "":
		if s.encrypter == nil {
			return dto.DocumentMailSettingResponse{}, nil, errors.New("document mail encrypter is not configured")
		}
		ciphertext, err := s.encrypter.EncryptString(newPassword)
		if err != nil {
			return dto.DocumentMailSettingResponse{}, nil, err
		}
		updated.SMTPPasswordEncrypted = ciphertext
	}

	updated = authrepo.NormalizeDocumentMailSettingRecord(updated)
	changed := documentMailChangedFields(existing, updated)

	if err := s.repo.UpdateDocumentMail(ctx, actorID, updated); err != nil {
		return dto.DocumentMailSettingResponse{}, nil, err
	}

	if len(changed) > 0 {
		s.notifyAdmins(ctx, actorID, changed)
	}

	return s.toResponse(updated), changed, nil
}

func documentMailChangedFields(before authrepo.DocumentMailSettingRecord, after authrepo.DocumentMailSettingRecord) []string {
	changed := make([]string, 0, 5)
	if before.Enabled != after.Enabled {
		changed = append(changed, "enabled")
	}
	if before.SMTPUsername != after.SMTPUsername {
		changed = append(changed, "smtp_username")
	}
	if before.SMTPPasswordEncrypted != after.SMTPPasswordEncrypted {
		changed = append(changed, "smtp_password")
	}
	if before.SMTPPort != after.SMTPPort {
		changed = append(changed, "smtp_port")
	}
	if before.SenderName != after.SenderName {
		changed = append(changed, "sender_name")
	}
	return changed
}

// notifyAdmins sends an in-app notice to every user who can manage settings
// (including the actor, so a hijacked session is visible to the real owner).
// Best effort: the settings are already saved.
func (s *DocumentMailService) notifyAdmins(ctx context.Context, actorID string, changed []string) {
	if s.notifier == nil {
		return
	}
	recipients, err := s.repo.ListUserIDsByPermission(ctx, documentMailAdminNotice)
	if err != nil {
		slog.WarnContext(ctx, "document mail: list admin recipients failed", "error", err)
		return
	}
	if len(recipients) == 0 {
		return
	}

	actorName := "Seorang admin"
	if name, err := s.repo.ResolveUserFullName(ctx, actorID); err == nil && strings.TrimSpace(name) != "" {
		actorName = strings.TrimSpace(name)
	}
	message := fmt.Sprintf("%s mengubah pengaturan Email Dokumen (Gmail): %s.", actorName, strings.Join(changed, ", "))
	referenceType := documentMailNoticeRefType

	params := make([]notificationsrepo.CreateParams, 0, len(recipients))
	seen := make(map[string]bool, len(recipients))
	for _, userID := range recipients {
		if userID == "" || seen[userID] {
			continue
		}
		seen[userID] = true
		params = append(params, notificationsrepo.CreateParams{
			UserID:        userID,
			Type:          documentMailNoticeType,
			Title:         "Pengaturan Email Dokumen diubah",
			Message:       message,
			ReferenceType: &referenceType,
		})
	}
	if err := s.notifier.CreateMany(ctx, params); err != nil {
		slog.WarnContext(ctx, "document mail: admin notification failed", "error", err)
	}
}

// credentials loads the tenant settings and decrypts the app password.
func (s *DocumentMailService) credentials(ctx context.Context) (mail.Credentials, error) {
	setting, err := s.repo.GetDocumentMailRecord(ctx)
	if err != nil {
		return mail.Credentials{}, err
	}
	if !s.isReady(setting) {
		return mail.Credentials{}, ErrDocumentMailNotReady
	}

	password := ""
	if setting.HasPassword() {
		if s.encrypter == nil {
			return mail.Credentials{}, errors.New("document mail encrypter is not configured")
		}
		password, err = s.encrypter.DecryptString(setting.SMTPPasswordEncrypted)
		if err != nil {
			return mail.Credentials{}, fmt.Errorf("decrypt document mail password: %w", err)
		}
	}

	return mail.Credentials{
		Username:   setting.SMTPUsername,
		Password:   password,
		Port:       setting.SMTPPort,
		SenderName: setting.SenderName,
	}, nil
}

func validateDocumentMailRequest(req DocumentMailRequest) error {
	switch req.Kind {
	case model.EmailDeliveryKindContract, model.EmailDeliveryKindPayslip, model.EmailDeliveryKindTest:
	default:
		return fmt.Errorf("%w: kind %q", ErrDocumentMailInvalidRequest, req.Kind)
	}
	switch req.RecipientSource {
	case model.EmailRecipientSourceLogin, model.EmailRecipientSourceEmployee, model.EmailRecipientSourcePersonal, model.EmailRecipientSourceSelf:
	default:
		return fmt.Errorf("%w: recipient source %q", ErrDocumentMailInvalidRequest, req.RecipientSource)
	}
	if (req.ReferenceType == nil) != (req.ReferenceID == nil) {
		return fmt.Errorf("%w: reference type and id go together", ErrDocumentMailInvalidRequest)
	}
	if err := mail.ValidateAddress(req.Recipient); err != nil {
		return fmt.Errorf("%w: recipient: %v", ErrDocumentMailInvalidRequest, err)
	}
	for _, cc := range req.Cc {
		if err := mail.ValidateAddress(cc); err != nil {
			return fmt.Errorf("%w: cc: %v", ErrDocumentMailInvalidRequest, err)
		}
	}
	if strings.TrimSpace(req.Subject) == "" {
		return fmt.Errorf("%w: subject is required", ErrDocumentMailInvalidRequest)
	}
	return nil
}

func attachmentDigests(attachments []mail.Attachment) ([]string, []string) {
	names := make([]string, 0, len(attachments))
	digests := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		sum := sha256.Sum256(attachment.Data)
		names = append(names, strings.TrimSpace(attachment.Filename))
		digests = append(digests, hex.EncodeToString(sum[:]))
	}
	return names, digests
}

func (s *DocumentMailService) createParams(req DocumentMailRequest, status string) notificationsrepo.CreateEmailDeliveryParams {
	names, digests := attachmentDigests(req.Attachments)
	var requestedBy *string
	if strings.TrimSpace(req.RequestedBy) != "" {
		value := strings.TrimSpace(req.RequestedBy)
		requestedBy = &value
	}
	return notificationsrepo.CreateEmailDeliveryParams{
		Kind:             req.Kind,
		ReferenceType:    req.ReferenceType,
		ReferenceID:      req.ReferenceID,
		Recipient:        strings.ToLower(strings.TrimSpace(req.Recipient)),
		RecipientSource:  req.RecipientSource,
		Cc:               req.Cc,
		Subject:          strings.TrimSpace(req.Subject),
		AttachmentNames:  names,
		AttachmentSHA256: digests,
		Status:           status,
		BatchID:          req.BatchID,
		RequestedBy:      requestedBy,
	}
}

// Enqueue records a queued delivery (batch sends). The batch worker later
// calls Deliver with DeliveryID set to the returned row.
func (s *DocumentMailService) Enqueue(ctx context.Context, req DocumentMailRequest) (model.EmailDelivery, error) {
	if err := validateDocumentMailRequest(req); err != nil {
		return model.EmailDelivery{}, err
	}
	s.releaseStale(ctx, req)
	delivery, err := s.deliveries.Create(ctx, s.createParams(req, model.EmailDeliveryStatusQueued))
	if errors.Is(err, notificationsrepo.ErrEmailDeliveryInFlight) {
		return model.EmailDelivery{}, ErrDocumentMailInFlight
	}
	return delivery, err
}

func (s *DocumentMailService) releaseStale(ctx context.Context, req DocumentMailRequest) {
	var err error
	if req.ReferenceType == nil || req.ReferenceID == nil {
		// Rows without a document (test emails) never block a send, but a
		// crashed attempt must not stay 'sending' in the log forever.
		_, err = s.deliveries.FailStaleUnreferenced(ctx, req.Kind, documentMailStaleAfter, documentMailStaleMessage)
	} else {
		_, err = s.deliveries.FailStaleInFlight(ctx, *req.ReferenceType, *req.ReferenceID, documentMailStaleAfter, documentMailStaleMessage)
	}
	if err != nil {
		slog.WarnContext(ctx, "document mail: releasing stale deliveries failed", "error", err)
	}
}

// matchesQueued reports whether req describes exactly what the queued row
// recorded, so continuing it cannot log an address or digest that was never
// sent.
func matchesQueued(row model.EmailDelivery, params notificationsrepo.CreateEmailDeliveryParams) bool {
	return row.Kind == params.Kind &&
		equalOptional(row.ReferenceType, params.ReferenceType) &&
		equalOptional(row.ReferenceID, params.ReferenceID) &&
		row.Recipient == params.Recipient &&
		row.RecipientSource == params.RecipientSource &&
		equalStrings(row.Cc, params.Cc) &&
		row.Subject == params.Subject &&
		equalStrings(row.AttachmentNames, params.AttachmentNames) &&
		equalStrings(row.AttachmentSHA256, params.AttachmentSHA256)
}

func equalOptional(a *string, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return strings.EqualFold(strings.TrimSpace(*a), strings.TrimSpace(*b))
}

func equalStrings(a []string, b []string) bool {
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

// startQueued moves a queued row (from Enqueue) to sending after checking
// that req matches it.
func (s *DocumentMailService) startQueued(ctx context.Context, id string, req DocumentMailRequest) (model.EmailDelivery, error) {
	row, err := s.deliveries.GetByID(ctx, id)
	if errors.Is(err, notificationsrepo.ErrEmailDeliveryNotFound) {
		return model.EmailDelivery{}, fmt.Errorf("%w: delivery %s not found", ErrDocumentMailInvalidRequest, id)
	}
	if err != nil {
		return model.EmailDelivery{}, err
	}
	switch row.Status {
	case model.EmailDeliveryStatusQueued:
	case model.EmailDeliveryStatusSending:
		return model.EmailDelivery{}, ErrDocumentMailInFlight
	default:
		return model.EmailDelivery{}, fmt.Errorf("%w: delivery %s is %s; a retry is a new delivery", ErrDocumentMailInvalidRequest, id, row.Status)
	}
	if !matchesQueued(row, s.createParams(req, model.EmailDeliveryStatusQueued)) {
		return model.EmailDelivery{}, fmt.Errorf("%w: request does not match queued delivery %s", ErrDocumentMailInvalidRequest, id)
	}
	delivery, err := s.deliveries.MarkSending(ctx, id)
	if errors.Is(err, notificationsrepo.ErrEmailDeliveryNotFound) {
		// Another worker picked the row up between the read and the update.
		return model.EmailDelivery{}, ErrDocumentMailInFlight
	}
	return delivery, err
}

// Deliver sends one document email synchronously (20 s budget) and records
// the attempt. When the SMTP send fails, the returned delivery is the failed
// row and the error is a *mail.SendError carrying the fixed category.
// ErrDocumentMailNotReady / ErrDocumentMailInFlight / ErrDocumentMailInvalidRequest
// mean nothing was sent and (for the first two) no row was written.
func (s *DocumentMailService) Deliver(ctx context.Context, req DocumentMailRequest) (model.EmailDelivery, error) {
	if err := validateDocumentMailRequest(req); err != nil {
		return model.EmailDelivery{}, err
	}
	creds, err := s.credentials(ctx)
	if err != nil {
		return model.EmailDelivery{}, err
	}

	// From here on the attempt is bound to its delivery row, not to the
	// caller: a client disconnect must neither cut an SMTP transaction that
	// may already have been accepted nor leave the row in 'sending'.
	// WithoutCancel keeps the context values (tenant DB connection); each
	// repository call still has its own query timeout and the send has
	// documentMailSendTimeout.
	ctx = context.WithoutCancel(ctx)

	var delivery model.EmailDelivery
	if id := strings.TrimSpace(req.DeliveryID); id != "" {
		delivery, err = s.startQueued(ctx, id, req)
	} else {
		s.releaseStale(ctx, req)
		delivery, err = s.deliveries.Create(ctx, s.createParams(req, model.EmailDeliveryStatusSending))
	}
	if errors.Is(err, notificationsrepo.ErrEmailDeliveryInFlight) {
		return model.EmailDelivery{}, ErrDocumentMailInFlight
	}
	if err != nil {
		return model.EmailDelivery{}, err
	}

	message := mail.Message{
		To:          strings.TrimSpace(req.Recipient),
		Cc:          req.Cc,
		ReplyTo:     strings.TrimSpace(req.ReplyTo),
		Subject:     req.Subject,
		Text:        req.Text,
		HTML:        req.HTML,
		Attachments: req.Attachments,
	}

	sendCtx, cancel := context.WithTimeout(ctx, documentMailSendTimeout)
	sendErr := s.transport.Send(sendCtx, creds, message)
	cancel()

	if sendErr != nil {
		category, code, text := "unknown", 0, "Pengiriman email gagal"
		if classified, ok := mail.AsSendError(sendErr); ok {
			category, code, text = string(classified.Category), classified.Code, classified.Message()
		}
		cause := ""
		if unwrapped := errors.Unwrap(sendErr); unwrapped != nil {
			cause = unwrapped.Error()
		}
		slog.WarnContext(ctx, "document mail send failed",
			"delivery_id", delivery.ID,
			"kind", req.Kind,
			"category", category,
			"smtp_code", code,
			"cause", cause,
		)
		failed, markErr := s.deliveries.MarkFailed(ctx, delivery.ID, text)
		if markErr != nil {
			slog.ErrorContext(ctx, "document mail: recording failed delivery failed", "delivery_id", delivery.ID, "error", markErr)
			delivery.Status = model.EmailDeliveryStatusFailed
			delivery.Error = &text
			return delivery, sendErr
		}
		return failed, sendErr
	}

	sent, err := s.deliveries.MarkSent(ctx, delivery.ID, s.now())
	if err != nil {
		// The mail left; only the bookkeeping failed. Report it as sent.
		slog.ErrorContext(ctx, "document mail: recording sent delivery failed", "delivery_id", delivery.ID, "error", err)
		delivery.Status = model.EmailDeliveryStatusSent
		return delivery, nil
	}
	return sent, nil
}

// SendTest emails the caller's own login address through the configured
// account and records a kind='test' delivery.
func (s *DocumentMailService) SendTest(ctx context.Context, actorID string) (dto.DocumentMailTestResponse, error) {
	user, err := s.repo.GetUserByID(ctx, actorID)
	if err != nil {
		return dto.DocumentMailTestResponse{}, err
	}
	setting, err := s.repo.GetDocumentMailRecord(ctx)
	if err != nil {
		return dto.DocumentMailTestResponse{}, err
	}
	if !s.isReady(setting) {
		return dto.DocumentMailTestResponse{}, ErrDocumentMailNotReady
	}

	text, html, err := mail.RenderDocumentEmail(mail.DocumentEmail{
		RecipientName: user.FullName,
		ParagraphsID: []string{
			"Ini adalah email uji dari pengaturan Email Dokumen (Gmail) di KANTOR.",
			fmt.Sprintf("Jika email ini sampai, slip gaji dan kontrak kerja dapat dikirim melalui akun %s. Email ini tidak perlu dibalas.", setting.SMTPUsername),
		},
		ParagraphsEN: []string{
			"This is a test email from the Document Email (Gmail) settings in KANTOR.",
			fmt.Sprintf("If it reached you, payslips and employment contracts can be sent through the %s account. No reply is needed.", setting.SMTPUsername),
		},
		Signature: setting.SenderName,
	})
	if err != nil {
		return dto.DocumentMailTestResponse{}, err
	}

	delivery, err := s.Deliver(ctx, DocumentMailRequest{
		Kind:            model.EmailDeliveryKindTest,
		Recipient:       user.Email,
		RecipientSource: model.EmailRecipientSourceSelf,
		Subject:         "Email uji Email Dokumen / Document email test",
		Text:            text,
		HTML:            html,
		RequestedBy:     actorID,
	})

	result := dto.DocumentMailTestResponse{
		Sent:       err == nil,
		DeliveryID: delivery.ID,
		Recipient:  strings.ToLower(strings.TrimSpace(user.Email)),
	}
	if err != nil {
		sendErr, ok := mail.AsSendError(err)
		if !ok || delivery.ID == "" {
			return dto.DocumentMailTestResponse{}, err
		}
		category := string(sendErr.Category)
		message := sendErr.Message()
		result.ErrorCategory = &category
		result.ErrorMessage = &message
	}
	return result, nil
}

func sanitizeSenderName(value string) string {
	cleaned := strings.Map(func(char rune) rune {
		if char < 0x20 || char == 0x7f {
			return -1
		}
		return char
	}, value)
	return strings.TrimSpace(cleaned)
}

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/mail"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

const testAppPassword = "abcd efgh ijkl mnop"

type fakeDocumentMailRepo struct {
	setting authrepo.DocumentMailSettingRecord
	saves   int
	admins  []string
	users   map[string]model.User
}

func (f *fakeDocumentMailRepo) GetDocumentMailRecord(context.Context) (authrepo.DocumentMailSettingRecord, error) {
	return authrepo.NormalizeDocumentMailSettingRecord(f.setting), nil
}

func (f *fakeDocumentMailRepo) UpdateDocumentMail(_ context.Context, _ string, setting authrepo.DocumentMailSettingRecord) error {
	f.saves++
	f.setting = setting
	return nil
}

func (f *fakeDocumentMailRepo) GetUserByID(_ context.Context, userID string) (model.User, error) {
	user, ok := f.users[userID]
	if !ok {
		return model.User{}, errors.New("not found")
	}
	return user, nil
}

func (f *fakeDocumentMailRepo) ListUserIDsByPermission(context.Context, string) ([]string, error) {
	return f.admins, nil
}

func (f *fakeDocumentMailRepo) ResolveUserFullName(_ context.Context, userID string) (string, error) {
	return f.users[userID].FullName, nil
}

type fakeDeliveries struct {
	mu   sync.Mutex
	rows map[string]*model.EmailDelivery
	seq  int
}

func newFakeDeliveries() *fakeDeliveries {
	return &fakeDeliveries{rows: map[string]*model.EmailDelivery{}}
}

// The fakes mirror the repository SQL and, like pgx, refuse a done context.
func (f *fakeDeliveries) Create(ctx context.Context, params notificationsrepo.CreateEmailDeliveryParams) (model.EmailDelivery, error) {
	if err := ctx.Err(); err != nil {
		return model.EmailDelivery{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	attempts := 0
	if params.Status == model.EmailDeliveryStatusSending {
		attempts = 1
	}
	row := &model.EmailDelivery{
		ID:               "delivery-" + string(rune('0'+f.seq)),
		Kind:             params.Kind,
		ReferenceType:    params.ReferenceType,
		ReferenceID:      params.ReferenceID,
		Recipient:        params.Recipient,
		RecipientSource:  params.RecipientSource,
		Cc:               nonNil(params.Cc),
		Subject:          params.Subject,
		AttachmentNames:  nonNil(params.AttachmentNames),
		AttachmentSHA256: nonNil(params.AttachmentSHA256),
		Status:           params.Status,
		Attempts:         attempts,
		RequestedBy:      params.RequestedBy,
	}
	f.rows[row.ID] = row
	return *row, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (f *fakeDeliveries) GetByID(ctx context.Context, id string) (model.EmailDelivery, error) {
	if err := ctx.Err(); err != nil {
		return model.EmailDelivery{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok {
		return model.EmailDelivery{}, notificationsrepo.ErrEmailDeliveryNotFound
	}
	return *row, nil
}

func (f *fakeDeliveries) MarkSending(ctx context.Context, id string) (model.EmailDelivery, error) {
	if err := ctx.Err(); err != nil {
		return model.EmailDelivery{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok || row.Status != model.EmailDeliveryStatusQueued {
		return model.EmailDelivery{}, notificationsrepo.ErrEmailDeliveryNotFound
	}
	row.Status = model.EmailDeliveryStatusSending
	row.Attempts++
	return *row, nil
}

func (f *fakeDeliveries) MarkSent(ctx context.Context, id string, sentAt time.Time) (model.EmailDelivery, error) {
	if err := ctx.Err(); err != nil {
		return model.EmailDelivery{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.rows[id]
	row.Status = model.EmailDeliveryStatusSent
	row.SentAt = &sentAt
	return *row, nil
}

func (f *fakeDeliveries) MarkFailed(ctx context.Context, id string, message string) (model.EmailDelivery, error) {
	if err := ctx.Err(); err != nil {
		return model.EmailDelivery{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.rows[id]
	row.Status = model.EmailDeliveryStatusFailed
	row.Error = &message
	return *row, nil
}

func (f *fakeDeliveries) FailStaleInFlight(context.Context, string, string, time.Duration, string) (int64, error) {
	return 0, nil
}

func (f *fakeDeliveries) FailStaleUnreferenced(context.Context, string, time.Duration, string) (int64, error) {
	return 0, nil
}

type fakeNotifier struct {
	params []notificationsrepo.CreateParams
}

func (f *fakeNotifier) CreateMany(_ context.Context, params []notificationsrepo.CreateParams) error {
	f.params = append(f.params, params...)
	return nil
}

type fakeTransport struct {
	devAddr string
	err     error
	creds   []mail.Credentials
	msgs    []mail.Message
	// onSend runs inside Send (e.g. to cancel the caller's request context
	// mid-send); its error, if any, is returned.
	onSend func(ctx context.Context) error
}

func (f *fakeTransport) Send(ctx context.Context, creds mail.Credentials, msg mail.Message) error {
	f.creds = append(f.creds, creds)
	f.msgs = append(f.msgs, msg)
	if f.onSend != nil {
		if err := f.onSend(ctx); err != nil {
			return err
		}
	}
	return f.err
}

func (f *fakeTransport) DevSMTPAddr() string { return f.devAddr }

var (
	testEncrypterOnce sync.Once
	testEncrypter     *security.Encrypter
)

func documentMailEncrypter(t *testing.T) *security.Encrypter {
	t.Helper()
	testEncrypterOnce.Do(func() {
		encrypter, err := security.NewEncrypter("document-mail-test-key-0123456789")
		if err != nil {
			panic(err)
		}
		testEncrypter = encrypter
	})
	return testEncrypter
}

type documentMailFixture struct {
	service    *DocumentMailService
	repo       *fakeDocumentMailRepo
	deliveries *fakeDeliveries
	notifier   *fakeNotifier
	transport  *fakeTransport
}

func newDocumentMailFixture(t *testing.T) documentMailFixture {
	t.Helper()
	repo := &fakeDocumentMailRepo{
		setting: authrepo.DocumentMailSettingRecord{SMTPPort: 587},
		admins:  []string{"admin-1", "admin-2"},
		users: map[string]model.User{
			"admin-1": {ID: "admin-1", Email: "superadmin@kantor.local", FullName: "Super Admin"},
		},
	}
	deliveries := newFakeDeliveries()
	notifier := &fakeNotifier{}
	transport := &fakeTransport{}
	return documentMailFixture{
		service:    NewDocumentMailService(repo, deliveries, notifier, transport, documentMailEncrypter(t)),
		repo:       repo,
		deliveries: deliveries,
		notifier:   notifier,
		transport:  transport,
	}
}

func strPtr(value string) *string { return &value }

func (f documentMailFixture) configure(t *testing.T) {
	t.Helper()
	_, _, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{
		Enabled:      true,
		SMTPUsername: "Slip@Example.com",
		SMTPPassword: strPtr(testAppPassword),
		SMTPPort:     587,
		SenderName:   "HR Kantor",
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	f.notifier.params = nil
}

func TestDocumentMailUpdateEncryptsAndNeverReturnsPassword(t *testing.T) {
	f := newDocumentMailFixture(t)

	response, changed, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{
		Enabled:      true,
		SMTPUsername: " Slip@Example.com ",
		SMTPPassword: strPtr(testAppPassword),
		SMTPPort:     587,
		SenderName:   "HR Kantor\r\nBcc: x@example.com",
	})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	stored := f.repo.setting
	if stored.SMTPUsername != "slip@example.com" {
		t.Fatalf("username = %q, want normalized", stored.SMTPUsername)
	}
	if stored.SenderName != "HR KantorBcc: x@example.com" {
		t.Fatalf("sender name must be stripped of control characters, got %q", stored.SenderName)
	}
	if !stored.HasPassword() || strings.Contains(stored.SMTPPasswordEncrypted, "abcd") {
		t.Fatalf("password must be stored encrypted, got %q", stored.SMTPPasswordEncrypted)
	}
	plain, err := documentMailEncrypter(t).DecryptString(stored.SMTPPasswordEncrypted)
	if err != nil || plain != "abcdefghijklmnop" {
		t.Fatalf("decrypted password = %q (%v), want spaces stripped", plain, err)
	}

	if !response.HasSMTPPassword || !response.Ready || response.SMTPHost != mail.GmailHost {
		t.Fatalf("unexpected response %+v", response)
	}
	if response.DevSMTPAddr != nil {
		t.Fatal("dev_smtp_addr must be absent when the override is off")
	}
	raw, _ := json.Marshal(response)
	if strings.Contains(string(raw), "abcd") || strings.Contains(string(raw), stored.SMTPPasswordEncrypted) || strings.Contains(string(raw), "password\":\"") {
		t.Fatalf("response leaks the password: %s", raw)
	}

	if strings.Join(changed, ",") != "enabled,smtp_username,smtp_password,sender_name" {
		t.Fatalf("changed fields = %v", changed)
	}
	if len(f.notifier.params) != 2 {
		t.Fatalf("expected an in-app notice per admin, got %d", len(f.notifier.params))
	}
	notice := f.notifier.params[0]
	if notice.Type != "admin.document_mail.updated" || !strings.Contains(notice.Message, "Super Admin") || strings.Contains(notice.Message, "abcd") {
		t.Fatalf("unexpected notice %+v", notice)
	}
}

func TestDocumentMailAccountChangeRequiresNewPassword(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*dto.UpdateDocumentMailRequest)
	}{
		{"username", func(r *dto.UpdateDocumentMailRequest) { r.SMTPUsername = "other@example.com" }},
		{"port", func(r *dto.UpdateDocumentMailRequest) { r.SMTPPort = 465 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDocumentMailFixture(t)
			f.configure(t)
			before := f.repo.setting.SMTPPasswordEncrypted

			request := dto.UpdateDocumentMailRequest{Enabled: true, SMTPUsername: "slip@example.com", SMTPPort: 587, SenderName: "HR Kantor"}
			tc.change(&request)

			if _, _, err := f.service.UpdateSettings(context.Background(), "admin-1", request); !errors.Is(err, ErrDocumentMailPasswordRequired) {
				t.Fatalf("err = %v, want ErrDocumentMailPasswordRequired", err)
			}
			if f.repo.setting.SMTPPasswordEncrypted != before {
				t.Fatal("a rejected save must not touch the stored password")
			}

			// Clearing the stored password does not waive the rule: the
			// account change still needs a new password in the same request.
			cleared := request
			cleared.ClearSMTPPassword = true
			if _, _, err := f.service.UpdateSettings(context.Background(), "admin-1", cleared); !errors.Is(err, ErrDocumentMailPasswordRequired) {
				t.Fatalf("clear without new password: err = %v, want ErrDocumentMailPasswordRequired", err)
			}
			if f.repo.setting.SMTPPasswordEncrypted != before || f.repo.setting.SMTPUsername != "slip@example.com" || f.repo.setting.SMTPPort != 587 {
				t.Fatalf("a rejected save must not change the stored setting, got %+v", f.repo.setting)
			}

			// A new password alongside clear_smtp_password is stored for the
			// new account (the old one is dropped either way).
			clearedWithPassword := cleared
			clearedWithPassword.SMTPPassword = strPtr("qrst qrst qrst qrst")
			response, changed, err := f.service.UpdateSettings(context.Background(), "admin-1", clearedWithPassword)
			if err != nil {
				t.Fatalf("clear with new password: %v", err)
			}
			if !response.HasSMTPPassword {
				t.Fatal("the new password must be stored for the new account")
			}
			plain, _ := documentMailEncrypter(t).DecryptString(f.repo.setting.SMTPPasswordEncrypted)
			if plain != "qrstqrstqrstqrst" {
				t.Fatalf("stored password = %q, want the new one", plain)
			}
			if !strings.Contains(strings.Join(changed, ","), "smtp_password") {
				t.Fatalf("changed = %v, want smtp_password", changed)
			}

			withPassword := request
			withPassword.SMTPPassword = strPtr("wxyz wxyz wxyz wxyz")
			f.configure(t)
			if _, _, err := f.service.UpdateSettings(context.Background(), "admin-1", withPassword); err != nil {
				t.Fatalf("change with new password: %v", err)
			}
			plain, _ = documentMailEncrypter(t).DecryptString(f.repo.setting.SMTPPasswordEncrypted)
			if plain != "wxyzwxyzwxyzwxyz" {
				t.Fatalf("stored password = %q, want the new one", plain)
			}
		})
	}
}

func TestDocumentMailClearPasswordWithoutAccountChange(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.configure(t)

	response, changed, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{
		Enabled: true, SMTPUsername: "slip@example.com", SMTPPort: 587, SenderName: "HR Kantor", ClearSMTPPassword: true,
	})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if response.HasSMTPPassword || f.repo.setting.HasPassword() {
		t.Fatal("clear_smtp_password must drop the stored password")
	}
	if strings.Join(changed, ",") != "smtp_password" {
		t.Fatalf("changed = %v, want smtp_password", changed)
	}
}

func TestDocumentMailKeepsPasswordWhenAccountUnchanged(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.configure(t)
	before := f.repo.setting.SMTPPasswordEncrypted

	_, changed, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{
		Enabled: true, SMTPUsername: "slip@example.com", SMTPPort: 587, SenderName: "HR Perusahaan",
	})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if f.repo.setting.SMTPPasswordEncrypted != before {
		t.Fatal("editing only the sender name must keep the stored password")
	}
	if strings.Join(changed, ",") != "sender_name" {
		t.Fatalf("changed = %v", changed)
	}
}

func TestDocumentMailEnabledRequiresUsername(t *testing.T) {
	f := newDocumentMailFixture(t)
	_, _, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{Enabled: true, SMTPPort: 587})
	if !errors.Is(err, ErrDocumentMailUsernameRequired) {
		t.Fatalf("err = %v", err)
	}
	if f.repo.saves != 0 {
		t.Fatal("nothing must be saved")
	}
}

func TestDocumentMailSendTestRecordsDelivery(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.configure(t)

	result, err := f.service.SendTest(context.Background(), "admin-1")
	if err != nil {
		t.Fatalf("SendTest: %v", err)
	}
	if !result.Sent || result.Recipient != "superadmin@kantor.local" || result.ErrorMessage != nil {
		t.Fatalf("unexpected result %+v", result)
	}

	if len(f.transport.msgs) != 1 {
		t.Fatalf("expected one send, got %d", len(f.transport.msgs))
	}
	creds := f.transport.creds[0]
	if creds.Username != "slip@example.com" || creds.Password != "abcdefghijklmnop" || creds.Port != 587 || creds.SenderName != "HR Kantor" {
		t.Fatalf("unexpected credentials %+v", creds)
	}
	msg := f.transport.msgs[0]
	if msg.To != "superadmin@kantor.local" || len(msg.Attachments) != 0 {
		t.Fatalf("unexpected message %+v", msg)
	}
	if !strings.Contains(msg.Text, "Ini adalah email uji") || !strings.Contains(msg.Text, "This is a test email") {
		t.Fatalf("test body must be bilingual:\n%s", msg.Text)
	}

	row := f.deliveries.rows[result.DeliveryID]
	if row == nil || row.Kind != model.EmailDeliveryKindTest || row.Status != model.EmailDeliveryStatusSent ||
		row.Attempts != 1 || row.RecipientSource != model.EmailRecipientSourceSelf || row.SentAt == nil {
		t.Fatalf("unexpected delivery row %+v", row)
	}
}

func TestDocumentMailSendTestReportsCategory(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.configure(t)
	f.transport.err = &mail.SendError{Category: mail.CategoryAuth, Code: 535}

	result, err := f.service.SendTest(context.Background(), "admin-1")
	if err != nil {
		t.Fatalf("SendTest must report a send failure in the result, got err %v", err)
	}
	if result.Sent || result.ErrorCategory == nil || *result.ErrorCategory != "auth" ||
		result.ErrorMessage == nil || *result.ErrorMessage != "Autentikasi ditolak (535)" {
		t.Fatalf("unexpected result %+v", result)
	}
	row := f.deliveries.rows[result.DeliveryID]
	if row.Status != model.EmailDeliveryStatusFailed || row.Error == nil || *row.Error != "Autentikasi ditolak (535)" {
		t.Fatalf("unexpected delivery row %+v", row)
	}
}

func TestDocumentMailNotReady(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.repo.setting = authrepo.DocumentMailSettingRecord{Enabled: true, SMTPUsername: "slip@example.com", SMTPPort: 587}

	if _, err := f.service.SendTest(context.Background(), "admin-1"); !errors.Is(err, ErrDocumentMailNotReady) {
		t.Fatalf("err = %v, want ErrDocumentMailNotReady without a password", err)
	}
	if len(f.deliveries.rows) != 0 || len(f.transport.msgs) != 0 {
		t.Fatal("nothing must be sent or recorded when not ready")
	}
}

func TestDocumentMailDevOverrideAllowsMissingPassword(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.transport.devAddr = "localhost:1025"
	f.repo.setting = authrepo.DocumentMailSettingRecord{Enabled: true, SMTPUsername: "slip@example.com", SMTPPort: 587, SenderName: "HR Kantor"}

	response, err := f.service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !response.Ready || response.HasSMTPPassword || response.DevSMTPAddr == nil || *response.DevSMTPAddr != "localhost:1025" {
		t.Fatalf("unexpected response %+v", response)
	}

	result, err := f.service.SendTest(context.Background(), "admin-1")
	if err != nil || !result.Sent {
		t.Fatalf("SendTest in dev mode = %+v, %v", result, err)
	}
}

func TestDocumentMailDeliverRecordsAttachmentDigests(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.configure(t)
	referenceType := "payslip"
	referenceID := "8f2b1c9e-0000-4000-8000-000000000001"

	delivery, err := f.service.Deliver(context.Background(), DocumentMailRequest{
		Kind:            model.EmailDeliveryKindPayslip,
		ReferenceType:   &referenceType,
		ReferenceID:     &referenceID,
		Recipient:       "budi@example.com",
		RecipientSource: model.EmailRecipientSourceLogin,
		Subject:         "Slip Gaji — Oktober 2026",
		Text:            "Terlampir slip gaji.",
		Attachments:     []mail.Attachment{{Filename: "slip.pdf", ContentType: "application/pdf", Data: []byte("abc")}},
		RequestedBy:     "admin-1",
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(delivery.AttachmentSHA256) != 1 || delivery.AttachmentSHA256[0] != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("attachment digests = %v", delivery.AttachmentSHA256)
	}
	if delivery.Status != model.EmailDeliveryStatusSent {
		t.Fatalf("status = %q", delivery.Status)
	}

	if _, err := f.service.Deliver(context.Background(), DocumentMailRequest{
		Kind: model.EmailDeliveryKindPayslip, Recipient: "Budi <budi@example.com>", RecipientSource: model.EmailRecipientSourceLogin, Subject: "x", Text: "x",
	}); !errors.Is(err, ErrDocumentMailInvalidRequest) {
		t.Fatalf("display-name recipient: err = %v", err)
	}
}

func payslipRequest(recipient string, source string, subject string, pdf []byte) DocumentMailRequest {
	referenceType := "payslip"
	referenceID := "8f2b1c9e-0000-4000-8000-000000000002"
	return DocumentMailRequest{
		Kind:            model.EmailDeliveryKindPayslip,
		ReferenceType:   &referenceType,
		ReferenceID:     &referenceID,
		Recipient:       recipient,
		RecipientSource: source,
		Subject:         subject,
		Text:            "Terlampir slip gaji.",
		Attachments:     []mail.Attachment{{Filename: "slip.pdf", ContentType: "application/pdf", Data: pdf}},
		RequestedBy:     "admin-1",
	}
}

// A queued row may only be continued by a request that describes exactly what
// it recorded; otherwise the log would show an address or digest that was
// never sent.
func TestDocumentMailDeliveryIDMustMatchQueuedRow(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.configure(t)

	queued, err := f.service.Enqueue(context.Background(), payslipRequest("budi@kantor.local", model.EmailRecipientSourceLogin, "Slip Gaji - Oktober 2026", []byte("slip-v1")))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	for _, tc := range []struct {
		name   string
		change func(*DocumentMailRequest)
	}{
		{"recipient", func(r *DocumentMailRequest) { r.Recipient = "budi.personal@gmail.com" }},
		{"recipient source", func(r *DocumentMailRequest) { r.RecipientSource = model.EmailRecipientSourcePersonal }},
		{"subject", func(r *DocumentMailRequest) { r.Subject = "Slip Gaji - Oktober 2026 (Revisi 1)" }},
		{"attachment", func(r *DocumentMailRequest) { r.Attachments[0].Data = []byte("slip-v2") }},
		{"cc", func(r *DocumentMailRequest) { r.Cc = []string{"hr@kantor.local"} }},
		{"reference", func(r *DocumentMailRequest) { other := "8f2b1c9e-0000-4000-8000-000000000003"; r.ReferenceID = &other }},
		{"kind", func(r *DocumentMailRequest) { r.Kind = model.EmailDeliveryKindContract }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := payslipRequest("budi@kantor.local", model.EmailRecipientSourceLogin, "Slip Gaji - Oktober 2026", []byte("slip-v1"))
			tc.change(&request)
			request.DeliveryID = queued.ID
			if _, err := f.service.Deliver(context.Background(), request); !errors.Is(err, ErrDocumentMailInvalidRequest) {
				t.Fatalf("err = %v, want ErrDocumentMailInvalidRequest", err)
			}
			if len(f.transport.msgs) != 0 {
				t.Fatal("nothing may be sent for a mismatched delivery")
			}
			if row := f.deliveries.rows[queued.ID]; row.Status != model.EmailDeliveryStatusQueued || row.Attempts != 0 {
				t.Fatalf("queued row must stay untouched, got %+v", row)
			}
		})
	}

	request := payslipRequest("Budi@Kantor.local ", model.EmailRecipientSourceLogin, "Slip Gaji - Oktober 2026", []byte("slip-v1"))
	request.DeliveryID = queued.ID
	sent, err := f.service.Deliver(context.Background(), request)
	if err != nil {
		t.Fatalf("matching Deliver: %v", err)
	}
	if sent.ID != queued.ID || sent.Status != model.EmailDeliveryStatusSent || sent.Attempts != 1 {
		t.Fatalf("unexpected delivery %+v", sent)
	}

	// A sent (or failed) row is never reopened: a retry is a new row.
	if _, err := f.service.Deliver(context.Background(), request); !errors.Is(err, ErrDocumentMailInvalidRequest) {
		t.Fatalf("re-delivering a sent row: err = %v, want ErrDocumentMailInvalidRequest", err)
	}
	unknown := request
	unknown.DeliveryID = "delivery-missing"
	if _, err := f.service.Deliver(context.Background(), unknown); !errors.Is(err, ErrDocumentMailInvalidRequest) {
		t.Fatalf("unknown delivery: err = %v, want ErrDocumentMailInvalidRequest", err)
	}
	if len(f.transport.msgs) != 1 {
		t.Fatalf("expected exactly one send, got %d", len(f.transport.msgs))
	}
}

// A client disconnect mid-send must neither abort the SMTP transaction nor
// leave the row in 'sending'.
func TestDocumentMailDeliverSurvivesRequestCancel(t *testing.T) {
	for _, tc := range []struct {
		name       string
		sendErr    error
		wantStatus string
	}{
		{"send succeeds", nil, model.EmailDeliveryStatusSent},
		{"send fails", &mail.SendError{Category: mail.CategoryRejected, Code: 552}, model.EmailDeliveryStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDocumentMailFixture(t)
			f.configure(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.transport.err = tc.sendErr
			f.transport.onSend = func(sendCtx context.Context) error {
				cancel()
				if err := sendCtx.Err(); err != nil {
					t.Errorf("the SMTP send must not be cancelled with the request: %v", err)
				}
				if _, ok := sendCtx.Deadline(); !ok {
					t.Error("the SMTP send must keep its own timeout")
				}
				return nil
			}

			delivery, _ := f.service.Deliver(ctx, payslipRequest("budi@kantor.local", model.EmailRecipientSourceLogin, "Slip Gaji - Oktober 2026", []byte("slip")))
			row := f.deliveries.rows[delivery.ID]
			if row == nil || row.Status != tc.wantStatus {
				t.Fatalf("row = %+v, want status %s", row, tc.wantStatus)
			}
		})
	}
}

func TestDocumentMailRejectsUnsendableUsername(t *testing.T) {
	f := newDocumentMailFixture(t)
	_, _, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{
		Enabled: true, SMTPUsername: `"a b"@example.com`, SMTPPassword: strPtr(testAppPassword), SMTPPort: 587,
	})
	if !errors.Is(err, ErrDocumentMailUsernameInvalid) {
		t.Fatalf("err = %v, want ErrDocumentMailUsernameInvalid", err)
	}
	if f.repo.saves != 0 {
		t.Fatal("nothing must be saved")
	}

	// A value stored before this check existed is never reported as ready.
	f.repo.setting = authrepo.DocumentMailSettingRecord{Enabled: true, SMTPUsername: `"a b"@example.com`, SMTPPort: 587, SMTPPasswordEncrypted: "x"}
	response, err := f.service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Ready {
		t.Fatal("ready must be false for an address the sender refuses")
	}
}

// Removing the account (blank address) drops the stored password in one
// save; there is no account left that would need a new one.
func TestDocumentMailRemoveAccountInOneSave(t *testing.T) {
	for _, clear := range []bool{false, true} {
		f := newDocumentMailFixture(t)
		f.configure(t)

		response, changed, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{
			Enabled: false, SMTPUsername: "", SMTPPort: 587, SenderName: "HR Kantor", ClearSMTPPassword: clear,
		})
		if err != nil {
			t.Fatalf("clear=%v: remove account: %v", clear, err)
		}
		if response.HasSMTPPassword || f.repo.setting.HasPassword() || f.repo.setting.SMTPUsername != "" {
			t.Fatalf("clear=%v: account and password must be gone, got %+v", clear, f.repo.setting)
		}
		if strings.Join(changed, ",") != "enabled,smtp_username,smtp_password" {
			t.Fatalf("clear=%v: changed = %v", clear, changed)
		}
	}

	// A password without an account is never kept either.
	f := newDocumentMailFixture(t)
	response, _, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{
		SMTPPort: 587, SMTPPassword: strPtr(testAppPassword),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.HasSMTPPassword {
		t.Fatal("a password without a Gmail address must not be stored")
	}
}

// The settings response carries read-only delivery and PDF status, never a
// filesystem path.
func TestDocumentMailSettingsReportDeliveryAndPDFStatus(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.service.SetPDFStatus(true, "auto")

	response, err := f.service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Delivery.Mode != DocumentMailModeGmail || response.Delivery.CaptureAddr != nil || response.DevSMTPAddr != nil {
		t.Fatalf("gmail mode delivery = %+v", response.Delivery)
	}
	if !response.PDF.Enabled || response.PDF.Source != "auto" {
		t.Fatalf("pdf = %+v", response.PDF)
	}

	f.transport.devAddr = "localhost:1025"
	f.service.SetPDFStatus(false, "env_invalid")
	response, err = f.service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Delivery.Mode != DocumentMailModeDevCapture || response.Delivery.CaptureAddr == nil || *response.Delivery.CaptureAddr != "localhost:1025" {
		t.Fatalf("dev capture delivery = %+v", response.Delivery)
	}
	if response.PDF.Enabled || response.PDF.Source != "env_invalid" {
		t.Fatalf("pdf = %+v", response.PDF)
	}
	raw, _ := json.Marshal(response)
	for _, want := range []string{`"delivery":{"mode":"dev_capture","capture_addr":"localhost:1025"}`, `"pdf":{"enabled":false,"source":"env_invalid"}`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("response %s lacks %s", raw, want)
		}
	}
	if strings.Contains(string(raw), "soffice") || strings.Contains(string(raw), "lo-profile") || strings.Contains(string(raw), "/") {
		t.Fatalf("response must not carry a filesystem path: %s", raw)
	}

	// The update response reports the same status; the request has no such
	// fields to set.
	updated, _, err := f.service.UpdateSettings(context.Background(), "admin-1", dto.UpdateDocumentMailRequest{
		Enabled: true, SMTPUsername: "slip@example.com", SMTPPort: 587,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Delivery.Mode != DocumentMailModeDevCapture || updated.PDF.Source != "env_invalid" {
		t.Fatalf("update response status = %+v / %+v", updated.Delivery, updated.PDF)
	}
}

// An unreachable development capture server is recorded with the clear
// Indonesian guidance, without credentials or recipients.
func TestDocumentMailSendTestDevCaptureUnreachable(t *testing.T) {
	f := newDocumentMailFixture(t)
	f.transport.devAddr = "localhost:1025"
	f.repo.setting = authrepo.DocumentMailSettingRecord{Enabled: true, SMTPUsername: "slip@example.com", SMTPPort: 587, SenderName: "HR Kantor"}
	f.transport.err = &mail.SendError{Category: mail.CategoryConnect, Addr: "localhost:1025", DevCapture: true}

	result, err := f.service.SendTest(context.Background(), "admin-1")
	if err != nil {
		t.Fatalf("SendTest: %v", err)
	}
	want := "Mode development: email dokumen diarahkan ke Mailpit di localhost:1025, tetapi server itu tidak bisa dihubungi. " +
		"Jalankan Mailpit (mis. docker run -d -p 1025:1025 -p 8025:8025 axllent/mailpit) atau set DOCUMENT_MAIL_DEV_SMTP_ADDR=off untuk mengirim lewat Gmail."
	if result.Sent || result.ErrorMessage == nil || *result.ErrorMessage != want {
		t.Fatalf("unexpected result %+v", result)
	}
	row := f.deliveries.rows[result.DeliveryID]
	if row.Error == nil || *row.Error != want {
		t.Fatalf("delivery row error = %v", row.Error)
	}
	for _, secret := range []string{"slip@example.com", "superadmin@kantor.local"} {
		if strings.Contains(*row.Error, secret) {
			t.Fatalf("error leaks %q", secret)
		}
	}
}

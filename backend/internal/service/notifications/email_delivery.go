package notifications

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	warepo "github.com/kana-consultant/kantor/backend/internal/repository/whatsapp"
	"github.com/kana-consultant/kantor/backend/internal/security"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

var ErrNotificationEmailsDisabled = errors.New("notification emails are disabled")

type emailSettingsRepository interface {
	GetMailDeliveryRecord(ctx context.Context) (authrepo.MailDeliverySettingRecord, error)
	GetTenantPrimaryDomain(ctx context.Context) (string, error)
}

type emailNotificationRepository interface {
	GetTaskWithProject(ctx context.Context, taskID string) (*warepo.TaskDueInfo, error)
	GetReimbursementWithSubmitter(ctx context.Context, reimbursementID string) (*warepo.ReimbursementNotifyInfo, error)
	GetWeeklyDigestData(ctx context.Context) ([]warepo.WeeklyDigestInfo, error)
	GetWAConfig(ctx context.Context) (warepo.WAConfig, error)
}

type EmailDeliveryService struct {
	settingsRepo        emailSettingsRepository
	notificationRepo    emailNotificationRepository
	encrypter           *security.Encrypter
	sender              *resendSender
	fallbackAppURL      string
	mu                  sync.Mutex
	lastWeeklyDigestRun map[string]time.Time
}

type emailRuntimeConfig struct {
	APIKey       string
	SenderName   string
	SenderEmail  string
	ReplyToEmail *string
}

func NewEmailDeliveryService(settingsRepo emailSettingsRepository, notificationRepo emailNotificationRepository, encrypter *security.Encrypter, cfg config.Config) *EmailDeliveryService {
	return &EmailDeliveryService{
		settingsRepo:        settingsRepo,
		notificationRepo:    notificationRepo,
		encrypter:           encrypter,
		sender:              newResendSender(),
		fallbackAppURL:      strings.TrimRight(strings.TrimSpace(cfg.AppURL), "/"),
		lastWeeklyDigestRun: make(map[string]time.Time),
	}
}

func (s *EmailDeliveryService) SendTaskAssignedNotification(ctx context.Context, taskID string, assigneeID string) {
	if _, err := platformmiddleware.WithScopedTenantConn(ctx, func(scopedCtx context.Context) (struct{}, error) {
		return struct{}{}, s.sendTaskAssignedNotification(scopedCtx, taskID)
	}); err != nil && !errors.Is(err, ErrNotificationEmailsDisabled) {
		slog.Error("failed to send task assigned email", "task_id", taskID, "assignee_id", assigneeID, "error", err)
	}
}

func (s *EmailDeliveryService) SendReimbursementStatusNotification(ctx context.Context, reimbursementID string, newStatus string, reviewerNotes string) {
	if _, err := platformmiddleware.WithScopedTenantConn(ctx, func(scopedCtx context.Context) (struct{}, error) {
		return struct{}{}, s.sendReimbursementStatusNotification(scopedCtx, reimbursementID, newStatus, reviewerNotes)
	}); err != nil && !errors.Is(err, ErrNotificationEmailsDisabled) {
		slog.Error("failed to send reimbursement status email", "reimbursement_id", reimbursementID, "error", err)
	}
}

func (s *EmailDeliveryService) RunCronJobs(ctx context.Context, now time.Time) error {
	_, err := platformmiddleware.WithScopedTenantConn(ctx, func(scopedCtx context.Context) (struct{}, error) {
		return struct{}{}, s.runCronJobs(scopedCtx, now)
	})
	if errors.Is(err, ErrNotificationEmailsDisabled) {
		return nil
	}
	return err
}

func (s *EmailDeliveryService) runCronJobs(ctx context.Context, now time.Time) error {
	if _, err := s.loadRuntimeConfig(ctx); err != nil {
		return err
	}

	waConfig, err := s.notificationRepo.GetWAConfig(ctx)
	if err != nil {
		return err
	}

	weeklyDigestCron := strings.TrimSpace(waConfig.WeeklyDigestCron)
	if weeklyDigestCron == "" {
		weeklyDigestCron = "0 8 * * 1"
	}
	if !emailCronMatches(weeklyDigestCron, now) || !s.shouldRunWeeklyDigest(ctx, now) {
		return nil
	}

	return s.sendWeeklyDigest(ctx, now)
}

func (s *EmailDeliveryService) sendTaskAssignedNotification(ctx context.Context, taskID string) error {
	cfg, err := s.loadRuntimeConfig(ctx)
	if err != nil {
		return err
	}

	task, err := s.notificationRepo.GetTaskWithProject(ctx, taskID)
	if err != nil || task == nil {
		if err != nil {
			return err
		}
		return nil
	}
	if strings.TrimSpace(task.UserEmail) == "" {
		return nil
	}

	baseURL, err := s.resolveTenantBaseURL(ctx)
	if err != nil {
		return err
	}

	boardURL := baseURL + "/operational/projects/" + url.PathEscape(task.ProjectID) + "?view=board"
	content := eventEmailContent{
		Eyebrow: "Task baru",
		Title:   task.TaskTitle,
		Intro:   fmt.Sprintf("Halo %s, Anda mendapat task baru di project %s.", task.UserName, task.ProjectName),
		Highlights: []string{
			"Project: " + task.ProjectName,
			"Deadline: " + fallbackText(task.DueDate, "Belum diatur"),
			"Prioritas: " + fallbackText(task.Priority, "Belum diatur"),
		},
		CTAURL:   boardURL,
		CTALabel: "Buka board project",
	}

	return s.sender.Send(ctx, cfg, tenantDisplayName(ctx), resendEmail{
		ToEmail: task.UserEmail,
		Subject: fmt.Sprintf("Task baru ditugaskan: %s", task.TaskTitle),
		HTML:    renderEventEmailHTML(content),
		Text:    renderEventEmailText(content),
	})
}

func (s *EmailDeliveryService) sendReimbursementStatusNotification(ctx context.Context, reimbursementID string, newStatus string, reviewerNotes string) error {
	cfg, err := s.loadRuntimeConfig(ctx)
	if err != nil {
		return err
	}

	info, err := s.notificationRepo.GetReimbursementWithSubmitter(ctx, reimbursementID)
	if err != nil || info == nil {
		if err != nil {
			return err
		}
		return nil
	}
	if strings.TrimSpace(info.SubmitterEmail) == "" {
		return nil
	}

	baseURL, err := s.resolveTenantBaseURL(ctx)
	if err != nil {
		return err
	}

	highlights := []string{
		"Judul: " + info.Title,
		"Nominal: " + exportutil.FormatIDR(info.Amount),
		"Status baru: " + humanizeStatus(newStatus),
	}
	if strings.TrimSpace(reviewerNotes) != "" {
		highlights = append(highlights, "Catatan reviewer: "+strings.TrimSpace(reviewerNotes))
	}

	content := eventEmailContent{
		Eyebrow:    "Reimbursement",
		Title:      "Status reimbursement diperbarui",
		Intro:      fmt.Sprintf("Halo %s, ada perubahan status pada reimbursement Anda.", info.SubmitterName),
		Highlights: highlights,
		CTAURL:     baseURL + "/hris/reimbursements",
		CTALabel:   "Buka reimbursement",
	}

	return s.sender.Send(ctx, cfg, tenantDisplayName(ctx), resendEmail{
		ToEmail: info.SubmitterEmail,
		Subject: fmt.Sprintf("Update reimbursement: %s", humanizeStatus(newStatus)),
		HTML:    renderEventEmailHTML(content),
		Text:    renderEventEmailText(content),
	})
}

func (s *EmailDeliveryService) sendWeeklyDigest(ctx context.Context, now time.Time) error {
	cfg, err := s.loadRuntimeConfig(ctx)
	if err != nil {
		return err
	}

	baseURL, err := s.resolveTenantBaseURL(ctx)
	if err != nil {
		return err
	}

	items, err := s.notificationRepo.GetWeeklyDigestData(ctx)
	if err != nil {
		return err
	}

	weekEnd := now.AddDate(0, 0, -int(now.Weekday()))
	weekStart := weekEnd.AddDate(0, 0, -6)

	for _, item := range items {
		if strings.TrimSpace(item.UserEmail) == "" {
			continue
		}
		if item.CompletedCount == 0 && item.OpenCount == 0 && item.OverdueCount == 0 {
			continue
		}

		if err := s.sender.Send(ctx, cfg, tenantDisplayName(ctx), resendEmail{
			ToEmail: item.UserEmail,
			Subject: fmt.Sprintf("Weekly digest %s", weekEnd.Format("02 Jan 2006")),
			HTML:    renderWeeklyDigestHTML(item, weekStart, weekEnd, baseURL),
			Text:    renderWeeklyDigestText(item, weekStart, weekEnd, baseURL),
		}); err != nil {
			slog.Error("failed to send weekly digest email", "user_id", item.UserID, "error", err)
		}
	}

	return nil
}

func (s *EmailDeliveryService) loadRuntimeConfig(ctx context.Context) (emailRuntimeConfig, error) {
	setting, err := s.settingsRepo.GetMailDeliveryRecord(ctx)
	if err != nil {
		return emailRuntimeConfig{}, err
	}
	if !setting.NotificationEmailsEnabled() {
		return emailRuntimeConfig{}, ErrNotificationEmailsDisabled
	}

	apiKey, err := s.encrypter.DecryptString(setting.APIKeyEncrypted)
	if err != nil {
		return emailRuntimeConfig{}, err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return emailRuntimeConfig{}, ErrNotificationEmailsDisabled
	}

	return emailRuntimeConfig{
		APIKey:       apiKey,
		SenderName:   setting.SenderName,
		SenderEmail:  setting.SenderEmail,
		ReplyToEmail: setting.ReplyToEmail,
	}, nil
}

func (s *EmailDeliveryService) resolveTenantBaseURL(ctx context.Context) (string, error) {
	return tenant.ResolveBaseURL(ctx, s.settingsRepo, s.fallbackAppURL)
}

func (s *EmailDeliveryService) shouldRunWeeklyDigest(ctx context.Context, now time.Time) bool {
	info, ok := tenant.FromContext(ctx)
	if !ok || strings.TrimSpace(info.ID) == "" {
		return true
	}

	runKey := now.In(time.Local).Truncate(time.Minute)
	s.mu.Lock()
	defer s.mu.Unlock()

	if lastRunAt, exists := s.lastWeeklyDigestRun[info.ID]; exists && lastRunAt.Equal(runKey) {
		return false
	}
	s.lastWeeklyDigestRun[info.ID] = runKey
	return true
}

type eventEmailContent struct {
	Eyebrow    string
	Title      string
	Intro      string
	Highlights []string
	CTAURL     string
	CTALabel   string
}

func renderEventEmailHTML(content eventEmailContent) string {
	items := make([]string, 0, len(content.Highlights))
	for _, item := range content.Highlights {
		items = append(items, "<li style=\"margin:0 0 8px;\">"+html.EscapeString(item)+"</li>")
	}

	return renderEmailLayoutHTML(renderEmailLayoutParams{
		Eyebrow: content.Eyebrow,
		Title:   content.Title,
		Intro:   content.Intro,
		BodyHTML: "<ul style=\"padding-left:20px;margin:0 0 24px;\">" +
			strings.Join(items, "") + "</ul>",
		CTAURL:   content.CTAURL,
		CTALabel: content.CTALabel,
	})
}

func renderEventEmailText(content eventEmailContent) string {
	lines := []string{content.Intro, ""}
	lines = append(lines, content.Highlights...)
	lines = append(lines, "", content.CTALabel+": "+content.CTAURL)
	return renderEmailText(content.Title, lines...)
}

func renderWeeklyDigestHTML(item warepo.WeeklyDigestInfo, weekStart time.Time, weekEnd time.Time, baseURL string) string {
	body := fmt.Sprintf(`
		<p style="margin:0 0 16px;font-size:15px;line-height:1.7;">Halo %s, ini ringkasan aktivitas Anda untuk periode <strong>%s - %s</strong>.</p>
		<ul style="padding-left:20px;margin:0 0 24px;">
			<li style="margin:0 0 8px;">Task selesai: <strong>%d</strong></li>
			<li style="margin:0 0 8px;">Task masih terbuka: <strong>%d</strong></li>
			<li style="margin:0 0 8px;">Task overdue: <strong>%d</strong></li>
		</ul>`,
		html.EscapeString(item.UserName),
		weekStart.Format("02 Jan 2006"),
		weekEnd.Format("02 Jan 2006"),
		item.CompletedCount,
		item.OpenCount,
		item.OverdueCount,
	)

	return renderEmailLayoutHTML(renderEmailLayoutParams{
		Eyebrow:  "Weekly digest",
		Title:    "Ringkasan aktivitas mingguan",
		BodyHTML: body,
		CTAURL:   baseURL + "/operational/overview",
		CTALabel: "Buka overview operasional",
	})
}

func renderWeeklyDigestText(item warepo.WeeklyDigestInfo, weekStart time.Time, weekEnd time.Time, baseURL string) string {
	return renderEmailText(
		"Ringkasan aktivitas mingguan",
		fmt.Sprintf("Halo %s, ini ringkasan aktivitas Anda untuk periode %s - %s.", item.UserName, weekStart.Format("02 Jan 2006"), weekEnd.Format("02 Jan 2006")),
		"",
		fmt.Sprintf("Task selesai: %d", item.CompletedCount),
		fmt.Sprintf("Task masih terbuka: %d", item.OpenCount),
		fmt.Sprintf("Task overdue: %d", item.OverdueCount),
		"",
		"Buka overview operasional: "+baseURL+"/operational/overview",
	)
}

func tenantDisplayName(ctx context.Context) string {
	if info, ok := tenant.FromContext(ctx); ok && strings.TrimSpace(info.Name) != "" {
		return strings.TrimSpace(info.Name)
	}
	return "Kantor"
}

func fallbackText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func humanizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "submitted":
		return "Submitted"
	case "approved":
		return "Approved"
	case "rejected":
		return "Rejected"
	case "paid":
		return "Paid"
	default:
		return strings.TrimSpace(status)
	}
}

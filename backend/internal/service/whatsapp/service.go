package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/model"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	warepo "github.com/kana-consultant/kantor/backend/internal/repository/whatsapp"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

var (
	ErrTemplateNotFound = errors.New("wa template not found")
	ErrScheduleNotFound = errors.New("wa schedule not found")
	ErrSystemTemplate   = errors.New("cannot delete system template")
	ErrInvalidPhone     = errors.New("invalid whatsapp phone number")
)

type waNotificationsService interface {
	CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error
}

type Service struct {
	repo                 *warepo.Repository
	cfg                  config.Config
	notificationsService waNotificationsService

	mu      sync.RWMutex
	clients map[string]*WAHAClient // tenantID → cached client
}

func NewService(repo *warepo.Repository, cfg config.Config, notificationsService waNotificationsService) *Service {
	return &Service{
		repo:                 repo,
		cfg:                  cfg,
		notificationsService: notificationsService,
		clients:              make(map[string]*WAHAClient),
	}
}

// getClient returns (or creates) the per-tenant WAHAClient.
func (s *Service) getClient(ctx context.Context) (*WAHAClient, error) {
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant context required for WA client")
	}

	s.mu.RLock()
	if c, exists := s.clients[info.ID]; exists {
		s.mu.RUnlock()
		return c, nil
	}
	s.mu.RUnlock()

	dbCfg, err := s.repo.GetWAConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load wa config: %w", err)
	}

	client := NewWAHAClientFromDBConfig(WADBConfig{
		APIURL:           dbCfg.APIURL,
		APIKey:           dbCfg.APIKey,
		SessionName:      dbCfg.SessionName,
		Enabled:          dbCfg.Enabled,
		MaxDailyMessages: dbCfg.MaxDailyMessages,
		MinDelayMS:       dbCfg.MinDelayMS,
		MaxDelayMS:       dbCfg.MaxDelayMS,
		ReminderCron:     dbCfg.ReminderCron,
		WeeklyDigestCron: dbCfg.WeeklyDigestCron,
	})

	s.mu.Lock()
	if existing, exists := s.clients[info.ID]; exists {
		s.mu.Unlock()
		return existing, nil
	}
	s.clients[info.ID] = client
	s.mu.Unlock()

	return client, nil
}

// InvalidateClient removes the cached client for the current tenant (e.g. after config update).
func (s *Service) InvalidateClient(ctx context.Context) {
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return
	}
	s.mu.Lock()
	delete(s.clients, info.ID)
	s.mu.Unlock()
}

// --------------- WA Config ---------------

func (s *Service) GetWAConfig(ctx context.Context) (warepo.WAConfig, error) {
	return s.repo.GetWAConfig(ctx)
}

func (s *Service) UpdateWAConfig(ctx context.Context, cfg warepo.WAConfig) error {
	if err := s.repo.UpsertWAConfig(ctx, cfg); err != nil {
		return err
	}
	// Invalidate cached client so next call picks up new config.
	s.InvalidateClient(ctx)
	return nil
}

// --------------- Connection ---------------

func (s *Service) GetStatus(ctx context.Context) (*SessionStatus, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetStatus()
}

func (s *Service) GetQR(ctx context.Context) (string, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return "", err
	}
	return client.GetQR()
}

func (s *Service) StartSession(ctx context.Context) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	return client.StartSession()
}

func (s *Service) StopSession(ctx context.Context) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	return client.StopSession()
}

func (s *Service) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetAccountInfo()
}

func (s *Service) IsEnabled(ctx context.Context) bool {
	client, err := s.getClient(ctx)
	if err != nil {
		return false
	}
	return client.IsEnabled()
}

func (s *Service) GetDailyStats(ctx context.Context) *DailyStats {
	client, err := s.getClient(ctx)
	if err != nil {
		return &DailyStats{}
	}
	stats := client.GetDailyStats()
	sentToday, countErr := s.repo.CountSentLogsToday(ctx)
	if countErr != nil {
		slog.Error("failed to count daily wa logs", "error", countErr)
		return stats
	}
	stats.SentToday = sentToday
	return stats
}

// --------------- Templates ---------------

func (s *Service) ListTemplates(ctx context.Context, category string, triggerType string) ([]model.WAMessageTemplate, error) {
	return s.repo.ListTemplates(ctx, category, triggerType)
}

func (s *Service) GetTemplate(ctx context.Context, id string) (model.WAMessageTemplate, error) {
	t, err := s.repo.GetTemplateByID(ctx, id)
	if errors.Is(err, warepo.ErrTemplateNotFound) {
		return t, ErrTemplateNotFound
	}
	return t, err
}

func (s *Service) CreateTemplate(ctx context.Context, params warepo.CreateTemplateParams) (model.WAMessageTemplate, error) {
	return s.repo.CreateTemplate(ctx, params)
}

func (s *Service) UpdateTemplate(ctx context.Context, id string, params warepo.UpdateTemplateParams) (model.WAMessageTemplate, error) {
	existing, err := s.repo.GetTemplateByID(ctx, id)
	if errors.Is(err, warepo.ErrTemplateNotFound) {
		return model.WAMessageTemplate{}, ErrTemplateNotFound
	}
	if err != nil {
		return model.WAMessageTemplate{}, err
	}

	t, err := s.repo.UpdateTemplate(ctx, id, params, existing.IsSystem)
	if errors.Is(err, warepo.ErrTemplateNotFound) {
		return t, ErrTemplateNotFound
	}
	return t, err
}

func (s *Service) DeleteTemplate(ctx context.Context, id string) error {
	err := s.repo.DeleteTemplate(ctx, id)
	switch {
	case errors.Is(err, warepo.ErrTemplateNotFound):
		return ErrTemplateNotFound
	case errors.Is(err, warepo.ErrSystemTemplate):
		return ErrSystemTemplate
	}
	return err
}

func (s *Service) PreviewTemplate(ctx context.Context, id string) (string, error) {
	t, err := s.repo.GetTemplateByID(ctx, id)
	if errors.Is(err, warepo.ErrTemplateNotFound) {
		return "", ErrTemplateNotFound
	}
	if err != nil {
		return "", err
	}
	return RenderTemplate(t.BodyTemplate, SampleVars(s.cfg.AppURL)), nil
}

// --------------- Schedules ---------------

func (s *Service) ListSchedules(ctx context.Context) ([]model.WABroadcastSchedule, error) {
	return s.repo.ListSchedules(ctx)
}

func (s *Service) GetSchedule(ctx context.Context, id string) (model.WABroadcastSchedule, error) {
	sch, err := s.repo.GetScheduleByID(ctx, id)
	if errors.Is(err, warepo.ErrScheduleNotFound) {
		return sch, ErrScheduleNotFound
	}
	return sch, err
}

func (s *Service) CreateSchedule(ctx context.Context, params warepo.CreateScheduleParams) (model.WABroadcastSchedule, error) {
	return s.repo.CreateSchedule(ctx, params)
}

func (s *Service) UpdateSchedule(ctx context.Context, id string, params warepo.UpdateScheduleParams) (model.WABroadcastSchedule, error) {
	sch, err := s.repo.UpdateSchedule(ctx, id, params)
	if errors.Is(err, warepo.ErrScheduleNotFound) {
		return sch, ErrScheduleNotFound
	}
	return sch, err
}

func (s *Service) ToggleSchedule(ctx context.Context, id string, active bool) (model.WABroadcastSchedule, error) {
	sch, err := s.repo.ToggleSchedule(ctx, id, active)
	if errors.Is(err, warepo.ErrScheduleNotFound) {
		return sch, ErrScheduleNotFound
	}
	return sch, err
}

func (s *Service) DeleteSchedule(ctx context.Context, id string) error {
	err := s.repo.DeleteSchedule(ctx, id)
	if errors.Is(err, warepo.ErrScheduleNotFound) {
		return ErrScheduleNotFound
	}
	return err
}

func (s *Service) TriggerSchedule(ctx context.Context, id string) error {
	schedule, err := s.repo.GetScheduleByID(ctx, id)
	if errors.Is(err, warepo.ErrScheduleNotFound) {
		return ErrScheduleNotFound
	}
	if err != nil {
		return err
	}
	if !s.IsEnabled(ctx) {
		return ErrWAHADisabled
	}
	return s.runSchedule(ctx, schedule, "manual")
}

// --------------- Logs ---------------

func (s *Service) ListLogs(ctx context.Context, params warepo.ListLogsParams) ([]model.WABroadcastLog, int64, error) {
	return s.repo.ListLogs(ctx, params)
}

func (s *Service) GetLogSummary(ctx context.Context, date string) (model.WALogSummary, error) {
	summary, err := s.repo.GetLogSummary(ctx, date)
	if err != nil {
		return summary, err
	}
	client, clientErr := s.getClient(ctx)
	if clientErr == nil {
		summary.DailyLimit = client.GetDailyStats().DailyLimit
	}
	return summary, nil
}

// --------------- Quick Send ---------------

func (s *Service) QuickSend(ctx context.Context, phone string, message string) error {
	client, clientErr := s.getClient(ctx)
	if clientErr != nil {
		return clientErr
	}
	normalized, err := normalizeValidatedPhone(phone)
	if err != nil {
		return err
	}
	if limitErr := s.ensureDailyLimit(ctx, client); limitErr != nil {
		err = limitErr
	} else {
		err = client.SendMessage(normalized, message)
	}

	status := "sent"
	errMsg := (*string)(nil)
	if err != nil {
		status = "failed"
		if errors.Is(err, ErrWAHADisabled) {
			status = "skipped_disabled"
		}
		msg := err.Error()
		errMsg = &msg
	}

	triggerType := "manual_quick_send"
	_, logErr := s.repo.CreateLog(ctx, warepo.CreateLogParams{
		TriggerType:    triggerType,
		RecipientPhone: normalized,
		MessageBody:    message,
		Status:         status,
		ErrorMessage:   errMsg,
	})
	if logErr != nil {
		slog.Error("failed to log quick send", "error", logErr)
	}

	return err
}

// --------------- User Phone ---------------

func (s *Service) UpdateUserPhone(ctx context.Context, userID string, phone *string) error {
	if phone == nil || strings.TrimSpace(*phone) == "" {
		return s.repo.UpdateUserPhone(ctx, userID, nil)
	}

	normalized, err := normalizeValidatedPhone(*phone)
	if err != nil {
		return err
	}

	return s.repo.UpdateUserPhone(ctx, userID, &normalized)
}

func (s *Service) GetUserPhone(ctx context.Context, userID string) (*string, error) {
	return s.repo.GetUserPhone(ctx, userID)
}

func normalizeValidatedPhone(phone string) (string, error) {
	normalized := NormalizePhone(phone)
	if !IsValidPhone(normalized) {
		return "", ErrInvalidPhone
	}
	return normalized, nil
}

// --------------- Event Triggers ---------------

// SendTaskAssignedNotification sends a WA notification when a task is assigned.
// Should be called from kanban service via goroutine.
func (s *Service) SendTaskAssignedNotification(ctx context.Context, taskID string, assigneeID string) {
	task, err := s.repo.GetTaskWithProject(ctx, taskID)
	if err != nil || task == nil {
		slog.Error("failed to get task for WA notification", "task_id", taskID, "error", err)
		return
	}

	if task.UserPhone == nil || strings.TrimSpace(*task.UserPhone) == "" {
		s.logSkipped(ctx, "task_assigned", &task.TaskID, &assigneeID, "", "skipped_no_phone")
		return
	}

	tmpl, err := s.repo.GetTemplateBySlug(ctx, "task_assigned")
	if err != nil {
		slog.Error("failed to get task_assigned template", "error", err)
		return
	}
	if !tmpl.IsActive {
		return
	}

	vars := map[string]string{
		"name":         task.UserName,
		"task_title":   task.TaskTitle,
		"project_name": task.ProjectName,
		"due_date":     task.DueDate,
		"priority":     task.Priority,
		"app_url":      s.cfg.AppURL,
	}
	body := RenderTemplate(tmpl.BodyTemplate, vars)

	s.sendAndLog(ctx, *task.UserPhone, body, "event_triggered", &tmpl.ID, &tmpl.Slug,
		&assigneeID, stringPtr("task"), &task.TaskID)
}

// SendReimbursementStatusNotification sends a WA notification on reimbursement status change.
func (s *Service) SendReimbursementStatusNotification(ctx context.Context, reimbursementID string, newStatus string, reviewerNotes string) {
	info, err := s.repo.GetReimbursementWithSubmitter(ctx, reimbursementID)
	if err != nil || info == nil {
		slog.Error("failed to get reimbursement for WA notification", "reimbursement_id", reimbursementID, "error", err)
		return
	}

	if info.SubmitterPhone == nil || strings.TrimSpace(*info.SubmitterPhone) == "" {
		s.logSkipped(ctx, "reimbursement_status", &reimbursementID, &info.SubmitterID, "", "skipped_no_phone")
		return
	}

	tmpl, err := s.repo.GetTemplateBySlug(ctx, "reimbursement_status")
	if err != nil {
		slog.Error("failed to get reimbursement_status template", "error", err)
		return
	}
	if !tmpl.IsActive {
		return
	}

	vars := map[string]string{
		"name":                   info.SubmitterName,
		"reimbursement_title":    info.Title,
		"amount":                 formatRupiah(info.Amount),
		"new_status":             newStatus,
		"reviewer_notes_section": BuildReviewerNotesSection(reviewerNotes),
		"app_url":                s.cfg.AppURL,
	}
	body := RenderTemplate(tmpl.BodyTemplate, vars)

	s.sendAndLog(ctx, *info.SubmitterPhone, body, "event_triggered", &tmpl.ID, &tmpl.Slug,
		&info.SubmitterID, stringPtr("reimbursement"), &reimbursementID)
}

func (s *Service) sendAndLog(ctx context.Context, phone string, body string, triggerType string,
	templateID *string, templateSlug *string, userID *string, refType *string, refID *string) {
	s.sendAndLogWithSchedule(ctx, nil, phone, body, triggerType, templateID, templateSlug, userID, refType, refID)
}

func (s *Service) sendAndLogWithSchedule(ctx context.Context, scheduleID *string, phone string, body string, triggerType string,
	templateID *string, templateSlug *string, userID *string, refType *string, refID *string) {
	var err error
	client, clientErr := s.getClient(ctx)
	if clientErr != nil {
		err = clientErr
	} else {
		if limitErr := s.ensureDailyLimit(ctx, client); limitErr != nil {
			err = limitErr
		} else {
			err = client.SendMessage(phone, body)
		}
	}

	status := "sent"
	errMsg := (*string)(nil)
	if err != nil {
		status = "failed"
		if errors.Is(err, ErrWAHADisabled) {
			status = "skipped_disabled"
		}
		msg := err.Error()
		errMsg = &msg
	}

	_, logErr := s.repo.CreateLog(ctx, warepo.CreateLogParams{
		ScheduleID:      scheduleID,
		TemplateID:      templateID,
		TemplateSlug:    templateSlug,
		TriggerType:     triggerType,
		RecipientUserID: userID,
		RecipientPhone:  NormalizePhone(phone),
		MessageBody:     body,
		Status:          status,
		ErrorMessage:    errMsg,
		ReferenceType:   refType,
		ReferenceID:     refID,
	})
	if logErr != nil {
		slog.Error("failed to create broadcast log", "error", logErr)
	}
	if status == "sent" {
		s.createInAppNotification(ctx, templateSlug, userID, refType, refID)
	}
}

func (s *Service) logSkipped(ctx context.Context, slug string, refID *string, userID *string, phone string, status string) {
	s.logSkippedWithSchedule(ctx, nil, slug, refID, userID, phone, status)
}

func (s *Service) logSkippedWithSchedule(ctx context.Context, scheduleID *string, slug string, refID *string, userID *string, phone string, status string) {
	_, err := s.repo.CreateLog(ctx, warepo.CreateLogParams{
		ScheduleID:      scheduleID,
		TemplateSlug:    &slug,
		TriggerType:     "event_triggered",
		RecipientUserID: userID,
		RecipientPhone:  phone,
		MessageBody:     "",
		Status:          status,
		ReferenceID:     refID,
	})
	if err != nil {
		slog.Error("failed to log skipped message", "error", err)
	}
}

func (s *Service) RunCronJobs(ctx context.Context, now time.Time) error {
	cfg, err := s.repo.GetWAConfig(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return nil
	}

	if cronMatches(cfg.ReminderCron, now) {
		s.RunDailyReminders(ctx)
	}
	if cronMatches(cfg.WeeklyDigestCron, now) {
		s.RunWeeklyDigest(ctx)
	}

	schedules, err := s.repo.ListSchedules(ctx)
	if err != nil {
		return err
	}
	for _, schedule := range schedules {
		if !schedule.IsActive || !shouldRunScheduleNow(schedule, now) {
			continue
		}
		if err := s.runSchedule(ctx, schedule, "auto_scheduled"); err != nil {
			slog.Error("failed to execute scheduled WA broadcast", "schedule_id", schedule.ID, "error", err)
		}
	}

	return nil
}

func (s *Service) ensureDailyLimit(ctx context.Context, client *WAHAClient) error {
	stats := client.GetDailyStats()
	if stats == nil || stats.DailyLimit <= 0 {
		return nil
	}

	sentToday, err := s.repo.CountSentLogsToday(ctx)
	if err != nil {
		return fmt.Errorf("count daily wa logs: %w", err)
	}
	if sentToday >= stats.DailyLimit {
		return fmt.Errorf("daily message limit reached (%d)", stats.DailyLimit)
	}
	return nil
}

func (s *Service) createInAppNotification(ctx context.Context, templateSlug *string, userID *string, refType *string, refID *string) {
	if s.notificationsService == nil || templateSlug == nil || userID == nil {
		return
	}

	recipientUserID := strings.TrimSpace(*userID)
	if recipientUserID == "" {
		return
	}

	item, ok := notificationTemplateForWASlug(strings.TrimSpace(*templateSlug))
	if !ok {
		return
	}

	var referenceType *string
	if refType != nil {
		trimmedRefType := strings.TrimSpace(*refType)
		if trimmedRefType != "" {
			referenceType = &trimmedRefType
		}
	}

	if err := s.notificationsService.CreateMany(ctx, []notificationsrepo.CreateParams{
		{
			UserID:        recipientUserID,
			Type:          item.Type,
			Title:         item.Title,
			Message:       item.Message,
			ReferenceType: referenceType,
			ReferenceID:   refID,
		},
	}); err != nil {
		slog.Error("failed to create in-app notification for wa delivery", "template_slug", *templateSlug, "user_id", recipientUserID, "error", err)
	}
}

type waNotificationTemplate struct {
	Type    string
	Title   string
	Message string
}

func notificationTemplateForWASlug(slug string) (waNotificationTemplate, bool) {
	switch slug {
	case "task_assigned":
		return waNotificationTemplate{
			Type:    "wa.task_assigned",
			Title:   "Task baru diassign",
			Message: "Notifikasi task assignment juga sudah dikirim via WhatsApp.",
		}, true
	case "task_due_today":
		return waNotificationTemplate{
			Type:    "wa.task_due_today",
			Title:   "Reminder task hari ini",
			Message: "Reminder task yang jatuh tempo hari ini juga sudah dikirim via WhatsApp.",
		}, true
	case "task_overdue":
		return waNotificationTemplate{
			Type:    "wa.task_overdue",
			Title:   "Reminder task overdue",
			Message: "Reminder task overdue juga sudah dikirim via WhatsApp.",
		}, true
	case "project_deadline_h3", "project_deadline_warning":
		return waNotificationTemplate{
			Type:    "wa.project_deadline_h3",
			Title:   "Reminder deadline project",
			Message: "Reminder deadline project H-3 juga sudah dikirim via WhatsApp.",
		}, true
	case "weekly_digest":
		return waNotificationTemplate{
			Type:    "wa.weekly_digest",
			Title:   "Weekly digest tersedia",
			Message: "Ringkasan mingguan juga sudah dikirim via WhatsApp.",
		}, true
	default:
		return waNotificationTemplate{}, false
	}
}

func (s *Service) runSchedule(ctx context.Context, schedule model.WABroadcastSchedule, triggerType string) error {
	template, err := s.repo.GetTemplateByID(ctx, schedule.TemplateID)
	if errors.Is(err, warepo.ErrTemplateNotFound) {
		return ErrTemplateNotFound
	}
	if err != nil {
		return err
	}
	if !template.IsActive {
		return nil
	}

	recipients, err := s.resolveScheduleRecipients(ctx, schedule)
	if err != nil {
		return err
	}

	for _, recipient := range recipients {
		if recipient.Phone == nil || strings.TrimSpace(*recipient.Phone) == "" {
			s.logSkippedWithSchedule(ctx, &schedule.ID, template.Slug, nil, &recipient.UserID, "", "skipped_no_phone")
			continue
		}

		body := RenderTemplate(template.BodyTemplate, map[string]string{
			"name":    recipient.UserName,
			"app_url": s.cfg.AppURL,
		})
		s.sendAndLogWithSchedule(ctx, &schedule.ID, *recipient.Phone, body, triggerType, &template.ID, &template.Slug, &recipient.UserID, nil, nil)
	}

	now := time.Now().UTC()
	var nextRunAt *time.Time
	if schedule.CronExpression != nil {
		nextRunAt = nextCronOccurrence(*schedule.CronExpression, now)
	}
	return s.repo.UpdateScheduleRunMetadata(ctx, schedule.ID, now, nextRunAt)
}

func (s *Service) resolveScheduleRecipients(ctx context.Context, schedule model.WABroadcastSchedule) ([]warepo.BroadcastRecipient, error) {
	switch schedule.TargetType {
	case "all_employees":
		return s.repo.ListActiveRecipients(ctx)
	case "department":
		department, err := parseDepartmentTargetConfig(schedule.TargetConfig)
		if err != nil {
			return nil, err
		}
		return s.repo.ListDepartmentRecipients(ctx, department)
	case "specific_users":
		userIDs, err := parseSpecificUsersTargetConfig(schedule.TargetConfig)
		if err != nil {
			return nil, err
		}
		return s.repo.ListSpecificRecipients(ctx, userIDs)
	case "project_members":
		projectID, err := parseProjectMembersTargetConfig(schedule.TargetConfig)
		if err != nil {
			return nil, err
		}
		return s.repo.ListProjectMemberRecipients(ctx, projectID)
	default:
		return nil, fmt.Errorf("unsupported schedule target type %q", schedule.TargetType)
	}
}

func shouldRunScheduleNow(schedule model.WABroadcastSchedule, now time.Time) bool {
	if schedule.CronExpression == nil || strings.TrimSpace(*schedule.CronExpression) == "" {
		return false
	}
	if !cronMatches(*schedule.CronExpression, now) {
		return false
	}
	if schedule.LastRunAt == nil {
		return true
	}
	return !schedule.LastRunAt.In(time.Local).Truncate(time.Minute).Equal(now.In(time.Local).Truncate(time.Minute))
}

func parseDepartmentTargetConfig(raw *string) (string, error) {
	trimmed := strings.TrimSpace(valueOrEmpty(raw))
	if trimmed == "" {
		return "", fmt.Errorf("department target config is required")
	}
	if !strings.HasPrefix(trimmed, "{") {
		return trimmed, nil
	}

	var payload struct {
		Department string `json:"department"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", fmt.Errorf("invalid department target config")
	}
	if strings.TrimSpace(payload.Department) == "" {
		return "", fmt.Errorf("department target config is required")
	}
	return strings.TrimSpace(payload.Department), nil
}

func parseSpecificUsersTargetConfig(raw *string) ([]string, error) {
	trimmed := strings.TrimSpace(valueOrEmpty(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("specific_users target config is required")
	}

	var direct []string
	if err := json.Unmarshal([]byte(trimmed), &direct); err == nil {
		return direct, nil
	}

	var payload struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, fmt.Errorf("invalid specific_users target config")
	}
	if len(payload.UserIDs) == 0 {
		return nil, fmt.Errorf("specific_users target config is required")
	}
	return payload.UserIDs, nil
}

func parseProjectMembersTargetConfig(raw *string) (string, error) {
	trimmed := strings.TrimSpace(valueOrEmpty(raw))
	if trimmed == "" {
		return "", fmt.Errorf("project_members target config is required")
	}
	if !strings.HasPrefix(trimmed, "{") {
		return trimmed, nil
	}

	var payload struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", fmt.Errorf("invalid project_members target config")
	}
	if strings.TrimSpace(payload.ProjectID) == "" {
		return "", fmt.Errorf("project_members target config is required")
	}
	return strings.TrimSpace(payload.ProjectID), nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatRupiah(amount int64) string {
	s := strconv.FormatInt(amount, 10)
	// Simple thousands separator
	if len(s) <= 3 {
		return "Rp " + s
	}
	var result []byte
	for i, d := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(d))
	}
	return "Rp " + string(result)
}

func stringPtr(s string) *string {
	return &s
}

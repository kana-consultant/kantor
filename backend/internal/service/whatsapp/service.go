package whatsapp

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/model"
	warepo "github.com/kana-consultant/kantor/backend/internal/repository/whatsapp"
)

var (
	ErrTemplateNotFound = errors.New("wa template not found")
	ErrScheduleNotFound = errors.New("wa schedule not found")
	ErrSystemTemplate   = errors.New("cannot delete system template")
)

type Service struct {
	repo   *warepo.Repository
	client *WAHAClient
	cfg    config.Config
}

func NewService(repo *warepo.Repository, client *WAHAClient, cfg config.Config) *Service {
	return &Service{repo: repo, client: client, cfg: cfg}
}

// --------------- Connection ---------------

func (s *Service) GetStatus() (*SessionStatus, error) {
	return s.client.GetStatus()
}

func (s *Service) GetQR() (string, error) {
	return s.client.GetQR()
}

func (s *Service) StartSession() error {
	return s.client.StartSession()
}

func (s *Service) StopSession() error {
	return s.client.StopSession()
}

func (s *Service) GetAccountInfo() (*AccountInfo, error) {
	return s.client.GetAccountInfo()
}

func (s *Service) IsEnabled() bool {
	return s.client.IsEnabled()
}

func (s *Service) GetDailyStats() *DailyStats {
	return s.client.GetDailyStats()
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

// --------------- Logs ---------------

func (s *Service) ListLogs(ctx context.Context, params warepo.ListLogsParams) ([]model.WABroadcastLog, int64, error) {
	return s.repo.ListLogs(ctx, params)
}

func (s *Service) GetLogSummary(ctx context.Context, date string) (model.WALogSummary, error) {
	summary, err := s.repo.GetLogSummary(ctx, date)
	if err != nil {
		return summary, err
	}
	summary.DailyLimit = s.cfg.WAHA.MaxDailyMessages
	return summary, nil
}

// --------------- Quick Send ---------------

func (s *Service) QuickSend(ctx context.Context, phone string, message string) error {
	normalized := NormalizePhone(phone)
	err := s.client.SendMessage(normalized, message)

	status := "sent"
	errMsg := (*string)(nil)
	if err != nil {
		status = "failed"
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
	return s.repo.UpdateUserPhone(ctx, userID, phone)
}

func (s *Service) GetUserPhone(ctx context.Context, userID string) (*string, error) {
	return s.repo.GetUserPhone(ctx, userID)
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

	err := s.client.SendMessage(phone, body)

	status := "sent"
	errMsg := (*string)(nil)
	if err != nil {
		status = "failed"
		msg := err.Error()
		errMsg = &msg
	}

	_, logErr := s.repo.CreateLog(ctx, warepo.CreateLogParams{
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
}

func (s *Service) logSkipped(ctx context.Context, slug string, refID *string, userID *string, phone string, status string) {
	_, err := s.repo.CreateLog(ctx, warepo.CreateLogParams{
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

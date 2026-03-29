package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	warepo "github.com/kana-consultant/kantor/backend/internal/repository/whatsapp"
)

// RunDailyReminders executes UC-1 (task due today), UC-2 (task overdue), UC-4 (project deadline H-3).
func (s *Service) RunDailyReminders(ctx context.Context) {
	slog.Info("starting daily WA reminders")

	s.sendTaskDueTodayReminders(ctx)
	s.sendTaskOverdueReminders(ctx)
	s.sendProjectDeadlineReminders(ctx)

	slog.Info("daily WA reminders completed")
}

// RunWeeklyDigest executes UC-5 (weekly digest).
func (s *Service) RunWeeklyDigest(ctx context.Context) {
	slog.Info("starting weekly WA digest")

	tmpl, err := s.repo.GetTemplateBySlug(ctx, "weekly_digest")
	if err != nil {
		slog.Error("failed to get weekly_digest template", "error", err)
		return
	}
	if !tmpl.IsActive {
		slog.Info("weekly_digest template is inactive, skipping")
		return
	}

	items, err := s.repo.GetWeeklyDigestData(ctx)
	if err != nil {
		slog.Error("failed to get weekly digest data", "error", err)
		return
	}

	now := time.Now()
	weekEnd := now.AddDate(0, 0, -int(now.Weekday()))
	weekStart := weekEnd.AddDate(0, 0, -6)

	for _, item := range items {
		if item.CompletedCount == 0 && item.OpenCount == 0 && item.OverdueCount == 0 {
			continue
		}
		if item.Phone == nil || strings.TrimSpace(*item.Phone) == "" {
			s.logSkipped(ctx, "weekly_digest", nil, &item.UserID, "", "skipped_no_phone")
			continue
		}

		vars := map[string]string{
			"name":            item.UserName,
			"week_start":      weekStart.Format("2006-01-02"),
			"week_end":        weekEnd.Format("2006-01-02"),
			"completed_count": fmt.Sprintf("%d", item.CompletedCount),
			"open_count":      fmt.Sprintf("%d", item.OpenCount),
			"overdue_count":   fmt.Sprintf("%d", item.OverdueCount),
			"app_url":         s.cfg.AppURL,
		}
		body := RenderTemplate(tmpl.BodyTemplate, vars)
		s.sendAndLog(ctx, *item.Phone, body, "auto_scheduled", &tmpl.ID, &tmpl.Slug,
			&item.UserID, nil, nil)
	}

	slog.Info("weekly WA digest completed", "recipients", len(items))
}

func (s *Service) sendTaskDueTodayReminders(ctx context.Context) {
	tmpl, err := s.repo.GetTemplateBySlug(ctx, "task_due_today")
	if err != nil {
		slog.Error("failed to get task_due_today template", "error", err)
		return
	}
	if !tmpl.IsActive {
		return
	}

	tasks, err := s.repo.GetTasksDueToday(ctx)
	if err != nil {
		slog.Error("failed to get tasks due today", "error", err)
		return
	}

	for _, task := range tasks {
		s.sendTaskReminder(ctx, task, tmpl)
	}
	slog.Info("task due today reminders sent", "count", len(tasks))
}

func (s *Service) sendTaskOverdueReminders(ctx context.Context) {
	tmpl, err := s.repo.GetTemplateBySlug(ctx, "task_overdue")
	if err != nil {
		slog.Error("failed to get task_overdue template", "error", err)
		return
	}
	if !tmpl.IsActive {
		return
	}

	tasks, err := s.repo.GetTasksOverdue(ctx)
	if err != nil {
		slog.Error("failed to get overdue tasks", "error", err)
		return
	}

	for _, task := range tasks {
		s.sendTaskReminder(ctx, task, tmpl)
	}
	slog.Info("task overdue reminders sent", "count", len(tasks))
}

func (s *Service) sendTaskReminder(ctx context.Context, task warepo.TaskDueInfo, tmpl model.WAMessageTemplate) {
	if task.UserPhone == nil || strings.TrimSpace(*task.UserPhone) == "" {
		s.logSkipped(ctx, tmpl.Slug, &task.TaskID, &task.AssigneeID, "", "skipped_no_phone")
		return
	}

	// Anti-spam: check if already sent today
	dup, err := s.repo.CheckDuplicateToday(ctx, task.AssigneeID, tmpl.Slug, task.TaskID)
	if err != nil {
		slog.Error("failed to check duplicate", "error", err)
		return
	}
	if dup {
		return
	}

	vars := map[string]string{
		"name":         task.UserName,
		"task_title":   task.TaskTitle,
		"project_name": task.ProjectName,
		"due_date":     task.DueDate,
		"app_url":      s.cfg.AppURL,
	}
	body := RenderTemplate(tmpl.BodyTemplate, vars)
	s.sendAndLog(ctx, *task.UserPhone, body, "auto_scheduled", &tmpl.ID, &tmpl.Slug,
		&task.AssigneeID, stringPtr("task"), &task.TaskID)
}

func (s *Service) sendProjectDeadlineReminders(ctx context.Context) {
	tmpl, err := s.repo.GetTemplateBySlug(ctx, "project_deadline_h3")
	if err != nil {
		slog.Error("failed to get project_deadline_h3 template", "error", err)
		return
	}
	if !tmpl.IsActive {
		return
	}

	projects, err := s.repo.GetProjectsDeadlineIn3Days(ctx)
	if err != nil {
		slog.Error("failed to get projects deadline in 3 days", "error", err)
		return
	}

	sent := 0
	for _, project := range projects {
		for _, member := range project.Members {
			if member.Phone == nil || strings.TrimSpace(*member.Phone) == "" {
				s.logSkipped(ctx, tmpl.Slug, &project.ProjectID, &member.UserID, "", "skipped_no_phone")
				continue
			}

			dup, err := s.repo.CheckDuplicateToday(ctx, member.UserID, tmpl.Slug, project.ProjectID)
			if err != nil {
				slog.Error("failed to check duplicate", "error", err)
				continue
			}
			if dup {
				continue
			}

			vars := map[string]string{
				"name":              member.UserName,
				"project_name":      project.ProjectName,
				"deadline":          project.Deadline,
				"project_status":    project.Status,
				"open_tasks_count":  fmt.Sprintf("%d", project.OpenTaskCount),
				"total_tasks_count": fmt.Sprintf("%d", project.TotalTaskCount),
				"app_url":           s.cfg.AppURL,
			}
			body := RenderTemplate(tmpl.BodyTemplate, vars)
			s.sendAndLog(ctx, *member.Phone, body, "auto_scheduled", &tmpl.ID, &tmpl.Slug,
				&member.UserID, stringPtr("project"), &project.ProjectID)
			sent++
		}
	}
	slog.Info("project deadline reminders sent", "count", sent)
}

package operational

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

const (
	discordReminderStatusOpen    = "open"
	discordReminderStatusOverdue = "overdue"
	discordReminderDateLayout    = "2006-01-02"
)

type discordReminderRepo interface {
	EnsureRow(ctx context.Context) error
	Get(ctx context.Context) (model.DiscordReminderConfig, error)
	Update(ctx context.Context, p operationalrepo.UpdateDiscordReminderConfigParams) (model.DiscordReminderConfig, error)
	ListOpenTasksByEmployee(ctx context.Context) ([]model.DiscordReminderTaskRow, error)
}

// DiscordReminderService builds and pushes the daily all-employees open-task
// digest to an external Discord bot, per tenant.
type DiscordReminderService struct {
	repo discordReminderRepo

	// In-memory guard against double-fire on process restart (a restart at
	// the top of the fire hour would otherwise re-trigger the digest). Keyed
	// by tenant ID, value is the tenant-local calendar date already fired.
	mu           sync.Mutex
	lastFireDate map[string]string
}

func NewDiscordReminderService(repo discordReminderRepo) *DiscordReminderService {
	return &DiscordReminderService{
		repo:         repo,
		lastFireDate: make(map[string]string),
	}
}

func (s *DiscordReminderService) GetConfig(ctx context.Context) (model.DiscordReminderConfig, error) {
	if err := s.repo.EnsureRow(ctx); err != nil {
		return model.DiscordReminderConfig{}, err
	}
	return s.repo.Get(ctx)
}

func (s *DiscordReminderService) UpdateConfig(ctx context.Context, p operationalrepo.UpdateDiscordReminderConfigParams) (model.DiscordReminderConfig, error) {
	if err := s.repo.EnsureRow(ctx); err != nil {
		return model.DiscordReminderConfig{}, err
	}
	return s.repo.Update(ctx, p)
}

// RunReminderJobs is invoked once per minute per tenant by the scheduler. It
// only pushes the digest at the top of the configured send hour (tenant
// timezone), and at most once per tenant-local calendar day.
func (s *DiscordReminderService) RunReminderJobs(ctx context.Context, now time.Time) error {
	if err := s.repo.EnsureRow(ctx); err != nil {
		return err
	}
	cfg, err := s.repo.Get(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return nil
	}

	nowLocal := now.In(s.loadLocation(ctx, cfg.Timezone))
	if !s.shouldFire(ctx, cfg, nowLocal) {
		return nil
	}

	count, err := s.pushDigest(ctx, cfg, nowLocal)
	if err != nil {
		slog.ErrorContext(ctx, "discord reminder digest push failed", "error", err)
		return nil
	}
	slog.InfoContext(ctx, "discord reminder digest pushed", "employees", count)
	return nil
}

// SendTestDigest builds and pushes the digest immediately, ignoring the fire
// schedule and the once-per-day guard. Used by the manual "test" endpoint.
func (s *DiscordReminderService) SendTestDigest(ctx context.Context, now time.Time) (int, error) {
	if err := s.repo.EnsureRow(ctx); err != nil {
		return 0, err
	}
	cfg, err := s.repo.Get(ctx)
	if err != nil {
		return 0, err
	}
	if !cfg.Enabled {
		return 0, ErrDiscordReminderDisabled
	}

	nowLocal := now.In(s.loadLocation(ctx, cfg.Timezone))
	return s.pushDigest(ctx, cfg, nowLocal)
}

func (s *DiscordReminderService) loadLocation(ctx context.Context, timezone string) *time.Location {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		slog.WarnContext(ctx, "discord reminder falling back to UTC", "tenant_timezone", timezone, "error", err)
		return time.UTC
	}
	return loc
}

func (s *DiscordReminderService) shouldFire(ctx context.Context, cfg model.DiscordReminderConfig, nowLocal time.Time) bool {
	if nowLocal.Minute() != 0 || nowLocal.Hour() != cfg.SendHour {
		return false
	}
	if cfg.WeekdaysOnly {
		wd := nowLocal.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			return false
		}
	}

	tenantID := tenantIDOrFallback(ctx, cfg.TenantID)
	dateKey := nowLocal.Format(discordReminderDateLayout)

	s.mu.Lock()
	defer s.mu.Unlock()
	if lastDate, exists := s.lastFireDate[tenantID]; exists && lastDate == dateKey {
		return false
	}
	s.lastFireDate[tenantID] = dateKey
	return true
}

func (s *DiscordReminderService) pushDigest(ctx context.Context, cfg model.DiscordReminderConfig, nowLocal time.Time) (int, error) {
	rows, err := s.repo.ListOpenTasksByEmployee(ctx)
	if err != nil {
		return 0, err
	}

	payload := buildDiscordReminderPayload(rows, nowLocal.Format(discordReminderDateLayout))
	if len(payload.Employees) == 0 {
		return 0, nil
	}

	client := NewDiscordReminderClientFromConfig(cfg.WebhookURL, cfg.SharedSecret, cfg.Enabled)
	if err := client.Push(payload); err != nil {
		return 0, err
	}
	return len(payload.Employees), nil
}

// buildDiscordReminderPayload groups the flat task rows per employee and
// computes each task's open/overdue status relative to todayLocal. Only
// employees with at least one open task are included.
func buildDiscordReminderPayload(rows []model.DiscordReminderTaskRow, todayLocal string) DiscordReminderDigestPayload {
	order := make([]string, 0, len(rows))
	grouped := make(map[string]*DiscordReminderEmployeePayload, len(rows))

	for _, row := range rows {
		emp, ok := grouped[row.UserID]
		if !ok {
			emp = &DiscordReminderEmployeePayload{
				KantorUserID: row.UserID,
				Name:         row.FullName,
				Tasks:        make([]DiscordReminderTaskPayload, 0, 4),
			}
			grouped[row.UserID] = emp
			order = append(order, row.UserID)
		}

		status := discordReminderStatusOpen
		if row.DueDate != "" && strings.Compare(row.DueDate, todayLocal) < 0 {
			status = discordReminderStatusOverdue
		}

		emp.Tasks = append(emp.Tasks, DiscordReminderTaskPayload{
			Title:    row.Title,
			Project:  row.ProjectName,
			Column:   row.ColumnName,
			Status:   status,
			DueDate:  row.DueDate,
			Priority: row.Priority,
		})
	}

	employees := make([]DiscordReminderEmployeePayload, 0, len(order))
	for _, userID := range order {
		emp := grouped[userID]
		if len(emp.Tasks) == 0 {
			continue
		}
		employees = append(employees, *emp)
	}

	return DiscordReminderDigestPayload{
		Job:       "task",
		Date:      todayLocal,
		Employees: employees,
	}
}

func tenantIDOrFallback(ctx context.Context, fallback string) string {
	if info, ok := tenant.FromContext(ctx); ok && strings.TrimSpace(info.ID) != "" {
		return info.ID
	}
	return fallback
}

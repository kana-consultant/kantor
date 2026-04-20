package operational

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

// ErrTrackerReminderConfigInvalid indicates the submitted reminder configuration is malformed.
var ErrTrackerReminderConfigInvalid = errors.New("invalid tracker reminder config")

const (
	trackerReminderType        = "tracker_reminder"
	trackerReminderNotifTitle  = "Activity tracker belum nyala"
	trackerReminderNotifBody   = "Aktifkan activity tracker agar aktivitas kerja Anda tercatat."
	trackerReminderTestTitle   = "(Uji coba) Activity tracker belum nyala"
	trackerReminderTestBody    = "Ini pesan uji coba dari pengaturan tracker reminder. Silakan buka dashboard dan aktifkan tracker Anda."
	trackerReminderDedupWindow = 50 * time.Minute
)

type trackerReminderRepo interface {
	EnsureRow(ctx context.Context) error
	Get(ctx context.Context) (model.TrackerReminderConfig, error)
	Update(ctx context.Context, p operationalrepo.UpdateTrackerReminderConfigParams) (model.TrackerReminderConfig, error)
	ListCandidates(ctx context.Context, staleCutoff time.Time) ([]model.TrackerReminderCandidate, error)
	HasRecentReminder(ctx context.Context, userID string, since time.Time) (bool, error)
	GetUserPhone(ctx context.Context, userID string) (*string, error)
}

type trackerReminderNotifications interface {
	CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error
}

type trackerReminderWASender interface {
	QuickSend(ctx context.Context, phone string, message string) error
}

type TrackerReminderService struct {
	repo   trackerReminderRepo
	notifs trackerReminderNotifications
	wa     trackerReminderWASender
}

func NewTrackerReminderService(repo trackerReminderRepo, notifs trackerReminderNotifications, wa trackerReminderWASender) *TrackerReminderService {
	return &TrackerReminderService{
		repo:   repo,
		notifs: notifs,
		wa:     wa,
	}
}

func (s *TrackerReminderService) GetConfig(ctx context.Context) (model.TrackerReminderConfig, error) {
	if err := s.repo.EnsureRow(ctx); err != nil {
		return model.TrackerReminderConfig{}, err
	}
	return s.repo.Get(ctx)
}

func (s *TrackerReminderService) UpdateConfig(ctx context.Context, p operationalrepo.UpdateTrackerReminderConfigParams) (model.TrackerReminderConfig, error) {
	p.Timezone = strings.TrimSpace(p.Timezone)
	if err := validateReminderConfig(p); err != nil {
		return model.TrackerReminderConfig{}, err
	}
	if err := s.repo.EnsureRow(ctx); err != nil {
		return model.TrackerReminderConfig{}, err
	}
	return s.repo.Update(ctx, p)
}

func validateReminderConfig(p operationalrepo.UpdateTrackerReminderConfigParams) error {
	if p.StartHour < 0 || p.StartHour > 23 {
		return fmt.Errorf("%w: start_hour harus antara 0-23", ErrTrackerReminderConfigInvalid)
	}
	if p.EndHour < 1 || p.EndHour > 24 {
		return fmt.Errorf("%w: end_hour harus antara 1-24", ErrTrackerReminderConfigInvalid)
	}
	if p.EndHour <= p.StartHour {
		return fmt.Errorf("%w: end_hour harus lebih besar dari start_hour", ErrTrackerReminderConfigInvalid)
	}
	if p.HeartbeatStaleMinutes < 1 || p.HeartbeatStaleMinutes > 1440 {
		return fmt.Errorf("%w: heartbeat_stale_minutes harus 1-1440", ErrTrackerReminderConfigInvalid)
	}
	if p.Timezone == "" {
		return fmt.Errorf("%w: timezone wajib diisi", ErrTrackerReminderConfigInvalid)
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("%w: timezone tidak dikenal", ErrTrackerReminderConfigInvalid)
	}
	return nil
}

// NextReminderAt returns the next top-of-hour instant (UTC) when a reminder
// would fire under the given config, starting strictly after "now". Returns
// nil if reminders are disabled.
func (s *TrackerReminderService) NextReminderAt(cfg model.TrackerReminderConfig, now time.Time) *time.Time {
	if !cfg.Enabled {
		return nil
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil
	}

	nowLocal := now.In(loc)
	// Start scanning at the next whole hour in local time.
	candidate := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), nowLocal.Hour()+1, 0, 0, 0, loc)
	// Scan up to 10 days ahead to cover long weekends.
	for i := 0; i < 24*10; i++ {
		if isWithinReminderWindow(candidate, cfg) {
			utc := candidate.UTC()
			return &utc
		}
		candidate = candidate.Add(time.Hour)
	}
	return nil
}

func isWithinReminderWindow(localTime time.Time, cfg model.TrackerReminderConfig) bool {
	hour := localTime.Hour()
	if hour < cfg.StartHour || hour >= cfg.EndHour {
		return false
	}
	if cfg.WeekdaysOnly {
		wd := localTime.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			return false
		}
	}
	return true
}

// SendTestReminder delivers a test reminder to the caller over every enabled channel.
// deliveredInApp indicates whether the in-app notification was created.
// deliveredWhatsapp indicates whether the WA send succeeded (false if channel disabled,
// no phone, or provider error). waErr is populated only when WA was attempted but failed.
func (s *TrackerReminderService) SendTestReminder(ctx context.Context, userID string) (deliveredInApp bool, deliveredWhatsapp bool, waErr error, err error) {
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		return false, false, nil, err
	}

	if cfg.NotifyInApp {
		if cerr := s.notifs.CreateMany(ctx, []notificationsrepo.CreateParams{{
			UserID:  userID,
			Type:    trackerReminderType,
			Title:   trackerReminderTestTitle,
			Message: trackerReminderTestBody,
		}}); cerr != nil {
			return false, false, nil, cerr
		}
		deliveredInApp = true
	}

	if cfg.NotifyWhatsapp {
		phone, perr := s.repo.GetUserPhone(ctx, userID)
		if perr != nil {
			return deliveredInApp, false, nil, perr
		}
		if phone != nil && strings.TrimSpace(*phone) != "" {
			if werr := s.wa.QuickSend(ctx, *phone, testWAMessage()); werr != nil {
				waErr = werr
			} else {
				deliveredWhatsapp = true
			}
		} else {
			msg := "nomor telepon belum diatur pada profil user"
			waErr = errors.New(msg)
		}
	}

	return deliveredInApp, deliveredWhatsapp, waErr, nil
}

// RunReminderJobs is invoked once per minute per tenant by the scheduler.
// It only dispatches at the top of the hour within configured working hours.
func (s *TrackerReminderService) RunReminderJobs(ctx context.Context, now time.Time) error {
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

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		slog.Warn("tracker reminder falling back to UTC", "tenant_timezone", cfg.Timezone, "error", err)
		loc = time.UTC
	}
	nowLocal := now.In(loc)
	if nowLocal.Minute() != 0 {
		return nil
	}
	if !isWithinReminderWindow(nowLocal, cfg) {
		return nil
	}

	staleCutoff := now.Add(-time.Duration(cfg.HeartbeatStaleMinutes) * time.Minute)
	candidates, err := s.repo.ListCandidates(ctx, staleCutoff)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}

	dedupSince := now.Add(-trackerReminderDedupWindow)

	var pendingInApp []notificationsrepo.CreateParams
	var waTargets []model.TrackerReminderCandidate

	for _, c := range candidates {
		exists, err := s.repo.HasRecentReminder(ctx, c.UserID, dedupSince)
		if err != nil {
			slog.Error("tracker reminder dedup check failed", "user_id", c.UserID, "error", err)
			continue
		}
		if exists {
			continue
		}
		if cfg.NotifyInApp {
			pendingInApp = append(pendingInApp, notificationsrepo.CreateParams{
				UserID:  c.UserID,
				Type:    trackerReminderType,
				Title:   trackerReminderNotifTitle,
				Message: trackerReminderNotifBody,
			})
		}
		if cfg.NotifyWhatsapp && c.Phone != nil && strings.TrimSpace(*c.Phone) != "" {
			waTargets = append(waTargets, c)
		}
	}

	if len(pendingInApp) > 0 {
		if err := s.notifs.CreateMany(ctx, pendingInApp); err != nil {
			slog.Error("tracker reminder in-app dispatch failed", "error", err, "count", len(pendingInApp))
		}
	}
	for _, c := range waTargets {
		if err := s.wa.QuickSend(ctx, *c.Phone, trackerReminderWAMessage(c.FullName)); err != nil {
			slog.Warn("tracker reminder WA dispatch failed", "user_id", c.UserID, "error", err)
		}
	}
	return nil
}

func testWAMessage() string {
	return "[KANTOR][UJI COBA] Activity tracker Anda belum aktif. Silakan buka dashboard KANTOR dan aktifkan tracker."
}

func trackerReminderWAMessage(fullName string) string {
	name := strings.TrimSpace(fullName)
	if name == "" {
		name = "Tim"
	}
	return fmt.Sprintf(
		"[KANTOR] Halo %s, activity tracker Anda belum aktif. Silakan buka dashboard KANTOR dan nyalakan tracker sebelum mulai bekerja.",
		name,
	)
}

package operational

import (
	"context"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

type fakeDiscordReminderRepo struct {
	cfg  model.DiscordReminderConfig
	rows []model.DiscordReminderTaskRow
}

func (f *fakeDiscordReminderRepo) EnsureRow(ctx context.Context) error { return nil }

func (f *fakeDiscordReminderRepo) Get(ctx context.Context) (model.DiscordReminderConfig, error) {
	return f.cfg, nil
}

func (f *fakeDiscordReminderRepo) Update(ctx context.Context, p operationalrepo.UpdateDiscordReminderConfigParams) (model.DiscordReminderConfig, error) {
	f.cfg.Enabled = p.Enabled
	f.cfg.WebhookURL = p.WebhookURL
	f.cfg.SharedSecret = p.SharedSecret
	f.cfg.SendHour = p.SendHour
	f.cfg.WeekdaysOnly = p.WeekdaysOnly
	f.cfg.Timezone = p.Timezone
	return f.cfg, nil
}

func (f *fakeDiscordReminderRepo) ListOpenTasksByEmployee(ctx context.Context) ([]model.DiscordReminderTaskRow, error) {
	return f.rows, nil
}

func TestBuildDiscordReminderPayloadGroupsAndComputesStatus(t *testing.T) {
	t.Parallel()

	rows := []model.DiscordReminderTaskRow{
		{UserID: "u1", FullName: "Alice", TaskID: "t1", Title: "Fix bug", ProjectName: "Proj A", ColumnName: "To Do", DueDate: "2026-07-01", Priority: "high"},
		{UserID: "u1", FullName: "Alice", TaskID: "t2", Title: "Write docs", ProjectName: "Proj A", ColumnName: "In Progress", DueDate: "", Priority: "low"},
		{UserID: "u2", FullName: "Bob", TaskID: "t3", Title: "Deploy", ProjectName: "Proj B", ColumnName: "To Do", DueDate: "2026-07-31", Priority: "critical"},
	}

	payload := buildDiscordReminderPayload(rows, "2026-07-25")

	if payload.Job != "task" {
		t.Fatalf("Job = %q, want %q", payload.Job, "task")
	}
	if payload.Date != "2026-07-25" {
		t.Fatalf("Date = %q, want %q", payload.Date, "2026-07-25")
	}
	if len(payload.Employees) != 2 {
		t.Fatalf("len(Employees) = %d, want 2", len(payload.Employees))
	}

	alice := payload.Employees[0]
	if alice.KantorUserID != "u1" || alice.Name != "Alice" || len(alice.Tasks) != 2 {
		t.Fatalf("unexpected alice payload: %+v", alice)
	}
	if alice.Tasks[0].Status != discordReminderStatusOverdue {
		t.Fatalf("task 1 status = %q, want overdue", alice.Tasks[0].Status)
	}
	if alice.Tasks[1].Status != discordReminderStatusOpen {
		t.Fatalf("task 2 status = %q, want open (no due date)", alice.Tasks[1].Status)
	}

	bob := payload.Employees[1]
	if bob.KantorUserID != "u2" || len(bob.Tasks) != 1 {
		t.Fatalf("unexpected bob payload: %+v", bob)
	}
	if bob.Tasks[0].Status != discordReminderStatusOpen {
		t.Fatalf("bob task status = %q, want open (due date in future)", bob.Tasks[0].Status)
	}
}

func TestBuildDiscordReminderPayloadEmptyRows(t *testing.T) {
	t.Parallel()

	payload := buildDiscordReminderPayload(nil, "2026-07-25")
	if len(payload.Employees) != 0 {
		t.Fatalf("len(Employees) = %d, want 0", len(payload.Employees))
	}
}

func TestRunReminderJobsSkipsWhenDisabled(t *testing.T) {
	t.Parallel()

	repo := &fakeDiscordReminderRepo{cfg: model.DiscordReminderConfig{
		TenantID: "tenant-1",
		Enabled:  false,
	}}
	service := NewDiscordReminderService(repo)

	if err := service.RunReminderJobs(context.Background(), time.Now()); err != nil {
		t.Fatalf("RunReminderJobs() error = %v", err)
	}
}

func TestShouldFireOncePerTenantLocalDay(t *testing.T) {
	t.Parallel()

	cfg := model.DiscordReminderConfig{
		TenantID:     "tenant-1",
		Enabled:      true,
		SendHour:     8,
		WeekdaysOnly: true,
		Timezone:     "UTC",
	}
	service := NewDiscordReminderService(&fakeDiscordReminderRepo{cfg: cfg})

	// Monday 08:00 — within window, should fire.
	monday8am := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	if !service.shouldFire(context.Background(), cfg, monday8am) {
		t.Fatalf("shouldFire() = false, want true for first fire of the day")
	}

	// Same tenant-local day, later tick — must not re-fire (restart guard).
	if service.shouldFire(context.Background(), cfg, monday8am.Add(time.Minute)) {
		t.Fatalf("shouldFire() = true, want false for a second tick same day")
	}

	// Off the top of the hour — never fires regardless of dedup state.
	notTopOfHour := time.Date(2026, 7, 28, 8, 5, 0, 0, time.UTC)
	if service.shouldFire(context.Background(), cfg, notTopOfHour) {
		t.Fatalf("shouldFire() = true, want false when minute != 0")
	}

	// Weekend — weekdays_only should block firing even at the right hour.
	saturday8am := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	if service.shouldFire(context.Background(), cfg, saturday8am) {
		t.Fatalf("shouldFire() = true, want false on a weekend with weekdays_only")
	}
}

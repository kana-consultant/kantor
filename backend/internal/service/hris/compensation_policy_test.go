package hris

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

type compensationPolicyRepoStub struct {
	policy       model.CompensationPolicy
	monthly      []hrisrepo.EmployeeMonthlyHoursRow
	daily        []hrisrepo.EmployeeDailyHoursRow
	baseSalaries []hrisrepo.EmployeeBaseSalaryRow

	gotMonthlyFrom, gotMonthlyTo time.Time
	gotBaseSalaryAsOf            time.Time
	dailyCalls                   int
}

func (s *compensationPolicyRepoStub) EnsureRow(context.Context) error { return nil }

func (s *compensationPolicyRepoStub) Get(context.Context) (model.CompensationPolicy, error) {
	return s.policy, nil
}

func (s *compensationPolicyRepoStub) Update(context.Context, hrisrepo.UpdateCompensationPolicyParams) (model.CompensationPolicy, error) {
	return s.policy, nil
}

func (s *compensationPolicyRepoStub) ListMonthlyActiveSeconds(_ context.Context, from time.Time, to time.Time, _ *string) ([]hrisrepo.EmployeeMonthlyHoursRow, error) {
	s.gotMonthlyFrom, s.gotMonthlyTo = from, to
	// Mirror the repository filter: employees who join after the period end
	// are not part of the period.
	out := make([]hrisrepo.EmployeeMonthlyHoursRow, 0, len(s.monthly))
	for _, row := range s.monthly {
		if !row.DateJoined.After(to) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *compensationPolicyRepoStub) ListDailyActiveSeconds(context.Context, time.Time, time.Time, *string) ([]hrisrepo.EmployeeDailyHoursRow, error) {
	s.dailyCalls++
	return s.daily, nil
}

func (s *compensationPolicyRepoStub) ListCurrentBaseSalaries(_ context.Context, asOf time.Time, _ *string) ([]hrisrepo.EmployeeBaseSalaryRow, error) {
	s.gotBaseSalaryAsOf = asOf
	return s.baseSalaries, nil
}

const safetyHour = int64(3600)

func safetyTestPolicy() model.CompensationPolicy {
	return model.CompensationPolicy{
		TenantID:          "tenant",
		MonthlyBaseSalary: 30000000,
		MinHoursPerDay:    4,
		MinHoursPerMonth:  160,
		Timezone:          "Asia/Jakarta",
	}
}

func safetyJakartaClock(t *testing.T, year int, month time.Month, day int, hourOfDay int) func() time.Time {
	t.Helper()
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	fixed := time.Date(year, month, day, hourOfDay, 0, 0, 0, location)
	return func() time.Time { return fixed }
}

func ymd(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func safetyStrPtr(value string) *string { return &value }

func safetyEmployeeRow(id string, position string, joined time.Time, activeSeconds int64, sessions int64, linked bool) hrisrepo.EmployeeMonthlyHoursRow {
	row := hrisrepo.EmployeeMonthlyHoursRow{
		EmployeeID:    id,
		FullName:      id,
		Position:      position,
		DateJoined:    joined,
		ActiveSeconds: activeSeconds,
		SessionCount:  sessions,
	}
	if linked {
		row.UserID = safetyStrPtr("user-" + id)
	}
	return row
}

// safetyFullDays returns rows giving the employee `seconds` on every weekday from
// start through end (inclusive).
func safetyFullDays(employeeID string, start time.Time, end time.Time, seconds int64) []hrisrepo.EmployeeDailyHoursRow {
	var rows []hrisrepo.EmployeeDailyHoursRow
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if isWorkday(day) {
			rows = append(rows, hrisrepo.EmployeeDailyHoursRow{EmployeeID: employeeID, Date: day, ActiveSeconds: seconds})
		}
	}
	return rows
}

func safetySumSeconds(rows []hrisrepo.EmployeeDailyHoursRow) int64 {
	var total int64
	for _, row := range rows {
		total += row.ActiveSeconds
	}
	return total
}

func evaluateSafety(t *testing.T, repo *compensationPolicyRepoStub, clock func() time.Time, year int, month int) map[string]model.SalarySafetyEvaluation {
	t.Helper()
	if repo.policy.Timezone == "" {
		repo.policy = safetyTestPolicy()
	}
	service := NewCompensationPolicyService(repo, nil)
	service.now = clock
	evaluations, err := service.EvaluateSalarySafety(context.Background(), year, month, nil)
	if err != nil {
		t.Fatalf("EvaluateSalarySafety returned error: %v", err)
	}
	out := make(map[string]model.SalarySafetyEvaluation, len(evaluations))
	for _, evaluation := range evaluations {
		out[evaluation.EmployeeID] = evaluation
	}
	return out
}

func assertSafetyStatus(t *testing.T, evaluation model.SalarySafetyEvaluation, want model.SalarySafetyStatus) {
	t.Helper()
	if evaluation.Status != want {
		t.Fatalf("%s: status = %q, want %q (actual %.2f h, expected %.2f h)", evaluation.EmployeeID, evaluation.Status, want, evaluation.MonthlyActiveHours, evaluation.ExpectedHoursToDate)
	}
}

func assertSafetyReason(t *testing.T, evaluation model.SalarySafetyEvaluation, want model.SalarySafetyReason) {
	t.Helper()
	if want == "" {
		if evaluation.Reason != nil {
			t.Fatalf("%s: reason = %q, want nil", evaluation.EmployeeID, *evaluation.Reason)
		}
		return
	}
	if evaluation.Reason == nil || *evaluation.Reason != want {
		t.Fatalf("%s: reason = %v, want %q", evaluation.EmployeeID, evaluation.Reason, want)
	}
}

func assertSafetyHours(t *testing.T, label string, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.005 {
		t.Fatalf("%s = %.4f, want %.4f", label, got, want)
	}
}

func TestCountWorkdaysSeptember2026(t *testing.T) {
	t.Parallel()

	if got := countWorkdays(ymd(2026, 9, 1), ymd(2026, 9, 30)); got != 22 {
		t.Fatalf("countWorkdays(Sep 2026) = %d, want 22", got)
	}
	if got := countWorkdays(ymd(2026, 8, 1), ymd(2026, 8, 31)); got != 21 {
		t.Fatalf("countWorkdays(Aug 2026) = %d, want 21", got)
	}
	if got := countWorkdays(ymd(2026, 9, 5), ymd(2026, 9, 6)); got != 0 {
		t.Fatalf("countWorkdays(weekend) = %d, want 0", got)
	}
	if got := countWorkdays(ymd(2026, 9, 10), ymd(2026, 9, 9)); got != 0 {
		t.Fatalf("countWorkdays(end before start) = %d, want 0", got)
	}
	if !isWorkday(ymd(2026, 9, 1)) || isWorkday(ymd(2026, 9, 5)) || isWorkday(ymd(2026, 9, 6)) {
		t.Fatalf("isWorkday: Tue 1 Sep must be a workday, Sat 5 / Sun 6 Sep must not")
	}
}

func TestSalarySafetyAboveTargetWithWeekendShortDay(t *testing.T) {
	t.Parallel()

	// 21 weekdays x 8 h = 168 h through 29 Sep, plus a 1.5 h Saturday check.
	daily := safetyFullDays("budi", ymd(2026, 9, 1), ymd(2026, 9, 29), 8*safetyHour)
	daily = append(daily, hrisrepo.EmployeeDailyHoursRow{EmployeeID: "budi", Date: ymd(2026, 9, 5), ActiveSeconds: 5400})
	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("budi", "Full Time", ymd(2024, 3, 4), safetySumSeconds(daily), 30, true),
		},
		daily: daily,
	}

	budi := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)["budi"]
	assertSafetyStatus(t, budi, model.SalarySafetyStatusSafe)
	assertSafetyReason(t, budi, "")
	assertSafetyHours(t, "monthly_active_hours", budi.MonthlyActiveHours, 169.5)
	if budi.ShortDays != 0 || len(budi.DailyViolations) != 0 {
		t.Fatalf("a weekend short day must not be a warning: short_days=%d violations=%v", budi.ShortDays, budi.DailyViolations)
	}
	if budi.AbsentDays != 0 {
		t.Fatalf("absent_days = %d, want 0", budi.AbsentDays)
	}
	if !budi.HasTrackerData || budi.PositionUnset {
		t.Fatalf("has_tracker_data=%v position_unset=%v, want true/false", budi.HasTrackerData, budi.PositionUnset)
	}
}

func TestSalarySafetyExcludesToday(t *testing.T) {
	t.Parallel()

	// 29 Sep 20:00 UTC is already 30 Sep 03:00 in Jakarta: "today" is 30 Sep.
	clock := func() time.Time { return time.Date(2026, 9, 29, 20, 0, 0, 0, time.UTC) }

	daily := safetyFullDays("andi", ymd(2026, 9, 1), ymd(2026, 9, 29), 8*safetyHour)
	daily = append(daily, hrisrepo.EmployeeDailyHoursRow{EmployeeID: "andi", Date: ymd(2026, 9, 30), ActiveSeconds: 4320}) // 1.2 h so far today
	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("andi", "Full Time", ymd(2023, 1, 9), safetySumSeconds(daily), 22, true),
		},
		daily: daily,
	}

	andi := evaluateSafety(t, repo, clock, 2026, 9)["andi"]
	assertSafetyStatus(t, andi, model.SalarySafetyStatusSafe)
	if andi.ShortDays != 0 {
		t.Fatalf("today's unfinished day must not be a short day, got short_days=%d", andi.ShortDays)
	}
	if andi.EvaluatedThrough == nil || !andi.EvaluatedThrough.Equal(ymd(2026, 9, 29)) {
		t.Fatalf("evaluated_through = %v, want 2026-09-29", andi.EvaluatedThrough)
	}
	if andi.WorkingDaysElapsed != 21 || andi.WorkingDaysTotal != 22 {
		t.Fatalf("working days = %d/%d, want 21/22", andi.WorkingDaysElapsed, andi.WorkingDaysTotal)
	}
	assertSafetyHours(t, "expected_hours_to_date", andi.ExpectedHoursToDate, 152.73)
	assertSafetyHours(t, "target_hours_month", andi.TargetHoursMonth, 160)
	// Today's hours still count towards the actual total.
	assertSafetyHours(t, "monthly_active_hours", andi.MonthlyActiveHours, 169.2)
}

func TestSalarySafetyMidMonthSafeAndAtRisk(t *testing.T) {
	t.Parallel()

	// Evaluated on 10 Sep: cutoff 9 Sep, 7 weekdays elapsed -> 7/22 x 160 = 50.91 h.
	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("dewi", "Full Time", ymd(2025, 1, 6), 58*safetyHour, 9, true),
			safetyEmployeeRow("ariel", "Full Time", ymd(2025, 1, 6), 40*safetyHour, 7, true),
		},
		daily: append(
			safetyFullDays("dewi", ymd(2026, 9, 1), ymd(2026, 9, 9), 58*safetyHour/7),
			safetyFullDays("ariel", ymd(2026, 9, 1), ymd(2026, 9, 9), 40*safetyHour/7)...,
		),
	}

	result := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 10, 9), 2026, 9)
	dewi, ariel := result["dewi"], result["ariel"]
	assertSafetyStatus(t, dewi, model.SalarySafetyStatusSafe)
	assertSafetyStatus(t, ariel, model.SalarySafetyStatusAtRisk)
	assertSafetyReason(t, ariel, "")
	assertSafetyHours(t, "expected_hours_to_date", dewi.ExpectedHoursToDate, 50.91)
	if dewi.WorkingDaysElapsed != 7 {
		t.Fatalf("working_days_elapsed = %d, want 7", dewi.WorkingDaysElapsed)
	}
	if dewi.EvaluatedThrough == nil || !dewi.EvaluatedThrough.Equal(ymd(2026, 9, 9)) {
		t.Fatalf("evaluated_through = %v, want 2026-09-09", dewi.EvaluatedThrough)
	}
}

func TestSalarySafetyNewJoiner(t *testing.T) {
	t.Parallel()

	// Joined Wed 16 Sep: window 16..29 Sep = 10 weekdays -> 10/22 x 160 = 72.73 h.
	daily := safetyFullDays("rahmat", ymd(2026, 9, 16), ymd(2026, 9, 29), 8*safetyHour)
	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("rahmat", "Full Time", ymd(2026, 9, 16), 88*safetyHour, 11, true),
			safetyEmployeeRow("late", "Full Time", ymd(2026, 9, 16), 70*safetyHour, 10, true),
		},
		daily: daily,
	}

	result := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)
	rahmat := result["rahmat"]
	assertSafetyStatus(t, rahmat, model.SalarySafetyStatusSafe)
	assertSafetyStatus(t, result["late"], model.SalarySafetyStatusAtRisk)
	if rahmat.WorkingDaysElapsed != 10 {
		t.Fatalf("working_days_elapsed = %d, want 10", rahmat.WorkingDaysElapsed)
	}
	assertSafetyHours(t, "expected_hours_to_date", rahmat.ExpectedHoursToDate, 72.73)
	// Days before joining are not absences.
	if rahmat.AbsentDays != 0 {
		t.Fatalf("absent_days = %d, want 0 (days before date_joined are outside the window)", rahmat.AbsentDays)
	}
}

func TestSalarySafetyJoinerInPastMonth(t *testing.T) {
	t.Parallel()

	// Joined Mon 21 Sep, evaluated after the month closed: 8/22 x 160 =
	// 58.1818 h, reported rounded up to 58.19 h.
	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("new", "Full Time", ymd(2026, 9, 21), 58*safetyHour, 8, true),
		},
	}

	joiner := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 10, 5, 10), 2026, 9)["new"]
	if joiner.WorkingDaysElapsed != 8 {
		t.Fatalf("working_days_elapsed = %d, want 8", joiner.WorkingDaysElapsed)
	}
	assertSafetyHours(t, "expected_hours_to_date", joiner.ExpectedHoursToDate, 58.19)
	assertSafetyStatus(t, joiner, model.SalarySafetyStatusAtRisk)
}

func TestSalarySafetyNonFullTimeExempt(t *testing.T) {
	t.Parallel()

	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("eka", "Internship", ymd(2026, 7, 1), 0, 0, true),
			safetyEmployeeRow("pt", "part time", ymd(2025, 1, 1), 20*safetyHour, 5, true),
			safetyEmployeeRow("pb", "Project Based", ymd(2025, 1, 1), 0, 0, true),
			safetyEmployeeRow("gita", " Outsourcing ", ymd(2025, 11, 3), 0, 0, false),
		},
	}

	result := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)
	for _, id := range []string{"eka", "pt", "pb", "gita"} {
		evaluation := result[id]
		assertSafetyStatus(t, evaluation, model.SalarySafetyStatusNoData)
		assertSafetyReason(t, evaluation, model.SalarySafetyReasonTargetNotApplicable)
		if evaluation.ExpectedHoursToDate != 0 || evaluation.TargetHoursMonth != 0 {
			t.Fatalf("%s: expected=%.2f target=%.2f, want 0/0 for an exempt type", id, evaluation.ExpectedHoursToDate, evaluation.TargetHoursMonth)
		}
		if evaluation.ShortDays != 0 || evaluation.AbsentDays != 0 || evaluation.PositionUnset {
			t.Fatalf("%s: exempt types carry no warnings (short=%d absent=%d unset=%v)", id, evaluation.ShortDays, evaluation.AbsentDays, evaluation.PositionUnset)
		}
	}
	assertSafetyHours(t, "part-time monthly_active_hours", result["pt"].MonthlyActiveHours, 20)
}

func TestSalarySafetyUnsetPositionEvaluated(t *testing.T) {
	t.Parallel()

	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("unset", "Belum Ditentukan", ymd(2025, 1, 1), 160*safetyHour, 21, true),
			safetyEmployeeRow("empty", "", ymd(2025, 1, 1), 100*safetyHour, 21, true),
			safetyEmployeeRow("full", "Full Time", ymd(2025, 1, 1), 160*safetyHour, 21, true),
		},
	}

	result := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)
	assertSafetyStatus(t, result["unset"], model.SalarySafetyStatusSafe)
	assertSafetyStatus(t, result["empty"], model.SalarySafetyStatusAtRisk)
	for _, id := range []string{"unset", "empty"} {
		if !result[id].PositionUnset {
			t.Fatalf("%s: position_unset = false, want true", id)
		}
		assertSafetyHours(t, id+" target_hours_month", result[id].TargetHoursMonth, 160)
	}
	if result["full"].PositionUnset {
		t.Fatalf("full: position_unset = true, want false")
	}
}

func TestSalarySafetyNoUser(t *testing.T) {
	t.Parallel()

	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("ghost", "Full Time", ymd(2025, 1, 1), 0, 0, false),
		},
	}

	ghost := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)["ghost"]
	assertSafetyStatus(t, ghost, model.SalarySafetyStatusNoData)
	assertSafetyReason(t, ghost, model.SalarySafetyReasonNoUser)
	if ghost.AbsentDays != 0 || ghost.HasTrackerData {
		t.Fatalf("no user: absent_days=%d has_tracker_data=%v, want 0/false", ghost.AbsentDays, ghost.HasTrackerData)
	}
}

func TestSalarySafetyNoSessions(t *testing.T) {
	t.Parallel()

	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("silent", "Full Time", ymd(2025, 1, 1), 0, 0, true),
		},
	}

	silent := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)["silent"]
	assertSafetyStatus(t, silent, model.SalarySafetyStatusAtRisk)
	assertSafetyReason(t, silent, "")
	if silent.HasTrackerData {
		t.Fatalf("has_tracker_data = true, want false")
	}
	if silent.AbsentDays != 21 {
		t.Fatalf("absent_days = %d, want 21", silent.AbsentDays)
	}
}

func TestSalarySafetyTrackerDataFollowsSessionCount(t *testing.T) {
	t.Parallel()

	// Sessions exist on every weekday but none recorded any active time: the
	// tracker is connected (has_tracker_data comes from the session count),
	// the employee is still below target and every weekday is absent.
	daily := safetyFullDays("idle", ymd(2026, 9, 1), ymd(2026, 9, 29), 0)
	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("idle", "Full Time", ymd(2025, 1, 1), 0, int64(len(daily)), true),
		},
		daily: daily,
	}

	idle := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)["idle"]
	if !idle.HasTrackerData {
		t.Fatalf("has_tracker_data = false, want true when sessions exist with 0 active seconds")
	}
	assertSafetyStatus(t, idle, model.SalarySafetyStatusAtRisk)
	assertSafetyReason(t, idle, "")
	if idle.AbsentDays != 21 || idle.ShortDays != 0 {
		t.Fatalf("absent_days=%d short_days=%d, want 21/0", idle.AbsentDays, idle.ShortDays)
	}
}

func TestSalarySafetySkipsHireAfterToday(t *testing.T) {
	t.Parallel()

	// Registered on 10 Sep with a start date of 21 Sep: not started yet, so not
	// part of September's evaluation (and not counted as AMAN).
	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("budi", "Full Time", ymd(2024, 3, 4), 60*safetyHour, 7, true),
			safetyEmployeeRow("future", "Full Time", ymd(2026, 9, 21), 0, 0, true),
		},
	}

	result := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 10, 9), 2026, 9)
	if _, listed := result["future"]; listed {
		t.Fatalf("hire dated after today is listed: %+v", result["future"])
	}
	if _, listed := result["budi"]; !listed {
		t.Fatalf("existing employee missing from the result")
	}
}

func TestSalarySafetyReportedHoursMatchStatus(t *testing.T) {
	t.Parallel()

	// Target to date on 30 Sep: 21/22 x 160 h = 549,818.18 s, reported as
	// 152.73 h (549,828 s). Seconds just below it are at_risk and must be
	// reported below 152.73 h; seconds at it are safe and reported at 152.73.
	cases := []struct {
		name   string
		actual int64
		status model.SalarySafetyStatus
		hours  float64
	}{
		{"just-below", 549827, model.SalarySafetyStatusAtRisk, 152.72},
		{"unrounded-target", 549819, model.SalarySafetyStatusAtRisk, 152.72},
		{"at-target", 549828, model.SalarySafetyStatusSafe, 152.73},
	}
	for _, tc := range cases {
		repo := &compensationPolicyRepoStub{
			monthly: []hrisrepo.EmployeeMonthlyHoursRow{
				safetyEmployeeRow("citra", "Full Time", ymd(2025, 1, 1), tc.actual, 21, true),
			},
		}
		citra := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)["citra"]
		assertSafetyStatus(t, citra, tc.status)
		assertSafetyHours(t, tc.name+" expected_hours_to_date", citra.ExpectedHoursToDate, 152.73)
		assertSafetyHours(t, tc.name+" monthly_active_hours", citra.MonthlyActiveHours, tc.hours)
		if (citra.MonthlyActiveHours >= citra.ExpectedHoursToDate) != (tc.status == model.SalarySafetyStatusSafe) {
			t.Fatalf("%s: reported %.2f vs %.2f h disagrees with status %q", tc.name, citra.MonthlyActiveHours, citra.ExpectedHoursToDate, citra.Status)
		}
	}
}

func TestExpectedSecondsToDateExactMultiples(t *testing.T) {
	t.Parallel()

	if got := expectedSecondsToDate(160*3600, 22, 22); got != 576000 {
		t.Fatalf("full month = %d s, want 576000", got)
	}
	if got := expectedSecondsToDate(160*3600, 0, 22); got != 0 {
		t.Fatalf("no elapsed weekdays = %d s, want 0", got)
	}
	if got := expectedSecondsToDate(160*3600, 10, 22); got != 261828 {
		t.Fatalf("10/22 = %d s, want 261828 (72.73 h)", got)
	}
}

func TestSalarySafetyAbsentAndShortWeekdaysAreWarningsOnly(t *testing.T) {
	t.Parallel()

	// 10 h on every weekday except: Mon 7 Sep absent, Tue 8 Sep 2.5 h,
	// Wed 9 Sep 3.5 h, Thu 10 Sep exactly 4 h (not short). Sat 12 Sep 0.5 h.
	var daily []hrisrepo.EmployeeDailyHoursRow
	for _, row := range safetyFullDays("dimas", ymd(2026, 9, 1), ymd(2026, 9, 29), 10*safetyHour) {
		switch row.Date.Day() {
		case 7:
			continue
		case 8:
			row.ActiveSeconds = 9000
		case 9:
			row.ActiveSeconds = 12600
		case 10:
			row.ActiveSeconds = 4 * safetyHour
		}
		daily = append(daily, row)
	}
	daily = append(daily, hrisrepo.EmployeeDailyHoursRow{EmployeeID: "dimas", Date: ymd(2026, 9, 12), ActiveSeconds: 1800})
	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("dimas", "Full Time", ymd(2024, 2, 12), safetySumSeconds(daily), int64(len(daily)), true),
		},
		daily: daily,
	}

	dimas := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)["dimas"]
	assertSafetyStatus(t, dimas, model.SalarySafetyStatusSafe)
	if dimas.AbsentDays != 1 {
		t.Fatalf("absent_days = %d, want 1", dimas.AbsentDays)
	}
	if dimas.ShortDays != 2 || len(dimas.DailyViolations) != 2 {
		t.Fatalf("short_days = %d (violations %v), want 2", dimas.ShortDays, dimas.DailyViolations)
	}
	if !dimas.DailyViolations[0].Date.Equal(ymd(2026, 9, 8)) || dimas.DailyViolations[0].ActiveHours != 2.5 {
		t.Fatalf("first violation = %+v, want 2026-09-08 2.5 h", dimas.DailyViolations[0])
	}
}

func TestSalarySafetyFutureMonth(t *testing.T) {
	t.Parallel()

	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("budi", "Full Time", ymd(2024, 3, 4), 0, 0, true),
			safetyEmployeeRow("gita", "Outsourcing", ymd(2025, 11, 3), 0, 0, false),
		},
	}

	result := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 11)
	for _, id := range []string{"budi", "gita"} {
		evaluation := result[id]
		assertSafetyStatus(t, evaluation, model.SalarySafetyStatusNoData)
		assertSafetyReason(t, evaluation, model.SalarySafetyReasonFutureMonth)
		if evaluation.EvaluatedThrough != nil || evaluation.ExpectedHoursToDate != 0 || evaluation.WorkingDaysElapsed != 0 {
			t.Fatalf("%s: future month must not evaluateSafety anything: %+v", id, evaluation)
		}
	}
	if repo.dailyCalls != 0 {
		t.Fatalf("daily sums were loaded %d times for a future month, want 0", repo.dailyCalls)
	}
}

func TestSalarySafetyFirstOfMonth(t *testing.T) {
	t.Parallel()

	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("budi", "Full Time", ymd(2024, 3, 4), 0, 0, true),
			safetyEmployeeRow("today", "Full Time", ymd(2026, 10, 1), 0, 0, true),
		},
	}

	result := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 10, 1, 8), 2026, 10)
	for _, id := range []string{"budi", "today"} {
		evaluation := result[id]
		assertSafetyStatus(t, evaluation, model.SalarySafetyStatusSafe)
		assertSafetyReason(t, evaluation, "")
		if evaluation.ExpectedHoursToDate != 0 || evaluation.WorkingDaysElapsed != 0 || evaluation.AbsentDays != 0 {
			t.Fatalf("%s: nothing is expected on the 1st: %+v", id, evaluation)
		}
		if evaluation.EvaluatedThrough != nil {
			t.Fatalf("%s: evaluated_through = %v, want nil on the 1st", id, evaluation.EvaluatedThrough)
		}
	}
	if result["budi"].WorkingDaysTotal != 22 {
		t.Fatalf("working_days_total(Oct 2026) = %d, want 22", result["budi"].WorkingDaysTotal)
	}
}

func TestSalarySafetyJoinedToday(t *testing.T) {
	t.Parallel()

	repo := &compensationPolicyRepoStub{
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("fresh", "Full Time", ymd(2026, 9, 30), 0, 0, true),
		},
	}

	fresh := evaluateSafety(t, repo, safetyJakartaClock(t, 2026, 9, 30, 10), 2026, 9)["fresh"]
	assertSafetyStatus(t, fresh, model.SalarySafetyStatusSafe)
	if fresh.ExpectedHoursToDate != 0 || fresh.AbsentDays != 0 {
		t.Fatalf("joined today: expected=%.2f absent=%d, want 0/0", fresh.ExpectedHoursToDate, fresh.AbsentDays)
	}
}

func TestSalarySafetyPastMonthUsesFullTarget(t *testing.T) {
	t.Parallel()

	encrypter, err := security.NewEncrypter("test-secret")
	if err != nil {
		t.Fatalf("NewEncrypter returned error: %v", err)
	}
	cipher, err := encrypter.EncryptString("12500000")
	if err != nil {
		t.Fatalf("EncryptString returned error: %v", err)
	}

	repo := &compensationPolicyRepoStub{
		policy: safetyTestPolicy(),
		monthly: []hrisrepo.EmployeeMonthlyHoursRow{
			safetyEmployeeRow("budi", "Full Time", ymd(2024, 3, 4), 168*safetyHour, 21, true),
			safetyEmployeeRow("citra", "Full Time", ymd(2024, 7, 1), 136*safetyHour+1800, 21, true),
			safetyEmployeeRow("andi", "Full Time", ymd(2023, 1, 9), 160*safetyHour, 21, true),
			safetyEmployeeRow("rahmat", "Full Time", ymd(2026, 9, 16), 0, 0, true),
		},
		baseSalaries: []hrisrepo.EmployeeBaseSalaryRow{{EmployeeID: "budi", BaseSalaryCipher: cipher}},
	}
	service := NewCompensationPolicyService(repo, encrypter)
	service.now = safetyJakartaClock(t, 2026, 9, 30, 10)

	evaluations, err := service.EvaluateSalarySafety(context.Background(), 2026, 8, nil)
	if err != nil {
		t.Fatalf("EvaluateSalarySafety returned error: %v", err)
	}
	result := make(map[string]model.SalarySafetyEvaluation, len(evaluations))
	for _, evaluation := range evaluations {
		result[evaluation.EmployeeID] = evaluation
	}

	if _, listed := result["rahmat"]; listed {
		t.Fatalf("an employee who joined after the month must not be listed")
	}
	budi := result["budi"]
	assertSafetyStatus(t, budi, model.SalarySafetyStatusSafe)
	assertSafetyStatus(t, result["andi"], model.SalarySafetyStatusSafe)
	assertSafetyStatus(t, result["citra"], model.SalarySafetyStatusAtRisk)
	assertSafetyHours(t, "expected_hours_to_date", budi.ExpectedHoursToDate, 160)
	if budi.WorkingDaysElapsed != 21 || budi.WorkingDaysTotal != 21 {
		t.Fatalf("working days = %d/%d, want 21/21", budi.WorkingDaysElapsed, budi.WorkingDaysTotal)
	}
	if budi.EvaluatedThrough == nil || !budi.EvaluatedThrough.Equal(ymd(2026, 8, 31)) {
		t.Fatalf("evaluated_through = %v, want 2026-08-31", budi.EvaluatedThrough)
	}
	if budi.BaseSalary != 12500000 || result["citra"].BaseSalary != 30000000 {
		t.Fatalf("base salary = %d / %d, want 12500000 / policy default 30000000", budi.BaseSalary, result["citra"].BaseSalary)
	}
	if !repo.gotBaseSalaryAsOf.Equal(ymd(2026, 8, 31)) {
		t.Fatalf("base salary looked up as of %v, want the period end 2026-08-31", repo.gotBaseSalaryAsOf)
	}
	if !repo.gotMonthlyFrom.Equal(ymd(2026, 8, 1)) || !repo.gotMonthlyTo.Equal(ymd(2026, 8, 31)) {
		t.Fatalf("monthly window = %v..%v, want the whole of August", repo.gotMonthlyFrom, repo.gotMonthlyTo)
	}
}

func TestSalarySafetyRejectsInvalidPeriod(t *testing.T) {
	t.Parallel()

	service := NewCompensationPolicyService(&compensationPolicyRepoStub{policy: safetyTestPolicy()}, nil)
	if _, err := service.EvaluateSalarySafety(context.Background(), 2026, 13, nil); err == nil {
		t.Fatalf("month 13 must be rejected")
	}
	if _, err := service.EvaluateSalarySafety(context.Background(), 1999, 1, nil); err == nil {
		t.Fatalf("year 1999 must be rejected")
	}
}

func TestCompensationPolicyCurrentPeriodUsesPolicyTimezone(t *testing.T) {
	t.Parallel()

	service := NewCompensationPolicyService(&compensationPolicyRepoStub{policy: safetyTestPolicy()}, nil)
	// 30 Sep 18:30 UTC is already 1 Oct 01:30 in Jakarta.
	service.now = func() time.Time { return time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC) }

	year, month, err := service.CurrentPeriod(context.Background())
	if err != nil {
		t.Fatalf("CurrentPeriod returned error: %v", err)
	}
	if year != 2026 || month != 10 {
		t.Fatalf("CurrentPeriod() = %d-%02d, want 2026-10", year, month)
	}
}

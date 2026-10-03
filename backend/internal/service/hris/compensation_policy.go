package hris

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

var ErrCompensationPolicyInvalid = errors.New("compensation policy is invalid")

type compensationPolicyRepository interface {
	EnsureRow(ctx context.Context) error
	Get(ctx context.Context) (model.CompensationPolicy, error)
	Update(ctx context.Context, params hrisrepo.UpdateCompensationPolicyParams) (model.CompensationPolicy, error)
	ListMonthlyActiveSeconds(ctx context.Context, from time.Time, to time.Time, employeeID *string) ([]hrisrepo.EmployeeMonthlyHoursRow, error)
	ListDailyActiveSeconds(ctx context.Context, from time.Time, to time.Time, employeeID *string) ([]hrisrepo.EmployeeDailyHoursRow, error)
	ListCurrentBaseSalaries(ctx context.Context, asOf time.Time, employeeID *string) ([]hrisrepo.EmployeeBaseSalaryRow, error)
}

type CompensationPolicyService struct {
	repo      compensationPolicyRepository
	encrypter *security.Encrypter
	now       func() time.Time
}

func NewCompensationPolicyService(repo compensationPolicyRepository, encrypter *security.Encrypter) *CompensationPolicyService {
	return &CompensationPolicyService{repo: repo, encrypter: encrypter, now: time.Now}
}

const dateKeyLayout = "2006-01-02"

// Employment types (employees.position) that have no monthly hour target in
// v1. Per-type targets are a later iteration; until then these employees are
// reported as no_data / target_not_applicable instead of at_risk.
var salarySafetyExemptPositions = map[string]struct{}{
	"part time":     {},
	"internship":    {},
	"project based": {},
	"outsourcing":   {},
}

// classifyPosition reports whether the monthly hour target applies to an
// employment type and whether the type is still unset. "Full Time" is the
// evaluated type; "Belum Ditentukan", empty and legacy free-text values are
// evaluated too (at the full target) but flagged so HR can set the type.
func classifyPosition(position string) (applicable bool, unset bool) {
	normalized := strings.ToLower(strings.TrimSpace(position))
	if _, exempt := salarySafetyExemptPositions[normalized]; exempt {
		return false, false
	}
	return true, normalized != "full time"
}

// CurrentPeriod returns the current year and month in the policy timezone, the
// default period of the salary-safety evaluation.
func (s *CompensationPolicyService) CurrentPeriod(ctx context.Context) (int, int, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return 0, 0, err
	}
	now := s.now().In(policyLocation(policy))
	return now.Year(), int(now.Month()), nil
}

func (s *CompensationPolicyService) GetPolicy(ctx context.Context) (model.CompensationPolicy, error) {
	if err := s.repo.EnsureRow(ctx); err != nil {
		return model.CompensationPolicy{}, err
	}
	return s.repo.Get(ctx)
}

func (s *CompensationPolicyService) UpdatePolicy(ctx context.Context, params hrisrepo.UpdateCompensationPolicyParams) (model.CompensationPolicy, error) {
	if err := validateCompensationPolicy(params); err != nil {
		return model.CompensationPolicy{}, err
	}
	if err := s.repo.EnsureRow(ctx); err != nil {
		return model.CompensationPolicy{}, err
	}
	return s.repo.Update(ctx, params)
}

// EvaluateSalarySafety applies the salary-safety rule to every active or
// probation employee who had joined by the end of the month:
//
//   - window: max(1st of month, date_joined) through the cutoff, which is the
//     month end for past months and yesterday for the current month, so the
//     unfinished day never counts against anyone;
//   - expected = min_hours_per_month x weekdays(window) / weekdays(month),
//     rounded up to whole 0.01 h (36 s) so the reported hours compare exactly
//     like the status does (see expectedSecondsToDate);
//   - actual = every tracked active second in the month, weekends and today
//     included (they only count in the employee's favour);
//   - safe when actual >= expected, at_risk otherwise;
//   - short weekdays (0 < h < min_hours_per_day) and absent weekdays in the
//     window are reported as warnings and never change the status.
//
// Employees whose date_joined is after today have not started and are left
// out, like joiners after the period end. Future months, exempt employment
// types and employees without a linked user get status no_data with a reason.
func (s *CompensationPolicyService) EvaluateSalarySafety(ctx context.Context, year int, month int, employeeID *string) ([]model.SalarySafetyEvaluation, error) {
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("%w: month must be between 1 and 12", ErrCompensationPolicyInvalid)
	}
	if year < 2000 || year > 9999 {
		return nil, fmt.Errorf("%w: year is out of range", ErrCompensationPolicyInvalid)
	}

	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}

	// All day arithmetic runs on civil dates (midnight UTC) so it is immune to
	// DST; "today" is taken in the policy timezone.
	today := civilDate(s.now().In(policyLocation(policy)))
	monthStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, -1)
	futureMonth := monthStart.After(today)

	cutoff := monthEnd
	if !monthEnd.Before(today) {
		cutoff = today.AddDate(0, 0, -1)
	}
	var evaluatedThrough *time.Time
	if !futureMonth && !cutoff.Before(monthStart) {
		value := cutoff
		evaluatedThrough = &value
	}
	workingDaysTotal := countWorkdays(monthStart, monthEnd)

	monthly, err := s.repo.ListMonthlyActiveSeconds(ctx, monthStart, monthEnd, employeeID)
	if err != nil {
		return nil, err
	}

	dailyByEmployee := make(map[string]map[string]int64)
	if !futureMonth {
		dailyRows, err := s.repo.ListDailyActiveSeconds(ctx, monthStart, monthEnd, employeeID)
		if err != nil {
			return nil, err
		}
		for _, row := range dailyRows {
			days, ok := dailyByEmployee[row.EmployeeID]
			if !ok {
				days = make(map[string]int64)
				dailyByEmployee[row.EmployeeID] = days
			}
			days[row.Date.Format(dateKeyLayout)] += row.ActiveSeconds
		}
	}

	baseSalaryByEmployee := s.currentBaseSalaries(ctx, monthEnd, employeeID)
	minDaySeconds := int64(math.Round(policy.MinHoursPerDay * 3600))
	targetMonthSeconds := policy.MinHoursPerMonth * 3600

	evaluations := make([]model.SalarySafetyEvaluation, 0, len(monthly))
	for _, row := range monthly {
		joined := civilDate(row.DateJoined)
		if joined.After(today) {
			continue
		}
		baseSalary := policy.MonthlyBaseSalary
		if value, ok := baseSalaryByEmployee[row.EmployeeID]; ok {
			baseSalary = value
		}
		applicable, positionUnset := classifyPosition(row.Position)

		evaluation := model.SalarySafetyEvaluation{
			EmployeeID:         row.EmployeeID,
			UserID:             row.UserID,
			FullName:           row.FullName,
			PeriodYear:         year,
			PeriodMonth:        month,
			MonthlyActiveHours: secondsToHours(row.ActiveSeconds),
			MinHoursPerMonth:   policy.MinHoursPerMonth,
			MinHoursPerDay:     policy.MinHoursPerDay,
			BaseSalary:         baseSalary,
			DailyViolations:    []model.DailyHoursViolation{},
			WorkingDaysTotal:   workingDaysTotal,
			EvaluatedThrough:   evaluatedThrough,
			HasTrackerData:     row.SessionCount > 0,
			PositionUnset:      positionUnset,
		}

		evaluable := !futureMonth && applicable
		var expectedSeconds int64
		if !futureMonth {
			windowStart := monthStart
			if joined.After(windowStart) {
				windowStart = joined
			}
			evaluation.WorkingDaysElapsed = countWorkdays(windowStart, cutoff)

			if evaluable {
				evaluation.TargetHoursMonth = policy.MinHoursPerMonth
				expectedSeconds = expectedSecondsToDate(targetMonthSeconds, evaluation.WorkingDaysElapsed, workingDaysTotal)
				evaluation.ExpectedHoursToDate = secondsToHours(expectedSeconds)
				if row.UserID != nil {
					evaluation.DailyViolations, evaluation.ShortDays, evaluation.AbsentDays =
						dailyWarnings(dailyByEmployee[row.EmployeeID], windowStart, cutoff, minDaySeconds)
				}
			}
		}

		evaluation.Status = evaluateStatus(row.UserID, row.ActiveSeconds, expectedSeconds, evaluable)
		evaluation.Reason = evaluationReason(row.UserID, futureMonth, applicable)
		evaluations = append(evaluations, evaluation)
	}
	return evaluations, nil
}

// centiHourSeconds is 0.01 h, the precision of the hour fields in the API.
const centiHourSeconds = 36

// expectedSecondsToDate pro-rates the monthly target by the elapsed weekdays
// and rounds it UP to a whole 0.01 h. Because the target is then an exact
// 0.01 h value and actual hours are reported rounded down (secondsToHours),
// monthly_active_hours >= expected_hours_to_date holds exactly when the
// status is safe; a half-up rounding could show equal hours on an at_risk
// row. The round-up adds under 36 seconds to the target.
func expectedSecondsToDate(targetMonthSeconds float64, elapsed int, total int) int64 {
	if total <= 0 || elapsed <= 0 {
		return 0
	}
	raw := targetMonthSeconds * float64(elapsed) / float64(total)
	// The epsilon keeps float noise on an exact multiple from adding 36 s.
	return int64(math.Ceil(raw/centiHourSeconds-1e-6)) * centiHourSeconds
}

// dailyWarnings walks the finished weekdays from start through end (inclusive)
// and returns the short weekdays (0 < active < minDaySeconds, also as the
// daily_violations list), the short-day count and the absent-day count.
// Weekends are skipped: work done on them only adds to the monthly total.
func dailyWarnings(days map[string]int64, start time.Time, end time.Time, minDaySeconds int64) ([]model.DailyHoursViolation, int, int) {
	violations := []model.DailyHoursViolation{}
	absent := 0
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if !isWorkday(day) {
			continue
		}
		seconds := days[day.Format(dateKeyLayout)]
		switch {
		case seconds <= 0:
			absent++
		case seconds < minDaySeconds:
			violations = append(violations, model.DailyHoursViolation{
				Date:        day,
				ActiveHours: secondsToHours(seconds),
			})
		}
	}
	return violations, len(violations), absent
}

// currentBaseSalaries returns each employee's base salary effective on asOf
// (the period end) in plain rupiah, decrypted from the salaries table.
// Employees without a salary record are absent from the map and fall back to
// the policy default. A decrypt failure on one row is logged and skipped
// rather than failing the whole evaluation.
func (s *CompensationPolicyService) currentBaseSalaries(ctx context.Context, asOf time.Time, employeeID *string) map[string]int64 {
	rows, err := s.repo.ListCurrentBaseSalaries(ctx, asOf, employeeID)
	if err != nil {
		slog.WarnContext(ctx, "failed to load current base salaries", "error", err)
		return map[string]int64{}
	}

	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		amount, decryptErr := decryptAmount(s.encrypter, row.BaseSalaryCipher)
		if decryptErr != nil {
			slog.WarnContext(ctx, "failed to decrypt base salary", "error", decryptErr, "employee_id", row.EmployeeID)
			continue
		}
		result[row.EmployeeID] = amount
	}
	return result
}

// evaluateStatus: no_data when the employee cannot be evaluated (future month,
// exempt type) or has no linked user; otherwise safe when the actual seconds
// reach the pro-rated expectation (always true when nothing is expected yet).
func evaluateStatus(userID *string, actualSeconds int64, expectedSeconds int64, evaluable bool) model.SalarySafetyStatus {
	if !evaluable || userID == nil {
		return model.SalarySafetyStatusNoData
	}
	if actualSeconds >= expectedSeconds {
		return model.SalarySafetyStatusSafe
	}
	return model.SalarySafetyStatusAtRisk
}

// evaluationReason explains a no_data status. A future month wins over the
// employment type, which wins over a missing login (linking a user would not
// make an exempt type evaluable).
func evaluationReason(userID *string, futureMonth bool, applicable bool) *model.SalarySafetyReason {
	var reason model.SalarySafetyReason
	switch {
	case futureMonth:
		reason = model.SalarySafetyReasonFutureMonth
	case !applicable:
		reason = model.SalarySafetyReasonTargetNotApplicable
	case userID == nil:
		reason = model.SalarySafetyReasonNoUser
	default:
		return nil
	}
	return &reason
}

// countWorkdays counts Monday-Friday dates from start through end, both
// inclusive. It returns 0 when end is before start. No holiday calendar (v1).
func countWorkdays(start time.Time, end time.Time) int {
	start, end = civilDate(start), civilDate(end)
	count := 0
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if isWorkday(day) {
			count++
		}
	}
	return count
}

func isWorkday(day time.Time) bool {
	weekday := day.Weekday()
	return weekday != time.Saturday && weekday != time.Sunday
}

// civilDate drops the clock and zone of t, keeping its calendar date as
// midnight UTC.
func civilDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func policyLocation(policy model.CompensationPolicy) *time.Location {
	location, err := time.LoadLocation(policy.Timezone)
	if err != nil {
		return time.UTC
	}
	return location
}

func validateCompensationPolicy(params hrisrepo.UpdateCompensationPolicyParams) error {
	if params.MonthlyBaseSalary < 0 {
		return fmt.Errorf("%w: base salary must not be negative", ErrCompensationPolicyInvalid)
	}
	if params.MinHoursPerDay < 0 || params.MinHoursPerDay > 24 {
		return fmt.Errorf("%w: min hours per day must be between 0 and 24", ErrCompensationPolicyInvalid)
	}
	if params.MinHoursPerMonth < 0 {
		return fmt.Errorf("%w: min hours per month must not be negative", ErrCompensationPolicyInvalid)
	}
	if strings.TrimSpace(params.Timezone) == "" {
		return fmt.Errorf("%w: timezone is required", ErrCompensationPolicyInvalid)
	}
	if _, err := time.LoadLocation(params.Timezone); err != nil {
		return fmt.Errorf("%w: timezone is not recognized", ErrCompensationPolicyInvalid)
	}
	return nil
}

// secondsToHours converts seconds to hours rounded DOWN to 0.01 h, so a
// reported value never exceeds what was tracked (a 3.999 h day shows 3.99, not
// 4.00, next to the 4 h minimum).
func secondsToHours(seconds int64) float64 {
	return float64(seconds/centiHourSeconds) / 100
}

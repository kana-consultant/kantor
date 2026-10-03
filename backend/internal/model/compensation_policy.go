package model

import "time"

type CompensationPolicy struct {
	TenantID          string    `json:"tenant_id"`
	MonthlyBaseSalary int64     `json:"monthly_base_salary"`
	MinHoursPerDay    float64   `json:"min_hours_per_day"`
	MinHoursPerMonth  float64   `json:"min_hours_per_month"`
	Timezone          string    `json:"timezone"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type SalarySafetyStatus string

const (
	SalarySafetyStatusSafe   SalarySafetyStatus = "safe"
	SalarySafetyStatusAtRisk SalarySafetyStatus = "at_risk"
	SalarySafetyStatusNoData SalarySafetyStatus = "no_data"
)

// SalarySafetyReason explains why an evaluation carries status no_data.
type SalarySafetyReason string

const (
	// SalarySafetyReasonNoUser: the employee has no linked login, so there is
	// no tracker data to evaluate.
	SalarySafetyReasonNoUser SalarySafetyReason = "no_user"
	// SalarySafetyReasonFutureMonth: the period has not started yet.
	SalarySafetyReasonFutureMonth SalarySafetyReason = "future_month"
	// SalarySafetyReasonTargetNotApplicable: the employment type (Part Time,
	// Internship, Project Based, Outsourcing) has no monthly hour target.
	SalarySafetyReasonTargetNotApplicable SalarySafetyReason = "target_not_applicable"
)

type DailyHoursViolation struct {
	Date        time.Time `json:"date"`
	ActiveHours float64   `json:"active_hours"`
}

// SalarySafetyEvaluation is one employee's standing for a month. The rule:
// actual active hours in the month (weekends and today included) compared to
// the monthly target pro-rated by the weekdays elapsed in the evaluation
// window (from max(1st of month, date_joined) through yesterday, or through
// month end for past months). At or above = safe, below = at_risk. Short and
// absent weekdays are warnings only and never change the status.
type SalarySafetyEvaluation struct {
	EmployeeID         string                `json:"employee_id"`
	UserID             *string               `json:"user_id,omitempty"`
	FullName           string                `json:"full_name"`
	PeriodYear         int                   `json:"period_year"`
	PeriodMonth        int                   `json:"period_month"`
	MonthlyActiveHours float64               `json:"monthly_active_hours"`
	MinHoursPerMonth   float64               `json:"min_hours_per_month"`
	MinHoursPerDay     float64               `json:"min_hours_per_day"`
	BaseSalary         int64                 `json:"base_salary"`
	DailyViolations    []DailyHoursViolation `json:"daily_violations"`
	Status             SalarySafetyStatus    `json:"status"`

	// ExpectedHoursToDate is the pro-rated target the actual hours are compared to.
	ExpectedHoursToDate float64 `json:"expected_hours_to_date"`
	// TargetHoursMonth is the full-month target (0 when the target does not apply).
	TargetHoursMonth   float64 `json:"target_hours_month"`
	WorkingDaysElapsed int     `json:"working_days_elapsed"`
	WorkingDaysTotal   int     `json:"working_days_total"`
	// EvaluatedThrough is the last finished day counted against the target; nil
	// for a future month or when no day of the month has finished yet.
	EvaluatedThrough *time.Time `json:"evaluated_through"`
	HasTrackerData   bool       `json:"has_tracker_data"`
	// ShortDays counts finished weekdays in the window with 0 < hours < min per day.
	ShortDays int `json:"short_days"`
	// AbsentDays counts finished weekdays in the window without any active time.
	AbsentDays    int                 `json:"absent_days"`
	Reason        *SalarySafetyReason `json:"reason"`
	PositionUnset bool                `json:"position_unset"`
}

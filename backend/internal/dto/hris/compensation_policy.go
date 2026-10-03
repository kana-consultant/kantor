package hris

type UpdateCompensationPolicyRequest struct {
	MonthlyBaseSalary int64   `json:"monthly_base_salary" validate:"min=0"`
	MinHoursPerDay    float64 `json:"min_hours_per_day" validate:"min=0,max=24"`
	MinHoursPerMonth  float64 `json:"min_hours_per_month" validate:"min=0,max=744"`
	Timezone          string  `json:"timezone" validate:"required,max=64"`
}

type CompensationPolicyResponse struct {
	MonthlyBaseSalary int64   `json:"monthly_base_salary"`
	MinHoursPerDay    float64 `json:"min_hours_per_day"`
	MinHoursPerMonth  float64 `json:"min_hours_per_month"`
	Timezone          string  `json:"timezone"`
	UpdatedAt         string  `json:"updated_at"`
}

type DailyHoursViolationResponse struct {
	Date        string  `json:"date"`
	ActiveHours float64 `json:"active_hours"`
}

type SalarySafetyResponse struct {
	EmployeeID          string                        `json:"employee_id"`
	UserID              *string                       `json:"user_id,omitempty"`
	FullName            string                        `json:"full_name"`
	PeriodYear          int                           `json:"period_year"`
	PeriodMonth         int                           `json:"period_month"`
	MonthlyActiveHours  float64                       `json:"monthly_active_hours"`
	MinHoursPerMonth    float64                       `json:"min_hours_per_month"`
	MinHoursPerDay      float64                       `json:"min_hours_per_day"`
	BaseSalary          int64                         `json:"base_salary"`
	DailyViolations     []DailyHoursViolationResponse `json:"daily_violations"`
	Status              string                        `json:"status"`
	ExpectedHoursToDate float64                       `json:"expected_hours_to_date"`
	TargetHoursMonth    float64                       `json:"target_hours_month"`
	WorkingDaysElapsed  int                           `json:"working_days_elapsed"`
	WorkingDaysTotal    int                           `json:"working_days_total"`
	EvaluatedThrough    *string                       `json:"evaluated_through"`
	HasTrackerData      bool                          `json:"has_tracker_data"`
	ShortDays           int                           `json:"short_days"`
	AbsentDays          int                           `json:"absent_days"`
	Reason              *string                       `json:"reason"`
	PositionUnset       bool                          `json:"position_unset"`
}

// SalarySafetyMeta is the meta of GET /hris/salary-safety: the evaluated
// period and the current period in the policy timezone (the default period and
// the latest month that can be evaluated).
type SalarySafetyMeta struct {
	PeriodYear   int `json:"period_year"`
	PeriodMonth  int `json:"period_month"`
	CurrentYear  int `json:"current_year"`
	CurrentMonth int `json:"current_month"`
}

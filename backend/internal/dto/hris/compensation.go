package hris

import "time"

type CreateSalaryRequest struct {
	BaseSalary    int64            `json:"base_salary" validate:"required,min=0"`
	Allowances    map[string]int64 `json:"allowances"`
	Deductions    map[string]int64 `json:"deductions"`
	EffectiveDate time.Time        `json:"effective_date" validate:"required"`
}

type CreateBonusRequest struct {
	Amount      int64  `json:"amount" validate:"required,min=0"`
	Reason      string `json:"reason" validate:"required,min=3,max=255"`
	PeriodMonth int    `json:"period_month" validate:"required,min=1,max=12"`
	PeriodYear  int    `json:"period_year" validate:"required,min=2000,max=2100"`
}

type UpdateBonusRequest struct {
	Amount      int64  `json:"amount" validate:"required,min=0"`
	Reason      string `json:"reason" validate:"required,min=3,max=255"`
	PeriodMonth int    `json:"period_month" validate:"required,min=1,max=12"`
	PeriodYear  int    `json:"period_year" validate:"required,min=2000,max=2100"`
}

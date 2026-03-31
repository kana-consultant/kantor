package hris

import dto "github.com/kana-consultant/kantor/backend/internal/dto"

type CreateSalaryRequest struct {
	BaseSalary    int64            `json:"base_salary" validate:"required,min=0"`
	Allowances    map[string]int64 `json:"allowances"`
	Deductions    map[string]int64 `json:"deductions"`
	EffectiveDate dto.DateOnly     `json:"effective_date" validate:"required,datetime=2006-01-02"`
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

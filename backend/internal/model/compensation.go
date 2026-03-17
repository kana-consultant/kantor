package model

import "time"

type SalaryRecord struct {
	ID            string           `json:"id"`
	EmployeeID    string           `json:"employee_id"`
	BaseSalary    int64            `json:"base_salary"`
	Allowances    map[string]int64 `json:"allowances"`
	Deductions    map[string]int64 `json:"deductions"`
	NetSalary     int64            `json:"net_salary"`
	EffectiveDate time.Time        `json:"effective_date"`
	CreatedBy     string           `json:"created_by"`
	CreatedAt     time.Time        `json:"created_at"`
}

type BonusRecord struct {
	ID             string     `json:"id"`
	EmployeeID     string     `json:"employee_id"`
	Amount         int64      `json:"amount"`
	Reason         string     `json:"reason"`
	PeriodMonth    int        `json:"period_month"`
	PeriodYear     int        `json:"period_year"`
	ApprovalStatus string     `json:"approval_status"`
	ApprovedBy     *string    `json:"approved_by,omitempty"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

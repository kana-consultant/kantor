package model

import "time"

type Reimbursement struct {
	ID              string     `json:"id"`
	EmployeeID      string     `json:"employee_id"`
	EmployeeName    string     `json:"employee_name"`
	Title           string     `json:"title"`
	Category        string     `json:"category"`
	Amount          int64      `json:"amount"`
	TransactionDate time.Time  `json:"transaction_date"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	Attachments     []string   `json:"attachments"`
	SubmittedBy     string     `json:"submitted_by"`
	ManagerID       *string    `json:"manager_id,omitempty"`
	ManagerActionAt *time.Time `json:"manager_action_at,omitempty"`
	ManagerNotes    *string    `json:"manager_notes,omitempty"`
	FinanceID       *string    `json:"finance_id,omitempty"`
	FinanceActionAt *time.Time `json:"finance_action_at,omitempty"`
	FinanceNotes    *string    `json:"finance_notes,omitempty"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ReimbursementSummary struct {
	Month               int            `json:"month"`
	Year                int            `json:"year"`
	CountsByStatus      map[string]int `json:"counts_by_status"`
	ApprovedAmountMonth int64          `json:"approved_amount_month"`
}

package hris

import "time"

type CreateReimbursementRequest struct {
	EmployeeID      string    `json:"employee_id" validate:"required,uuid4"`
	Title           string    `json:"title" validate:"required,min=2,max=200"`
	Category        string    `json:"category" validate:"required,min=2,max=120"`
	Amount          int64     `json:"amount" validate:"required,min=0"`
	TransactionDate time.Time `json:"transaction_date" validate:"required"`
	Description     string    `json:"description" validate:"required,min=2,max=2000"`
}

type ListReimbursementsQuery struct {
	Page       int    `validate:"omitempty,min=1"`
	PerPage    int    `validate:"omitempty,min=1,max=100"`
	Status     string `validate:"omitempty,oneof=submitted approved rejected paid"`
	EmployeeID string `validate:"omitempty,uuid4"`
	Month      int    `validate:"omitempty,min=1,max=12"`
	Year       int    `validate:"omitempty,min=2000,max=2100"`
}

type ReviewReimbursementRequest struct {
	Decision string  `json:"decision" validate:"required,oneof=approved rejected"`
	Notes    *string `json:"notes" validate:"omitempty,max=2000"`
}

type MarkPaidRequest struct {
	Notes *string `json:"notes" validate:"omitempty,max=2000"`
}

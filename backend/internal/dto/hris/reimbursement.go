package hris

import dto "github.com/kana-consultant/kantor/backend/internal/dto"

type CreateReimbursementRequest struct {
	EmployeeID      string       `json:"employee_id" validate:"required,uuid4"`
	Title           string       `json:"title" validate:"required,min=2,max=200"`
	Category        string       `json:"category" validate:"required,min=2,max=120"`
	Amount          int64        `json:"amount" validate:"required,min=0"`
	TransactionDate dto.DateOnly `json:"transaction_date" validate:"required,datetime=2006-01-02"`
	Description     string       `json:"description" validate:"omitempty,max=2000"`
}

type UpdateReimbursementRequest struct {
	Title           string       `json:"title" validate:"required,min=2,max=200"`
	Category        string       `json:"category" validate:"required,min=2,max=120"`
	Amount          int64        `json:"amount" validate:"required,min=0"`
	TransactionDate dto.DateOnly `json:"transaction_date" validate:"required,datetime=2006-01-02"`
	Description     string       `json:"description" validate:"omitempty,max=2000"`
	KeptAttachments []string     `json:"kept_attachments" validate:"omitempty"`
}

type ListReimbursementsQuery struct {
	Page       int    `validate:"omitempty,min=1"`
	PerPage    int    `validate:"omitempty,min=1,max=100"`
	Status     string `validate:"omitempty,oneof=submitted approved rejected paid"`
	EmployeeID string `validate:"omitempty,uuid4"`
	Month      int    `validate:"omitempty,min=1,max=12"`
	Year       int    `validate:"omitempty,min=2000,max=2100"`
	SortBy     string `validate:"omitempty,oneof=title category amount status transaction_date created_at"`
	SortOrder  string `validate:"omitempty,oneof=asc desc"`
}

type ReviewReimbursementRequest struct {
	Decision string  `json:"decision" validate:"required,oneof=approved rejected"`
	Notes    *string `json:"notes" validate:"omitempty,max=2000"`
}

type MarkPaidRequest struct {
	Notes *string `json:"notes" validate:"omitempty,max=2000"`
}

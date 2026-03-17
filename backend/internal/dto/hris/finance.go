package hris

import "time"

type CreateFinanceCategoryRequest struct {
	Name string `json:"name" validate:"required,min=2,max=120"`
	Type string `json:"type" validate:"required,oneof=income outcome"`
}

type UpdateFinanceCategoryRequest = CreateFinanceCategoryRequest

type CreateFinanceRecordRequest struct {
	CategoryID  string    `json:"category_id" validate:"required,uuid4"`
	Type        string    `json:"type" validate:"required,oneof=income outcome"`
	Amount      int64     `json:"amount" validate:"required,min=0"`
	Description string    `json:"description" validate:"required,min=2,max=2000"`
	RecordDate  time.Time `json:"record_date" validate:"required"`
}

type UpdateFinanceRecordRequest = CreateFinanceRecordRequest

type ReviewFinanceRecordRequest struct {
	Decision string `json:"decision" validate:"required,oneof=approved rejected"`
}

type ListFinanceRecordsQuery struct {
	Page       int    `validate:"omitempty,min=1"`
	PerPage    int    `validate:"omitempty,min=1,max=100"`
	Type       string `validate:"omitempty,oneof=income outcome"`
	CategoryID string `validate:"omitempty,uuid4"`
	Month      int    `validate:"omitempty,min=1,max=12"`
	Year       int    `validate:"omitempty,min=2000,max=2100"`
	Status     string `validate:"omitempty,oneof=draft pending_review approved rejected"`
}

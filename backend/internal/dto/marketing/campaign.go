package marketing

import dto "github.com/kana-consultant/kantor/backend/internal/dto"

type CreateCampaignRequest struct {
	Name           string       `json:"name" validate:"required,min=3,max=180"`
	Description    *string      `json:"description" validate:"omitempty,max=5000"`
	Channel        string       `json:"channel" validate:"required,campaign_channel"`
	BudgetAmount   int64        `json:"budget_amount" validate:"min=0"`
	BudgetCurrency string       `json:"budget_currency" validate:"omitempty,max=8"`
	PICEmployeeID  *string      `json:"pic_employee_id" validate:"omitempty,uuid4"`
	StartDate      dto.DateOnly `json:"start_date" validate:"required,datetime=2006-01-02"`
	EndDate        dto.DateOnly `json:"end_date" validate:"required,datetime=2006-01-02"`
	BriefText      *string      `json:"brief_text" validate:"omitempty,max=20000"`
	Status         string       `json:"status" validate:"required,campaign_stage"`
}

type UpdateCampaignRequest struct {
	Name           string       `json:"name" validate:"required,min=3,max=180"`
	Description    *string      `json:"description" validate:"omitempty,max=5000"`
	Channel        string       `json:"channel" validate:"required,campaign_channel"`
	BudgetAmount   int64        `json:"budget_amount" validate:"min=0"`
	BudgetCurrency string       `json:"budget_currency" validate:"omitempty,max=8"`
	PICEmployeeID  *string      `json:"pic_employee_id" validate:"omitempty,uuid4"`
	StartDate      dto.DateOnly `json:"start_date" validate:"required,datetime=2006-01-02"`
	EndDate        dto.DateOnly `json:"end_date" validate:"required,datetime=2006-01-02"`
	BriefText      *string      `json:"brief_text" validate:"omitempty,max=20000"`
	Status         string       `json:"status" validate:"required,campaign_stage"`
}

// ListCampaignsQuery is filled from the query string. The json tags only
// name the fields in validation details (page, per_page, date_from, ...).
type ListCampaignsQuery struct {
	Page     int    `json:"page" validate:"omitempty,min=1"`
	PerPage  int    `json:"per_page" validate:"omitempty,min=1,max=100"`
	Search   string `json:"search" validate:"omitempty,max=180"`
	Channel  string `json:"channel" validate:"omitempty,campaign_channel"`
	Status   string `json:"status" validate:"omitempty,campaign_stage"`
	PIC      string `json:"pic" validate:"omitempty,uuid4"`
	DateFrom string `json:"date_from" validate:"omitempty,datetime=2006-01-02"`
	DateTo   string `json:"date_to" validate:"omitempty,datetime=2006-01-02"`
}

type MoveCampaignRequest struct {
	ColumnID string `json:"column_id" validate:"required,uuid4"`
	Position int    `json:"position" validate:"required,min=1"`
}

type CreateCampaignColumnRequest struct {
	Name     string  `json:"name" validate:"required,min=2,max=80"`
	Color    *string `json:"color" validate:"omitempty,max=20"`
	Position *int    `json:"position" validate:"omitempty,min=1"`
}

type UpdateCampaignColumnRequest struct {
	Name  string  `json:"name" validate:"required,min=2,max=80"`
	Color *string `json:"color" validate:"omitempty,max=20"`
}

type ReorderCampaignColumnsRequest struct {
	ColumnIDs []string `json:"column_ids" validate:"required,min=1,dive,uuid4"`
}

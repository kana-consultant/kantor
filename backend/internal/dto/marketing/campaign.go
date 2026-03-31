package marketing

import dto "github.com/kana-consultant/kantor/backend/internal/dto"

type CreateCampaignRequest struct {
	Name           string       `json:"name" validate:"required,min=3,max=180"`
	Description    *string      `json:"description"`
	Channel        string       `json:"channel" validate:"required,oneof=instagram facebook google_ads tiktok youtube email other"`
	BudgetAmount   int64        `json:"budget_amount" validate:"min=0"`
	BudgetCurrency string       `json:"budget_currency" validate:"omitempty,max=8"`
	PICEmployeeID  *string      `json:"pic_employee_id" validate:"omitempty,uuid4"`
	StartDate      dto.DateOnly `json:"start_date" validate:"required,datetime=2006-01-02"`
	EndDate        dto.DateOnly `json:"end_date" validate:"required,datetime=2006-01-02"`
	BriefText      *string      `json:"brief_text"`
	Status         string       `json:"status" validate:"required,oneof=ideation planning in_production live completed archived"`
}

type UpdateCampaignRequest struct {
	Name           string       `json:"name" validate:"required,min=3,max=180"`
	Description    *string      `json:"description"`
	Channel        string       `json:"channel" validate:"required,oneof=instagram facebook google_ads tiktok youtube email other"`
	BudgetAmount   int64        `json:"budget_amount" validate:"min=0"`
	BudgetCurrency string       `json:"budget_currency" validate:"omitempty,max=8"`
	PICEmployeeID  *string      `json:"pic_employee_id" validate:"omitempty,uuid4"`
	StartDate      dto.DateOnly `json:"start_date" validate:"required,datetime=2006-01-02"`
	EndDate        dto.DateOnly `json:"end_date" validate:"required,datetime=2006-01-02"`
	BriefText      *string      `json:"brief_text"`
	Status         string       `json:"status" validate:"required,oneof=ideation planning in_production live completed archived"`
}

type ListCampaignsQuery struct {
	Page     int    `validate:"omitempty,min=1"`
	PerPage  int    `validate:"omitempty,min=1,max=100"`
	Search   string `validate:"omitempty,max=180"`
	Channel  string `validate:"omitempty,oneof=instagram facebook google_ads tiktok youtube email other"`
	Status   string `validate:"omitempty,oneof=ideation planning in_production live completed archived"`
	PIC      string `validate:"omitempty,uuid4"`
	DateFrom string `validate:"omitempty,datetime=2006-01-02"`
	DateTo   string `validate:"omitempty,datetime=2006-01-02"`
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

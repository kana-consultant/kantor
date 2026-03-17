package model

import "time"

type Campaign struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	Channel         string    `json:"channel"`
	BudgetAmount    int64     `json:"budget_amount"`
	BudgetCurrency  string    `json:"budget_currency"`
	PICEmployeeID   *string   `json:"pic_employee_id,omitempty"`
	PICEmployeeName *string   `json:"pic_employee_name,omitempty"`
	PICAvatarURL    *string   `json:"pic_avatar_url,omitempty"`
	StartDate       time.Time `json:"start_date"`
	EndDate         time.Time `json:"end_date"`
	BriefText       *string   `json:"brief_text,omitempty"`
	Status          string    `json:"status"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	ColumnID        *string   `json:"column_id,omitempty"`
	ColumnName      *string   `json:"column_name,omitempty"`
	ColumnColor     *string   `json:"column_color,omitempty"`
	ColumnPosition  *int      `json:"column_position,omitempty"`
	AttachmentCount int       `json:"attachment_count"`
}

type CampaignAttachment struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaign_id"`
	FileName   string    `json:"file_name"`
	FilePath   string    `json:"file_path"`
	FileType   string    `json:"file_type"`
	FileSize   int64     `json:"file_size"`
	UploadedBy string    `json:"uploaded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

type CampaignColumn struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Position    int        `json:"position"`
	Color       *string    `json:"color,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Campaigns   []Campaign `json:"campaigns,omitempty"`
	CampaignsNo int        `json:"campaign_count,omitempty"`
}

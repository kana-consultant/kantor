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

// CampaignColumn is one lane of the campaign board.
//
// Stage is the campaigns.status the lane stands for; exactly one lane per
// tenant carries each stage and that lane cannot be deleted. A nil Stage is
// a custom lane: campaigns can be parked in it without changing their status.
//
// CampaignCount is always present. Campaigns is present (possibly empty) on
// the kanban response and left out of the plain column list.
type CampaignColumn struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Position    int        `json:"position"`
	Color       *string    `json:"color,omitempty"`
	Stage       *string    `json:"stage"`
	CreatedAt   time.Time  `json:"created_at"`
	Campaigns   []Campaign `json:"campaigns,omitzero"`
	CampaignsNo int        `json:"campaign_count"`
}

// CampaignPICOption is one entry of the person-in-charge picker: just enough
// to show and choose an employee, without any HR data.
type CampaignPICOption struct {
	ID        string  `json:"id"`
	FullName  string  `json:"full_name"`
	Position  string  `json:"position"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

type CampaignActivity struct {
	ID          string    `json:"id"`
	CampaignID  string    `json:"campaign_id"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	ActorID     *string   `json:"actor_id,omitempty"`
	ActorName   *string   `json:"actor_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

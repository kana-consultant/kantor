package model

import "time"

type Lead struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Phone            *string   `json:"phone,omitempty"`
	Email            *string   `json:"email,omitempty"`
	SourceChannel    string    `json:"source_channel"`
	PipelineStatus   string    `json:"pipeline_status"`
	CampaignID       *string   `json:"campaign_id,omitempty"`
	CampaignName     *string   `json:"campaign_name,omitempty"`
	AssignedTo       *string   `json:"assigned_to,omitempty"`
	AssignedToName   *string   `json:"assigned_to_name,omitempty"`
	AssignedToAvatar *string   `json:"assigned_to_avatar,omitempty"`
	Notes            *string   `json:"notes,omitempty"`
	CompanyName      *string   `json:"company_name,omitempty"`
	EstimatedValue   int64     `json:"estimated_value"`
	CreatedBy        string    `json:"created_by"`
	CreatedByName    *string   `json:"created_by_name,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type LeadActivity struct {
	ID            string    `json:"id"`
	LeadID        string    `json:"lead_id"`
	ActivityType  string    `json:"activity_type"`
	Description   string    `json:"description"`
	OldStatus     *string   `json:"old_status,omitempty"`
	NewStatus     *string   `json:"new_status,omitempty"`
	CreatedBy     string    `json:"created_by"`
	CreatedByName *string   `json:"created_by_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type LeadPipelineColumn struct {
	Status string `json:"status"`
	Label  string `json:"label"`
	Leads  []Lead `json:"leads"`
}

type LeadSummaryRow struct {
	Status         string `json:"status"`
	Label          string `json:"label"`
	LeadCount      int64  `json:"lead_count"`
	EstimatedValue int64  `json:"estimated_value"`
}

type LeadSummary struct {
	TotalLeads     int64            `json:"total_leads"`
	WonLeads       int64            `json:"won_leads"`
	ConversionRate float64          `json:"conversion_rate"`
	ByStatus       []LeadSummaryRow `json:"by_status"`
}

type LeadImportError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type LeadImportSummary struct {
	SuccessCount int               `json:"success_count"`
	FailedCount  int               `json:"failed_count"`
	Errors       []LeadImportError `json:"errors"`
}

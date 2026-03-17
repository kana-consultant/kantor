package marketing

type CreateLeadRequest struct {
	Name           string  `json:"name" validate:"required,min=2,max=180"`
	Phone          *string `json:"phone" validate:"omitempty,max=32"`
	Email          *string `json:"email" validate:"omitempty,email,max=255"`
	SourceChannel  string  `json:"source_channel" validate:"required,oneof=whatsapp email instagram facebook website referral other"`
	PipelineStatus string  `json:"pipeline_status" validate:"required,oneof=new contacted qualified proposal negotiation won lost"`
	CampaignID     *string `json:"campaign_id" validate:"omitempty,uuid4"`
	AssignedTo     *string `json:"assigned_to" validate:"omitempty,uuid4"`
	Notes          *string `json:"notes" validate:"omitempty,max=4000"`
	CompanyName    *string `json:"company_name" validate:"omitempty,max=180"`
	EstimatedValue int64   `json:"estimated_value" validate:"min=0"`
}

type UpdateLeadRequest = CreateLeadRequest

type ListLeadsQuery struct {
	Page           int    `validate:"omitempty,min=1"`
	PerPage        int    `validate:"omitempty,min=1,max=100"`
	PipelineStatus string `validate:"omitempty,oneof=new contacted qualified proposal negotiation won lost"`
	SourceChannel  string `validate:"omitempty,oneof=whatsapp email instagram facebook website referral other"`
	CampaignID     string `validate:"omitempty,uuid4"`
	AssignedTo     string `validate:"omitempty,uuid4"`
	DateFrom       string `validate:"omitempty,datetime=2006-01-02"`
	DateTo         string `validate:"omitempty,datetime=2006-01-02"`
	Search         string `validate:"omitempty,max=180"`
}

type MoveLeadStatusRequest struct {
	PipelineStatus string `json:"pipeline_status" validate:"required,oneof=new contacted qualified proposal negotiation won lost"`
}

type CreateLeadActivityRequest struct {
	ActivityType string `json:"activity_type" validate:"required,oneof=note_added call email_sent whatsapp_sent meeting follow_up"`
	Description  string `json:"description" validate:"required,min=2,max=4000"`
}

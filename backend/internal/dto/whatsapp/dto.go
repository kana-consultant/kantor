package whatsapp

type CreateTemplateRequest struct {
	Name               string   `json:"name" validate:"required,max=100"`
	Slug               string   `json:"slug" validate:"required,max=50"`
	Category           string   `json:"category" validate:"required,oneof=operational hris marketing general"`
	TriggerType        string   `json:"trigger_type" validate:"required,oneof=auto_scheduled event_triggered manual"`
	BodyTemplate       string   `json:"body_template" validate:"required"`
	Description        *string  `json:"description"`
	AvailableVariables []string `json:"available_variables"`
	IsActive           bool     `json:"is_active"`
}

type UpdateTemplateRequest struct {
	Name               *string  `json:"name"`
	Category           *string  `json:"category" validate:"omitempty,oneof=operational hris marketing general"`
	TriggerType        *string  `json:"trigger_type" validate:"omitempty,oneof=auto_scheduled event_triggered manual"`
	BodyTemplate       string   `json:"body_template" validate:"required"`
	Description        *string  `json:"description"`
	AvailableVariables []string `json:"available_variables"`
	IsActive           bool     `json:"is_active"`
}

type CreateScheduleRequest struct {
	Name           string  `json:"name" validate:"required,max=100"`
	TemplateID     string  `json:"template_id" validate:"required,uuid"`
	ScheduleType   string  `json:"schedule_type" validate:"required,oneof=daily weekly monthly once"`
	CronExpression *string `json:"cron_expression"`
	TargetType     string  `json:"target_type" validate:"required,oneof=all_employees department specific_users project_members"`
	TargetConfig   *string `json:"target_config"`
	IsActive       bool    `json:"is_active"`
}

type UpdateScheduleRequest struct {
	Name           string  `json:"name" validate:"required,max=100"`
	TemplateID     string  `json:"template_id" validate:"required,uuid"`
	ScheduleType   string  `json:"schedule_type" validate:"required,oneof=daily weekly monthly once"`
	CronExpression *string `json:"cron_expression"`
	TargetType     string  `json:"target_type" validate:"required,oneof=all_employees department specific_users project_members"`
	TargetConfig   *string `json:"target_config"`
	IsActive       bool    `json:"is_active"`
}

type ToggleRequest struct {
	IsActive bool `json:"is_active"`
}

type QuickSendRequest struct {
	Phone   string `json:"phone" validate:"required"`
	Message string `json:"message" validate:"required"`
}

type UpdatePhoneRequest struct {
	Phone *string `json:"phone"`
}

type ListLogsQuery struct {
	Page         int    `json:"page"`
	PerPage      int    `json:"per_page"`
	ScheduleID   string `json:"schedule_id"`
	TriggerType  string `json:"trigger_type"`
	TemplateSlug string `json:"template_slug"`
	Status       string `json:"status"`
	DateFrom     string `json:"date_from"`
	DateTo       string `json:"date_to"`
	Search       string `json:"search"`
}

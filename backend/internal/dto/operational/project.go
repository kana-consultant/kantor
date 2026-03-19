package operational

import "time"

type CreateProjectRequest struct {
	Name         string     `json:"name" validate:"required,min=3,max=150"`
	Description  *string    `json:"description"`
	Deadline     *time.Time `json:"deadline"`
	Status       string     `json:"status" validate:"required,oneof=draft active on_hold completed archived"`
	Priority     string     `json:"priority" validate:"required,oneof=low medium high critical"`
	MemberEmails []string   `json:"member_emails" validate:"omitempty,dive,email"`
}

type UpdateProjectRequest struct {
	Name           string     `json:"name" validate:"required,min=3,max=150"`
	Description    *string    `json:"description"`
	Deadline       *time.Time `json:"deadline"`
	Status         string     `json:"status" validate:"required,oneof=draft active on_hold completed archived"`
	Priority       string     `json:"priority" validate:"required,oneof=low medium high critical"`
	AutoAssignMode *string    `json:"auto_assign_mode" validate:"omitempty,oneof=off round_robin least_busy"`
}

type ProjectMembersMutationRequest struct {
	Operation     string `json:"operation" validate:"required,oneof=assign remove"`
	UserID        string `json:"user_id" validate:"omitempty,uuid4"`
	UserEmail     string `json:"user_email" validate:"omitempty,email"`
	RoleInProject string `json:"role_in_project" validate:"omitempty,min=2,max=80"`
}

type ListProjectsQuery struct {
	Page     int    `validate:"omitempty,min=1"`
	PerPage  int    `validate:"omitempty,min=1,max=100"`
	Search   string `validate:"omitempty,max=150"`
	Status   string `validate:"omitempty,oneof=draft active on_hold completed archived"`
	Priority string `validate:"omitempty,oneof=low medium high critical"`
}

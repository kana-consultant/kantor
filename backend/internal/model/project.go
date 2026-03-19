package model

import "time"

type Project struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Description      *string    `json:"description,omitempty"`
	Deadline         *time.Time `json:"deadline,omitempty"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority"`
	AutoAssignMode   string     `json:"auto_assign_mode"`
	AutoAssignCursor int        `json:"-"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	MemberCount      int        `json:"member_count,omitempty"`
}

type ProjectMember struct {
	ProjectID     string    `json:"project_id"`
	UserID        string    `json:"user_id"`
	RoleInProject string    `json:"role_in_project"`
	AssignedAt    time.Time `json:"assigned_at"`
	UserEmail     string    `json:"user_email,omitempty"`
	FullName      string    `json:"full_name,omitempty"`
	AvatarURL     *string   `json:"avatar_url,omitempty"`
}

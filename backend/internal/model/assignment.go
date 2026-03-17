package model

import "time"

type AssignmentRule struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"project_id"`
	RuleType   string         `json:"rule_type"`
	RuleConfig map[string]any `json:"rule_config"`
	Priority   int            `json:"priority"`
	IsActive   bool           `json:"is_active"`
	CreatedBy  string         `json:"created_by"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AssignmentCandidate struct {
	UserID        string    `json:"user_id"`
	FullName      string    `json:"full_name"`
	Email         string    `json:"email"`
	AvatarURL     *string   `json:"avatar_url,omitempty"`
	Department    *string   `json:"department,omitempty"`
	Skills        []string  `json:"skills,omitempty"`
	RoleInProject string    `json:"role_in_project"`
	AssignedAt    time.Time `json:"assigned_at"`
	Workload      int       `json:"workload"`
}

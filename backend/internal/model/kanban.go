package model

import "time"

const (
	KanbanColumnTypeTodo       = "todo"
	KanbanColumnTypeInProgress = "in_progress"
	KanbanColumnTypeDone       = "done"
	KanbanColumnTypeCustom     = "custom"
)

type KanbanColumn struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Name       string    `json:"name"`
	ColumnType string    `json:"column_type"`
	Position   int       `json:"position"`
	Color      *string   `json:"color,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type KanbanTask struct {
	ID           string     `json:"id"`
	ColumnID     string     `json:"column_id"`
	ProjectID    string     `json:"project_id"`
	Title        string     `json:"title"`
	Description  *string    `json:"description,omitempty"`
	AssigneeID   *string    `json:"assignee_id,omitempty"`
	AssigneeName *string    `json:"assignee_name,omitempty"`
	AvatarURL    *string    `json:"avatar_url,omitempty"`
	DueDate      *time.Time `json:"due_date,omitempty"`
	Priority     string     `json:"priority"`
	Label        *string    `json:"label,omitempty"`
	AssignedVia  string     `json:"assigned_via"`
	Position     int        `json:"position"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

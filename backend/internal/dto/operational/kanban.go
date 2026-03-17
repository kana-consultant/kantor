package operational

import "time"

type CreateKanbanColumnRequest struct {
	Name     string  `json:"name" validate:"required,min=2,max=120"`
	Color    *string `json:"color"`
	Position *int    `json:"position" validate:"omitempty,min=1"`
}

type UpdateKanbanColumnRequest struct {
	Name  string  `json:"name" validate:"required,min=2,max=120"`
	Color *string `json:"color"`
}

type ReorderKanbanColumnsRequest struct {
	ColumnIDs []string `json:"column_ids" validate:"required,min=1,dive,uuid4"`
}

type CreateKanbanTaskRequest struct {
	ColumnID    string     `json:"column_id" validate:"required,uuid4"`
	Title       string     `json:"title" validate:"required,min=2,max=160"`
	Description *string    `json:"description"`
	AssigneeID  *string    `json:"assignee_id" validate:"omitempty,uuid4"`
	DueDate     *time.Time `json:"due_date"`
	Priority    string     `json:"priority" validate:"required,oneof=low medium high critical"`
	Label       *string    `json:"label"`
}

type UpdateKanbanTaskRequest struct {
	Title       string     `json:"title" validate:"required,min=2,max=160"`
	Description *string    `json:"description"`
	AssigneeID  *string    `json:"assignee_id" validate:"omitempty,uuid4"`
	DueDate     *time.Time `json:"due_date"`
	Priority    string     `json:"priority" validate:"required,oneof=low medium high critical"`
	Label       *string    `json:"label"`
}

type MoveKanbanTaskRequest struct {
	ColumnID string `json:"column_id" validate:"required,uuid4"`
	Position int    `json:"position" validate:"required,min=1"`
}

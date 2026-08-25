package operational

import "github.com/kana-consultant/kantor/backend/internal/model"

type KanbanTaskPage struct {
	Items   []model.KanbanTaskListItem `json:"items"`
	Total   int                `json:"total"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
	HasMore bool               `json:"has_more"`
}

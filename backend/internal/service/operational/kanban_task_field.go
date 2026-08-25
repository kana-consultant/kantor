package operational

import (
	"context"
	"strings"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

type kanbanTaskFieldRepository interface {
	ListByTask(ctx context.Context, taskID string) ([]model.KanbanTaskField, error)
	Replace(ctx context.Context, taskID string, params []operationalrepo.KanbanTaskFieldParams) error
}

func toTaskFieldParams(inputs []operationaldto.KanbanTaskFieldInput) []operationalrepo.KanbanTaskFieldParams {
	params := make([]operationalrepo.KanbanTaskFieldParams, 0, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		value := strings.TrimSpace(input.Value)
		if name == "" || value == "" {
			continue
		}
		params = append(params, operationalrepo.KanbanTaskFieldParams{Name: name, Value: value})
	}

	return params
}

func (s *KanbanService) attachTaskFields(ctx context.Context, task *model.KanbanTask) {
	if s.fieldsRepo == nil || task == nil {
		return
	}

	fields, err := s.fieldsRepo.ListByTask(ctx, task.ID)
	if err != nil {
		return
	}
	task.Fields = fields
}

func (s *KanbanService) replaceTaskFields(ctx context.Context, taskID string, inputs []operationaldto.KanbanTaskFieldInput) error {
	if s.fieldsRepo == nil {
		return nil
	}

	return s.fieldsRepo.Replace(ctx, taskID, toTaskFieldParams(inputs))
}

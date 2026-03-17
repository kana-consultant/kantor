package operational

import (
	"context"
	"errors"
	"strings"
	"time"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

var (
	ErrKanbanColumnNotFound = errors.New("kanban column not found")
	ErrKanbanTaskNotFound   = errors.New("kanban task not found")
)

type KanbanService struct {
	repo *operationalrepo.KanbanRepository
}

func NewKanbanService(repo *operationalrepo.KanbanRepository) *KanbanService {
	return &KanbanService{repo: repo}
}

func (s *KanbanService) CreateDefaultColumns(ctx context.Context, projectID string) error {
	return s.repo.CreateDefaultColumns(ctx, projectID)
}

func (s *KanbanService) ListColumns(ctx context.Context, projectID string) ([]model.KanbanColumn, error) {
	return s.repo.ListColumns(ctx, projectID)
}

func (s *KanbanService) CreateColumn(ctx context.Context, projectID string, request operationaldto.CreateKanbanColumnRequest) (model.KanbanColumn, error) {
	column, err := s.repo.CreateColumn(ctx, projectID, operationalrepo.CreateKanbanColumnParams{
		Name:     strings.TrimSpace(request.Name),
		Color:    normalizeStringPointer(request.Color),
		Position: request.Position,
	})
	if errors.Is(err, operationalrepo.ErrKanbanColumnNotFound) {
		return model.KanbanColumn{}, ErrKanbanColumnNotFound
	}

	return column, err
}

func (s *KanbanService) UpdateColumn(ctx context.Context, projectID string, columnID string, request operationaldto.UpdateKanbanColumnRequest) (model.KanbanColumn, error) {
	column, err := s.repo.UpdateColumn(ctx, projectID, columnID, operationalrepo.UpdateKanbanColumnParams{
		Name:  strings.TrimSpace(request.Name),
		Color: normalizeStringPointer(request.Color),
	})
	if errors.Is(err, operationalrepo.ErrKanbanColumnNotFound) {
		return model.KanbanColumn{}, ErrKanbanColumnNotFound
	}

	return column, err
}

func (s *KanbanService) DeleteColumn(ctx context.Context, projectID string, columnID string) error {
	err := s.repo.DeleteColumn(ctx, projectID, columnID)
	if errors.Is(err, operationalrepo.ErrKanbanColumnNotFound) {
		return ErrKanbanColumnNotFound
	}

	return err
}

func (s *KanbanService) ReorderColumns(ctx context.Context, projectID string, request operationaldto.ReorderKanbanColumnsRequest) error {
	err := s.repo.ReorderColumns(ctx, projectID, request.ColumnIDs)
	if errors.Is(err, operationalrepo.ErrKanbanColumnNotFound) {
		return ErrKanbanColumnNotFound
	}

	return err
}

func (s *KanbanService) ListTasks(ctx context.Context, projectID string) ([]model.KanbanTask, error) {
	return s.repo.ListTasks(ctx, projectID)
}

func (s *KanbanService) CreateTask(ctx context.Context, projectID string, request operationaldto.CreateKanbanTaskRequest, createdBy string) (model.KanbanTask, error) {
	task, err := s.repo.CreateTask(ctx, projectID, operationalrepo.CreateKanbanTaskParams{
		ColumnID:    request.ColumnID,
		Title:       strings.TrimSpace(request.Title),
		Description: normalizeStringPointer(request.Description),
		AssigneeID:  normalizeStringPointer(request.AssigneeID),
		DueDate:     normalizeTimePointer(request.DueDate),
		Priority:    request.Priority,
		Label:       normalizeStringPointer(request.Label),
		CreatedBy:   createdBy,
	})
	switch {
	case errors.Is(err, operationalrepo.ErrKanbanColumnNotFound):
		return model.KanbanTask{}, ErrKanbanColumnNotFound
	default:
		return task, err
	}
}

func (s *KanbanService) UpdateTask(ctx context.Context, projectID string, taskID string, request operationaldto.UpdateKanbanTaskRequest) (model.KanbanTask, error) {
	task, err := s.repo.UpdateTask(ctx, projectID, taskID, operationalrepo.UpdateKanbanTaskParams{
		Title:       strings.TrimSpace(request.Title),
		Description: normalizeStringPointer(request.Description),
		AssigneeID:  normalizeStringPointer(request.AssigneeID),
		DueDate:     normalizeTimePointer(request.DueDate),
		Priority:    request.Priority,
		Label:       normalizeStringPointer(request.Label),
	})
	switch {
	case errors.Is(err, operationalrepo.ErrKanbanTaskNotFound):
		return model.KanbanTask{}, ErrKanbanTaskNotFound
	default:
		return task, err
	}
}

func (s *KanbanService) DeleteTask(ctx context.Context, projectID string, taskID string) error {
	err := s.repo.DeleteTask(ctx, projectID, taskID)
	if errors.Is(err, operationalrepo.ErrKanbanTaskNotFound) {
		return ErrKanbanTaskNotFound
	}

	return err
}

func (s *KanbanService) MoveTask(ctx context.Context, projectID string, taskID string, request operationaldto.MoveKanbanTaskRequest) error {
	err := s.repo.MoveTask(ctx, projectID, taskID, request.ColumnID, request.Position)
	switch {
	case errors.Is(err, operationalrepo.ErrKanbanTaskNotFound):
		return ErrKanbanTaskNotFound
	case errors.Is(err, operationalrepo.ErrKanbanColumnNotFound):
		return ErrKanbanColumnNotFound
	default:
		return err
	}
}

func (s *KanbanService) Snapshot(ctx context.Context, projectID string) (operationalrepo.KanbanSnapshot, error) {
	return s.repo.Snapshot(ctx, projectID)
}

func normalizeTimePointer(value *time.Time) *string {
	if value == nil {
		return nil
	}

	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func normalizeStringPointer(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

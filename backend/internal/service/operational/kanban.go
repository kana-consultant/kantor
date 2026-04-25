package operational

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

// TaskAssignNotifier is called when a task is assigned to a user.
type TaskAssignNotifier interface {
	SendTaskAssignedNotification(ctx context.Context, taskID string, assigneeID string)
}

var (
	ErrKanbanColumnNotFound        = errors.New("kanban column not found")
	ErrKanbanTaskNotFound          = errors.New("kanban task not found")
	ErrKanbanTaskAssigneeNotMember = errors.New("task assignee must be a project member")
)

type kanbanRepository interface {
	CreateDefaultColumns(ctx context.Context, projectID string) error
	ListColumns(ctx context.Context, projectID string) ([]model.KanbanColumn, error)
	CreateColumn(ctx context.Context, projectID string, params operationalrepo.CreateKanbanColumnParams) (model.KanbanColumn, error)
	UpdateColumn(ctx context.Context, projectID string, columnID string, params operationalrepo.UpdateKanbanColumnParams) (model.KanbanColumn, error)
	DeleteColumn(ctx context.Context, projectID string, columnID string) error
	ReorderColumns(ctx context.Context, projectID string, columnIDs []string) error
	ListTasks(ctx context.Context, projectID string) ([]model.KanbanTask, error)
	CreateTask(ctx context.Context, projectID string, params operationalrepo.CreateKanbanTaskParams) (model.KanbanTask, error)
	UpdateTask(ctx context.Context, projectID string, taskID string, params operationalrepo.UpdateKanbanTaskParams) (model.KanbanTask, error)
	DeleteTask(ctx context.Context, projectID string, taskID string) error
	MoveTask(ctx context.Context, projectID string, taskID string, destinationColumnID string, destinationPosition int) error
	Snapshot(ctx context.Context, projectID string) (operationalrepo.KanbanSnapshot, error)
	GetTask(ctx context.Context, projectID string, taskID string) (model.KanbanTask, error)
}

type kanbanProjectsRepository interface {
	GetProjectByID(ctx context.Context, projectID string) (model.Project, error)
	ListMemberIDsOrdered(ctx context.Context, projectID string) ([]string, error)
	AdvanceCursor(ctx context.Context, projectID string, newCursor int) error
	GetMemberWorkloads(ctx context.Context, projectID string) ([]operationalrepo.MemberWorkload, error)
}

type KanbanService struct {
	repo         kanbanRepository
	projectsRepo kanbanProjectsRepository
	notifiers    []TaskAssignNotifier
}

func NewKanbanService(repo kanbanRepository, projectsRepo kanbanProjectsRepository) *KanbanService {
	return &KanbanService{repo: repo, projectsRepo: projectsRepo}
}

func (s *KanbanService) SetTaskAssignNotifier(n TaskAssignNotifier) {
	if n != nil {
		s.notifiers = append(s.notifiers, n)
	}
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
	assigneeID := normalizeStringPointer(request.AssigneeID)

	// Auto-assign: if no assignee specified, try to auto-assign based on project mode
	if (assigneeID == nil || *assigneeID == "") && s.projectsRepo != nil {
		if autoID := s.resolveAutoAssign(ctx, projectID); autoID != "" {
			assigneeID = &autoID
		}
	}
	if err := s.ensureTaskAssigneeIsProjectMember(ctx, projectID, assigneeID); err != nil {
		return model.KanbanTask{}, err
	}

	task, err := s.repo.CreateTask(ctx, projectID, operationalrepo.CreateKanbanTaskParams{
		ColumnID:    request.ColumnID,
		Title:       strings.TrimSpace(request.Title),
		Description: normalizeStringPointer(request.Description),
		AssigneeID:  assigneeID,
		DueDate:     normalizeTimePointer(request.DueDate),
		Priority:    request.Priority,
		Label:       normalizeStringPointer(request.Label),
		AssignedVia: createAssignedVia(request.AssigneeID, assigneeID),
		CreatedBy:   createdBy,
	})
	switch {
	case errors.Is(err, operationalrepo.ErrKanbanColumnNotFound):
		return model.KanbanTask{}, ErrKanbanColumnNotFound
	}
	if err != nil {
		return task, err
	}

	// Notify assignee (skip self-assign)
	if task.AssigneeID != nil && *task.AssigneeID != createdBy {
		slog.InfoContext(ctx,
			"dispatching task assigned notification",
			"task_id", task.ID,
			"assignee_id", *task.AssigneeID,
			"assigned_via", task.AssignedVia,
			"source", "create",
			"notifier_count", len(s.notifiers),
		)
		s.dispatchTaskAssignedNotification(ctx, task.ID, *task.AssigneeID)
	}

	return task, nil
}

func (s *KanbanService) UpdateTask(ctx context.Context, projectID string, taskID string, request operationaldto.UpdateKanbanTaskRequest, actorID string) (model.KanbanTask, error) {
	// Fetch old task to detect assignee changes
	var oldAssigneeID string
	if old, err := s.repo.GetTask(ctx, projectID, taskID); err == nil && old.AssigneeID != nil {
		oldAssigneeID = *old.AssigneeID
	}

	assigneeID := normalizeStringPointer(request.AssigneeID)

	// Auto-assign: if assignee is being removed (was set, now nil), try auto-assign
	if (assigneeID == nil || *assigneeID == "") && oldAssigneeID != "" && s.projectsRepo != nil {
		if autoID := s.resolveAutoAssign(ctx, projectID); autoID != "" {
			assigneeID = &autoID
		}
	}
	if err := s.ensureTaskAssigneeIsProjectMember(ctx, projectID, assigneeID); err != nil {
		return model.KanbanTask{}, err
	}

	task, err := s.repo.UpdateTask(ctx, projectID, taskID, operationalrepo.UpdateKanbanTaskParams{
		Title:       strings.TrimSpace(request.Title),
		Description: normalizeStringPointer(request.Description),
		AssigneeID:  assigneeID,
		DueDate:     normalizeTimePointer(request.DueDate),
		Priority:    request.Priority,
		Label:       normalizeStringPointer(request.Label),
		AssignedVia: updateAssignedVia(request.AssigneeID, assigneeID, oldAssigneeID),
	})
	switch {
	case errors.Is(err, operationalrepo.ErrKanbanTaskNotFound):
		return model.KanbanTask{}, ErrKanbanTaskNotFound
	}
	if err != nil {
		return task, err
	}

	// Notify new assignee if changed and not self-assign
	if task.AssigneeID != nil {
		newAssigneeID := *task.AssigneeID
		if newAssigneeID != oldAssigneeID && newAssigneeID != actorID {
			slog.InfoContext(ctx,
				"dispatching task assigned notification",
				"task_id", task.ID,
				"assignee_id", newAssigneeID,
				"previous_assignee_id", oldAssigneeID,
				"assigned_via", task.AssignedVia,
				"source", "update",
				"notifier_count", len(s.notifiers),
			)
			s.dispatchTaskAssignedNotification(ctx, task.ID, newAssigneeID)
		}
	}

	return task, nil
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

func (s *KanbanService) resolveAutoAssign(ctx context.Context, projectID string) string {
	project, err := s.projectsRepo.GetProjectByID(ctx, projectID)
	if err != nil || project.AutoAssignMode == "off" {
		return ""
	}

	switch project.AutoAssignMode {
	case "round_robin":
		members, err := s.projectsRepo.ListMemberIDsOrdered(ctx, projectID)
		if err != nil || len(members) == 0 {
			return ""
		}
		idx := project.AutoAssignCursor % len(members)
		selected := members[idx]
		_ = s.projectsRepo.AdvanceCursor(ctx, projectID, idx+1)
		return selected

	case "least_busy":
		workloads, err := s.projectsRepo.GetMemberWorkloads(ctx, projectID)
		if err != nil || len(workloads) == 0 {
			return ""
		}
		return workloads[0].UserID

	default:
		return ""
	}
}

func (s *KanbanService) ensureTaskAssigneeIsProjectMember(ctx context.Context, projectID string, assigneeID *string) error {
	if assigneeID == nil {
		return nil
	}

	trimmedAssigneeID := strings.TrimSpace(*assigneeID)
	if trimmedAssigneeID == "" || s.projectsRepo == nil {
		return nil
	}

	memberIDs, err := s.projectsRepo.ListMemberIDsOrdered(ctx, projectID)
	if err != nil {
		return err
	}
	for _, memberID := range memberIDs {
		if memberID == trimmedAssigneeID {
			return nil
		}
	}

	return ErrKanbanTaskAssigneeNotMember
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

func (s *KanbanService) dispatchTaskAssignedNotification(ctx context.Context, taskID string, assigneeID string) {
	if len(s.notifiers) == 0 {
		slog.WarnContext(ctx, "task assigned notification skipped because no notifier is registered", "task_id", taskID, "assignee_id", assigneeID)
		return
	}

	notificationCtx := platformmiddleware.DetachTenantContext(ctx)
	for _, notifier := range s.notifiers {
		if notifier == nil {
			continue
		}
		currentNotifier := notifier
		go func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					slog.ErrorContext(notificationCtx,
						"task assigned notifier panicked",
						"task_id", taskID,
						"assignee_id", assigneeID,
						"notifier_type", fmt.Sprintf("%T", currentNotifier),
						"panic", recovered,
					)
				}
			}()
			currentNotifier.SendTaskAssignedNotification(notificationCtx, taskID, assigneeID)
		}()
	}
}

func createAssignedVia(requestAssigneeID *string, resolvedAssigneeID *string) string {
	if isBlankStringPointer(requestAssigneeID) && !isBlankStringPointer(resolvedAssigneeID) {
		return model.KanbanTaskAssignedViaAuto
	}
	return model.KanbanTaskAssignedViaManual
}

func updateAssignedVia(requestAssigneeID *string, resolvedAssigneeID *string, oldAssigneeID string) string {
	if isBlankStringPointer(resolvedAssigneeID) || valueOrEmpty(resolvedAssigneeID) == oldAssigneeID {
		return model.KanbanTaskAssignedViaManual
	}
	if isBlankStringPointer(requestAssigneeID) {
		return model.KanbanTaskAssignedViaAuto
	}
	return model.KanbanTaskAssignedViaManual
}

func isBlankStringPointer(value *string) bool {
	return value == nil || strings.TrimSpace(*value) == ""
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

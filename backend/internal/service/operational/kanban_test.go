package operational

import (
	"context"
	"testing"
	"time"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

func TestCreateTaskAutoAssignDispatchesNotification(t *testing.T) {
	t.Parallel()

	repo := &fakeKanbanRepository{
		createTaskFunc: func(ctx context.Context, projectID string, params operationalrepo.CreateKanbanTaskParams) (model.KanbanTask, error) {
			if params.AssignedVia != model.KanbanTaskAssignedViaAuto {
				t.Fatalf("CreateTask() assigned via = %q, want %q", params.AssignedVia, model.KanbanTaskAssignedViaAuto)
			}
			if params.AssigneeID == nil || *params.AssigneeID != "assignee-1" {
				t.Fatalf("CreateTask() assignee = %v, want assignee-1", params.AssigneeID)
			}

			return model.KanbanTask{
				ID:          "task-1",
				ProjectID:   projectID,
				AssigneeID:  stringPtr("assignee-1"),
				AssignedVia: model.KanbanTaskAssignedViaAuto,
			}, nil
		},
	}
	projectsRepo := &fakeKanbanProjectsRepository{
		project:   model.Project{AutoAssignMode: "round_robin", AutoAssignCursor: 0},
		memberIDs: []string{"assignee-1"},
	}
	notifier := &recordingTaskAssignNotifier{calls: make(chan taskAssignCall, 1)}

	service := NewKanbanService(repo, projectsRepo)
	service.SetTaskAssignNotifier(notifier)

	_, err := service.CreateTask(context.Background(), "project-1", operationaldto.CreateKanbanTaskRequest{
		ColumnID: "column-1",
		Title:    "Auto assigned task",
		Priority: "medium",
	}, "creator-1")
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	assertTaskAssignCall(t, notifier.calls, taskAssignCall{
		TaskID:     "task-1",
		AssigneeID: "assignee-1",
	})
}

func TestUpdateTaskReassignDispatchesNotification(t *testing.T) {
	t.Parallel()

	repo := &fakeKanbanRepository{
		getTaskFunc: func(ctx context.Context, projectID string, taskID string) (model.KanbanTask, error) {
			return model.KanbanTask{
				ID:         taskID,
				ProjectID:  projectID,
				AssigneeID: stringPtr("assignee-old"),
			}, nil
		},
		updateTaskFunc: func(ctx context.Context, projectID string, taskID string, params operationalrepo.UpdateKanbanTaskParams) (model.KanbanTask, error) {
			if params.AssignedVia != model.KanbanTaskAssignedViaManual {
				t.Fatalf("UpdateTask() assigned via = %q, want %q", params.AssignedVia, model.KanbanTaskAssignedViaManual)
			}
			if params.AssigneeID == nil || *params.AssigneeID != "assignee-new" {
				t.Fatalf("UpdateTask() assignee = %v, want assignee-new", params.AssigneeID)
			}

			return model.KanbanTask{
				ID:          taskID,
				ProjectID:   projectID,
				AssigneeID:  stringPtr("assignee-new"),
				AssignedVia: model.KanbanTaskAssignedViaManual,
			}, nil
		},
	}
	notifier := &recordingTaskAssignNotifier{calls: make(chan taskAssignCall, 1)}

	service := NewKanbanService(repo, &fakeKanbanProjectsRepository{
		memberIDs: []string{"assignee-old", "assignee-new"},
	})
	service.SetTaskAssignNotifier(notifier)

	_, err := service.UpdateTask(context.Background(), "project-1", "task-1", operationaldto.UpdateKanbanTaskRequest{
		Title:      "Reassigned task",
		AssigneeID: stringPtr("assignee-new"),
		Priority:   "high",
	}, "actor-1")
	if err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}

	assertTaskAssignCall(t, notifier.calls, taskAssignCall{
		TaskID:     "task-1",
		AssigneeID: "assignee-new",
	})
}

func TestCreateTaskSelfAssignDoesNotDispatchNotification(t *testing.T) {
	t.Parallel()

	repo := &fakeKanbanRepository{
		createTaskFunc: func(ctx context.Context, projectID string, params operationalrepo.CreateKanbanTaskParams) (model.KanbanTask, error) {
			return model.KanbanTask{
				ID:         "task-1",
				ProjectID:  projectID,
				AssigneeID: stringPtr("creator-1"),
			}, nil
		},
	}
	notifier := &recordingTaskAssignNotifier{calls: make(chan taskAssignCall, 1)}

	service := NewKanbanService(repo, &fakeKanbanProjectsRepository{
		memberIDs: []string{"creator-1"},
	})
	service.SetTaskAssignNotifier(notifier)

	_, err := service.CreateTask(context.Background(), "project-1", operationaldto.CreateKanbanTaskRequest{
		ColumnID:   "column-1",
		Title:      "Self assigned task",
		AssigneeID: stringPtr("creator-1"),
		Priority:   "medium",
	}, "creator-1")
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	select {
	case call := <-notifier.calls:
		t.Fatalf("unexpected task assignment notification: %+v", call)
	case <-time.After(100 * time.Millisecond):
	}
}

type fakeKanbanRepository struct {
	createTaskFunc func(ctx context.Context, projectID string, params operationalrepo.CreateKanbanTaskParams) (model.KanbanTask, error)
	updateTaskFunc func(ctx context.Context, projectID string, taskID string, params operationalrepo.UpdateKanbanTaskParams) (model.KanbanTask, error)
	getTaskFunc    func(ctx context.Context, projectID string, taskID string) (model.KanbanTask, error)
}

func (f *fakeKanbanRepository) CreateDefaultColumns(ctx context.Context, projectID string) error {
	return nil
}

func (f *fakeKanbanRepository) ListColumns(ctx context.Context, projectID string) ([]model.KanbanColumn, error) {
	return nil, nil
}

func (f *fakeKanbanRepository) CreateColumn(ctx context.Context, projectID string, params operationalrepo.CreateKanbanColumnParams) (model.KanbanColumn, error) {
	return model.KanbanColumn{}, nil
}

func (f *fakeKanbanRepository) UpdateColumn(ctx context.Context, projectID string, columnID string, params operationalrepo.UpdateKanbanColumnParams) (model.KanbanColumn, error) {
	return model.KanbanColumn{}, nil
}

func (f *fakeKanbanRepository) DeleteColumn(ctx context.Context, projectID string, columnID string) error {
	return nil
}

func (f *fakeKanbanRepository) ReorderColumns(ctx context.Context, projectID string, columnIDs []string) error {
	return nil
}

func (f *fakeKanbanRepository) ListTasks(ctx context.Context, projectID string) ([]model.KanbanTask, error) {
	return nil, nil
}

func (f *fakeKanbanRepository) CreateTask(ctx context.Context, projectID string, params operationalrepo.CreateKanbanTaskParams) (model.KanbanTask, error) {
	if f.createTaskFunc == nil {
		return model.KanbanTask{}, nil
	}
	return f.createTaskFunc(ctx, projectID, params)
}

func (f *fakeKanbanRepository) UpdateTask(ctx context.Context, projectID string, taskID string, params operationalrepo.UpdateKanbanTaskParams) (model.KanbanTask, error) {
	if f.updateTaskFunc == nil {
		return model.KanbanTask{}, nil
	}
	return f.updateTaskFunc(ctx, projectID, taskID, params)
}

func (f *fakeKanbanRepository) DeleteTask(ctx context.Context, projectID string, taskID string) error {
	return nil
}

func (f *fakeKanbanRepository) MoveTask(ctx context.Context, projectID string, taskID string, destinationColumnID string, destinationPosition int) error {
	return nil
}

func (f *fakeKanbanRepository) Snapshot(ctx context.Context, projectID string) (operationalrepo.KanbanSnapshot, error) {
	return operationalrepo.KanbanSnapshot{}, nil
}

func (f *fakeKanbanRepository) GetTask(ctx context.Context, projectID string, taskID string) (model.KanbanTask, error) {
	if f.getTaskFunc == nil {
		return model.KanbanTask{}, nil
	}
	return f.getTaskFunc(ctx, projectID, taskID)
}

type fakeKanbanProjectsRepository struct {
	project   model.Project
	memberIDs []string
}

func (f *fakeKanbanProjectsRepository) GetProjectByID(ctx context.Context, projectID string) (model.Project, error) {
	return f.project, nil
}

func (f *fakeKanbanProjectsRepository) ListMemberIDsOrdered(ctx context.Context, projectID string) ([]string, error) {
	return append([]string(nil), f.memberIDs...), nil
}

func (f *fakeKanbanProjectsRepository) AdvanceCursor(ctx context.Context, projectID string, newCursor int) error {
	return nil
}

func (f *fakeKanbanProjectsRepository) GetMemberWorkloads(ctx context.Context, projectID string) ([]operationalrepo.MemberWorkload, error) {
	return nil, nil
}

type recordingTaskAssignNotifier struct {
	calls chan taskAssignCall
}

type taskAssignCall struct {
	TaskID     string
	AssigneeID string
}

func (n *recordingTaskAssignNotifier) SendTaskAssignedNotification(ctx context.Context, taskID string, assigneeID string) {
	n.calls <- taskAssignCall{TaskID: taskID, AssigneeID: assigneeID}
}

func assertTaskAssignCall(t *testing.T, calls <-chan taskAssignCall, want taskAssignCall) {
	t.Helper()

	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("task assignment notification = %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for task assignment notification %+v", want)
	}
}

func stringPtr(value string) *string {
	return &value
}

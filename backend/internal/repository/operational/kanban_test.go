package operational

import (
	"reflect"
	"testing"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

func TestDefaultKanbanColumns(t *testing.T) {
	t.Parallel()

	got := defaultKanbanColumns()
	want := []kanbanColumnSeed{
		{Name: "Backlog", Color: "#94A3B8", ColumnType: model.KanbanColumnTypeTodo},
		{Name: "To Do", Color: "#38BDF8", ColumnType: model.KanbanColumnTypeTodo},
		{Name: "In Progress", Color: "#F59E0B", ColumnType: model.KanbanColumnTypeInProgress},
		{Name: "Review", Color: "#8B5CF6", ColumnType: model.KanbanColumnTypeCustom},
		{Name: "Done", Color: "#22C55E", ColumnType: model.KanbanColumnTypeDone},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("defaultKanbanColumns() = %#v, want %#v", got, want)
	}
}

import { useEffect } from "react";

import { KanbanColumnCard } from "@/components/shared/kanban-cards";
import type { KanbanTaskQuery } from "@/services/operational-kanban";
import { sortTaskList } from "@/components/shared/kanban-dnd";
import { useKanbanColumnTasks } from "@/hooks/use-kanban-column-tasks";
import type { KanbanColumn, KanbanTask } from "@/types/kanban";

interface KanbanColumnContainerProps {
	projectId: string;
	column: KanbanColumn;
	filters: KanbanTaskQuery;
	onTasksLoaded: (columnId: string, tasks: KanbanTask[]) => void;
	onTaskClick: (task: KanbanTask) => void;
	onTaskCreate: () => void;
	onEditColumn: () => void;
	onDeleteColumn: () => void;
	quickDraft: string;
	onQuickDraftChange: (value: string) => void;
	onQuickAdd: () => void;
}

export function KanbanColumnContainer({
	projectId,
	column,
	filters,
	onTasksLoaded,
	...rest
}: KanbanColumnContainerProps) {
	const { tasks, total, hasMore, isLoading, isFetchingMore, fetchMore } =
		useKanbanColumnTasks(projectId, column.id, filters);

	useEffect(() => {
		onTasksLoaded(column.id, tasks);
	}, [column.id, onTasksLoaded, tasks]);

	const visibleTasks = [...tasks].sort(sortTaskList);

	return (
		<KanbanColumnCard
			column={column}
			hasMore={hasMore}
			isFetchingMore={isFetchingMore}
			isLoadingTasks={isLoading}
			onLoadMore={fetchMore}
			tasks={visibleTasks}
			totalTasks={total}
			{...rest}
		/>
	);
}

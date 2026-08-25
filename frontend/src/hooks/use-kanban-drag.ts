import { useState } from "react";
import type { DragEndEvent, DragStartEvent } from "@dnd-kit/core";
import { useQueryClient } from "@tanstack/react-query";

import {
	finishColumnDrag,
	finishTaskDrag,
	isColumnDragData,
	isTaskDragData,
	moveTaskInMemory,
	setTasksCache,
	type DragSnapshot,
} from "@/components/shared/kanban-dnd";
import { kanbanKeys } from "@/services/operational-kanban";
import type {
	KanbanColumn,
	KanbanTask,
} from "@/types/kanban";

export function useKanbanDrag(
	projectId: string,
	columns: KanbanColumn[],
	tasks: KanbanTask[],
) {
	const queryClient = useQueryClient();
	const [activeTask, setActiveTask] = useState<KanbanTask | null>(null);
	const [activeColumn, setActiveColumn] = useState<KanbanColumn | null>(null);
	const [dragSnapshot, setDragSnapshot] = useState<DragSnapshot | null>(null);

	function resetDrag() {
		setActiveTask(null);
		setActiveColumn(null);
		setDragSnapshot(null);
	}

	function rollbackDrag() {
		if (!dragSnapshot) {
			return;
		}

		queryClient.setQueryData(
			kanbanKeys.columns(projectId),
			dragSnapshot.columns,
		);
		setTasksCache(queryClient, projectId, dragSnapshot.tasks);
	}

	function handleDragStart(event: DragStartEvent) {
		const data = event.active.data.current;

		if (isTaskDragData(data)) {
			setActiveTask(data.task);
			setDragSnapshot({ columns, tasks });
			return;
		}

		if (isColumnDragData(data)) {
			setActiveColumn(data.column);
			setDragSnapshot({ columns, tasks });
		}
	}

	function handleDragEnd(event: DragEndEvent) {
		if (!event.over) {
			rollbackDrag();
			resetDrag();
			return;
		}

		const activeData = event.active.data.current;

		if (isColumnDragData(activeData)) {
			finishColumnDrag(
				event,
				columns,
				tasks,
				dragSnapshot,
				queryClient,
				projectId,
			);
			resetDrag();
			return;
		}

		if (isTaskDragData(activeData)) {
			const nextTasks = moveTaskInMemory(
				dragSnapshot?.tasks ?? tasks,
				activeData.task.id,
				event.over.data.current,
				event.over.id.toString(),
			);

			finishTaskDrag(
				activeData.task.id,
				nextTasks,
				dragSnapshot,
				queryClient,
				projectId,
			);
		}

		resetDrag();
	}

	return { activeTask, activeColumn, handleDragStart, handleDragEnd };
}

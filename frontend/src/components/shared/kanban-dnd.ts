import type { DragEndEvent } from "@dnd-kit/core";
import { arrayMove } from "@dnd-kit/sortable";
import type { useQueryClient } from "@tanstack/react-query";

import { extractDateInputValue } from "@/lib/date";
import {
	kanbanKeys,
	moveKanbanTask,
	reorderKanbanColumns,
} from "@/services/operational-kanban";
import type {
	KanbanColumn,
	KanbanFilters,
	KanbanTask,
} from "@/types/kanban";

export interface DragSnapshot {
	columns: KanbanColumn[];
	tasks: KanbanTask[];
}

export type DragTaskData = { type: "task"; task: KanbanTask };
export type DragColumnData = { type: "column"; column: KanbanColumn };

export function matchesFilters(task: KanbanTask, filters: KanbanFilters) {
	if (filters.assignee && task.assignee_id !== filters.assignee) {
		return false;
	}
	if (filters.priority && task.priority !== filters.priority) {
		return false;
	}
	if (filters.label) {
		const label = task.label?.toLowerCase() ?? "";
		if (!label.includes(filters.label.toLowerCase())) {
			return false;
		}
	}
	if (
		filters.dueDate &&
		(!task.due_date || extractDateInputValue(task.due_date) !== filters.dueDate)
	) {
		return false;
	}
	return true;
}

export function moveTaskInMemory(
	tasks: KanbanTask[],
	activeTaskId: string,
	overData: unknown,
	overId: string,
) {
	const activeTask = tasks.find((task) => task.id === activeTaskId);
	if (!activeTask) {
		return null;
	}

	const buckets = buildTaskBuckets(tasks);
	const sourceList = [...(buckets[activeTask.column_id] ?? [])];
	const sourceIndex = sourceList.findIndex((task) => task.id === activeTaskId);
	if (sourceIndex < 0) {
		return null;
	}

	const [movingTask] = sourceList.splice(sourceIndex, 1);
	buckets[activeTask.column_id] = sourceList;

	let destinationColumnId = activeTask.column_id;
	let destinationIndex = sourceList.length;

	if (isTaskDragData(overData)) {
		destinationColumnId = overData.task.column_id;
		const targetList =
			destinationColumnId === activeTask.column_id
				? sourceList
				: [...(buckets[destinationColumnId] ?? [])];
		const overIndex = targetList.findIndex(
			(task) => task.id === overData.task.id,
		);
		destinationIndex = overIndex >= 0 ? overIndex : targetList.length;
	} else if (isColumnDragData(overData)) {
		destinationColumnId = overData.column.id;
		const targetList =
			destinationColumnId === activeTask.column_id
				? sourceList
				: [...(buckets[destinationColumnId] ?? [])];
		destinationIndex = targetList.length;
	} else {
		const targetList = buckets[activeTask.column_id] ?? [];
		const overIndex = targetList.findIndex((task) => task.id === overId);
		destinationIndex = overIndex >= 0 ? overIndex : targetList.length;
	}

	const destinationList =
		destinationColumnId === activeTask.column_id
			? sourceList
			: [...(buckets[destinationColumnId] ?? [])];
	destinationList.splice(destinationIndex, 0, {
		...movingTask!,
		column_id: destinationColumnId,
	});
	buckets[destinationColumnId] = destinationList;

	return flattenTaskBuckets(buckets);
}

function buildTaskBuckets(tasks: KanbanTask[]) {
	const buckets: Record<string, KanbanTask[]> = {};
	for (const task of [...tasks].sort(sortTaskList)) {
		if (!buckets[task.column_id]) {
			buckets[task.column_id] = [];
		}
		buckets[task.column_id]!.push(task);
	}
	return buckets;
}

function flattenTaskBuckets(buckets: Record<string, KanbanTask[]>) {
	const nextTasks: KanbanTask[] = [];
	for (const columnId of Object.keys(buckets)) {
		buckets[columnId]!.forEach((task, index) => {
			nextTasks.push({ ...task, column_id: columnId, position: index + 1 });
		});
	}
	return nextTasks.sort(sortTaskList);
}

export function sortTaskList(
	left: { column_id: string; position: number },
	right: { column_id: string; position: number },
) {
	if (left.column_id === right.column_id) {
		return left.position - right.position;
	}
	return left.column_id.localeCompare(right.column_id);
}

interface TaskPageShape {
	items: KanbanTask[];
	total: number;
	limit: number;
	offset: number;
	has_more: boolean;
}

interface TaskInfiniteShape {
	pages: TaskPageShape[];
	pageParams: unknown[];
}

export function setTasksCache(
	queryClient: ReturnType<typeof useQueryClient>,
	projectId: string,
	tasks: KanbanTask[],
) {
	const byId = new Map(tasks.map((task) => [task.id, task]));
	const entries = queryClient.getQueriesData<TaskInfiniteShape>({
		queryKey: kanbanKeys.tasks(projectId),
	});

	for (const [queryKey, current] of entries) {
		if (!current?.pages) {
			continue;
		}

		const columnIndex = queryKey.indexOf("column");
		const columnId =
			columnIndex >= 0 ? (queryKey[columnIndex + 1] as string) : undefined;

		const loaded = current.pages.flatMap((page) => page.items);
		const known = new Set(loaded.map((item) => item.id));
		const next = loaded
			.map((item) => byId.get(item.id) ?? item)
			.filter((item) => !columnId || item.column_id === columnId);

		if (columnId) {
			for (const task of tasks) {
				if (task.column_id === columnId && !known.has(task.id)) {
					next.push(task);
				}
			}
		}
		next.sort(sortTaskList);

		let cursor = 0;
		queryClient.setQueryData<TaskInfiniteShape>(queryKey, {
			...current,
			pages: current.pages.map((page, index) => {
				const size =
					index === current.pages.length - 1
						? next.length - cursor
						: page.items.length;
				const items = next.slice(cursor, cursor + Math.max(size, 0));
				cursor += items.length;
				return { ...page, items };
			}),
		});
	}
}

export function getTasksCache(
	queryClient: ReturnType<typeof useQueryClient>,
	projectId: string,
) {
	const entries = queryClient.getQueriesData<TaskInfiniteShape>({
		queryKey: kanbanKeys.tasks(projectId),
	});

	const tasks: KanbanTask[] = [];
	for (const [, data] of entries) {
		for (const page of data?.pages ?? []) {
			tasks.push(...page.items);
		}
	}

	return tasks.length > 0 ? tasks : undefined;
}

export async function invalidateBoard(
	queryClient: ReturnType<typeof useQueryClient>,
	projectId: string,
) {
	await queryClient.invalidateQueries({ queryKey: kanbanKeys.all(projectId) });
}

export function finishTaskDrag(
	taskId: string,
	nextTasks: KanbanTask[] | null,
	snapshot: DragSnapshot | null,
	queryClient: ReturnType<typeof useQueryClient>,
	projectId: string,
) {
	if (!snapshot || !nextTasks) {
		return;
	}

	const previousTask = snapshot.tasks.find((task) => task.id === taskId);
	const nextTask = nextTasks.find((task) => task.id === taskId);

	if (!previousTask || !nextTask) {
		setTasksCache(queryClient, projectId, snapshot.tasks);
		return;
	}

	if (
		previousTask.column_id === nextTask.column_id &&
		previousTask.position === nextTask.position
	) {
		return;
	}

	setTasksCache(queryClient, projectId, nextTasks);
	void moveKanbanTask(
		projectId,
		nextTask.id,
		nextTask.column_id,
		nextTask.position,
	)
		.then(async () => {
			await invalidateBoard(queryClient, projectId);
		})
		.catch(async () => {
			setTasksCache(queryClient, projectId, snapshot.tasks);
			await invalidateBoard(queryClient, projectId);
		});
}

export function finishColumnDrag(
	event: DragEndEvent,
	columns: KanbanColumn[],
	tasks: KanbanTask[],
	snapshot: DragSnapshot | null,
	queryClient: ReturnType<typeof useQueryClient>,
	projectId: string,
) {
	if (!snapshot) {
		return;
	}

	const currentColumns =
		queryClient.getQueryData<KanbanColumn[]>(kanbanKeys.columns(projectId)) ??
		columns;
	const activeIndex = currentColumns.findIndex(
		(column) => column.id === event.active.id,
	);
	const currentTasks =
		getTasksCache(queryClient, projectId) ??
		tasks;
	const overColumnId = resolveOverColumnId(event, currentColumns, currentTasks);
	const overIndex = currentColumns.findIndex(
		(column) => column.id === overColumnId,
	);
	if (activeIndex < 0 || overIndex < 0 || activeIndex === overIndex) {
		return;
	}

	const nextColumns = arrayMove(currentColumns, activeIndex, overIndex).map(
		(column, index) => ({
			...column,
			position: index + 1,
		}),
	);

	queryClient.setQueryData(kanbanKeys.columns(projectId), nextColumns);
	void reorderKanbanColumns(
		projectId,
		nextColumns.map((column) => column.id),
	)
		.then(async () => {
			await invalidateBoard(queryClient, projectId);
		})
		.catch(async () => {
			queryClient.setQueryData(kanbanKeys.columns(projectId), snapshot.columns);
			await invalidateBoard(queryClient, projectId);
		});
}

function resolveOverColumnId(
	event: DragEndEvent,
	columns: KanbanColumn[],
	tasks: KanbanTask[],
) {
	const overData = event.over?.data.current;
	if (isColumnDragData(overData)) {
		return overData.column.id;
	}

	const overId = event.over?.id?.toString();
	if (!overId) {
		return null;
	}

	if (columns.some((column) => column.id === overId)) {
		return overId;
	}

	return tasks.find((task) => task.id === overId)?.column_id ?? null;
}

export function isTaskDragData(value: unknown): value is DragTaskData {
	return (
		typeof value === "object" &&
		value !== null &&
		"type" in value &&
		value.type === "task"
	);
}

export function isColumnDragData(value: unknown): value is DragColumnData {
	return (
		typeof value === "object" &&
		value !== null &&
		"type" in value &&
		value.type === "column"
	);
}

export function extractErrorMessage(error: unknown) {
	return error instanceof Error
		? error.message
		: "Terjadi kesalahan yang tidak terduga";
}

import { useMutation, useQueryClient } from "@tanstack/react-query";

import {
	extractErrorMessage,
	invalidateBoard,
} from "@/components/shared/kanban-dnd";
import {
	createKanbanColumn,
	createKanbanTask,
	deleteKanbanColumn,
	deleteKanbanTask,
	updateKanbanColumn,
	updateKanbanTask,
} from "@/services/operational-kanban";
import type { TaskFormValues } from "@/types/kanban";

interface KanbanMutationCallbacks {
	onError: (message: string) => void;
	onColumnSaved: () => void;
	onColumnDeleted: () => void;
	onTaskSaved: () => void;
	onTaskDeleted: () => void;
}

export function useKanbanMutations(
	projectId: string,
	callbacks: KanbanMutationCallbacks,
) {
	const queryClient = useQueryClient();
	const handleError = (error: unknown) =>
		callbacks.onError(extractErrorMessage(error));

	const createColumn = useMutation({
		mutationFn: (payload: { name: string; color?: string }) =>
			createKanbanColumn(projectId, payload),
		onSuccess: async () => {
			callbacks.onColumnSaved();
			await invalidateBoard(queryClient, projectId);
		},
		onError: handleError,
	});

	const updateColumn = useMutation({
		mutationFn: (payload: { columnId: string; name: string; color?: string }) =>
			updateKanbanColumn(projectId, payload.columnId, {
				name: payload.name,
				color: payload.color,
			}),
		onSuccess: async () => {
			callbacks.onColumnSaved();
			await invalidateBoard(queryClient, projectId);
		},
		onError: handleError,
	});

	const deleteColumn = useMutation({
		mutationFn: (columnId: string) => deleteKanbanColumn(projectId, columnId),
		onSuccess: async () => {
			callbacks.onColumnDeleted();
			await invalidateBoard(queryClient, projectId);
		},
		onError: handleError,
	});

	const createTask = useMutation({
		mutationFn: (payload: { column_id: string } & TaskFormValues) =>
			createKanbanTask(projectId, payload),
		onSuccess: async () => {
			callbacks.onTaskSaved();
			await invalidateBoard(queryClient, projectId);
		},
		onError: handleError,
	});

	const updateTask = useMutation({
		mutationFn: (payload: { taskId: string; values: TaskFormValues }) =>
			updateKanbanTask(projectId, payload.taskId, payload.values),
		onSuccess: async () => {
			callbacks.onTaskSaved();
			await invalidateBoard(queryClient, projectId);
		},
		onError: handleError,
	});

	const deleteTask = useMutation({
		mutationFn: (taskId: string) => deleteKanbanTask(projectId, taskId),
		onSuccess: async () => {
			callbacks.onTaskDeleted();
			await invalidateBoard(queryClient, projectId);
		},
		onError: handleError,
	});

	return {
		createColumn,
		updateColumn,
		deleteColumn,
		createTask,
		updateTask,
		deleteTask,
	};
}

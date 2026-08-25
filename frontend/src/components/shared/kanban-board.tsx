import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import {
	DndContext,
	DragOverlay,
	PointerSensor,
	closestCorners,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import {
	SortableContext,
	horizontalListSortingStrategy,
} from "@dnd-kit/sortable";
import { useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";

import {
	KanbanDialogs,
	type ColumnModalState,
} from "@/components/shared/kanban-dialogs";
import { KanbanToolbar } from "@/components/shared/kanban-toolbar";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useKanbanDrag } from "@/hooks/use-kanban-drag";
import { useKanbanMutations } from "@/hooks/use-kanban-mutations";
import {
	ColumnOverlay,
	TaskOverlay,
} from "@/components/shared/kanban-cards";
import { KanbanColumnContainer } from "@/components/shared/kanban-column-container";
import {
	KanbanDialogs,
	type ColumnModalState,
} from "@/components/shared/kanban-dialogs";
import { useKanbanDrag } from "@/hooks/use-kanban-drag";
import { useKanbanMutations } from "@/hooks/use-kanban-mutations";
import { KanbanToolbar } from "@/components/shared/kanban-toolbar";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { TaskModal } from "@/components/shared/task-modal";
import { Card } from "@/components/ui/card";
import { extractDateInputValue } from "@/lib/date";
import {
	getKanbanTask,
	kanbanKeys,
	listKanbanColumns,
} from "@/services/operational-kanban";
import type {
	KanbanColumn,
	KanbanFilters,
	KanbanTaskListItem,
	TaskFormValues,
} from "@/types/kanban";
import type { ProjectMember } from "@/types/project";

const taskSchema = z.object({
	title: z.string().trim().min(2, "Judul minimal 2 karakter").max(160),
	description: z.string(),
	assignee_id: z.string(),
	due_date: z.string(),
	priority: z.enum(["low", "medium", "high", "critical"]),
	label: z.string(),
	fields: z.array(z.object({ name: z.string(), value: z.string() })),
});

const emptyTaskForm: TaskFormValues = {
	title: "",
	description: "",
	assignee_id: "",
	due_date: "",
	priority: "medium",
	label: "",
	fields: [],
};

const columnColorOptions = [
	"#38BDF8",
	"#10B981",
	"#F97316",
	"#F43F5E",
	"#8B5CF6",
	"#FACC15",
];

interface KanbanBoardProps {
	projectId: string;
	members: ProjectMember[];
}

type TaskModalState =
	| { mode: "create"; columnId: string }
	| { mode: "edit"; columnId: string; taskId: string };

export function KanbanBoard({ projectId, members }: KanbanBoardProps) {
	const sensors = useSensors(
		useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
	);
	const [filters, setFilters] = useState<KanbanFilters>({
		assignee: "",
		priority: "",
		label: "",
		dueDate: "",
		search: "",
		columnId: "",
	});
	const debouncedSearch = useDebouncedValue(filters.search, 500);
	const searchTerm = debouncedSearch.trim();
	const [quickDrafts, setQuickDrafts] = useState<Record<string, string>>({});
	const [columnModal, setColumnModal] = useState<ColumnModalState | null>(null);
	const [columnForm, setColumnForm] = useState({ name: "", color: "#38BDF8" });
	const [taskModal, setTaskModal] = useState<TaskModalState | null>(null);
	const [boardError, setBoardError] = useState<string | null>(null);
	const [columnToDelete, setColumnToDelete] = useState<KanbanColumn | null>(
		null,
	);
	const [taskToDelete, setTaskToDelete] = useState<KanbanTaskListItem | null>(null);

	const columnsQuery = useQuery({
		queryKey: kanbanKeys.columns(projectId),
		queryFn: () => listKanbanColumns(projectId),
	});

	const [loadedTasks, setLoadedTasks] = useState<Record<string, KanbanTaskListItem[]>>(
		{},
	);

	const handleTasksLoaded = useCallback(
		(columnId: string, columnTasks: KanbanTaskListItem[]) => {
			setLoadedTasks((current) =>
				current[columnId] === columnTasks
					? current
					: { ...current, [columnId]: columnTasks },
			);
		},
		[],
	);

	const form = useForm<TaskFormValues>({
		resolver: zodResolver(taskSchema),
		defaultValues: emptyTaskForm,
	});

	const {
		createColumn: createColumnMutation,
		updateColumn: updateColumnMutation,
		deleteColumn: deleteColumnMutation,
		createTask: createTaskMutation,
		updateTask: updateTaskMutation,
		deleteTask: deleteTaskMutation,
	} = useKanbanMutations(projectId, {
		onError: setBoardError,
		onColumnSaved: () => {
			setColumnForm({ name: "", color: "#38BDF8" });
			setColumnModal(null);
		},
		onColumnDeleted: () => setColumnToDelete(null),
		onTaskSaved: () => {
			form.reset(emptyTaskForm);
			setTaskModal(null);
		},
		onTaskDeleted: () => {
			form.reset(emptyTaskForm);
			setTaskModal(null);
			setTaskToDelete(null);
		},
	});

	const columns = columnsQuery.data ?? [];
	const visibleColumnIds = useMemo(
		() =>
			columns
				.filter((column) => !filters.columnId || column.id === filters.columnId)
				.map((column) => column.id),
		[columns, filters.columnId],
	);
	// Derive the working task list from only the currently-visible columns so a
	// hidden (column filter) or deleted column's stale loadedTasks entries do not
	// leak into drag state, the overlay count, or the task-modal lookup.
	const tasks = useMemo(
		() => visibleColumnIds.flatMap((columnId) => loadedTasks[columnId] ?? []),
		[loadedTasks, visibleColumnIds],
	);
	const { activeTask, activeColumn, handleDragStart, handleDragEnd } =
		useKanbanDrag(projectId, columns, tasks);

	const taskFilters = useMemo(
		() => ({
			search: searchTerm,
			assigneeId: filters.assignee,
			priority: filters.priority,
			label: filters.label,
			dueDate: filters.dueDate,
		}),
		[filters.assignee, filters.dueDate, filters.label, filters.priority, searchTerm],
	);

	const tasksRef = useRef(tasks);
	tasksRef.current = tasks;
	const editingTaskId = taskModal?.mode === "edit" ? taskModal.taskId : null;

	const editingTaskQuery = useQuery({
		queryKey: kanbanKeys.task(projectId, editingTaskId ?? ""),
		queryFn: () => getKanbanTask(projectId, editingTaskId as string),
		enabled: Boolean(editingTaskId),
	});
	const editingTaskDetail = editingTaskQuery.data;

	useEffect(() => {
		if (!editingTaskId) {
			form.reset(emptyTaskForm);
			return;
		}

		const summary = tasksRef.current.find((item) => item.id === editingTaskId);
		if (!summary) {
			return;
		}

		form.reset({
			title: summary.title,
			description: summary.description ?? "",
			assignee_id: summary.assignee_id ?? "",
			due_date: summary.due_date ? extractDateInputValue(summary.due_date) : "",
			priority: summary.priority,
			label: summary.label ?? "",
			fields: [],
		});
	}, [editingTaskId, form]);

	// Custom fields are only returned by the task detail endpoint, so load them
	// separately and populate the editor once they arrive — without clobbering
	// any edits already made to the other fields.
	useEffect(() => {
		if (!editingTaskId || editingTaskDetail?.id !== editingTaskId) {
			return;
		}

		form.setValue(
			"fields",
			(editingTaskDetail.fields ?? []).map((field) => ({
				name: field.name,
				value: field.value,
			})),
		);
	}, [editingTaskId, editingTaskDetail, form]);


	async function handleQuickAdd(columnId: string) {
		const title = quickDrafts[columnId]?.trim() ?? "";
		if (title.length < 2) {
			setBoardError("Judul task cepat minimal 2 karakter.");
			return;
		}

		setBoardError(null);
		await createTaskMutation.mutateAsync({
			column_id: columnId,
			...emptyTaskForm,
			title,
		});
		setQuickDrafts((current) => ({ ...current, [columnId]: "" }));
	}

	async function handleTaskSubmit(values: TaskFormValues) {
		setBoardError(null);
		if (!taskModal) {
			return;
		}

		if (taskModal.mode === "create") {
			await createTaskMutation.mutateAsync({
				column_id: taskModal.columnId,
				...values,
			});
			return;
		}

		await updateTaskMutation.mutateAsync({ taskId: taskModal.taskId, values });
	}

	function handleColumnSubmit() {
		const name = columnForm.name.trim();
		if (name.length < 2) {
			setBoardError("Nama kolom minimal 2 karakter.");
			return;
		}

		setBoardError(null);
		if (columnModal?.mode === "edit") {
			updateColumnMutation.mutate({
				columnId: columnModal.columnId,
				name,
				color: columnForm.color,
			});
			return;
		}

		createColumnMutation.mutate({
			name,
			color: columnForm.color,
		});
	}

	function startColumnEdit(column: KanbanColumn) {
		setColumnModal({ mode: "edit", columnId: column.id });
		setColumnForm({ name: column.name, color: column.color ?? "#38BDF8" });
	}

	function startColumnCreate() {
		setColumnModal({ mode: "create" });
		setColumnForm({ name: "", color: "#38BDF8" });
	}


	if (columnsQuery.isLoading) {
		return <Card className="p-8">Memuat board proyek...</Card>;
	}

	if (columnsQuery.error instanceof Error) {
		return (
			<Card className="p-8 text-error">
				{columnsQuery.error.message ?? "Gagal memuat board proyek"}
			</Card>
		);
	}

	return (
		<div className="space-y-6">
			<KanbanToolbar
				columns={columns}
				filters={filters}
				members={members}
				onColumnCreate={() => {
					setBoardError(null);
					startColumnCreate();
				}}
				onFiltersChange={setFilters}
			/>

			{boardError ? (
				<Card className="p-4 text-[13px] font-[500] text-priority-high border-priority-high/20 bg-priority-high/5">
					{boardError}
				</Card>
			) : null}

			<DndContext
				collisionDetection={closestCorners}
				onDragEnd={handleDragEnd}
				onDragStart={handleDragStart}
				sensors={sensors}>
				<SortableContext
					items={columns.map((column) => column.id)}
					strategy={horizontalListSortingStrategy}>
					<div className="-mx-1 overflow-x-auto px-1 pb-3">
						<div className="flex min-w-max gap-3 md:gap-5">
							{columns
								.filter(
									(column) =>
										!filters.columnId || column.id === filters.columnId,
								)
								.map((column) => (
									<KanbanColumnContainer
										column={column}
										key={column.id}
										onDeleteColumn={() => setColumnToDelete(column)}
										onEditColumn={() => startColumnEdit(column)}
										onQuickAdd={() => void handleQuickAdd(column.id)}
										onQuickDraftChange={(value) =>
											setQuickDrafts((current) => ({
												...current,
												[column.id]: value,
											}))
										}
										onTaskClick={(task) =>
											setTaskModal({
												mode: "edit",
												columnId: task.column_id,
												taskId: task.id,
											})
										}
										onTaskCreate={() =>
											setTaskModal({ mode: "create", columnId: column.id })
										}
										filters={taskFilters}
										onTasksLoaded={handleTasksLoaded}
										projectId={projectId}
										quickDraft={quickDrafts[column.id] ?? ""}
									/>
								))}
						</div>
					</div>
				</SortableContext>

				<DragOverlay>
					{activeTask ? <TaskOverlay task={activeTask} /> : null}
					{activeColumn ? (
						<ColumnOverlay
							column={activeColumn}
							taskCount={
								tasks.filter((task) => task.column_id === activeColumn.id)
									.length
							}
						/>
					) : null}
				</DragOverlay>
			</DndContext>

			{taskModal ? (
				<TaskModal
					error={boardError}
					form={form}
					isDeleting={deleteTaskMutation.isPending}
					isSubmitting={
						createTaskMutation.isPending || updateTaskMutation.isPending
					}
					members={members}
					mode={taskModal.mode}
					onClose={() => setTaskModal(null)}
					onDelete={() => {
						if (taskModal.mode === "edit") {
							const task = tasks.find((item) => item.id === taskModal.taskId);
							if (task) {
								setTaskToDelete(task);
							}
						}
					}}
					onSubmit={(values) => void handleTaskSubmit(values)}
				/>
			) : null}

			<KanbanDialogs
				columnColorOptions={columnColorOptions}
				columnForm={columnForm}
				columnModal={columnModal}
				columnToDelete={columnToDelete}
				isColumnDeleting={deleteColumnMutation.isPending}
				isColumnSaving={
					createColumnMutation.isPending || updateColumnMutation.isPending
				}
				isTaskDeleting={deleteTaskMutation.isPending}
				onColumnDeleteClose={() => setColumnToDelete(null)}
				onColumnDeleteConfirm={() => {
					if (columnToDelete) {
						deleteColumnMutation.mutate(columnToDelete.id);
					}
				}}
				onColumnFormChange={setColumnForm}
				onColumnModalClose={() => {
					setColumnModal(null);
					setColumnForm({ name: "", color: "#38BDF8" });
				}}
				onColumnSubmit={handleColumnSubmit}
				onTaskDeleteClose={() => setTaskToDelete(null)}
				onTaskDeleteConfirm={() => {
					if (taskToDelete) {
						deleteTaskMutation.mutate(taskToDelete.id);
					}
				}}
				taskToDelete={taskToDelete}
			/>
		</div>
	);
}


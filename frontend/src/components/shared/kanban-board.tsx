import { useEffect, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  closestCorners,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  horizontalListSortingStrategy,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm, type UseFormReturn } from "react-hook-form";
import { z } from "zod";
import { Plus } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Drawer, DrawerBody, DrawerClose, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from "@/components/shared/drawer";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { permissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { autoAssignTask } from "@/services/operational-assignment-rules";
import {
  createKanbanColumn,
  createKanbanTask,
  deleteKanbanColumn,
  deleteKanbanTask,
  kanbanKeys,
  listKanbanColumns,
  listKanbanTasks,
  moveKanbanTask,
  reorderKanbanColumns,
  updateKanbanColumn,
  updateKanbanTask,
} from "@/services/operational-kanban";
import type { KanbanColumn, KanbanFilters, KanbanTask, TaskFormValues } from "@/types/kanban";
import type { ProjectMember, ProjectPriority } from "@/types/project";

const taskSchema = z.object({
  title: z.string().trim().min(2, "Title must be at least 2 characters").max(160),
  description: z.string(),
  assignee_id: z.string(),
  due_date: z.string(),
  priority: z.enum(["low", "medium", "high", "critical"]),
  label: z.string(),
});

const emptyTaskForm: TaskFormValues = {
  title: "",
  description: "",
  assignee_id: "",
  due_date: "",
  priority: "medium",
  label: "",
};

const columnColorOptions = ["#38BDF8", "#10B981", "#F97316", "#F43F5E", "#8B5CF6", "#FACC15"];

interface KanbanBoardProps {
  projectId: string;
  members: ProjectMember[];
}

interface DragSnapshot {
  columns: KanbanColumn[];
  tasks: KanbanTask[];
}

type ColumnModalState =
  | { mode: "create" }
  | { mode: "edit"; columnId: string };

type TaskModalState =
  | { mode: "create"; columnId: string }
  | { mode: "edit"; columnId: string; taskId: string };

type DragTaskData = { type: "task"; task: KanbanTask };
type DragColumnData = { type: "column"; column: KanbanColumn };

export function KanbanBoard({ projectId, members }: KanbanBoardProps) {
  const queryClient = useQueryClient();
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));
  const [filters, setFilters] = useState<KanbanFilters>({
    assignee: "",
    priority: "",
    label: "",
    dueDate: "",
  });
  const [quickDrafts, setQuickDrafts] = useState<Record<string, string>>({});
  const [columnModal, setColumnModal] = useState<ColumnModalState | null>(null);
  const [columnForm, setColumnForm] = useState({ name: "", color: "#38BDF8" });
  const [taskModal, setTaskModal] = useState<TaskModalState | null>(null);
  const [boardError, setBoardError] = useState<string | null>(null);
  const [activeTask, setActiveTask] = useState<KanbanTask | null>(null);
  const [activeColumn, setActiveColumn] = useState<KanbanColumn | null>(null);
  const [dragSnapshot, setDragSnapshot] = useState<DragSnapshot | null>(null);
  const [columnToDelete, setColumnToDelete] = useState<KanbanColumn | null>(null);
  const [taskToDelete, setTaskToDelete] = useState<KanbanTask | null>(null);

  const columnsQuery = useQuery({
    queryKey: kanbanKeys.columns(projectId),
    queryFn: () => listKanbanColumns(projectId),
  });

  const tasksQuery = useQuery({
    queryKey: kanbanKeys.tasks(projectId),
    queryFn: () => listKanbanTasks(projectId),
  });

  const form = useForm<TaskFormValues>({
    resolver: zodResolver(taskSchema),
    defaultValues: emptyTaskForm,
  });

  const createColumnMutation = useMutation({
    mutationFn: (payload: { name: string; color?: string }) => createKanbanColumn(projectId, payload),
    onSuccess: async () => {
      setColumnForm({ name: "", color: "#38BDF8" });
      setColumnModal(null);
      await invalidateBoard(queryClient, projectId);
    },
    onError: (error) => setBoardError(extractErrorMessage(error)),
  });

  const updateColumnMutation = useMutation({
    mutationFn: (payload: { columnId: string; name: string; color?: string }) =>
      updateKanbanColumn(projectId, payload.columnId, { name: payload.name, color: payload.color }),
    onSuccess: async () => {
      setColumnModal(null);
      await invalidateBoard(queryClient, projectId);
    },
    onError: (error) => setBoardError(extractErrorMessage(error)),
  });

  const deleteColumnMutation = useMutation({
    mutationFn: (columnId: string) => deleteKanbanColumn(projectId, columnId),
    onSuccess: async () => {
      setColumnToDelete(null);
      await invalidateBoard(queryClient, projectId);
    },
    onError: (error) => setBoardError(extractErrorMessage(error)),
  });

  const createTaskMutation = useMutation({
    mutationFn: (payload: { column_id: string } & TaskFormValues) => createKanbanTask(projectId, payload),
    onSuccess: async () => {
      form.reset(emptyTaskForm);
      setTaskModal(null);
      await invalidateBoard(queryClient, projectId);
    },
    onError: (error) => setBoardError(extractErrorMessage(error)),
  });

  const updateTaskMutation = useMutation({
    mutationFn: (payload: { taskId: string; values: TaskFormValues }) =>
      updateKanbanTask(projectId, payload.taskId, payload.values),
    onSuccess: async () => {
      form.reset(emptyTaskForm);
      setTaskModal(null);
      await invalidateBoard(queryClient, projectId);
    },
    onError: (error) => setBoardError(extractErrorMessage(error)),
  });

  const deleteTaskMutation = useMutation({
    mutationFn: (taskId: string) => deleteKanbanTask(projectId, taskId),
    onSuccess: async () => {
      form.reset(emptyTaskForm);
      setTaskModal(null);
      setTaskToDelete(null);
      await invalidateBoard(queryClient, projectId);
    },
    onError: (error) => setBoardError(extractErrorMessage(error)),
  });

  const autoAssignMutation = useMutation({
    mutationFn: (taskId: string) => autoAssignTask(projectId, taskId),
    onSuccess: async () => {
      await invalidateBoard(queryClient, projectId);
    },
    onError: (error) => setBoardError(extractErrorMessage(error)),
  });

  const columns = columnsQuery.data ?? [];
  const tasks = tasksQuery.data ?? [];
  const filteredTasks = tasks.filter((task) => matchesFilters(task, filters));
  const activeFilterCount = [filters.assignee, filters.priority, filters.label, filters.dueDate].filter(Boolean).length;

  useEffect(() => {
    if (!taskModal) {
      form.reset(emptyTaskForm);
      return;
    }

    if (taskModal.mode === "create") {
      form.reset(emptyTaskForm);
      return;
    }

    const task = tasks.find((item) => item.id === taskModal.taskId);
    if (!task) {
      form.reset(emptyTaskForm);
      return;
    }

    form.reset({
      title: task.title,
      description: task.description ?? "",
      assignee_id: task.assignee_id ?? "",
      due_date: task.due_date ? task.due_date.slice(0, 10) : "",
      priority: task.priority,
      label: task.label ?? "",
    });
  }, [form, taskModal, tasks]);

  async function handleQuickAdd(columnId: string) {
    const title = quickDrafts[columnId]?.trim() ?? "";
    if (title.length < 2) {
      setBoardError("Quick add title must be at least 2 characters.");
      return;
    }

    setBoardError(null);
    await createTaskMutation.mutateAsync({ column_id: columnId, ...emptyTaskForm, title });
    setQuickDrafts((current) => ({ ...current, [columnId]: "" }));
  }

  async function handleTaskSubmit(values: TaskFormValues) {
    setBoardError(null);
    if (!taskModal) {
      return;
    }

    if (taskModal.mode === "create") {
      await createTaskMutation.mutateAsync({ column_id: taskModal.columnId, ...values });
      return;
    }

    await updateTaskMutation.mutateAsync({ taskId: taskModal.taskId, values });
  }

  function handleColumnSubmit() {
    const name = columnForm.name.trim();
    if (name.length < 2) {
      setBoardError("List name must be at least 2 characters.");
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

  function handleDragOver(event: DragOverEvent) {
    const activeData = event.active.data.current;
    const overData = event.over?.data.current;
    if (!isTaskDragData(activeData) || !event.over) {
      return;
    }

    const cachedTasks = queryClient.getQueryData<KanbanTask[]>(kanbanKeys.tasks(projectId)) ?? tasks;
    const nextTasks = moveTaskInMemory(cachedTasks, activeData.task.id, overData, event.over.id.toString());
    if (nextTasks && taskPlacementChanged(cachedTasks, nextTasks, activeData.task.id)) {
      queryClient.setQueryData(kanbanKeys.tasks(projectId), nextTasks);
    }
  }

  function rollbackDrag() {
    if (!dragSnapshot) {
      return;
    }

    queryClient.setQueryData(kanbanKeys.columns(projectId), dragSnapshot.columns);
    queryClient.setQueryData(kanbanKeys.tasks(projectId), dragSnapshot.tasks);
  }

  function resetDrag() {
    setActiveTask(null);
    setActiveColumn(null);
    setDragSnapshot(null);
  }

  function handleDragEnd(event: DragEndEvent) {
    if (!event.over) {
      rollbackDrag();
      resetDrag();
      return;
    }

    const activeData = event.active.data.current;

    if (isColumnDragData(activeData)) {
      finishColumnDrag(event, columns, dragSnapshot, queryClient, projectId);
      resetDrag();
      return;
    }

    if (isTaskDragData(activeData)) {
      finishTaskDrag(activeData.task.id, tasks, dragSnapshot, queryClient, projectId);
    }

    resetDrag();
  }

  if (columnsQuery.isLoading || tasksQuery.isLoading) {
    return <Card className="p-8">Loading Kanban board...</Card>;
  }

  if (columnsQuery.error instanceof Error || tasksQuery.error instanceof Error) {
    return (
      <Card className="p-8 text-error">
        {(columnsQuery.error as Error | undefined)?.message ??
          (tasksQuery.error as Error | undefined)?.message ??
          "Failed to load Kanban board"}
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <Card className="p-6">
        <div className="flex flex-col gap-5">
          <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
            <div>
              <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-ops mb-1">Board view</p>
              <h4 className="text-[20px] font-[700] text-text-primary leading-tight">Project execution board</h4>
              <p className="mt-1 max-w-3xl text-[13px] text-text-secondary">
                Kelola task proyek dalam satu board, pindahkan kartu antar tahap kerja, dan gunakan filter tanpa keluar dari halaman ini.
              </p>
            </div>

            <PermissionGate permission={permissions.operationalKanbanCreate}>
              <div className="flex w-full flex-col items-stretch gap-3 xl:w-auto xl:items-end">
                <Button
                  variant="ops"
                  className="xl:self-end"
                  onClick={() => {
                    setBoardError(null);
                    startColumnCreate();
                  }}
                  type="button"
                >
                  <Plus className="h-4 w-4" />
                  New list
                </Button>
              </div>
            </PermissionGate>
          </div>

          <div className="grid gap-4 xl:grid-cols-[repeat(4,minmax(0,1fr))_auto]">
          <select
            className="flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10"
            onChange={(event) => setFilters((current) => ({ ...current, assignee: event.target.value }))}
            value={filters.assignee}
          >
            <option value="">All assignees</option>
            {members.map((member) => (
              <option key={member.user_id} value={member.user_id}>
                {member.full_name || member.user_email || member.user_id}
              </option>
            ))}
          </select>
          <select
            className="flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10"
            onChange={(event) => setFilters((current) => ({ ...current, priority: event.target.value }))}
            value={filters.priority}
          >
            <option value="">All priorities</option>
            <option value="low">Low</option>
            <option value="medium">Medium</option>
            <option value="high">High</option>
            <option value="critical">Critical</option>
          </select>
          <Input
            className="h-[44px] focus-visible:border-ops focus-visible:ring-ops/10"
            onChange={(event) => setFilters((current) => ({ ...current, label: event.target.value }))}
            placeholder="Filter by label"
            value={filters.label}
          />
          <Input
            className="h-[44px] focus-visible:border-ops focus-visible:ring-ops/10"
            onChange={(event) => setFilters((current) => ({ ...current, dueDate: event.target.value }))}
            type="date"
            value={filters.dueDate}
          />
            <Button
              onClick={() =>
                setFilters({
                  assignee: "",
                  priority: "",
                  label: "",
                  dueDate: "",
                })
              }
              variant="secondary"
              className="h-[44px]"
            >
              Clear {activeFilterCount > 0 ? `(${activeFilterCount})` : ""}
            </Button>
          </div>
        </div>
      </Card>

      {boardError ? <Card className="p-4 text-[13px] font-[500] text-priority-high border-priority-high/20 bg-priority-high/5">{boardError}</Card> : null}

      <DndContext
        collisionDetection={closestCorners}
        onDragEnd={handleDragEnd}
        onDragOver={handleDragOver}
        onDragStart={handleDragStart}
        sensors={sensors}
      >
        <SortableContext items={columns.map((column) => column.id)} strategy={horizontalListSortingStrategy}>
          <div className="overflow-x-auto pb-3">
            <div className="flex min-w-max gap-5">
            {columns.map((column) => (
              <KanbanColumnCard
                column={column}
                onDeleteColumn={() => setColumnToDelete(column)}
                onEditColumn={() => startColumnEdit(column)}
                onQuickAdd={() => void handleQuickAdd(column.id)}
                onQuickDraftChange={(value) => setQuickDrafts((current) => ({ ...current, [column.id]: value }))}
                onAutoAssignTask={(taskId) => autoAssignMutation.mutate(taskId)}
                onTaskClick={(task) => setTaskModal({ mode: "edit", columnId: task.column_id, taskId: task.id })}
                onTaskCreate={() => setTaskModal({ mode: "create", columnId: column.id })}
                quickDraft={quickDrafts[column.id] ?? ""}
                tasks={filteredTasks.filter((task) => task.column_id === column.id).sort(sortTaskList)}
              />
            ))}
            </div>
          </div>
        </SortableContext>

        <DragOverlay>
          {activeTask ? <TaskOverlay task={activeTask} /> : null}
          {activeColumn ? <ColumnOverlay column={activeColumn} taskCount={tasks.filter((task) => task.column_id === activeColumn.id).length} /> : null}
        </DragOverlay>
      </DndContext>

      {taskModal ? (
        <TaskModal
          form={form}
          isDeleting={deleteTaskMutation.isPending}
          isSubmitting={createTaskMutation.isPending || updateTaskMutation.isPending}
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

      <FormModal
        isLoading={createColumnMutation.isPending || updateColumnMutation.isPending}
        isOpen={Boolean(columnModal)}
        onClose={() => {
          setColumnModal(null);
          setColumnForm({ name: "", color: "#38BDF8" });
        }}
        onSubmit={(event) => {
          event.preventDefault();
          handleColumnSubmit();
        }}
        size="sm"
        submitLabel={columnModal?.mode === "edit" ? "Save list" : "Create list"}
        title={columnModal?.mode === "edit" ? "Edit list" : "Create list"}
        subtitle="Use a short label and a single accent color so this column stays easy to scan on the board."
      >
        <div className="space-y-2">
          <label className="text-[13px] font-[600] text-text-primary">List name</label>
          <Input
            className="focus-visible:border-ops focus-visible:ring-ops/10"
            onChange={(event) => setColumnForm((current) => ({ ...current, name: event.target.value }))}
            placeholder="Review"
            value={columnForm.name}
          />
        </div>
        <div className="space-y-2">
          <p className="text-[13px] font-[600] text-text-primary">Accent color</p>
          <div className="flex flex-wrap gap-2">
            {columnColorOptions.map((color) => (
              <button
                aria-label={`Choose list color ${color}`}
                className={cn(
                  "h-9 w-9 rounded-full border-2 transition hover:scale-105",
                  columnForm.color === color ? "border-foreground shadow-sm" : "border-transparent",
                )}
                key={color}
                onClick={() => setColumnForm((current) => ({ ...current, color }))}
                style={{ backgroundColor: color }}
                type="button"
              />
            ))}
          </div>
        </div>
      </FormModal>

      <ConfirmDialog
        confirmLabel="Delete list"
        description={
          columnToDelete
            ? `All tasks inside "${columnToDelete.name}" will be deleted with this column.`
            : ""
        }
        isLoading={deleteColumnMutation.isPending}
        isOpen={Boolean(columnToDelete)}
        onClose={() => setColumnToDelete(null)}
        onConfirm={() => {
          if (columnToDelete) {
            deleteColumnMutation.mutate(columnToDelete.id);
          }
        }}
        title={columnToDelete ? `Delete ${columnToDelete.name}?` : "Delete list?"}
      />

      <ConfirmDialog
        confirmLabel="Delete task"
        description={taskToDelete ? `Task "${taskToDelete.title}" will be removed from this board.` : ""}
        isLoading={deleteTaskMutation.isPending}
        isOpen={Boolean(taskToDelete)}
        onClose={() => setTaskToDelete(null)}
        onConfirm={() => {
          if (taskToDelete) {
            deleteTaskMutation.mutate(taskToDelete.id);
          }
        }}
        title={taskToDelete ? `Delete ${taskToDelete.title}?` : "Delete task?"}
      />
    </div>
  );
}

function matchesFilters(task: KanbanTask, filters: KanbanFilters) {
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
  if (filters.dueDate && (!task.due_date || task.due_date.slice(0, 10) !== filters.dueDate)) {
    return false;
  }
  return true;
}

function moveTaskInMemory(tasks: KanbanTask[], activeTaskId: string, overData: unknown, overId: string) {
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
    const targetList = destinationColumnId === activeTask.column_id ? sourceList : [...(buckets[destinationColumnId] ?? [])];
    const overIndex = targetList.findIndex((task) => task.id === overData.task.id);
    destinationIndex = overIndex >= 0 ? overIndex : targetList.length;
  } else if (isColumnDragData(overData)) {
    destinationColumnId = overData.column.id;
    const targetList = destinationColumnId === activeTask.column_id ? sourceList : [...(buckets[destinationColumnId] ?? [])];
    destinationIndex = targetList.length;
  } else {
    const targetList = buckets[activeTask.column_id] ?? [];
    const overIndex = targetList.findIndex((task) => task.id === overId);
    destinationIndex = overIndex >= 0 ? overIndex : targetList.length;
  }

  const destinationList =
    destinationColumnId === activeTask.column_id ? sourceList : [...(buckets[destinationColumnId] ?? [])];
  destinationList.splice(destinationIndex, 0, { ...movingTask, column_id: destinationColumnId });
  buckets[destinationColumnId] = destinationList;

  return flattenTaskBuckets(buckets);
}

function buildTaskBuckets(tasks: KanbanTask[]) {
  const buckets: Record<string, KanbanTask[]> = {};
  for (const task of [...tasks].sort(sortTaskList)) {
    if (!buckets[task.column_id]) {
      buckets[task.column_id] = [];
    }
    buckets[task.column_id].push(task);
  }
  return buckets;
}

function flattenTaskBuckets(buckets: Record<string, KanbanTask[]>) {
  const nextTasks: KanbanTask[] = [];
  for (const columnId of Object.keys(buckets)) {
    buckets[columnId].forEach((task, index) => {
      nextTasks.push({ ...task, column_id: columnId, position: index + 1 });
    });
  }
  return nextTasks.sort(sortTaskList);
}

function taskPlacementChanged(currentTasks: KanbanTask[], nextTasks: KanbanTask[], taskId: string) {
  const currentTask = currentTasks.find((task) => task.id === taskId);
  const nextTask = nextTasks.find((task) => task.id === taskId);

  if (!currentTask || !nextTask) {
    return false;
  }

  return currentTask.column_id !== nextTask.column_id || currentTask.position !== nextTask.position;
}

function sortTaskList(left: KanbanTask, right: KanbanTask) {
  if (left.column_id === right.column_id) {
    return left.position - right.position;
  }
  return left.column_id.localeCompare(right.column_id);
}

async function invalidateBoard(queryClient: ReturnType<typeof useQueryClient>, projectId: string) {
  await queryClient.invalidateQueries({ queryKey: kanbanKeys.all(projectId) });
}

function finishTaskDrag(
  taskId: string,
  tasks: KanbanTask[],
  snapshot: DragSnapshot | null,
  queryClient: ReturnType<typeof useQueryClient>,
  projectId: string,
) {
  if (!snapshot) {
    return;
  }

  const previousTask = snapshot.tasks.find((task) => task.id === taskId);
  const currentTasks = queryClient.getQueryData<KanbanTask[]>(kanbanKeys.tasks(projectId)) ?? tasks;
  const nextTask = currentTasks.find((task) => task.id === taskId);

  if (!previousTask || !nextTask) {
    queryClient.setQueryData(kanbanKeys.tasks(projectId), snapshot.tasks);
    return;
  }

  if (previousTask.column_id === nextTask.column_id && previousTask.position === nextTask.position) {
    return;
  }

  void moveKanbanTask(projectId, nextTask.id, nextTask.column_id, nextTask.position)
    .then(async () => {
      await invalidateBoard(queryClient, projectId);
    })
    .catch(async () => {
      queryClient.setQueryData(kanbanKeys.tasks(projectId), snapshot.tasks);
      await invalidateBoard(queryClient, projectId);
    });
}

function finishColumnDrag(
  event: DragEndEvent,
  columns: KanbanColumn[],
  snapshot: DragSnapshot | null,
  queryClient: ReturnType<typeof useQueryClient>,
  projectId: string,
) {
  if (!snapshot) {
    return;
  }

  const currentColumns = queryClient.getQueryData<KanbanColumn[]>(kanbanKeys.columns(projectId)) ?? columns;
  const activeIndex = currentColumns.findIndex((column) => column.id === event.active.id);
  const overIndex = currentColumns.findIndex((column) => column.id === event.over?.id);
  if (activeIndex < 0 || overIndex < 0 || activeIndex === overIndex) {
    return;
  }

  const nextColumns = arrayMove(currentColumns, activeIndex, overIndex).map((column, index) => ({
    ...column,
    position: index + 1,
  }));

  queryClient.setQueryData(kanbanKeys.columns(projectId), nextColumns);
  void reorderKanbanColumns(projectId, nextColumns.map((column) => column.id))
    .then(async () => {
      await invalidateBoard(queryClient, projectId);
    })
    .catch(async () => {
      queryClient.setQueryData(kanbanKeys.columns(projectId), snapshot.columns);
      await invalidateBoard(queryClient, projectId);
    });
}

function isTaskDragData(value: unknown): value is DragTaskData {
  return typeof value === "object" && value !== null && "type" in value && value.type === "task";
}

function isColumnDragData(value: unknown): value is DragColumnData {
  return typeof value === "object" && value !== null && "type" in value && value.type === "column";
}

function extractErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Unexpected request error";
}

interface KanbanColumnCardProps {
  column: KanbanColumn;
  tasks: KanbanTask[];
  quickDraft: string;
  onQuickDraftChange: (value: string) => void;
  onQuickAdd: () => void;
  onAutoAssignTask: (taskId: string) => void;
  onTaskClick: (task: KanbanTask) => void;
  onTaskCreate: () => void;
  onEditColumn: () => void;
  onDeleteColumn: () => void;
}

function KanbanColumnCard(props: KanbanColumnCardProps) {
  const sortable = useSortable({
    id: props.column.id,
    data: { type: "column", column: props.column } satisfies DragColumnData,
  });
  const style = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  };

  return (
    <div className="w-[320px] shrink-0" ref={sortable.setNodeRef} style={style}>
      <Card className={cn("flex flex-col h-full min-h-[500px] border-border bg-surface-muted p-4 shadow-sm", sortable.isDragging && "opacity-70")}>
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3">
            <button
              {...sortable.attributes}
              {...sortable.listeners}
              className="rounded-[6px] border border-border bg-background px-2 py-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-secondary hover:bg-surface-muted transition-colors cursor-grab"
              type="button"
            >
              Drag
            </button>
            <div>
              <div className="flex items-center gap-2">
                <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: props.column.color ?? "#94A3B8" }} />
                <h5 className="font-[600] text-[14px] text-text-primary truncate max-w-[120px]">{props.column.name}</h5>
              </div>
              <p className="text-[12px] font-[500] text-text-tertiary">{props.tasks.length} cards</p>
            </div>
          </div>

          <PermissionGate permission={permissions.operationalKanbanEdit}>
            <div className="flex gap-2">
              <Button onClick={props.onEditColumn} size="sm" variant="secondary" className="h-7 px-2 text-[11px]">
                Edit
              </Button>
              <PermissionGate permission={permissions.operationalKanbanDelete}>
                <Button onClick={props.onDeleteColumn} size="sm" variant="ghost" className="h-7 px-2 text-[11px]">
                   Delete
                </Button>
              </PermissionGate>
            </div>
          </PermissionGate>
        </div>

        <div className="mt-4 flex-1 space-y-3 overflow-y-auto pr-1">
          <SortableContext items={props.tasks.map((task) => task.id)} strategy={verticalListSortingStrategy}>
            {props.tasks.map((task) => (
              <KanbanTaskCard
                key={task.id}
                onAutoAssign={() => props.onAutoAssignTask(task.id)}
                onClick={() => props.onTaskClick(task)}
                task={task}
              />
            ))}
          </SortableContext>

          {props.tasks.length === 0 ? (
            <div className="rounded-[12px] border border-dashed border-border bg-background/50 px-4 py-8 text-center text-[13px] font-[500] text-text-tertiary">
              No matching tasks in this column.
            </div>
          ) : null}
        </div>

        <PermissionGate permission={permissions.operationalKanbanCreate}>
          <div className="mt-4 rounded-[12px] border border-border bg-background p-3 space-y-3">
            <Input
              className="focus-visible:border-ops focus-visible:ring-ops/10"
              onChange={(event) => props.onQuickDraftChange(event.target.value)}
              placeholder="Add another card"
              value={props.quickDraft}
            />
            <div className="flex gap-3">
              <Button variant="ops" onClick={props.onQuickAdd} size="sm">
                Quick add
              </Button>
              <Button onClick={props.onTaskCreate} size="sm" variant="ghost">
                Open form
              </Button>
            </div>
          </div>
        </PermissionGate>
      </Card>
    </div>
  );
}

function KanbanTaskCard({
  task,
  onClick,
  onAutoAssign,
}: {
  task: KanbanTask;
  onClick: () => void;
  onAutoAssign: () => void;
}) {
  const sortable = useSortable({
    id: task.id,
    data: { type: "task", task } satisfies DragTaskData,
  });
  const style = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  };

  return (
    <div ref={sortable.setNodeRef} style={style}>
      <Card
        {...sortable.attributes}
        {...sortable.listeners}
        className={cn(
          "cursor-grab p-4 shadow-sm transition-all hover:border-ops/30 hover:shadow-card active:cursor-grabbing group",
          sortable.isDragging && "opacity-60 ring-2 ring-ops",
        )}
        onClick={onClick}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className="flex flex-wrap items-center gap-2 mb-2">
              <PriorityBadge priority={task.priority} />
              <AssignmentBadge assignedVia={task.assigned_via} />
              {task.label ? (
                <span className="rounded-full bg-surface-muted border border-border px-2 py-0.5 text-[11px] font-[600] uppercase tracking-wider text-text-secondary">
                  {task.label}
                </span>
              ) : null}
            </div>
            <h6 className="text-[14px] font-[600] text-text-primary leading-tight">{task.title}</h6>
          </div>
          <span className="opacity-0 group-hover:opacity-100 transition-opacity rounded-[6px] border border-border bg-surface-muted px-2 py-1 text-[10px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
            Move
          </span>
        </div>

        {task.description ? <p className="mt-2 line-clamp-2 text-[12px] text-text-secondary leading-relaxed">{task.description}</p> : null}

        <div className="mt-4 flex items-center justify-between gap-3 border-t border-border pt-4">
          <div className="flex items-center gap-2">
            <AvatarBadge name={task.assignee_name} />
            <span className="text-[12px] font-[500] text-text-secondary truncate max-w-[120px]">{task.assignee_name ?? "Unassigned"}</span>
          </div>
          <span className="text-[11px] font-[600] text-text-tertiary uppercase tracking-wider">{task.due_date ? formatDate(task.due_date) : "No due date"}</span>
        </div>

        {!task.assignee_id ? (
          <PermissionGate permission={permissions.operationalAssignmentEdit}>
            <div className="mt-3">
              <Button
                onClick={(event) => {
                  event.stopPropagation();
                  onAutoAssign();
                }}
                size="sm"
                variant="ops"
                className="w-full text-[12px] h-8"
              >
                Auto assign
              </Button>
            </div>
          </PermissionGate>
        ) : null}
      </Card>
    </div>
  );
}

function TaskOverlay({ task }: { task: KanbanTask }) {
  return (
    <div className="w-[320px]">
      <Card className="border-ops shadow-2xl p-4 rotate-2">
        <PriorityBadge priority={task.priority} />
        <p className="mt-2 text-[14px] font-[600] text-text-primary leading-tight">{task.title}</p>
      </Card>
    </div>
  );
}

function ColumnOverlay({ column, taskCount }: { column: KanbanColumn; taskCount: number }) {
  return (
    <div className="w-[320px]">
      <Card className="border-ops shadow-2xl p-4 rotate-2">
        <div className="flex items-center gap-2">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: column.color ?? "#94A3B8" }} />
          <p className="text-[14px] font-[600] text-text-primary">{column.name}</p>
        </div>
        <p className="mt-1 text-[12px] font-[500] text-text-tertiary">{taskCount} tasks</p>
      </Card>
    </div>
  );
}

function TaskModal({
  mode,
  form,
  members,
  isSubmitting,
  isDeleting,
  onSubmit,
  onDelete,
  onClose,
}: {
  mode: "create" | "edit";
  form: UseFormReturn<TaskFormValues>;
  members: ProjectMember[];
  isSubmitting: boolean;
  isDeleting: boolean;
  onSubmit: (values: TaskFormValues) => void;
  onDelete: () => void;
  onClose: () => void;
}) {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = form;

  return (
    <Drawer onOpenChange={(open) => (!open ? onClose() : undefined)} open>
      <DrawerContent size="lg">
        <DrawerHeader className="flex items-start justify-between gap-4">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-ops mb-1">
              {mode === "create" ? "Create task" : "Task detail"}
            </p>
            <DrawerTitle>{mode === "create" ? "New task" : "Edit task"}</DrawerTitle>
            <DrawerDescription>
              {mode === "create"
                ? "Capture the task brief, assignee, and due date without losing the board context."
                : "Review task details, edit delivery info, or remove the task from the board."}
            </DrawerDescription>
          </div>
          <DrawerClose />
        </DrawerHeader>

        <DrawerBody>
        <form className="space-y-6" onSubmit={handleSubmit(onSubmit)}>
          <div className="grid gap-2">
            <label className="text-[13px] font-[600] text-text-primary" htmlFor="task-title">
              Title
            </label>
            <Input className="focus-visible:border-ops focus-visible:ring-ops/10" id="task-title" {...register("title")} />
            {errors.title ? <p className="text-[13px] font-[500] text-priority-high">{errors.title.message}</p> : null}
          </div>

          <div className="grid gap-2">
            <label className="text-[13px] font-[600] text-text-primary" htmlFor="task-description">
              Description
            </label>
            <textarea
              className="min-h-32 w-full rounded-[6px] border border-border bg-surface px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10 disabled:cursor-not-allowed disabled:opacity-50"
              id="task-description"
              {...register("description")}
            />
          </div>

          <div className="grid gap-5 md:grid-cols-2">
            <div className="grid gap-2">
              <label className="text-[13px] font-[600] text-text-primary" htmlFor="task-assignee">
                Assignee
              </label>
              <select
                className="flex h-[44px] w-full rounded-[6px] border border-border bg-surface px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10"
                id="task-assignee"
                {...register("assignee_id")}
              >
                <option value="">Unassigned</option>
                {members.map((member) => (
                  <option key={member.user_id} value={member.user_id}>
                    {member.full_name || member.user_email || member.user_id}
                  </option>
                ))}
              </select>
            </div>

            <div className="grid gap-2">
              <label className="text-[13px] font-[600] text-text-primary" htmlFor="task-due-date">
                Due date
              </label>
              <Input className="focus-visible:border-ops focus-visible:ring-ops/10" id="task-due-date" type="date" {...register("due_date")} />
            </div>
          </div>

          <div className="grid gap-5 md:grid-cols-2">
            <div className="grid gap-2">
              <label className="text-[13px] font-[600] text-text-primary" htmlFor="task-priority">
                Priority
              </label>
              <select
                className="flex h-[44px] w-full rounded-[6px] border border-border bg-surface px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10"
                id="task-priority"
                {...register("priority")}
              >
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
              </select>
            </div>

            <div className="grid gap-2">
              <label className="text-[13px] font-[600] text-text-primary" htmlFor="task-label">
                Label
              </label>
              <Input className="focus-visible:border-ops focus-visible:ring-ops/10" id="task-label" placeholder="Bug, design, backend" {...register("label")} />
            </div>
          </div>

          <div className="flex flex-wrap gap-3 pt-4 border-t border-border">
            <Button variant="ops" disabled={isSubmitting} type="submit">
              {isSubmitting ? "Saving..." : mode === "create" ? "Create task" : "Save changes"}
            </Button>
            {mode === "edit" ? (
              <PermissionGate permission={permissions.operationalKanbanDelete}>
                <Button disabled={isDeleting} onClick={onDelete} type="button" variant="ghost">
                  {isDeleting ? "Deleting..." : "Delete task"}
                </Button>
              </PermissionGate>
            ) : null}
          </div>
        </form>
        </DrawerBody>
      </DrawerContent>
    </Drawer>
  );
}

function PriorityBadge({ priority }: { priority: ProjectPriority }) {
  const tone =
    priority === "critical"
      ? "bg-priority-critical-bg text-priority-critical"
      : priority === "high"
        ? "bg-priority-high-bg text-priority-high"
        : priority === "medium"
          ? "bg-priority-medium-bg text-priority-medium"
          : "bg-surface-muted text-text-secondary border border-border";

  return <span className={cn("rounded-[6px] px-2 py-0.5 text-[11px] font-[700] uppercase tracking-[0.08em]", tone)}>{priority}</span>;
}

function AssignmentBadge({ assignedVia }: { assignedVia: "manual" | "auto" }) {
  const tone =
    assignedVia === "auto"
      ? "bg-ops/10 text-ops"
      : "bg-surface-muted text-text-secondary border border-border";

  return (
    <span className={cn("rounded-[6px] px-2 py-0.5 text-[11px] font-[700] uppercase tracking-[0.08em]", tone)}>
      {assignedVia}
    </span>
  );
}

function AvatarBadge({ name }: { name?: string | null }) {
  return (
    <div className="flex h-8 w-8 items-center justify-center rounded-full bg-ops text-[11px] font-[600] uppercase text-white shadow-sm ring-2 ring-background">
      {initials(name ?? "NA")}
    </div>
  );
}

function initials(value: string) {
  return value
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

function formatDate(value: string) {
  return new Date(value).toLocaleDateString();
}

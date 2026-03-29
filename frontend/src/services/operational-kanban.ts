import { authRequestJSON } from "@/lib/api-client";
import { toUTCDateOnlyISOString } from "@/lib/date";
import type { KanbanColumn, KanbanTask, TaskFormValues } from "@/types/kanban";

export const kanbanKeys = {
  all: (projectId: string) => ["operational", "projects", projectId, "kanban"] as const,
  columns: (projectId: string) => [...kanbanKeys.all(projectId), "columns"] as const,
  tasks: (projectId: string) => [...kanbanKeys.all(projectId), "tasks"] as const,
};

export async function listKanbanColumns(projectId: string) {
  return authRequestJSON<KanbanColumn[]>(
    `/operational/projects/${projectId}/columns`,
    { method: "GET" },
  );
}

export async function createKanbanColumn(projectId: string, input: { name: string; color?: string }) {
  return authRequestJSON<KanbanColumn>(
    `/operational/projects/${projectId}/columns`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
  );
}

export async function updateKanbanColumn(projectId: string, columnId: string, input: { name: string; color?: string }) {
  return authRequestJSON<KanbanColumn>(
    `/operational/projects/${projectId}/columns/${columnId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
  );
}

export async function deleteKanbanColumn(projectId: string, columnId: string) {
  await authRequestJSON<{ message: string }>(
    `/operational/projects/${projectId}/columns/${columnId}`,
    {
      method: "DELETE",
    },
  );
}

export async function reorderKanbanColumns(projectId: string, columnIds: string[]) {
  await authRequestJSON<{ message: string }>(
    `/operational/projects/${projectId}/columns/reorder`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ column_ids: columnIds }),
    },
  );
}

export async function listKanbanTasks(projectId: string) {
  return authRequestJSON<KanbanTask[]>(
    `/operational/projects/${projectId}/tasks`,
    { method: "GET" },
  );
}

export async function createKanbanTask(projectId: string, input: { column_id: string } & TaskFormValues) {
  return authRequestJSON<KanbanTask>(
    `/operational/projects/${projectId}/tasks`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeTaskForm(input)),
    },
  );
}

export async function updateKanbanTask(projectId: string, taskId: string, input: TaskFormValues) {
  return authRequestJSON<KanbanTask>(
    `/operational/projects/${projectId}/tasks/${taskId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeTaskForm(input)),
    },
  );
}

export async function deleteKanbanTask(projectId: string, taskId: string) {
  await authRequestJSON<{ message: string }>(
    `/operational/projects/${projectId}/tasks/${taskId}`,
    {
      method: "DELETE",
    },
  );
}

export async function moveKanbanTask(projectId: string, taskId: string, columnId: string, position: number) {
  await authRequestJSON<{ message: string }>(
    `/operational/projects/${projectId}/tasks/${taskId}/move`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        column_id: columnId,
        position,
      }),
    },
  );
}

function serializeTaskForm(input: Partial<{ column_id: string }> & TaskFormValues) {
  return {
    ...(input.column_id ? { column_id: input.column_id } : {}),
    title: input.title.trim(),
    description: input.description.trim() || null,
    assignee_id: input.assignee_id.trim() || null,
    due_date: input.due_date ? toUTCDateOnlyISOString(input.due_date) : null,
    priority: input.priority,
    label: input.label.trim() || null,
  };
}

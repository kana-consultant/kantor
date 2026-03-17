import { ApiError, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type { KanbanColumn, KanbanTask, TaskFormValues } from "@/types/kanban";

export const kanbanKeys = {
  all: (projectId: string) => ["operational", "projects", projectId, "kanban"] as const,
  columns: (projectId: string) => [...kanbanKeys.all(projectId), "columns"] as const,
  tasks: (projectId: string) => [...kanbanKeys.all(projectId), "tasks"] as const,
};

export async function listKanbanColumns(projectId: string) {
  const token = await requireAccessToken();
  return requestJSON<KanbanColumn[]>(
    `/operational/projects/${projectId}/columns`,
    { method: "GET" },
    token,
  );
}

export async function createKanbanColumn(projectId: string, input: { name: string; color?: string }) {
  const token = await requireAccessToken();
  return requestJSON<KanbanColumn>(
    `/operational/projects/${projectId}/columns`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
    token,
  );
}

export async function updateKanbanColumn(projectId: string, columnId: string, input: { name: string; color?: string }) {
  const token = await requireAccessToken();
  return requestJSON<KanbanColumn>(
    `/operational/projects/${projectId}/columns/${columnId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
    token,
  );
}

export async function deleteKanbanColumn(projectId: string, columnId: string) {
  const token = await requireAccessToken();
  await requestJSON<{ message: string }>(
    `/operational/projects/${projectId}/columns/${columnId}`,
    {
      method: "DELETE",
    },
    token,
  );
}

export async function reorderKanbanColumns(projectId: string, columnIds: string[]) {
  const token = await requireAccessToken();
  await requestJSON<{ message: string }>(
    `/operational/projects/${projectId}/columns/reorder`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ column_ids: columnIds }),
    },
    token,
  );
}

export async function listKanbanTasks(projectId: string) {
  const token = await requireAccessToken();
  return requestJSON<KanbanTask[]>(
    `/operational/projects/${projectId}/tasks`,
    { method: "GET" },
    token,
  );
}

export async function createKanbanTask(projectId: string, input: { column_id: string } & TaskFormValues) {
  const token = await requireAccessToken();
  return requestJSON<KanbanTask>(
    `/operational/projects/${projectId}/tasks`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeTaskForm(input)),
    },
    token,
  );
}

export async function updateKanbanTask(projectId: string, taskId: string, input: TaskFormValues) {
  const token = await requireAccessToken();
  return requestJSON<KanbanTask>(
    `/operational/projects/${projectId}/tasks/${taskId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeTaskForm(input)),
    },
    token,
  );
}

export async function deleteKanbanTask(projectId: string, taskId: string) {
  const token = await requireAccessToken();
  await requestJSON<{ message: string }>(
    `/operational/projects/${projectId}/tasks/${taskId}`,
    {
      method: "DELETE",
    },
    token,
  );
}

export async function moveKanbanTask(projectId: string, taskId: string, columnId: string, position: number) {
  const token = await requireAccessToken();
  await requestJSON<{ message: string }>(
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
    token,
  );
}

function serializeTaskForm(input: Partial<{ column_id: string }> & TaskFormValues) {
  return {
    ...(input.column_id ? { column_id: input.column_id } : {}),
    title: input.title.trim(),
    description: input.description.trim() || null,
    assignee_id: input.assignee_id.trim() || null,
    due_date: input.due_date ? new Date(input.due_date).toISOString() : null,
    priority: input.priority,
    label: input.label.trim() || null,
  };
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

import type { ProjectPriority } from "@/types/project";

export interface KanbanColumn {
  id: string;
  project_id: string;
  name: string;
  column_type: "todo" | "in_progress" | "done" | "custom";
  position: number;
  color?: string | null;
  created_at: string;
}

export interface KanbanTaskField {
  id: string;
  task_id: string;
  name: string;
  value: string;
  position: number;
  created_at: string;
  updated_at: string;
}

export interface TaskFieldDraft {
  name: string;
  value: string;
}

// Custom task-field limits. Keep in sync with the backend validation in
// backend/internal/dto/operational/kanban_task_field.go.
export const MAX_TASK_FIELDS = 50;
export const MAX_TASK_FIELD_NAME = 120;
export const MAX_TASK_FIELD_VALUE = 20000;

export interface KanbanTaskListItem {
  id: string;
  column_id: string;
  title: string;
  description?: string | null;
  assignee_id?: string | null;
  assignee_name?: string | null;
  avatar_url?: string | null;
  due_date?: string | null;
  priority: ProjectPriority;
  label?: string | null;
  assigned_via: "manual" | "auto";
  position: number;
}

export interface KanbanTask {
  id: string;
  column_id: string;
  project_id: string;
  title: string;
  description?: string | null;
  assignee_id?: string | null;
  assignee_name?: string | null;
  avatar_url?: string | null;
  due_date?: string | null;
  priority: ProjectPriority;
  label?: string | null;
  assigned_via: "manual" | "auto";
  position: number;
  created_by: string;
  created_at: string;
  updated_at: string;
  fields?: KanbanTaskField[] | null;
}

export interface KanbanFilters {
  assignee: string;
  priority: string;
  label: string;
  dueDate: string;
  search: string;
  columnId: string;
}

export interface TaskFormValues {
  title: string;
  description: string;
  assignee_id: string;
  due_date: string;
  priority: ProjectPriority;
  label: string;
  fields: TaskFieldDraft[];
}

export type ProjectStatus =
  | "draft"
  | "active"
  | "on_hold"
  | "completed"
  | "archived";

export type ProjectPriority = "low" | "medium" | "high" | "critical";

export type AutoAssignMode = "off" | "round_robin" | "least_busy";

export interface Project {
  id: string;
  name: string;
  description?: string | null;
  deadline?: string | null;
  status: ProjectStatus;
  priority: ProjectPriority;
  auto_assign_mode: AutoAssignMode;
  created_by: string;
  created_at: string;
  updated_at: string;
  member_count: number;
}

export interface ProjectMember {
  project_id: string;
  user_id: string;
  role_in_project: string;
  assigned_at: string;
  user_email?: string;
  full_name?: string;
  avatar_url?: string | null;
}

export interface ProjectDetail {
  project: Project;
  members: ProjectMember[];
}

export interface PaginationMeta {
  page: number;
  per_page: number;
  total: number;
}

export interface ListProjectsResponse {
  items: Project[];
  meta: PaginationMeta;
}

export interface ProjectFilters {
  page: number;
  perPage: number;
  search: string;
  status: string;
  priority: string;
}

export interface ProjectFormValues {
  name: string;
  description: string;
  deadline: string;
  status: ProjectStatus;
  priority: ProjectPriority;
  auto_assign_mode?: AutoAssignMode;
  member_emails?: string[];
}

export interface AvailableUser {
  id: string;
  email: string;
  full_name: string;
  avatar_url?: string | null;
}

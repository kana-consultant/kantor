import { ApiError, requestEnvelope, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type {
  AvailableUser,
  ListProjectsResponse,
  PaginationMeta,
  ProjectDetail,
  ProjectFilters,
  ProjectFormValues,
} from "@/types/project";

interface ProjectMemberMutationInput {
  operation: "assign" | "remove";
  user_id?: string;
  user_email?: string;
  role_in_project?: string;
}

export const projectsKeys = {
  all: ["operational", "projects"] as const,
  list: (filters: ProjectFilters) =>
    [...projectsKeys.all, { ...filters }] as const,
  detail: (projectId: string) => [...projectsKeys.all, projectId] as const,
};

export async function listAvailableUsers(): Promise<AvailableUser[]> {
  const token = await requireAccessToken();
  return requestJSON<AvailableUser[]>("/operational/projects/available-users", { method: "GET" }, token);
}

export async function listProjects(filters: ProjectFilters): Promise<ListProjectsResponse> {
  const token = await requireAccessToken();
  const params = new URLSearchParams();

  params.set("page", String(filters.page));
  params.set("per_page", String(filters.perPage));
  if (filters.search) {
    params.set("search", filters.search);
  }
  if (filters.status) {
    params.set("status", filters.status);
  }
  if (filters.priority) {
    params.set("priority", filters.priority);
  }

  const payload = await requestEnvelope<ListProjectsResponse["items"]>(
    `/operational/projects?${params.toString()}`,
    { method: "GET" },
    token,
  );

  return {
    items: payload.data,
    meta: (payload.meta as PaginationMeta | undefined) ?? {
      page: filters.page,
      per_page: filters.perPage,
      total: 0,
    },
  };
}

export async function getProject(projectId: string) {
  const token = await requireAccessToken();
  return requestJSON<ProjectDetail>(`/operational/projects/${projectId}`, { method: "GET" }, token);
}

export async function createProject(input: ProjectFormValues) {
  const token = await requireAccessToken();
  return requestJSON<ProjectDetail>(
    "/operational/projects",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeProjectForm(input)),
    },
    token,
  );
}

export async function updateProject(projectId: string, input: ProjectFormValues) {
  const token = await requireAccessToken();
  return requestJSON<ProjectDetail>(
    `/operational/projects/${projectId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeProjectForm(input)),
    },
    token,
  );
}

export async function deleteProject(projectId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(
    `/operational/projects/${projectId}`,
    {
      method: "DELETE",
    },
    token,
  );
}

export async function mutateProjectMember(
  projectId: string,
  input: ProjectMemberMutationInput,
) {
  const token = await requireAccessToken();
  return requestJSON<ProjectDetail>(
    `/operational/projects/${projectId}/members`,
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

function serializeProjectForm(input: ProjectFormValues) {
  return {
    name: input.name.trim(),
    description: input.description.trim() || null,
    deadline: input.deadline ? new Date(input.deadline).toISOString() : null,
    status: input.status,
    priority: input.priority,
    ...(input.auto_assign_mode ? { auto_assign_mode: input.auto_assign_mode } : {}),
    ...(input.member_emails && input.member_emails.length > 0 ? { member_emails: input.member_emails } : {}),
  };
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

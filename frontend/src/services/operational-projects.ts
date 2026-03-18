import { authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
import type {
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

export async function listProjects(filters: ProjectFilters): Promise<ListProjectsResponse> {
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

  const payload = await authRequestEnvelope<ListProjectsResponse["items"]>(
    `/operational/projects?${params.toString()}`,
    { method: "GET" },
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
  return authRequestJSON<ProjectDetail>(`/operational/projects/${projectId}`, { method: "GET" });
}

export async function createProject(input: ProjectFormValues) {
  return authRequestJSON<ProjectDetail>(
    "/operational/projects",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeProjectForm(input)),
    },
  );
}

export async function updateProject(projectId: string, input: ProjectFormValues) {
  return authRequestJSON<ProjectDetail>(
    `/operational/projects/${projectId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeProjectForm(input)),
    },
  );
}

export async function deleteProject(projectId: string) {
  return authRequestJSON<{ message: string }>(
    `/operational/projects/${projectId}`,
    {
      method: "DELETE",
    },
  );
}

export async function mutateProjectMember(
  projectId: string,
  input: ProjectMemberMutationInput,
) {
  return authRequestJSON<ProjectDetail>(
    `/operational/projects/${projectId}/members`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
  );
}

function serializeProjectForm(input: ProjectFormValues) {
  return {
    name: input.name.trim(),
    description: input.description.trim() || null,
    deadline: input.deadline ? new Date(input.deadline).toISOString() : null,
    status: input.status,
    priority: input.priority,
  };
}

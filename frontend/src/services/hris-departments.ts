import { ApiError, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type { Department, DepartmentFormValues } from "@/types/hris";

export const departmentsKeys = {
  all: ["hris", "departments"] as const,
  list: () => [...departmentsKeys.all, "list"] as const,
  detail: (departmentId: string) => [...departmentsKeys.all, departmentId] as const,
};

export async function listDepartments() {
  const token = await requireAccessToken();
  return requestJSON<Department[]>("/hris/departments", { method: "GET" }, token);
}

export async function getDepartment(departmentId: string) {
  const token = await requireAccessToken();
  return requestJSON<Department>(`/hris/departments/${departmentId}`, { method: "GET" }, token);
}

export async function createDepartment(input: DepartmentFormValues) {
  const token = await requireAccessToken();
  return requestJSON<Department>(
    "/hris/departments",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeDepartmentForm(input)),
    },
    token,
  );
}

export async function updateDepartment(departmentId: string, input: DepartmentFormValues) {
  const token = await requireAccessToken();
  return requestJSON<Department>(
    `/hris/departments/${departmentId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeDepartmentForm(input)),
    },
    token,
  );
}

export async function deleteDepartment(departmentId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(`/hris/departments/${departmentId}`, { method: "DELETE" }, token);
}

function serializeDepartmentForm(input: DepartmentFormValues) {
  return {
    name: input.name.trim(),
    description: input.description.trim() || null,
    head_id: input.head_id.trim() || null,
  };
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

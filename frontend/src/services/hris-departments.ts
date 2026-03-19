import { authRequestJSON } from "@/lib/api-client";
import type { Department, DepartmentFormValues } from "@/types/hris";

export const departmentsKeys = {
  all: ["hris", "departments"] as const,
  list: () => [...departmentsKeys.all, "list"] as const,
  detail: (departmentId: string) => [...departmentsKeys.all, departmentId] as const,
};

export async function listDepartments() {
  return authRequestJSON<Department[]>("/hris/departments", { method: "GET" });
}

export async function getDepartment(departmentId: string) {
  return authRequestJSON<Department>(`/hris/departments/${departmentId}`, { method: "GET" });
}

export async function createDepartment(input: DepartmentFormValues) {
  return authRequestJSON<Department>(
    "/hris/departments",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeDepartmentForm(input)),
    },
  );
}

export async function updateDepartment(departmentId: string, input: DepartmentFormValues) {
  return authRequestJSON<Department>(
    `/hris/departments/${departmentId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeDepartmentForm(input)),
    },
  );
}

export async function deleteDepartment(departmentId: string) {
  return authRequestJSON<{ message: string }>(`/hris/departments/${departmentId}`, { method: "DELETE" });
}

function serializeDepartmentForm(input: DepartmentFormValues) {
  return {
    name: input.name.trim(),
    description: input.description.trim() || null,
    head_id: input.head_id.trim() || null,
  };
}

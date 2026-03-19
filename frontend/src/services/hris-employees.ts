import { ApiError, requestEnvelope, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type {
  Employee,
  EmployeeFilters,
  EmployeeFormValues,
  ListEmployeesResponse,
  PaginationMeta,
} from "@/types/hris";

export const employeesKeys = {
  all: ["hris", "employees"] as const,
  list: (filters: EmployeeFilters) => [...employeesKeys.all, { ...filters }] as const,
  detail: (employeeId: string) => [...employeesKeys.all, employeeId] as const,
};

export async function listEmployees(filters: EmployeeFilters): Promise<ListEmployeesResponse> {
  const token = await requireAccessToken();
  const params = new URLSearchParams();

  params.set("page", String(filters.page));
  params.set("per_page", String(filters.perPage));
  if (filters.search) {
    params.set("search", filters.search);
  }
  if (filters.department) {
    params.set("department", filters.department);
  }
  if (filters.status) {
    params.set("status", filters.status);
  }

  const payload = await requestEnvelope<ListEmployeesResponse["items"]>(
    `/hris/employees?${params.toString()}`,
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

export async function getEmployee(employeeId: string) {
  const token = await requireAccessToken();
  return requestJSON<Employee>(`/hris/employees/${employeeId}`, { method: "GET" }, token);
}

export async function createEmployee(input: EmployeeFormValues) {
  const token = await requireAccessToken();
  return requestJSON<Employee>(
    "/hris/employees",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeEmployeeForm(input)),
    },
    token,
  );
}

export async function updateEmployee(employeeId: string, input: EmployeeFormValues) {
  const token = await requireAccessToken();
  return requestJSON<Employee>(
    `/hris/employees/${employeeId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeEmployeeForm(input)),
    },
    token,
  );
}

export async function deleteEmployee(employeeId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(`/hris/employees/${employeeId}`, { method: "DELETE" }, token);
}

function serializeEmployeeForm(input: EmployeeFormValues) {
  return {
    full_name: input.full_name.trim(),
    email: input.email.trim().toLowerCase(),
    phone: input.phone.trim() || null,
    position: input.position.trim(),
    department: input.department.trim() || null,
    date_joined: new Date(input.date_joined).toISOString(),
    employment_status: input.employment_status,
    address: input.address.trim() || null,
    emergency_contact: input.emergency_contact.trim() || null,
    avatar_url: input.avatar_url.trim() || null,
  };
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

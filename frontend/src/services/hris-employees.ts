import { authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
import { toDateOnlyString } from "@/lib/date";
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

  const payload = await authRequestEnvelope<ListEmployeesResponse["items"]>(
    `/hris/employees?${params.toString()}`,
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

export async function getEmployee(employeeId: string) {
  return authRequestJSON<Employee>(`/hris/employees/${employeeId}`, { method: "GET" });
}

export async function getMyEmployee() {
  return authRequestJSON<Employee>("/hris/employees/me", { method: "GET" });
}

export async function createEmployee(input: EmployeeFormValues) {
  return authRequestJSON<Employee>(
    "/hris/employees",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeEmployeeForm(input)),
    },
  );
}

export async function updateEmployee(employeeId: string, input: EmployeeFormValues) {
  return authRequestJSON<Employee>(
    `/hris/employees/${employeeId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeEmployeeForm(input)),
    },
  );
}

export async function uploadEmployeeAvatar(employeeId: string, file: File) {
  const formData = new FormData();
  formData.set("avatar", file);

  return authRequestJSON<Employee>(
    `/hris/employees/${employeeId}/avatar`,
    {
      method: "POST",
      body: formData,
    },
  );
}

export async function deleteEmployee(employeeId: string) {
  return authRequestJSON<{ message: string }>(`/hris/employees/${employeeId}`, { method: "DELETE" });
}

function serializeEmployeeForm(input: EmployeeFormValues) {
  return {
    full_name: input.full_name.trim(),
    email: input.email.trim().toLowerCase(),
    phone: input.phone.trim() || null,
    position: input.position.trim(),
    department: input.department.trim() || null,
    date_joined: toDateOnlyString(input.date_joined),
    employment_status: input.employment_status,
    address: input.address.trim() || null,
    emergency_contact: input.emergency_contact.trim() || null,
    avatar_url: input.avatar_url.trim() || null,
    bank_account_number: input.bank_account_number.trim() || null,
    bank_name: input.bank_name.trim() || null,
    linkedin_profile: input.linkedin_profile.trim() || null,
    ssh_keys: input.ssh_keys.trim() || null,
  };
}


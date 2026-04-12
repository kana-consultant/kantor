import { authGetJSON, authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
import type {
  AdminRoleDetail,
  AdminRoleSummary,
  AdminSettings,
  AdminUserDetail,
  AdminUserFilters,
  AdminUserSummary,
  ListAdminUsersResponse,
  ListRolesFilters,
  ModuleItem,
  PermissionModuleGroup,
  SetUserModuleRolePayload,
  UpsertRolePayload,
} from "@/types/admin";
import type { PaginationMeta } from "@/types/hris";
import type { Department } from "@/types/hris";

export const adminRbacKeys = {
  all: ["admin-rbac"] as const,
  roles: () => [...adminRbacKeys.all, "roles"] as const,
  roleList: (filters: ListRolesFilters) =>
    [...adminRbacKeys.roles(), "list", { ...filters }] as const,
  roleDetail: (roleID: string) => [...adminRbacKeys.roles(), roleID] as const,
  permissions: () => [...adminRbacKeys.all, "permissions"] as const,
  users: () => [...adminRbacKeys.all, "users"] as const,
  userList: (filters: AdminUserFilters) =>
    [...adminRbacKeys.users(), "list", { ...filters }] as const,
  userDetail: (userID: string) => [...adminRbacKeys.users(), userID] as const,
  settings: () => [...adminRbacKeys.all, "settings"] as const,
  settingsDepartments: () => [...adminRbacKeys.settings(), "departments"] as const,
  modules: () => [...adminRbacKeys.all, "modules"] as const,
};

export async function listRoles(
  filters: ListRolesFilters,
): Promise<AdminRoleSummary[]> {
  const params = new URLSearchParams();
  if (filters.search.trim()) {
    params.set("search", filters.search.trim());
  }
  if (filters.isSystem != null) {
    params.set("is_system", String(filters.isSystem));
  }
  if (filters.isActive != null) {
    params.set("is_active", String(filters.isActive));
  }

  const suffix = params.toString();
  return authGetJSON<AdminRoleSummary[]>(
    `/admin/roles${suffix ? `?${suffix}` : ""}`,
  );
}

export function getRole(roleID: string) {
  return authGetJSON<AdminRoleDetail>(`/admin/roles/${roleID}`);
}

export function createRole(payload: UpsertRolePayload) {
  return authRequestJSON<AdminRoleDetail>("/admin/roles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function updateRole(roleID: string, payload: UpsertRolePayload) {
  return authRequestJSON<AdminRoleDetail>(`/admin/roles/${roleID}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function deleteRole(roleID: string) {
  return authRequestJSON<{ success: boolean }>(`/admin/roles/${roleID}`, {
    method: "DELETE",
  });
}

export function toggleRole(roleID: string) {
  return authRequestJSON<AdminRoleDetail>(`/admin/roles/${roleID}/toggle`, {
    method: "PATCH",
  });
}

export function duplicateRole(roleID: string) {
  return authRequestJSON<AdminRoleDetail>(`/admin/roles/${roleID}/duplicate`, {
    method: "POST",
  });
}

export async function listPermissionGroups(): Promise<PermissionModuleGroup[]> {
  const response = await authGetJSON<{ modules: PermissionModuleGroup[] }>(
    "/admin/permissions",
  );
  return response.modules ?? [];
}

export async function listAdminUsers(
  filters: AdminUserFilters,
): Promise<ListAdminUsersResponse> {
  const params = new URLSearchParams();
  params.set("page", String(filters.page));
  params.set("per_page", String(filters.perPage));
  if (filters.search.trim()) {
    params.set("search", filters.search.trim());
  }
  if (filters.moduleId) {
    params.set("module", filters.moduleId);
  }
  if (filters.roleId) {
    params.set("role", filters.roleId);
  }
  if (filters.superAdmin != null) {
    params.set("super_admin", String(filters.superAdmin));
  }

  const response = await authRequestEnvelope<AdminUserDetail[] | AdminUserSummary[]>(
    `/admin/users?${params.toString()}`,
    { method: "GET" },
  );

  return {
    items: (response.data ?? []) as AdminUserSummary[],
    meta:
      (response.meta as PaginationMeta | undefined) ?? {
        page: filters.page,
        per_page: filters.perPage,
        total: 0,
      },
  };
}

export function getAdminUser(userID: string) {
  return authGetJSON<AdminUserDetail>(`/admin/users/${userID}`);
}

export function updateAdminUserModuleRoles(
  userID: string,
  moduleRoles: SetUserModuleRolePayload[],
) {
  return authRequestJSON<AdminUserDetail>(`/admin/users/${userID}/roles`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ module_roles: moduleRoles }),
  });
}

export function toggleAdminUserActive(userID: string, active: boolean) {
  return authRequestJSON<{ success: boolean }>(`/admin/users/${userID}/active`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ active }),
  });
}

export function toggleAdminUserSuperAdmin(userID: string, enabled: boolean) {
  return authRequestJSON<AdminUserDetail>(
    `/admin/users/${userID}/toggle-super-admin`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled }),
    },
  );
}

export function ensureAdminUserEmployeeProfile(userID: string) {
  const normalizedUserID = userID.trim();
  if (!normalizedUserID) {
    throw new Error("User ID tidak valid");
  }
  return authRequestJSON<AdminUserDetail>(
    `/admin/users/${normalizedUserID}/ensure-employee-profile`,
    {
      method: "POST",
    },
  );
}

export function getAdminSettings() {
  return authGetJSON<AdminSettings>("/admin/settings");
}

export function listSettingsDepartments() {
  return authGetJSON<Department[]>("/admin/settings/departments");
}

export function updateDefaultRoles(defaultRoles: Record<string, string | null>) {
  return authRequestJSON<AdminSettings>("/admin/settings/default-roles", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ default_roles: defaultRoles }),
  });
}

export function updateAutoCreateEmployee(payload: {
  enabled: boolean;
  default_department_id: string | null;
}) {
  return authRequestJSON<AdminSettings>("/admin/settings/auto-create-employee", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function updateMailDelivery(payload: {
  enabled: boolean;
  provider: string;
  sender_name: string;
  sender_email: string;
  reply_to_email: string | null;
  api_key: string | null;
  clear_api_key: boolean;
  password_reset_enabled: boolean;
  password_reset_expiry_minutes: number;
  notification_enabled: boolean;
}) {
  return authRequestJSON<AdminSettings>("/admin/settings/mail-delivery", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function updateReimbursementReminder(payload: {
  enabled: boolean;
  review: {
    enabled: boolean;
    cron: string;
    channels: {
      in_app: boolean;
      email: boolean;
      whatsapp: boolean;
    };
  };
  payment: {
    enabled: boolean;
    cron: string;
    channels: {
      in_app: boolean;
      email: boolean;
      whatsapp: boolean;
    };
  };
}) {
  return authRequestJSON<AdminSettings>("/admin/settings/reimbursement-reminder", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function listModules() {
  return authGetJSON<ModuleItem[]>("/modules");
}

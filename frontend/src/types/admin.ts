import type { PaginationMeta } from "@/types/hris";
import type { AuthModuleRole, AuthUser } from "@/types/auth";

export interface AdminRoleSummary {
  id: string;
  name: string;
  slug: string;
  description: string;
  is_system: boolean;
  is_active: boolean;
  hierarchy_level: number;
  permissions_count: number;
  users_count: number;
}

export interface AdminRoleDetail extends AdminRoleSummary {
  permission_ids: string[];
}

export interface PermissionItem {
  id: string;
  resource: string;
  action: string;
  description: string;
  is_sensitive: boolean;
}

export interface PermissionModuleGroup {
  id: string;
  name: string;
  description: string;
  permissions: PermissionItem[];
}

export interface ModuleItem {
  id: string;
  name: string;
  description: string;
  display_order: number;
}

export interface AdminUserSummary {
  user: AuthUser;
  module_roles: Record<string, AuthModuleRole>;
  is_super_admin: boolean;
  has_employee_profile: boolean;
  employee_id?: string | null;
}

export interface AdminUserDetail extends AdminUserSummary {
  effective_permissions: string[];
}

export interface AdminUserFilters {
  page: number;
  perPage: number;
  search: string;
  moduleId?: string;
  roleId?: string;
  superAdmin?: boolean | null;
}

export interface ListAdminUsersResponse {
  items: AdminUserSummary[];
  meta: PaginationMeta;
}

export interface ListRolesFilters {
  search: string;
  isSystem?: boolean | null;
  isActive?: boolean | null;
}

export interface UpsertRolePayload {
  name: string;
  slug: string;
  description: string;
  hierarchy_level: number;
  permission_ids: string[];
}

export interface SetUserModuleRolePayload {
  module_id: string;
  role_id: string | null;
}

export interface RoleReference {
  role_id: string | null;
  role_name: string | null;
  role_slug: string | null;
}

export interface AutoCreateEmployeeSetting {
  enabled: boolean;
  default_department_id: string | null;
}

export interface AdminSettings {
  default_roles: Record<string, RoleReference>;
  auto_create_employee: AutoCreateEmployeeSetting;
}

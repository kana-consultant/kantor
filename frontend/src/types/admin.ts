import type { PaginationMeta } from "@/types/hris";

export interface AdminUser {
  user: {
    id: string;
    email: string;
    full_name: string;
    avatar_url?: string | null;
    department?: string | null;
    skills?: string[];
    is_active: boolean;
    created_at: string;
    updated_at: string;
  };
  roles: string[];
}

export interface ListUsersResponse {
  items: AdminUser[];
  meta: PaginationMeta;
}

export interface RoleKeyDTO {
  name: string;
  module: string;
}

export interface AdminUserFilters {
  page: number;
  perPage: number;
  search: string;
}

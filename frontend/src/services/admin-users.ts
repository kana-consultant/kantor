import { ApiError, requestJSON, requestEnvelope } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type { AdminUser, AdminUserFilters, ListUsersResponse, RoleKeyDTO } from "@/types/admin";
import type { PaginationMeta } from "@/types/hris";

export const adminUsersKeys = {
  all: ["admin", "users"] as const,
  list: (filters: AdminUserFilters) => [...adminUsersKeys.all, { ...filters }] as const,
  detail: (userId: string) => [...adminUsersKeys.all, userId] as const,
};

export async function listUsers(filters: AdminUserFilters): Promise<ListUsersResponse> {
  const token = await requireAccessToken();
  const params = new URLSearchParams();
  params.set("page", String(filters.page));
  params.set("per_page", String(filters.perPage));
  if (filters.search) params.set("search", filters.search);

  const payload = await requestEnvelope<AdminUser[]>(
    `/admin/users?${params.toString()}`,
    { method: "GET" },
    token,
  );

  return {
    items: payload.data ?? [],
    meta: (payload.meta as PaginationMeta | undefined) ?? {
      page: filters.page,
      per_page: filters.perPage,
      total: 0,
    },
  };
}

export async function getUser(userId: string): Promise<AdminUser> {
  const token = await requireAccessToken();
  return requestJSON<AdminUser>(`/admin/users/${userId}`, { method: "GET" }, token);
}

export async function updateUserRoles(userId: string, roles: RoleKeyDTO[]): Promise<AdminUser> {
  const token = await requireAccessToken();
  return requestJSON<AdminUser>(
    `/admin/users/${userId}/roles`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ roles }),
    },
    token,
  );
}

export async function toggleUserActive(userId: string, active: boolean): Promise<void> {
  const token = await requireAccessToken();
  await requestJSON(
    `/admin/users/${userId}/active`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ active }),
    },
    token,
  );
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }
  return session.tokens.access_token;
}

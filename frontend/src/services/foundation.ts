import { authGetJSON, getJSON } from "@/lib/api-client";
import type { AuthModuleRole, AuthUser } from "@/types/auth";

export interface HealthStatus {
  status: string;
}

export interface SessionProfile {
  user: AuthUser;
  module_roles: Record<string, AuthModuleRole>;
  permissions: string[];
  is_super_admin: boolean;
}

export function fetchApiHealth() {
  return getJSON<HealthStatus>("/health");
}

export async function fetchSessionProfile() {
  return authGetJSON<SessionProfile>("/auth/me");
}

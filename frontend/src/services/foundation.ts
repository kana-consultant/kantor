import { authGetJSON, authRequestJSON, getJSON } from "@/lib/api-client";
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


export interface BrowserClientContext {
  timezone: string | null;
  timezone_offset_minutes: number;
  locale: string | null;
}

export async function syncClientContext(input: BrowserClientContext) {
  const payload = await authRequestJSON<{ user: AuthUser }>("/auth/client-context", {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(input),
  });
  return payload.user;
}

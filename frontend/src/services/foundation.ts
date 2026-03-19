import { authGetJSON, getJSON } from "@/lib/api-client";
import type { AuthUser } from "@/types/auth";

export interface HealthStatus {
  status: string;
}

export interface SessionProfile {
  user: AuthUser;
  roles: string[];
  permissions: string[];
}

export function fetchApiHealth() {
  return getJSON<HealthStatus>("/health");
}

export async function fetchSessionProfile() {
  return authGetJSON<SessionProfile>("/auth/me");
}

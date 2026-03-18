import { ApiError, getJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
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
  const token = await requireAccessToken();
  return getJSON<SessionProfile>("/auth/me", token);
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

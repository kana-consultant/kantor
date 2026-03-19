import { ApiError, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type { Employee } from "@/types/hris";

export const profileKeys = {
  me: ["profile", "me"] as const,
};

export interface UpdateProfileInput {
  full_name: string;
  phone: string | null;
  address: string | null;
  emergency_contact: string | null;
  avatar_url: string | null;
}

export async function getProfile(): Promise<Employee> {
  const token = await requireAccessToken();
  return requestJSON<Employee>("/auth/profile", { method: "GET" }, token);
}

export async function updateProfile(input: UpdateProfileInput): Promise<Employee> {
  const token = await requireAccessToken();
  return requestJSON<Employee>(
    "/auth/profile",
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
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

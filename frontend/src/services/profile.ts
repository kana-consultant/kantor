import { ApiError, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type { Employee } from "@/types/hris";
import { env } from "@/lib/env";

export const profileKeys = {
  me: ["profile", "me"] as const,
};

export interface UpdateProfileInput {
  full_name: string;
  phone: string | null;
  address: string | null;
  emergency_contact: string | null;
  avatar_url: string | null;
  bank_account_number: string | null;
  bank_name: string | null;
  linkedin_profile: string | null;
  ssh_keys: string | null;
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

export interface ChangeEmailInput {
  email: string;
  password: string;
}

export async function changeEmail(input: ChangeEmailInput): Promise<{ message: string }> {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(
    "/auth/profile/email",
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    },
    token,
  );
}

export async function uploadProfileAvatar(file: File): Promise<{ avatar_url: string }> {
  const token = await requireAccessToken();
  const formData = new FormData();
  formData.append("avatar", file);

  const headers = new Headers();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(`${env.VITE_API_BASE_URL}/auth/profile/avatar`, {
    method: "POST",
    headers,
    body: formData,
    credentials: "include",
  });

  const payload = await response.json();
  if (!response.ok || !payload.success) {
    if ("error" in payload) {
      throw new ApiError(response.status, payload.error.message, payload.error.code);
    }
    throw new ApiError(response.status, "Upload failed");
  }

  return payload.data;
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }
  return session.tokens.access_token;
}

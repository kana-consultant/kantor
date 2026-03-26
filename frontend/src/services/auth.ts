import { authPostJSON, postJSON } from "@/lib/api-client";
import { useAuthStore } from "@/stores/auth-store";
import type { AuthPayload, AuthSession } from "@/types/auth";

interface LoginRequest {
  email: string;
  password: string;
}

interface RegisterRequest {
  email: string;
  password: string;
  full_name: string;
}

interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
}

interface ChangePasswordResponse {
  message: string;
}

export function login(payload: LoginRequest) {
  return postJSON<AuthPayload, LoginRequest>("/auth/login", payload);
}

export function register(payload: RegisterRequest) {
  return postJSON<AuthPayload, RegisterRequest>("/auth/register", payload);
}

export function refresh() {
  return postJSON<AuthPayload, Record<string, never>>("/auth/refresh", {});
}

export function changePassword(payload: ChangePasswordRequest) {
  return authPostJSON<ChangePasswordResponse, ChangePasswordRequest>(
    "/auth/change-password",
    payload,
  );
}

export function revokeRefreshToken() {
  return postJSON<{ revoked: boolean }, Record<string, never>>(
    "/auth/logout",
    {},
  );
}

let refreshSessionPromise: Promise<AuthSession> | null = null;

export async function refreshSession() {
  if (!useAuthStore.getState().session) {
    throw new Error("No active session");
  }

  if (!refreshSessionPromise) {
    refreshSessionPromise = refresh()
      .then((refreshedSession) => {
        useAuthStore.getState().setSession(refreshedSession);
        return refreshedSession;
      })
      .finally(() => {
        refreshSessionPromise = null;
      });
  }

  return refreshSessionPromise;
}

export async function logout() {
  const store = useAuthStore.getState();

  try {
    if (store.session) {
      // Disconnect extension (fire-and-forget)
      try {
        const { disconnectExtension } = await import("@/services/extension");
        await disconnectExtension();
      } catch {
        // Extension may not be installed — don't block logout
      }

      await revokeRefreshToken();
    }
  } finally {
    store.clearSession();
  }
}

export async function ensureAuthenticated(): Promise<AuthSession | null> {
  const store = useAuthStore.getState();
  const session = store.session;

  if (!session) {
    return null;
  }

  if (!isJwtExpired(session.tokens.access_token)) {
    return session;
  }

  try {
    return await refreshSession();
  } catch {
    store.clearSession();
    return null;
  }
}

export function getValidStoredSession(): AuthSession | null {
  const session = useAuthStore.getState().session;
  if (!session) {
    return null;
  }

  return isJwtExpired(session.tokens.access_token) ? null : session;
}

function isJwtExpired(token: string) {
  const payload = parseJwtPayload(token);

  if (!payload?.exp || typeof payload.exp !== "number") {
    return true;
  }

  const nowInSeconds = Math.floor(Date.now() / 1000);
  return payload.exp <= nowInSeconds + 30;
}

function parseJwtPayload(token: string): Record<string, unknown> | null {
  const segments = token.split(".");
  if (segments.length < 2) {
    return null;
  }

  try {
    const payloadSegment = segments[1]!
      .replace(/-/g, "+")
      .replace(/_/g, "/")
      .padEnd(Math.ceil(segments[1]!.length / 4) * 4, "=");

    const json = window.atob(payloadSegment);
    return JSON.parse(json) as Record<string, unknown>;
  } catch {
    return null;
  }
}

import { postJSON } from "@/lib/api-client";
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

interface RefreshRequest {
  refresh_token: string;
}

interface LogoutRequest {
  refresh_token: string;
}

export function login(payload: LoginRequest) {
  return postJSON<AuthPayload, LoginRequest>("/auth/login", payload);
}

export function register(payload: RegisterRequest) {
  return postJSON<AuthPayload, RegisterRequest>("/auth/register", payload);
}

export function refresh(refreshToken: string) {
  return postJSON<AuthPayload, RefreshRequest>("/auth/refresh", {
    refresh_token: refreshToken,
  });
}

export function revokeRefreshToken(refreshToken: string) {
  return postJSON<{ revoked: boolean }, LogoutRequest>("/auth/logout", {
    refresh_token: refreshToken,
  });
}

export async function refreshSession() {
  const store = useAuthStore.getState();
  const refreshToken = store.session?.tokens.refresh_token;

  if (!refreshToken) {
    throw new Error("Refresh token is not available");
  }

  const refreshedSession = await refresh(refreshToken);
  store.setSession(refreshedSession);
  return refreshedSession;
}

export async function logout() {
  const store = useAuthStore.getState();
  const refreshToken = store.session?.tokens.refresh_token;

  try {
    if (refreshToken) {
      await revokeRefreshToken(refreshToken);
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

  if (!session.tokens.refresh_token) {
    store.clearSession();
    return null;
  }

  try {
    const refreshedSession = await refresh(session.tokens.refresh_token);
    store.setSession(refreshedSession);
    return refreshedSession;
  } catch {
    store.clearSession();
    return null;
  }
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
    const payloadSegment = segments[1]
      .replace(/-/g, "+")
      .replace(/_/g, "/")
      .padEnd(Math.ceil(segments[1].length / 4) * 4, "=");

    const json = window.atob(payloadSegment);
    return JSON.parse(json) as Record<string, unknown>;
  } catch {
    return null;
  }
}

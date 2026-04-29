import { env } from "@/lib/env";
import { getStoredSession, useAuthStore } from "@/stores/auth-store";
import type { AuthPayload } from "@/types/auth";

export interface ApiSuccess<TData> {
  success: true;
  data: TData;
  meta?: unknown;
}

export interface ApiFailure {
  success: false;
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}

export class ApiError extends Error {
  status: number;
  code?: string;
  details?: unknown;

  constructor(status: number, message: string, code?: string, details?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

const API_BASE_URL = env.VITE_API_BASE_URL;

export async function requestJSON<TData>(
  path: string,
  init: RequestInit = {},
  token?: string,
): Promise<TData> {
  const payload = await requestEnvelope<TData>(path, init, token);
  return payload.data;
}

export async function requestEnvelope<TData>(
  path: string,
  init: RequestInit = {},
  token?: string,
): Promise<ApiSuccess<TData>> {
  const headers = new Headers(init.headers);

  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  // Always advertise XHR. The backend requires this header on cookie-only
  // endpoints (/auth/refresh, /auth/logout) as a CSRF guard, and tagging
  // every request keeps the header set even when callers go through
  // requestEnvelope directly.
  if (!headers.has("X-Requested-With")) {
    headers.set("X-Requested-With", "XMLHttpRequest");
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });

  const payload = (await response.json()) as ApiSuccess<TData> | ApiFailure;

  if (!response.ok || !payload.success) {
    if ("error" in payload) {
      throw new ApiError(
        response.status,
        payload.error.message,
        payload.error.code,
        payload.error.details,
      );
    }

    throw new ApiError(response.status, "Request failed");
  }

  return payload;
}

export async function postJSON<TData, TBody>(
  path: string,
  body: TBody,
  token?: string,
): Promise<TData> {
  return requestJSON<TData>(
    path,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function getJSON<TData>(path: string, token?: string) {
  return requestJSON<TData>(
    path,
    {
      method: "GET",
    },
    token,
  );
}

// --- Authenticated wrappers (auto-attach token + 401 refresh retry) ---

function getAccessToken(): string | undefined {
  return getStoredSession()?.tokens.access_token;
}

let refreshSessionPromise: Promise<AuthPayload> | null = null;

async function refreshAuthenticatedSession() {
  if (!refreshSessionPromise) {
    refreshSessionPromise = (async () => {
      const response = await fetch(`${API_BASE_URL}/auth/refresh`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          // CSRF guard — backend requires this header on /auth/refresh.
          // Browsers cannot set it on cross-site form posts without a CORS
          // preflight, so it shuts down navigation/CSRF replays of the cookie.
          "X-Requested-With": "XMLHttpRequest",
        },
        body: JSON.stringify({}),
        credentials: "include",
      });

      const payload = (await response.json()) as ApiSuccess<AuthPayload> | ApiFailure;

      if (!response.ok || !payload.success) {
        if ("error" in payload) {
          throw new ApiError(
            response.status,
            payload.error.message,
            payload.error.code,
            payload.error.details,
          );
        }

        throw new ApiError(response.status, "Request failed");
      }

      useAuthStore.getState().setSession(payload.data);
      return payload.data;
    })().finally(() => {
      refreshSessionPromise = null;
    });
  }

  return refreshSessionPromise;
}

export async function refreshAuthSession() {
  return refreshAuthenticatedSession();
}

function clearSessionAndRedirectToLogin() {
  useAuthStore.getState().clearSession();
  // Hard navigation: app state is potentially corrupted at this
  // point, so a full reload ensures no stale data remains.
  window.location.href = "/login";
}

async function handleAuthRetry<T>(
  fn: (token?: string) => Promise<T>,
): Promise<T> {
  const attemptedToken = getAccessToken();

  try {
    return await fn(attemptedToken);
  } catch (err) {
    if (
      err instanceof ApiError &&
      err.status === 403 &&
      err.code === "INACTIVE_USER"
    ) {
      clearSessionAndRedirectToLogin();
      throw err;
    }

    if (err instanceof ApiError && err.status === 401) {
      const currentToken = getAccessToken();
      if (attemptedToken && currentToken && currentToken !== attemptedToken) {
        return await fn(currentToken);
      }

      try {
        await refreshAuthenticatedSession();

        const refreshedToken = getAccessToken();
        return await fn(refreshedToken);
      } catch (refreshErr) {
        const latestToken = getAccessToken();
        if (attemptedToken && latestToken && latestToken !== attemptedToken) {
          return await fn(latestToken);
        }

        clearSessionAndRedirectToLogin();
        throw refreshErr;
      }
    }
    throw err;
  }
}

export function authGetJSON<TData>(path: string): Promise<TData> {
  return handleAuthRetry((token) => getJSON<TData>(path, token));
}

export function authPostJSON<TData, TBody>(
  path: string,
  body: TBody,
): Promise<TData> {
  return handleAuthRetry((token) => postJSON<TData, TBody>(path, body, token));
}

export function authRequestJSON<TData>(
  path: string,
  init: RequestInit = {},
): Promise<TData> {
  return handleAuthRetry((token) => requestJSON<TData>(path, init, token));
}

export function authRequestEnvelope<TData>(
  path: string,
  init: RequestInit = {},
): Promise<ApiSuccess<TData>> {
  return handleAuthRetry((token) => requestEnvelope<TData>(path, init, token));
}

export interface DownloadResult {
  blob: Blob;
  filename?: string;
}

async function requestBinary(
  path: string,
  init: RequestInit = {},
  token?: string,
): Promise<DownloadResult> {
  const headers = new Headers(init.headers);

  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });

  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as ApiFailure | null;
    if (payload && "error" in payload) {
      throw new ApiError(
        response.status,
        payload.error.message,
        payload.error.code,
        payload.error.details,
      );
    }

    throw new ApiError(response.status, "Request failed");
  }

  const contentDisposition = response.headers.get("Content-Disposition") || "";
  const filenameMatch = /filename="?(?<filename>[^";]+)"?/i.exec(contentDisposition);

  return {
    blob: await response.blob(),
    filename: filenameMatch?.groups?.filename,
  };
}

export function authDownload(path: string, init: RequestInit = {}): Promise<DownloadResult> {
  return handleAuthRetry((token) => requestBinary(path, init, token));
}

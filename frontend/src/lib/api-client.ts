import { env } from "@/lib/env";

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

import { getStoredSession } from "@/stores/auth-store";

function getAccessToken(): string | undefined {
  return getStoredSession()?.tokens.access_token;
}

let refreshPromise: Promise<unknown> | null = null;

async function handleAuthRetry<T>(
  fn: (token?: string) => Promise<T>,
): Promise<T> {
  try {
    return await fn(getAccessToken());
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      try {
        if (!refreshPromise) {
          const { refreshSession } = await import("@/services/auth");
          refreshPromise = refreshSession();
        }
        await refreshPromise;
        return await fn(getAccessToken());
      } catch {
        const { useAuthStore } = await import("@/stores/auth-store");
        useAuthStore.getState().clearSession();
        // Hard navigation: app state is potentially corrupted at this
        // point, so a full reload ensures no stale data remains.
        window.location.href = "/login";
        throw err;
      } finally {
        refreshPromise = null;
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

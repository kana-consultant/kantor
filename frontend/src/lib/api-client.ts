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

const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1";

export async function requestJSON<TData>(
  path: string,
  init: RequestInit = {},
  token?: string,
): Promise<TData> {
  const headers = new Headers(init.headers);

  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers,
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

  return payload.data;
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

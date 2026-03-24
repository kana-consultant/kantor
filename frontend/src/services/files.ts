import { ApiError } from "@/lib/api-client";
import { env } from "@/lib/env";
import { ensureAuthenticated } from "@/services/auth";

export type ProtectedFileType = "campaigns" | "reimbursements" | "employees" | "profiles";

export interface ProtectedFileReference {
  type: ProtectedFileType;
  resourceId: string;
  filename: string;
}

const API_BASE_URL = env.VITE_API_BASE_URL;

export async function fetchProtectedFileBlob(
  type: ProtectedFileType,
  resourceId: string,
  filename: string,
): Promise<Blob> {
  const session = await ensureAuthenticated();
  const token = session?.tokens.access_token;

  if (!token) {
    throw new ApiError(401, "Session is not available");
  }

  const response = await fetch(
    `${API_BASE_URL}/files/${type}/${resourceId}/${encodeURIComponent(filename)}`,
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    },
  );

  if (!response.ok) {
    let message = "Request failed";
    let code: string | undefined;

    try {
      const payload = (await response.json()) as {
        success?: boolean;
        error?: { code?: string; message?: string };
      };
      message = payload.error?.message ?? message;
      code = payload.error?.code;
    } catch {
      message = "File request failed";
    }

    throw new ApiError(response.status, message, code);
  }

  return response.blob();
}

export async function openProtectedFile(
  type: ProtectedFileType,
  resourceId: string,
  filename: string,
) {
  const blob = await fetchProtectedFileBlob(type, resourceId, filename);
  const objectURL = URL.createObjectURL(blob);
  const popup = window.open(objectURL, "_blank", "noopener,noreferrer");

  if (!popup) {
    const link = document.createElement("a");
    link.href = objectURL;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.click();
  }

  window.setTimeout(() => {
    URL.revokeObjectURL(objectURL);
  }, 60_000);
}

export function getProtectedFileName(filePath: string) {
  const parts = filePath.split("/");
  return parts[parts.length - 1] ?? filePath;
}

export function parseProtectedFilePath(filePath: string): ProtectedFileReference | null {
  const normalized = filePath.trim().replace(/\\/g, "/").replace(/^\/+/, "");
  const parts = normalized.split("/").filter(Boolean);

  if (parts.length < 3) {
    return null;
  }

  const [type, resourceId, ...filenameParts] = parts;
  if (type !== "campaigns" && type !== "reimbursements" && type !== "employees" && type !== "profiles") {
    return null;
  }

  const filename = filenameParts.join("/");
  if (!resourceId || !filename) {
    return null;
  }

  return {
    type,
    resourceId,
    filename,
  };
}

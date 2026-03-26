import { env } from "@/lib/env";
import { authPostJSON } from "@/lib/api-client";

declare const chrome: {
  runtime: {
    sendMessage: (
      extensionId: string,
      message: unknown,
      callback: (response: unknown) => void,
    ) => void;
    lastError?: { message?: string } | null;
  };
} | undefined;

const EXTENSION_ID = env.VITE_EXTENSION_ID;

interface ExtensionPingResponse {
  ok: boolean;
  connected?: boolean;
  consented?: boolean;
}

interface ExtensionActionResponse {
  ok: boolean;
  error?: string;
}

export type ExtensionStatus = "not_installed" | "disconnected" | "connected";

function canUseChromeMessaging(): boolean {
  return Boolean(EXTENSION_ID && typeof chrome !== "undefined" && chrome.runtime?.sendMessage);
}

function sendToExtension<T>(message: Record<string, unknown>): Promise<T> {
  return new Promise((resolve, reject) => {
    if (typeof chrome === "undefined" || !chrome.runtime?.sendMessage || !EXTENSION_ID) {
      reject(new Error("Chrome extension messaging not available"));
      return;
    }

    const chromeRuntime = chrome.runtime;

    const timeout = setTimeout(() => {
      reject(new Error("Extension did not respond"));
    }, 2500);

    chromeRuntime.sendMessage(EXTENSION_ID, message, (response: unknown) => {
      clearTimeout(timeout);
      if (chromeRuntime.lastError) {
        reject(new Error(chromeRuntime.lastError.message ?? "Extension messaging failed"));
        return;
      }
      resolve(response as T);
    });
  });
}

export async function pingExtension(): Promise<ExtensionStatus> {
  if (!canUseChromeMessaging()) {
    return "not_installed";
  }

  try {
    const response = await sendToExtension<ExtensionPingResponse>({
      type: "KANTOR_TRACKER_PING",
    });
    if (response?.connected) {
      return "connected";
    }
    return "disconnected";
  } catch {
    return "not_installed";
  }
}

interface ExtensionTokenResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
}

export async function connectExtension(apiBaseUrl: string, dashboardUrl: string): Promise<void> {
  const tokens = await authPostJSON<ExtensionTokenResponse, Record<string, never>>(
    "/auth/extension-token",
    {},
  );

  const response = await sendToExtension<ExtensionActionResponse>({
    type: "KANTOR_TRACKER_CONNECT",
    accessToken: tokens.access_token,
    refreshToken: tokens.refresh_token,
    apiBaseUrl,
    dashboardUrl: dashboardUrl || window.location.origin,
  });

  if (!response?.ok) {
    throw new Error(response?.error ?? "Failed to connect extension");
  }
}

export async function enableExtension(apiBaseUrl: string, dashboardUrl: string): Promise<void> {
  const tokens = await authPostJSON<ExtensionTokenResponse, Record<string, never>>(
    "/auth/extension-token",
    {},
  );

  const response = await sendToExtension<ExtensionActionResponse>({
    type: "KANTOR_TRACKER_ENABLE",
    accessToken: tokens.access_token,
    refreshToken: tokens.refresh_token,
    apiBaseUrl,
    dashboardUrl: dashboardUrl || window.location.origin,
  });

  if (!response?.ok) {
    throw new Error(response?.error ?? "Failed to enable extension tracking");
  }
}

export async function disconnectExtension(): Promise<void> {
  try {
    await sendToExtension<ExtensionActionResponse>({
      type: "KANTOR_TRACKER_DISCONNECT",
    });
  } catch {
    // Fire-and-forget — extension may not be installed
  }

  await authPostJSON<{ message: string }, Record<string, never>>(
    "/auth/extension-disconnect",
    {},
  );
}

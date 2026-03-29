import { authRequestEnvelope, authRequestJSON, refreshAuthSession } from "@/lib/api-client";
import { env } from "@/lib/env";
import { getStoredSession } from "@/stores/auth-store";
import type { NotificationFilters, NotificationItem } from "@/types/notification";

export const notificationsKeys = {
  all: ["notifications"] as const,
  list: (filters: NotificationFilters = {}) => [...notificationsKeys.all, "list", filters] as const,
  unreadCount: () => [...notificationsKeys.all, "unread-count"] as const,
};

export async function listNotifications(filters: NotificationFilters = {}) {
  const search = new URLSearchParams();

  search.set("page", String(filters.page ?? 1));
  search.set("per_page", String(filters.perPage ?? 10));
  if (typeof filters.read === "boolean") {
    search.set("read", String(filters.read));
  }

  const envelope = await authRequestEnvelope<NotificationItem[]>(
    `/notifications?${search.toString()}`,
    { method: "GET" },
  );

  return {
    items: envelope.data,
    meta: envelope.meta,
  };
}

export async function markNotificationRead(notificationId: string) {
  return authRequestJSON<{ marked: boolean }>(
    `/notifications/${notificationId}/read`,
    { method: "PATCH" },
  );
}

export async function markAllNotificationsRead() {
  return authRequestJSON<{ marked_all: boolean }>(
    "/notifications/read-all",
    { method: "PATCH" },
  );
}

export async function getUnreadNotificationsCount() {
  return authRequestJSON<{ unread_count: number }>(
    "/notifications/unread-count",
    { method: "GET" },
  );
}

export interface NotificationsStreamEvent {
  type: "notifications_updated";
  unread_count: number;
  latest_id: string;
}

export async function connectNotificationsStream({
  signal,
  onEvent,
  onOpen,
}: {
  signal: AbortSignal;
  onEvent: (event: NotificationsStreamEvent) => void;
  onOpen?: () => void;
}) {
  const response = await openNotificationsStreamResponse(signal, true);
  if (!response.body) {
    throw new Error("Notifications stream is unavailable");
  }

  onOpen?.();

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }

    buffer += decoder.decode(value, { stream: true });

    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      const chunk = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      const event = parseNotificationsEvent(chunk);
      if (event) {
        onEvent(event);
      }
      boundary = buffer.indexOf("\n\n");
    }
  }
}

async function openNotificationsStreamResponse(signal: AbortSignal, allowRefreshRetry: boolean): Promise<Response> {
  const token = getStoredSession()?.tokens.access_token;
  if (!token) {
    throw new Error("Missing access token for notifications stream");
  }

  const response = await fetch(`${env.VITE_API_BASE_URL}/notifications/stream`, {
    method: "GET",
    headers: {
      Accept: "text/event-stream",
      Authorization: `Bearer ${token}`,
    },
    credentials: "include",
    signal,
  });

  if (response.status === 401 && allowRefreshRetry) {
    await refreshAuthSession();
    return openNotificationsStreamResponse(signal, false);
  }

  if (!response.ok) {
    throw new Error(`Notifications stream failed (${response.status})`);
  }

  return response;
}

function parseNotificationsEvent(chunk: string): NotificationsStreamEvent | null {
  const lines = chunk.split("\n").map((line) => line.trim()).filter(Boolean);
  if (lines.length === 0 || lines.every((line) => line.startsWith(":"))) {
    return null;
  }

  let eventName = "";
  const dataLines: string[] = [];
  for (const line of lines) {
    if (line.startsWith("event:")) {
      eventName = line.slice(6).trim();
      continue;
    }
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trim());
    }
  }

  if (eventName !== "notifications_updated" || dataLines.length === 0) {
    return null;
  }

  const payload = JSON.parse(dataLines.join("\n")) as { unread_count?: number; latest_id?: string };
  return {
    type: "notifications_updated",
    unread_count: payload.unread_count ?? 0,
    latest_id: payload.latest_id ?? "",
  };
}

import { requestEnvelope, requestJSON } from "@/lib/api-client";
import { getStoredSession } from "@/stores/auth-store";
import type { NotificationFilters, NotificationItem } from "@/types/notification";

export const notificationsKeys = {
  all: ["notifications"] as const,
  list: (filters: NotificationFilters = {}) => [...notificationsKeys.all, "list", filters] as const,
  unreadCount: () => [...notificationsKeys.all, "unread-count"] as const,
};

export async function listNotifications(filters: NotificationFilters = {}) {
  const session = getStoredSession();
  const search = new URLSearchParams();

  search.set("page", String(filters.page ?? 1));
  search.set("per_page", String(filters.perPage ?? 10));
  if (typeof filters.read === "boolean") {
    search.set("read", String(filters.read));
  }

  const envelope = await requestEnvelope<NotificationItem[]>(
    `/notifications?${search.toString()}`,
    { method: "GET" },
    session?.tokens.access_token,
  );

  return {
    items: envelope.data,
    meta: envelope.meta,
  };
}

export async function markNotificationRead(notificationId: string) {
  const session = getStoredSession();
  return requestJSON<{ marked: boolean }>(
    `/notifications/${notificationId}/read`,
    { method: "PATCH" },
    session?.tokens.access_token,
  );
}

export async function markAllNotificationsRead() {
  const session = getStoredSession();
  return requestJSON<{ marked_all: boolean }>(
    "/notifications/read-all",
    { method: "PATCH" },
    session?.tokens.access_token,
  );
}

export async function getUnreadNotificationsCount() {
  const session = getStoredSession();
  return requestJSON<{ unread_count: number }>(
    "/notifications/unread-count",
    { method: "GET" },
    session?.tokens.access_token,
  );
}

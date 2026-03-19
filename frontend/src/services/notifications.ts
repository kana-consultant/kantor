import { authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
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

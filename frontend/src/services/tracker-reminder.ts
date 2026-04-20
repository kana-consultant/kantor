import { authGetJSON, authPostJSON, authRequestJSON } from "@/lib/api-client";

export interface TrackerReminderConfig {
  tenant_id: string;
  enabled: boolean;
  start_hour: number;
  end_hour: number;
  weekdays_only: boolean;
  timezone: string;
  heartbeat_stale_minutes: number;
  notify_in_app: boolean;
  notify_whatsapp: boolean;
  next_reminder_at?: string | null;
  updated_at: string;
}

export interface UpdateTrackerReminderConfigPayload {
  enabled: boolean;
  start_hour: number;
  end_hour: number;
  weekdays_only: boolean;
  timezone: string;
  heartbeat_stale_minutes: number;
  notify_in_app: boolean;
  notify_whatsapp: boolean;
}

export interface TrackerReminderTestResult {
  delivered_in_app: boolean;
  delivered_whatsapp: boolean;
  whatsapp_error?: string | null;
}

export const trackerReminderKeys = {
  config: () => ["operational", "tracker", "reminder-config"] as const,
};

export async function getTrackerReminderConfig() {
  return authGetJSON<TrackerReminderConfig>("/tracker/reminder-config");
}

export async function updateTrackerReminderConfig(payload: UpdateTrackerReminderConfigPayload) {
  return authRequestJSON<TrackerReminderConfig>("/tracker/reminder-config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export async function sendTrackerReminderTest() {
  return authPostJSON<TrackerReminderTestResult, Record<string, never>>(
    "/tracker/reminder-config/test",
    {},
  );
}

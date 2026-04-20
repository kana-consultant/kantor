import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing, Clock3, Send } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { toast } from "@/stores/toast-store";
import {
  getTrackerReminderConfig,
  sendTrackerReminderTest,
  trackerReminderKeys,
  updateTrackerReminderConfig,
  type TrackerReminderConfig,
  type UpdateTrackerReminderConfigPayload,
} from "@/services/tracker-reminder";

const TIMEZONE_OPTIONS = [
  { value: "Asia/Jakarta", label: "Asia/Jakarta (WIB)" },
  { value: "Asia/Makassar", label: "Asia/Makassar (WITA)" },
  { value: "Asia/Jayapura", label: "Asia/Jayapura (WIT)" },
  { value: "UTC", label: "UTC" },
];

const HOUR_OPTIONS = Array.from({ length: 25 }, (_, i) => ({
  value: String(i),
  label: `${String(i).padStart(2, "0")}:00`,
}));

const STALE_OPTIONS = [
  { value: "5", label: "5 menit" },
  { value: "10", label: "10 menit" },
  { value: "15", label: "15 menit" },
  { value: "30", label: "30 menit" },
  { value: "60", label: "60 menit" },
];

function formatNextReminder(iso: string | null | undefined, timezone: string) {
  if (!iso) return "Belum ada jadwal aktif";
  const date = new Date(iso);
  try {
    return new Intl.DateTimeFormat("id-ID", {
      weekday: "long",
      day: "numeric",
      month: "long",
      hour: "2-digit",
      minute: "2-digit",
      timeZone: timezone,
      hour12: false,
    }).format(date);
  } catch {
    return date.toLocaleString("id-ID");
  }
}

export function ReminderSettingsCard() {
  const queryClient = useQueryClient();
  const configQuery = useQuery({
    queryKey: trackerReminderKeys.config(),
    queryFn: getTrackerReminderConfig,
  });

  const [form, setForm] = useState<UpdateTrackerReminderConfigPayload | null>(null);

  useEffect(() => {
    if (configQuery.data && form === null) {
      const d = configQuery.data;
      setForm({
        enabled: d.enabled,
        start_hour: d.start_hour,
        end_hour: d.end_hour,
        weekdays_only: d.weekdays_only,
        timezone: d.timezone,
        heartbeat_stale_minutes: d.heartbeat_stale_minutes,
        notify_in_app: d.notify_in_app,
        notify_whatsapp: d.notify_whatsapp,
      });
    }
  }, [configQuery.data, form]);

  const updateMutation = useMutation({
    mutationFn: updateTrackerReminderConfig,
    onSuccess: (data) => {
      queryClient.setQueryData<TrackerReminderConfig>(trackerReminderKeys.config(), data);
      toast.success("Pengaturan reminder tersimpan");
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : "Gagal menyimpan pengaturan";
      toast.error(msg);
    },
  });

  const testMutation = useMutation({
    mutationFn: sendTrackerReminderTest,
    onSuccess: (data) => {
      const parts: string[] = [];
      if (data.delivered_in_app) parts.push("in-app");
      if (data.delivered_whatsapp) parts.push("WhatsApp");
      if (parts.length === 0) {
        toast.info("Tidak ada channel aktif untuk dikirim");
      } else {
        toast.success(`Test reminder terkirim via ${parts.join(" + ")}`);
      }
      if (data.whatsapp_error) {
        toast.error(`WA: ${data.whatsapp_error}`);
      }
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : "Gagal mengirim test reminder";
      toast.error(msg);
    },
  });

  const nextText = useMemo(() => {
    if (!configQuery.data) return "";
    return formatNextReminder(configQuery.data.next_reminder_at, configQuery.data.timezone);
  }, [configQuery.data]);

  const saved = configQuery.data;
  const isCurrentlyInWindow = useMemo(() => {
    if (!saved || !saved.enabled) return false;
    try {
      const fmt = new Intl.DateTimeFormat("en-US", {
        timeZone: saved.timezone,
        hour: "2-digit",
        weekday: "short",
        hour12: false,
      });
      const parts = fmt.formatToParts(new Date());
      const hour = Number(parts.find((p) => p.type === "hour")?.value ?? "0");
      const weekday = parts.find((p) => p.type === "weekday")?.value ?? "";
      if (hour < saved.start_hour || hour >= saved.end_hour) return false;
      if (saved.weekdays_only && (weekday === "Sat" || weekday === "Sun")) return false;
      return true;
    } catch {
      return false;
    }
  }, [saved]);

  const hasUnsavedChanges = useMemo(() => {
    if (!saved || !form) return false;
    return (
      saved.enabled !== form.enabled ||
      saved.start_hour !== form.start_hour ||
      saved.end_hour !== form.end_hour ||
      saved.weekdays_only !== form.weekdays_only ||
      saved.timezone !== form.timezone ||
      saved.heartbeat_stale_minutes !== form.heartbeat_stale_minutes ||
      saved.notify_in_app !== form.notify_in_app ||
      saved.notify_whatsapp !== form.notify_whatsapp
    );
  }, [saved, form]);

  if (configQuery.isLoading || !form) {
    return (
      <Card className="p-6">
        <div className="h-6 w-48 animate-pulse rounded bg-surface-muted" />
        <div className="mt-4 h-24 animate-pulse rounded bg-surface-muted" />
      </Card>
    );
  }

  if (configQuery.isError) {
    return (
      <Card className="p-6 text-sm text-danger">Gagal memuat pengaturan reminder.</Card>
    );
  }

  const hoursInvalid = form.end_hour <= form.start_hour;

  const statusLabel = !saved?.enabled
    ? { text: "Nonaktif", tone: "muted" as const }
    : isCurrentlyInWindow
      ? { text: "Aktif sekarang", tone: "success" as const }
      : { text: "Aktif (di luar jam kerja)", tone: "warning" as const };

  const statusToneClass =
    statusLabel.tone === "success"
      ? "bg-success/10 text-success border-success/30"
      : statusLabel.tone === "warning"
        ? "bg-warning/10 text-warning border-warning/30"
        : "bg-surface-muted text-text-secondary border-border";

  const handleSave = () => {
    if (hoursInvalid) {
      toast.error("Jam selesai harus lebih besar dari jam mulai");
      return;
    }
    updateMutation.mutate(form);
  };

  return (
    <Card className="space-y-5 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2 text-base font-semibold text-text-primary">
            <BellRing className="h-4 w-4 text-ops" />
            Pengingat Aktivitas Tracker
            <span
              className={cn(
                "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-semibold",
                statusToneClass,
              )}
            >
              <span
                className={cn(
                  "h-1.5 w-1.5 rounded-full",
                  statusLabel.tone === "success"
                    ? "bg-success animate-pulse"
                    : statusLabel.tone === "warning"
                      ? "bg-warning"
                      : "bg-text-secondary",
                )}
              />
              {statusLabel.text}
            </span>
            {hasUnsavedChanges ? (
              <span className="rounded-full border border-warning/30 bg-warning/10 px-2 py-0.5 text-[11px] font-semibold text-warning">
                Belum disimpan
              </span>
            ) : null}
          </div>
          <p className="mt-1 text-sm text-text-secondary">
            Kirim reminder otomatis tiap jam saat activity tracker user belum menyala di jam kerja.
          </p>
        </div>
        <label className="inline-flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4"
            checked={form.enabled}
            onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
          />
          <span className="font-medium">{form.enabled ? "Aktifkan" : "Nonaktifkan"}</span>
        </label>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <label className="text-xs font-medium text-text-secondary">Jam Mulai</label>
          <Select
            value={String(form.start_hour)}
            options={HOUR_OPTIONS.filter((o) => Number(o.value) <= 23)}
            onValueChange={(v) => setForm({ ...form, start_hour: Number(v) })}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-text-secondary">Jam Selesai</label>
          <Select
            value={String(form.end_hour)}
            options={HOUR_OPTIONS.filter((o) => Number(o.value) >= 1)}
            onValueChange={(v) => setForm({ ...form, end_hour: Number(v) })}
          />
          {hoursInvalid ? (
            <p className="mt-1 text-xs text-danger">Jam selesai harus lebih besar dari jam mulai</p>
          ) : null}
        </div>
        <div>
          <label className="text-xs font-medium text-text-secondary">Timezone</label>
          <Select
            value={form.timezone}
            options={TIMEZONE_OPTIONS}
            onValueChange={(v) => setForm({ ...form, timezone: v })}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-text-secondary">Heartbeat dianggap mati setelah</label>
          <Select
            value={String(form.heartbeat_stale_minutes)}
            options={STALE_OPTIONS}
            onValueChange={(v) => setForm({ ...form, heartbeat_stale_minutes: Number(v) })}
          />
        </div>
      </div>

      <div className="space-y-2 border-t border-border pt-4">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4"
            checked={form.weekdays_only}
            onChange={(e) => setForm({ ...form, weekdays_only: e.target.checked })}
          />
          Kirim hanya di hari kerja (Senin–Jumat)
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4"
            checked={form.notify_in_app}
            onChange={(e) => setForm({ ...form, notify_in_app: e.target.checked })}
          />
          Kirim via in-app notification (Notification Center)
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4"
            checked={form.notify_whatsapp}
            onChange={(e) => setForm({ ...form, notify_whatsapp: e.target.checked })}
          />
          Kirim via WhatsApp (butuh WA broadcast aktif + nomor user)
        </label>
      </div>

      <div className="flex items-center gap-2 rounded-md border border-border bg-surface-muted/40 px-3 py-2 text-sm">
        <Clock3 className="h-4 w-4 text-text-secondary" />
        <span className="text-text-secondary">Reminder berikutnya:</span>
        <span className="font-medium text-text-primary">{form.enabled ? nextText : "Nonaktif"}</span>
      </div>

      <div className="flex flex-wrap items-center justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => testMutation.mutate()}
          disabled={testMutation.isPending || (!form.notify_in_app && !form.notify_whatsapp)}
        >
          <Send className="mr-2 h-4 w-4" />
          {testMutation.isPending ? "Mengirim..." : "Kirim test reminder"}
        </Button>
        <Button
          type="button"
          onClick={handleSave}
          disabled={updateMutation.isPending || hoursInvalid}
        >
          {updateMutation.isPending ? "Menyimpan..." : "Simpan Pengaturan"}
        </Button>
      </div>
    </Card>
  );
}

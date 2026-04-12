import { useEffect, useMemo, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Mail, Save, Settings2, UserRoundPlus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { useRBAC } from "@/hooks/use-rbac";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { permissions } from "@/lib/permissions";
import {
  adminRbacKeys,
  getAdminSettings,
  getRole,
  listSettingsDepartments,
  listModules,
  listPermissionGroups,
  listRoles,
  updateAutoCreateEmployee,
  updateDefaultRoles,
  updateMailDelivery,
  updateReimbursementReminder,
} from "@/services/admin-rbac";
import { toast } from "@/stores/toast-store";

export const Route = createFileRoute("/_authenticated/admin/settings")({
  beforeLoad: async () => {
    await ensureModuleAccess("admin");
    await ensurePermission(permissions.adminSettingsView);
  },
  component: AdminSettingsPage,
});

type CronPreset = {
  label: string;
  value: string;
  description: string;
};

const reviewReminderPresets: CronPreset[] = [
  {
    label: "Hari kerja 09:00",
    value: "0 9 * * 1-5",
    description: "Setiap hari kerja pukul 09:00.",
  },
  {
    label: "Hari kerja 13:00",
    value: "0 13 * * 1-5",
    description: "Setiap hari kerja pukul 13:00.",
  },
  {
    label: "Setiap hari 09:00",
    value: "0 9 * * *",
    description: "Setiap hari pukul 09:00.",
  },
  {
    label: "Setiap 30 menit",
    value: "*/30 * * * *",
    description: "Setiap 30 menit sekali.",
  },
];

const paymentReminderPresets: CronPreset[] = [
  {
    label: "Hari kerja 10:00",
    value: "0 10 * * 1-5",
    description: "Setiap hari kerja pukul 10:00.",
  },
  {
    label: "Hari kerja 15:00",
    value: "0 15 * * 1-5",
    description: "Setiap hari kerja pukul 15:00.",
  },
  {
    label: "Setiap hari 10:00",
    value: "0 10 * * *",
    description: "Setiap hari pukul 10:00.",
  },
  {
    label: "Setiap 1 jam",
    value: "0 * * * *",
    description: "Setiap awal jam.",
  },
];

const weekdayLabels: Record<string, string> = {
  "0": "Minggu",
  "1": "Senin",
  "2": "Selasa",
  "3": "Rabu",
  "4": "Kamis",
  "5": "Jumat",
  "6": "Sabtu",
  "7": "Minggu",
  "1-5": "hari kerja",
  "1,2,3,4,5": "hari kerja",
  "0,6": "akhir pekan",
  "6,0": "akhir pekan",
};

function padCronTime(value: string) {
  return value.padStart(2, "0");
}

function describeCronExpression(expression: string) {
  const parts = expression.trim().split(/\s+/);
  if (parts.length !== 5) {
    return "Gunakan format 5 bagian: menit jam tanggal bulan hari-minggu. Contoh: 0 9 * * 1-5.";
  }

  const [minute, hour, dayOfMonth, month, dayOfWeek] = parts;

  if (
    minute.startsWith("*/") &&
    hour === "*" &&
    dayOfMonth === "*" &&
    month === "*" &&
    dayOfWeek === "*"
  ) {
    return `Dikirim setiap ${minute.slice(2)} menit sekali.`;
  }

  if (
    /^\d+$/.test(minute) &&
    hour === "*" &&
    dayOfMonth === "*" &&
    month === "*" &&
    dayOfWeek === "*"
  ) {
    return `Dikirim setiap jam pada menit ke-${minute}.`;
  }

  if (
    /^\d+$/.test(minute) &&
    /^\d+$/.test(hour) &&
    dayOfMonth === "*" &&
    month === "*"
  ) {
    const timeLabel = `${padCronTime(hour)}:${padCronTime(minute)}`;
    if (dayOfWeek === "*") {
      return `Dikirim setiap hari pukul ${timeLabel}.`;
    }
    const weekday = weekdayLabels[dayOfWeek];
    if (weekday) {
      return `Dikirim setiap ${weekday} pukul ${timeLabel}.`;
    }
    return `Dikirim pada hari-minggu "${dayOfWeek}" pukul ${timeLabel}.`;
  }

  if (
    /^\d+$/.test(minute) &&
    /^\d+$/.test(hour) &&
    /^\d+$/.test(dayOfMonth) &&
    month === "*" &&
    dayOfWeek === "*"
  ) {
    return `Dikirim tiap bulan tanggal ${dayOfMonth} pukul ${padCronTime(hour)}:${padCronTime(minute)}.`;
  }

  return "Format cron valid, tetapi pola ini cukup spesifik. Gunakan preset jika ingin jadwal yang lebih mudah dipahami.";
}

type ReminderCronFieldProps = {
  currentValue: string;
  disabled: boolean;
  id: string;
  label: string;
  onChange: (value: string) => void;
  presets: CronPreset[];
};

function ReminderCronField({
  currentValue,
  disabled,
  id,
  label,
  onChange,
  presets,
}: ReminderCronFieldProps) {
  const description = useMemo(
    () => describeCronExpression(currentValue),
    [currentValue],
  );

  return (
    <div className="space-y-3 rounded-md border border-border bg-surface-muted/40 p-4">
      <div className="space-y-1.5">
        <label className="text-[13px] font-[600] text-text-primary" htmlFor={id}>
          {label}
        </label>
        <Input
          className="h-10 rounded-[6px] border-transparent bg-surface px-3 font-mono text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
          disabled={disabled}
          id={id}
          onChange={(event) => onChange(event.target.value)}
          placeholder="0 9 * * 1-5"
          value={currentValue}
        />
      </div>

      <div className="space-y-2">
        <p className="text-[11px] font-semibold uppercase tracking-[0.08em] text-text-secondary">
          Preset cepat
        </p>
        <div className="flex flex-wrap gap-2">
          {presets.map((preset) => (
            <button
              className={cn(
                "rounded-full border px-3 py-1.5 text-xs font-semibold transition-colors",
                currentValue.trim() === preset.value
                  ? "border-ops bg-ops/10 text-ops"
                  : "border-border bg-surface text-text-secondary hover:border-ops/40 hover:text-text-primary",
              )}
              disabled={disabled}
              key={preset.value}
              onClick={() => onChange(preset.value)}
              type="button"
            >
              {preset.label}
            </button>
          ))}
        </div>
      </div>

      <div className="rounded-md border border-border bg-surface px-3 py-2 text-sm">
        <p className="font-semibold text-text-primary">Arti jadwal</p>
        <p className="mt-1 text-text-secondary">{description}</p>
        <p className="mt-2 text-xs text-text-secondary">
          Format: <code>menit jam tanggal bulan hari-minggu</code>. Contoh <code>0 9 * * 1-5</code> berarti setiap hari kerja pukul 09:00.
        </p>
      </div>
    </div>
  );
}

function AdminSettingsPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canManageSettings = hasPermission(permissions.adminSettingsManage);
  const [selectedDefaultRoleID, setSelectedDefaultRoleID] = useState("");
  const [autoCreateEmployeeEnabled, setAutoCreateEmployeeEnabled] = useState(true);
  const [defaultDepartmentID, setDefaultDepartmentID] = useState<string | null>(null);
  const [mailDeliveryEnabled, setMailDeliveryEnabled] = useState(false);
  const [senderName, setSenderName] = useState("");
  const [senderEmail, setSenderEmail] = useState("");
  const [replyToEmail, setReplyToEmail] = useState("");
  const [mailAPIKey, setMailAPIKey] = useState("");
  const [clearMailAPIKey, setClearMailAPIKey] = useState(false);
  const [passwordResetEnabled, setPasswordResetEnabled] = useState(false);
  const [passwordResetExpiryMinutes, setPasswordResetExpiryMinutes] = useState(30);
  const [notificationEnabled, setNotificationEnabled] = useState(false);
  const [reimbursementReminderEnabled, setReimbursementReminderEnabled] = useState(false);
  const [reviewReminderEnabled, setReviewReminderEnabled] = useState(true);
  const [reviewReminderCron, setReviewReminderCron] = useState("0 9 * * 1-5");
  const [reviewReminderInApp, setReviewReminderInApp] = useState(true);
  const [reviewReminderEmail, setReviewReminderEmail] = useState(false);
  const [reviewReminderWhatsApp, setReviewReminderWhatsApp] = useState(false);
  const [paymentReminderEnabled, setPaymentReminderEnabled] = useState(true);
  const [paymentReminderCron, setPaymentReminderCron] = useState("0 10 * * 1-5");
  const [paymentReminderInApp, setPaymentReminderInApp] = useState(true);
  const [paymentReminderEmail, setPaymentReminderEmail] = useState(false);
  const [paymentReminderWhatsApp, setPaymentReminderWhatsApp] = useState(false);

  const settingsQuery = useQuery({
    queryKey: adminRbacKeys.settings(),
    queryFn: getAdminSettings,
  });
  const rolesQuery = useQuery({
    queryKey: adminRbacKeys.roleList({
      search: "",
      isActive: true,
      isSystem: null,
    }),
    queryFn: () =>
      listRoles({
        search: "",
        isActive: true,
        isSystem: null,
      }),
  });
  const modulesQuery = useQuery({
    queryKey: adminRbacKeys.modules(),
    queryFn: listModules,
  });
  const permissionsQuery = useQuery({
    queryKey: adminRbacKeys.permissions(),
    queryFn: listPermissionGroups,
  });
  const selectedRoleQuery = useQuery({
    queryKey: adminRbacKeys.roleDetail(selectedDefaultRoleID || "pending"),
    queryFn: () => getRole(selectedDefaultRoleID),
    enabled: Boolean(selectedDefaultRoleID),
  });
  const departmentsQuery = useQuery({
    queryKey: adminRbacKeys.settingsDepartments(),
    queryFn: listSettingsDepartments,
  });

  useEffect(() => {
    if (!settingsQuery.data) {
      return;
    }

    const uniqueRoleIDs = Array.from(
      new Set(
        Object.values(settingsQuery.data.default_roles)
          .map((role) => role?.role_id)
          .filter((roleID): roleID is string => Boolean(roleID)),
      ),
    );
    setSelectedDefaultRoleID(uniqueRoleIDs.length === 1 ? (uniqueRoleIDs[0] ?? "") : "");
    setAutoCreateEmployeeEnabled(settingsQuery.data.auto_create_employee.enabled);
    setDefaultDepartmentID(
      settingsQuery.data.auto_create_employee.default_department_id ?? null,
    );
    setMailDeliveryEnabled(settingsQuery.data.mail_delivery.enabled);
    setSenderName(settingsQuery.data.mail_delivery.sender_name);
    setSenderEmail(settingsQuery.data.mail_delivery.sender_email);
    setReplyToEmail(settingsQuery.data.mail_delivery.reply_to_email ?? "");
    setMailAPIKey("");
    setClearMailAPIKey(false);
    setPasswordResetEnabled(settingsQuery.data.mail_delivery.password_reset_enabled);
    setPasswordResetExpiryMinutes(settingsQuery.data.mail_delivery.password_reset_expiry_minutes);
    setNotificationEnabled(settingsQuery.data.mail_delivery.notification_enabled);
    setReimbursementReminderEnabled(settingsQuery.data.reimbursement_reminder.enabled);
    setReviewReminderEnabled(settingsQuery.data.reimbursement_reminder.review.enabled);
    setReviewReminderCron(settingsQuery.data.reimbursement_reminder.review.cron);
    setReviewReminderInApp(settingsQuery.data.reimbursement_reminder.review.channels.in_app);
    setReviewReminderEmail(settingsQuery.data.reimbursement_reminder.review.channels.email);
    setReviewReminderWhatsApp(settingsQuery.data.reimbursement_reminder.review.channels.whatsapp);
    setPaymentReminderEnabled(settingsQuery.data.reimbursement_reminder.payment.enabled);
    setPaymentReminderCron(settingsQuery.data.reimbursement_reminder.payment.cron);
    setPaymentReminderInApp(settingsQuery.data.reimbursement_reminder.payment.channels.in_app);
    setPaymentReminderEmail(settingsQuery.data.reimbursement_reminder.payment.channels.email);
    setPaymentReminderWhatsApp(settingsQuery.data.reimbursement_reminder.payment.channels.whatsapp);
  }, [settingsQuery.data]);

  const permissionModuleMap = useMemo(() => {
    const mapping = new Map<string, string>();
    for (const group of permissionsQuery.data ?? []) {
      for (const permission of group.permissions) {
        mapping.set(permission.id, group.id);
      }
    }
    return mapping;
  }, [permissionsQuery.data]);

  const selectedRoleModules = useMemo(() => {
    if (!selectedRoleQuery.data) {
      return [];
    }
    return Array.from(
      new Set(
        selectedRoleQuery.data.permission_ids
          .map((permissionID: string) => permissionModuleMap.get(permissionID))
          .filter((moduleID): moduleID is string => Boolean(moduleID)),
      ),
    );
  }, [permissionModuleMap, selectedRoleQuery.data]);

  const defaultRolesMutation = useMutation({
    mutationFn: (roleID: string) => {
      const moduleRoles: Record<string, string | null> = {};
      for (const module of moduleOptions) {
        moduleRoles[module.id] = roleID && selectedRoleModules.includes(module.id) ? roleID : null;
      }
      return updateDefaultRoles(moduleRoles);
    },
    onSuccess: async () => {
      toast.success("Default role berhasil diperbarui");
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.settings() });
    },
    onError: (error) => {
      toast.error(
        "Gagal memperbarui default role",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  const autoCreateMutation = useMutation({
    mutationFn: updateAutoCreateEmployee,
    onSuccess: async () => {
      toast.success("Pengaturan auto-create employee berhasil diperbarui");
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.settings() });
    },
    onError: (error) => {
      toast.error(
        "Gagal memperbarui pengaturan employee",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  const mailDeliveryMutation = useMutation({
    mutationFn: updateMailDelivery,
    onSuccess: async () => {
      toast.success("Pengaturan email tenant berhasil diperbarui");
      setMailAPIKey("");
      setClearMailAPIKey(false);
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.settings() });
    },
    onError: (error) => {
      toast.error(
        "Gagal memperbarui pengaturan email tenant",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  const reimbursementReminderMutation = useMutation({
    mutationFn: updateReimbursementReminder,
    onSuccess: async () => {
      toast.success("Pengaturan reminder reimbursement berhasil diperbarui");
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.settings() });
    },
    onError: (error) => {
      toast.error(
        "Gagal memperbarui reminder reimbursement",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  const roleOptions = useMemo(
    () => (rolesQuery.data ?? []).filter((role) => role.slug !== "super_admin"),
    [rolesQuery.data],
  );

  const moduleOptions = modulesQuery.data?.filter((module) => module.id !== "admin") ?? [];

  return (
    <div className="space-y-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.08em] text-error">
          Admin
        </p>
        <h1 className="mt-2 font-display text-[28px] font-[700] tracking-[-0.02em] text-text-primary">
          Settings
        </h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
          Atur default akses user baru dan automasi pembentukan employee record
          saat register.
        </p>
      </div>

      {!canManageSettings ? (
        <Card className="border-warning/30 bg-warning-light p-4">
          <p className="text-sm font-semibold text-text-primary">Mode baca saja</p>
          <p className="mt-1 text-sm text-text-secondary">
            Role Anda bisa melihat konfigurasi sistem, tetapi tidak memiliki izin untuk mengubahnya.
          </p>
        </Card>
      ) : null}

      <div className="grid gap-6 xl:grid-cols-2">
        <Card className="space-y-5 p-6">
          <div className="flex items-start gap-3">
            <div className="rounded-md bg-error-light p-3 text-error">
              <Settings2 className="h-5 w-5" />
            </div>
            <div>
              <h2 className="font-display text-[18px] font-[700] text-text-primary">
                Default Role User Baru
              </h2>
              <p className="mt-1 text-sm text-text-secondary">
                Saat user register, sistem akan mengisi role per modul sesuai
                mapping ini.
              </p>
            </div>
          </div>

          <div className="space-y-4">
            <div className="space-y-1.5">
              <label
                className="text-[13px] font-[600] text-text-primary"
                htmlFor="default-role"
              >
                Role
              </label>
              <select
                className="h-10 w-full rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
                disabled={!canManageSettings}
                id="default-role"
                onChange={(event) => setSelectedDefaultRoleID(event.target.value)}
                value={selectedDefaultRoleID}
              >
                <option value="">Tidak ada akses</option>
                {roleOptions.map((role) => (
                  <option key={role.id} value={role.id}>
                    {role.name} ({role.slug})
                  </option>
                ))}
              </select>
              <p className="text-xs text-text-secondary">
                Sistem akan mengaktifkan modul otomatis berdasarkan permission di role ini.
              </p>
            </div>

            <div className="rounded-md border border-border p-4">
              <p className="text-sm font-semibold text-text-primary">Modul yang akan aktif</p>
              {selectedDefaultRoleID && selectedRoleQuery.isLoading ? (
                <div className="mt-3 h-10 animate-pulse rounded-md bg-surface-muted" />
              ) : selectedRoleModules.length === 0 ? (
                <p className="mt-3 text-sm text-text-secondary">
                  Tidak ada modul aktif. Pilih role untuk memberi akses default.
                </p>
              ) : (
                <div className="mt-3 flex flex-wrap gap-2">
                  {moduleOptions
                    .filter((module) => selectedRoleModules.includes(module.id))
                    .map((module) => (
                      <span
                        className={cn(
                          "inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.08em]",
                          module.id === "operational"
                            ? "bg-ops-light text-ops"
                            : module.id === "hris"
                              ? "bg-hr-light text-hr"
                              : module.id === "marketing"
                                ? "bg-mkt-light text-mkt"
                                : "bg-error-light text-error",
                        )}
                        key={module.id}
                      >
                        {module.name}
                      </span>
                    ))}
                </div>
              )}
            </div>
          </div>

          <div className="flex justify-end">
            <Button
              disabled={!canManageSettings || defaultRolesMutation.isPending || settingsQuery.isLoading}
              onClick={() => defaultRolesMutation.mutate(selectedDefaultRoleID)}
              type="button"
            >
              <Save className="h-4 w-4" />
              Simpan Default Role
            </Button>
          </div>
        </Card>

        <Card className="space-y-5 p-6">
          <div className="flex items-start gap-3">
            <div className="rounded-md bg-error-light p-3 text-error">
              <UserRoundPlus className="h-5 w-5" />
            </div>
            <div>
              <h2 className="font-display text-[18px] font-[700] text-text-primary">
                Auto-Create Employee
              </h2>
              <p className="mt-1 text-sm text-text-secondary">
                Aktifkan agar user baru langsung mendapatkan record employee saat
                akun dibuat.
              </p>
            </div>
          </div>

          <div className="rounded-md border border-border bg-surface-muted/60 p-4">
            <label className="flex items-center justify-between gap-4">
              <div>
                <p className="text-sm font-semibold text-text-primary">
                  Buat employee otomatis
                </p>
                <p className="text-xs text-text-secondary">
                  Cocok untuk flow register internal tanpa setup manual tambahan.
                </p>
              </div>
              <input
                checked={autoCreateEmployeeEnabled}
                className="h-5 w-5 accent-[var(--module-primary)]"
                disabled={!canManageSettings}
                onChange={(event) => setAutoCreateEmployeeEnabled(event.target.checked)}
                type="checkbox"
              />
            </label>
          </div>

          <div className="space-y-1.5">
            <label
              className="text-[13px] font-[600] text-text-primary"
              htmlFor="default-department"
            >
              Default Department
            </label>
            <select
              className="h-10 w-full rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
              disabled={!canManageSettings || !autoCreateEmployeeEnabled}
              id="default-department"
              onChange={(event) => setDefaultDepartmentID(event.target.value || null)}
              value={defaultDepartmentID ?? ""}
            >
              <option value="">Tanpa default department</option>
              {(departmentsQuery.data ?? []).map((department) => (
                <option key={department.id} value={department.id}>
                  {department.name}
                </option>
              ))}
            </select>
            <p className="text-xs text-text-secondary">
              Jika kosong, employee baru tetap dibuat tetapi department tidak diisi.
            </p>
          </div>

          <div className="flex justify-end">
            <Button
              disabled={!canManageSettings || autoCreateMutation.isPending || settingsQuery.isLoading}
              onClick={() =>
                autoCreateMutation.mutate({
                  enabled: autoCreateEmployeeEnabled,
                  default_department_id: defaultDepartmentID,
                })
              }
              type="button"
            >
              <Save className="h-4 w-4" />
              Simpan Pengaturan
            </Button>
          </div>
        </Card>

        <Card className="space-y-5 p-6 xl:col-span-2">
          <div className="flex items-start gap-3">
            <div className="rounded-md bg-error-light p-3 text-error">
              <Mail className="h-5 w-5" />
            </div>
            <div>
              <h2 className="font-display text-[18px] font-[700] text-text-primary">
                Email Tenant
              </h2>
              <p className="mt-1 text-sm text-text-secondary">
                Konfigurasi provider email untuk reset kata sandi dan notifikasi tenant ini.
                Jika belum diisi lengkap, fitur email akan tetap nonaktif dan link lupa kata sandi
                tidak akan muncul di halaman login.
              </p>
            </div>
          </div>

          <div className="grid gap-6 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,0.8fr)]">
            <div className="space-y-4">
              <div className="rounded-md border border-border bg-surface-muted/60 p-4">
                <label className="flex items-center justify-between gap-4">
                  <div>
                    <p className="text-sm font-semibold text-text-primary">
                      Aktifkan email tenant
                    </p>
                    <p className="text-xs text-text-secondary">
                      Saat nonaktif, semua fitur email tenant dianggap off.
                    </p>
                  </div>
                  <input
                    checked={mailDeliveryEnabled}
                    className="h-5 w-5 accent-[var(--module-primary)]"
                    disabled={!canManageSettings}
                    onChange={(event) => setMailDeliveryEnabled(event.target.checked)}
                    type="checkbox"
                  />
                </label>
              </div>

              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-1.5">
                  <label className="text-[13px] font-[600] text-text-primary" htmlFor="mail-provider">
                    Provider
                  </label>
                  <Input
                    className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px]"
                    disabled
                    id="mail-provider"
                    value="Resend"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-[13px] font-[600] text-text-primary" htmlFor="mail-sender-name">
                    Sender Name
                  </label>
                  <Input
                    className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
                    disabled={!canManageSettings}
                    id="mail-sender-name"
                    onChange={(event) => setSenderName(event.target.value)}
                    placeholder="Kantor"
                    value={senderName}
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-[13px] font-[600] text-text-primary" htmlFor="mail-sender-email">
                    Sender Email
                  </label>
                  <Input
                    className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
                    disabled={!canManageSettings}
                    id="mail-sender-email"
                    onChange={(event) => setSenderEmail(event.target.value)}
                    placeholder="no-reply@sentinelhub.ai"
                    type="email"
                    value={senderEmail}
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-[13px] font-[600] text-text-primary" htmlFor="mail-reply-to">
                    Reply-To Email
                  </label>
                  <Input
                    className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
                    disabled={!canManageSettings}
                    id="mail-reply-to"
                    onChange={(event) => setReplyToEmail(event.target.value)}
                    placeholder="ops@sentinelhub.ai"
                    type="email"
                    value={replyToEmail}
                  />
                </div>
              </div>

              <div className="space-y-1.5">
                <label className="text-[13px] font-[600] text-text-primary" htmlFor="mail-api-key">
                  Resend API Key
                </label>
                <Input
                  className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
                  disabled={!canManageSettings || clearMailAPIKey}
                  id="mail-api-key"
                  onChange={(event) => setMailAPIKey(event.target.value)}
                  placeholder={
                    settingsQuery.data?.mail_delivery.has_api_key
                      ? "Kosongkan untuk mempertahankan API key saat ini"
                      : "re_xxxxxxxxx"
                  }
                  type="password"
                  value={mailAPIKey}
                />
                <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-text-secondary">
                  <span>
                    {settingsQuery.data?.mail_delivery.has_api_key
                      ? "API key sudah tersimpan. Isi lagi hanya jika ingin mengganti."
                      : "API key belum tersimpan untuk tenant ini."}
                  </span>
                  <label className="inline-flex items-center gap-2">
                    <input
                      checked={clearMailAPIKey}
                      className="h-4 w-4 accent-[var(--module-primary)]"
                      disabled={!canManageSettings || !settingsQuery.data?.mail_delivery.has_api_key}
                      onChange={(event) => setClearMailAPIKey(event.target.checked)}
                      type="checkbox"
                    />
                    Hapus API key tersimpan
                  </label>
                </div>
              </div>
            </div>

            <div className="space-y-4 rounded-md border border-border bg-surface-muted/40 p-4">
              <div className="rounded-md border border-border bg-surface px-4 py-3">
                <p className="text-sm font-semibold text-text-primary">Kesiapan fitur publik</p>
                <p className="mt-1 text-xs text-text-secondary">
                  Link lupa kata sandi hanya muncul kalau email tenant aktif, API key ada,
                  sender email terisi, dan reset password diaktifkan.
                </p>
              </div>

              <label className="flex items-center justify-between gap-4 rounded-md border border-border bg-surface px-4 py-3">
                <div>
                  <p className="text-sm font-semibold text-text-primary">Aktifkan reset password</p>
                  <p className="text-xs text-text-secondary">
                    Mengizinkan tenant ini mengirim email reset kata sandi.
                  </p>
                </div>
                <input
                  checked={passwordResetEnabled}
                  className="h-5 w-5 accent-[var(--module-primary)]"
                  disabled={!canManageSettings || !mailDeliveryEnabled}
                  onChange={(event) => setPasswordResetEnabled(event.target.checked)}
                  type="checkbox"
                />
              </label>

              <div className="space-y-1.5">
                <label className="text-[13px] font-[600] text-text-primary" htmlFor="reset-expiry">
                  Masa Berlaku Link Reset (menit)
                </label>
                <Input
                  className="h-10 rounded-[6px] border-transparent bg-surface px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
                  disabled={!canManageSettings || !passwordResetEnabled || !mailDeliveryEnabled}
                  id="reset-expiry"
                  min={5}
                  onChange={(event) => setPasswordResetExpiryMinutes(Number(event.target.value) || 30)}
                  type="number"
                  value={passwordResetExpiryMinutes}
                />
              </div>

              <label className="flex items-center justify-between gap-4 rounded-md border border-border bg-surface px-4 py-3">
                <div>
                  <p className="text-sm font-semibold text-text-primary">Siapkan notifikasi email</p>
                  <p className="text-xs text-text-secondary">
                    Mengaktifkan email untuk task assigned, status reimbursement, dan weekly digest.
                  </p>
                </div>
                <input
                  checked={notificationEnabled}
                  className="h-5 w-5 accent-[var(--module-primary)]"
                  disabled={!canManageSettings || !mailDeliveryEnabled}
                  onChange={(event) => setNotificationEnabled(event.target.checked)}
                  type="checkbox"
                />
              </label>

              <div className="rounded-md border border-border bg-surface px-4 py-3 text-sm">
                <p className="font-semibold text-text-primary">Status saat ini</p>
                <div className="mt-2 space-y-1 text-text-secondary">
                  <p>Email aktif: {mailDeliveryEnabled ? "Ya" : "Belum"}</p>
                  <p>API key: {settingsQuery.data?.mail_delivery.has_api_key && !clearMailAPIKey ? "Tersimpan" : mailAPIKey.trim() ? "Baru diisi" : "Belum ada"}</p>
                  <p>Reset password: {passwordResetEnabled ? "Aktif" : "Off"}</p>
                  <p>Notifikasi email: {notificationEnabled ? "Aktif" : "Off"}</p>
                </div>
              </div>
            </div>
          </div>

          <div className="flex justify-end">
            <Button
              disabled={!canManageSettings || mailDeliveryMutation.isPending || settingsQuery.isLoading}
              onClick={() =>
                mailDeliveryMutation.mutate({
                  enabled: mailDeliveryEnabled,
                  provider: "resend",
                  sender_name: senderName.trim(),
                  sender_email: senderEmail.trim(),
                  reply_to_email: replyToEmail.trim() ? replyToEmail.trim() : null,
                  api_key: clearMailAPIKey ? null : mailAPIKey.trim() || null,
                  clear_api_key: clearMailAPIKey,
                  password_reset_enabled: passwordResetEnabled,
                  password_reset_expiry_minutes: passwordResetExpiryMinutes,
                  notification_enabled: notificationEnabled,
                })
              }
              type="button"
            >
              <Save className="h-4 w-4" />
              Simpan Pengaturan Email
            </Button>
          </div>
        </Card>

        <Card className="space-y-5 p-6 xl:col-span-2">
          <div className="flex items-start gap-3">
            <div className="rounded-md bg-error-light p-3 text-error">
              <Settings2 className="h-5 w-5" />
            </div>
            <div>
              <h2 className="font-display text-[18px] font-[700] text-text-primary">
                Reminder Reimbursement
              </h2>
              <p className="mt-1 text-sm text-text-secondary">
                Reminder dikirim otomatis ke user tenant yang punya permission tindakan reimbursement
                dan juga akses <code>view_all</code>. Review memakai
                <code> hris:reimbursement:approve</code>, pembayaran memakai
                <code> hris:reimbursement:mark_paid</code>.
              </p>
            </div>
          </div>

          <div className="rounded-md border border-border bg-surface-muted/60 p-4">
            <label className="flex items-center justify-between gap-4">
              <div>
                <p className="text-sm font-semibold text-text-primary">
                  Aktifkan reminder reimbursement
                </p>
                <p className="text-xs text-text-secondary">
                  Saat nonaktif, scheduler reimbursement reminder tenant ini tidak akan mengirim apapun.
                </p>
              </div>
              <input
                checked={reimbursementReminderEnabled}
                className="h-5 w-5 accent-[var(--module-primary)]"
                disabled={!canManageSettings}
                onChange={(event) => setReimbursementReminderEnabled(event.target.checked)}
                type="checkbox"
              />
            </label>
          </div>

          <div className="grid gap-6 xl:grid-cols-2">
            <div className="space-y-4 rounded-md border border-border p-4">
              <div>
                <p className="text-sm font-semibold text-text-primary">Reminder Review</p>
                <p className="mt-1 text-xs text-text-secondary">
                  Untuk reimbursement status <code>submitted</code>. Recipient otomatis:
                  permission <code>approve</code> + <code>view_all</code>.
                </p>
              </div>

              <label className="flex items-center justify-between gap-4 rounded-md border border-border bg-surface-muted/50 px-4 py-3">
                <div>
                  <p className="text-sm font-semibold text-text-primary">Aktifkan reminder review</p>
                  <p className="text-xs text-text-secondary">Digest pending review akan dikirim sesuai cron.</p>
                </div>
                <input
                  checked={reviewReminderEnabled}
                  className="h-5 w-5 accent-[var(--module-primary)]"
                  disabled={!canManageSettings || !reimbursementReminderEnabled}
                  onChange={(event) => setReviewReminderEnabled(event.target.checked)}
                  type="checkbox"
                />
              </label>

              <ReminderCronField
                currentValue={reviewReminderCron}
                disabled={!canManageSettings || !reimbursementReminderEnabled || !reviewReminderEnabled}
                id="review-reminder-cron"
                label="Jadwal Reminder Review"
                onChange={setReviewReminderCron}
                presets={reviewReminderPresets}
              />

              <div className="grid gap-3 sm:grid-cols-3">
                <label className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-text-primary">
                  <input
                    checked={reviewReminderInApp}
                    className="h-4 w-4 accent-[var(--module-primary)]"
                    disabled={!canManageSettings || !reimbursementReminderEnabled || !reviewReminderEnabled}
                    onChange={(event) => setReviewReminderInApp(event.target.checked)}
                    type="checkbox"
                  />
                  In-app
                </label>
                <label className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-text-primary">
                  <input
                    checked={reviewReminderEmail}
                    className="h-4 w-4 accent-[var(--module-primary)]"
                    disabled={!canManageSettings || !reimbursementReminderEnabled || !reviewReminderEnabled}
                    onChange={(event) => setReviewReminderEmail(event.target.checked)}
                    type="checkbox"
                  />
                  Email
                </label>
                <label className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-text-primary">
                  <input
                    checked={reviewReminderWhatsApp}
                    className="h-4 w-4 accent-[var(--module-primary)]"
                    disabled={!canManageSettings || !reimbursementReminderEnabled || !reviewReminderEnabled}
                    onChange={(event) => setReviewReminderWhatsApp(event.target.checked)}
                    type="checkbox"
                  />
                  WhatsApp
                </label>
              </div>
            </div>

            <div className="space-y-4 rounded-md border border-border p-4">
              <div>
                <p className="text-sm font-semibold text-text-primary">Reminder Pembayaran</p>
                <p className="mt-1 text-xs text-text-secondary">
                  Untuk reimbursement status <code>approved</code>. Recipient otomatis:
                  permission <code>mark_paid</code> + <code>view_all</code>.
                </p>
              </div>

              <label className="flex items-center justify-between gap-4 rounded-md border border-border bg-surface-muted/50 px-4 py-3">
                <div>
                  <p className="text-sm font-semibold text-text-primary">Aktifkan reminder pembayaran</p>
                  <p className="text-xs text-text-secondary">Digest approved-not-paid akan dikirim sesuai cron.</p>
                </div>
                <input
                  checked={paymentReminderEnabled}
                  className="h-5 w-5 accent-[var(--module-primary)]"
                  disabled={!canManageSettings || !reimbursementReminderEnabled}
                  onChange={(event) => setPaymentReminderEnabled(event.target.checked)}
                  type="checkbox"
                />
              </label>

              <ReminderCronField
                currentValue={paymentReminderCron}
                disabled={!canManageSettings || !reimbursementReminderEnabled || !paymentReminderEnabled}
                id="payment-reminder-cron"
                label="Jadwal Reminder Pembayaran"
                onChange={setPaymentReminderCron}
                presets={paymentReminderPresets}
              />

              <div className="grid gap-3 sm:grid-cols-3">
                <label className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-text-primary">
                  <input
                    checked={paymentReminderInApp}
                    className="h-4 w-4 accent-[var(--module-primary)]"
                    disabled={!canManageSettings || !reimbursementReminderEnabled || !paymentReminderEnabled}
                    onChange={(event) => setPaymentReminderInApp(event.target.checked)}
                    type="checkbox"
                  />
                  In-app
                </label>
                <label className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-text-primary">
                  <input
                    checked={paymentReminderEmail}
                    className="h-4 w-4 accent-[var(--module-primary)]"
                    disabled={!canManageSettings || !reimbursementReminderEnabled || !paymentReminderEnabled}
                    onChange={(event) => setPaymentReminderEmail(event.target.checked)}
                    type="checkbox"
                  />
                  Email
                </label>
                <label className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-text-primary">
                  <input
                    checked={paymentReminderWhatsApp}
                    className="h-4 w-4 accent-[var(--module-primary)]"
                    disabled={!canManageSettings || !reimbursementReminderEnabled || !paymentReminderEnabled}
                    onChange={(event) => setPaymentReminderWhatsApp(event.target.checked)}
                    type="checkbox"
                  />
                  WhatsApp
                </label>
              </div>
            </div>
          </div>

          <div className="rounded-md border border-border bg-surface-muted/40 px-4 py-3 text-sm text-text-secondary">
            <p className="font-semibold text-text-primary">Catatan channel</p>
            <ul className="mt-2 space-y-1">
              <li>In-app akan tetap jalan tanpa konfigurasi email/WA.</li>
              <li>Email butuh Email Tenant aktif dan notifikasi email aktif.</li>
              <li>WhatsApp butuh WA Broadcast tenant aktif dan template default reminder tersedia.</li>
            </ul>
          </div>

          <div className="flex justify-end">
            <Button
              disabled={!canManageSettings || reimbursementReminderMutation.isPending || settingsQuery.isLoading}
              onClick={() =>
                reimbursementReminderMutation.mutate({
                  enabled: reimbursementReminderEnabled,
                  review: {
                    enabled: reviewReminderEnabled,
                    cron: reviewReminderCron.trim(),
                    channels: {
                      in_app: reviewReminderInApp,
                      email: reviewReminderEmail,
                      whatsapp: reviewReminderWhatsApp,
                    },
                  },
                  payment: {
                    enabled: paymentReminderEnabled,
                    cron: paymentReminderCron.trim(),
                    channels: {
                      in_app: paymentReminderInApp,
                      email: paymentReminderEmail,
                      whatsapp: paymentReminderWhatsApp,
                    },
                  },
                })
              }
              type="button"
            >
              <Save className="h-4 w-4" />
              Simpan Reminder Reimbursement
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

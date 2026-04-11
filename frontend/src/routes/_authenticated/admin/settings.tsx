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
} from "@/services/admin-rbac";
import { toast } from "@/stores/toast-store";

export const Route = createFileRoute("/_authenticated/admin/settings")({
  beforeLoad: async () => {
    await ensureModuleAccess("admin");
    await ensurePermission(permissions.adminSettingsView);
  },
  component: AdminSettingsPage,
});

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
                    Menandai tenant ini siap memakai fitur notifikasi email berikutnya.
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
      </div>
    </div>
  );
}

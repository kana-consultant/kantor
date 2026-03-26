import { useEffect, useMemo, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save, Settings2, UserRoundPlus } from "lucide-react";

import { ExtensionConnector } from "@/components/settings/extension-connector";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
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
    onSuccess: () => {
      toast.success("Default role berhasil diperbarui");
      void queryClient.invalidateQueries({ queryKey: adminRbacKeys.settings() });
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
    onSuccess: () => {
      toast.success("Pengaturan auto-create employee berhasil diperbarui");
      void queryClient.invalidateQueries({ queryKey: adminRbacKeys.settings() });
    },
    onError: (error) => {
      toast.error(
        "Gagal memperbarui pengaturan employee",
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
      </div>

      <ExtensionConnector />
    </div>
  );
}

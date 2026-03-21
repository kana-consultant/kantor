import { useEffect, useMemo, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Pencil,
  Search,
  Shield,
  ShieldCheck,
  ShieldOff,
  UserCog,
} from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { FormModal } from "@/components/shared/form-modal";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { permissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import {
  adminRbacKeys,
  getAdminUser,
  getRole,
  listPermissionGroups,
  listAdminUsers,
  listModules,
  listRoles,
  toggleAdminUserActive,
  toggleAdminUserSuperAdmin,
  updateAdminUserModuleRoles,
} from "@/services/admin-rbac";
import { toast } from "@/stores/toast-store";
import type {
  AdminRoleSummary,
  AdminUserDetail,
  AdminUserFilters,
  AdminUserSummary,
  SetUserModuleRolePayload,
} from "@/types/admin";
import { useAuth } from "@/hooks/use-auth";
import { fetchSessionProfile } from "@/services/foundation";

export const Route = createFileRoute("/_authenticated/admin/users")({
  beforeLoad: async () => {
    await ensureModuleAccess("admin");
    await ensurePermission(permissions.adminUsersView);
  },
  component: AdminUsersPage,
});

function AdminUsersPage() {
  const queryClient = useQueryClient();
  const { user, isSuperAdmin, setSession, session } = useAuth();
  const { hasPermission } = useRBAC();
  const canManageUsers = hasPermission(permissions.adminUsersManage);

  const [filters, setFilters] = useState<AdminUserFilters>({
    page: 1,
    perPage: 20,
    search: "",
    moduleId: "",
    roleId: "",
    superAdmin: null,
  });
  const [searchInput, setSearchInput] = useState("");
  const [editingUserID, setEditingUserID] = useState<string | null>(null);
  const [selectedRoleID, setSelectedRoleID] = useState("");
  const [selectedIsSuperAdmin, setSelectedIsSuperAdmin] = useState(false);

  const usersQuery = useQuery({
    queryKey: adminRbacKeys.userList(filters),
    queryFn: () => listAdminUsers(filters),
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
  const userDetailQuery = useQuery({
    queryKey: adminRbacKeys.userDetail(editingUserID ?? "pending"),
    queryFn: () => getAdminUser(editingUserID!),
    enabled: Boolean(editingUserID),
  });
  const selectedRoleQuery = useQuery({
    queryKey: adminRbacKeys.roleDetail(selectedRoleID || "pending"),
    queryFn: () => getRole(selectedRoleID),
    enabled: Boolean(editingUserID && selectedRoleID),
  });

  useEffect(() => {
    if (!userDetailQuery.data) {
      return;
    }

    const uniqueRoleIDs = Array.from(
      new Set(
        Object.values(userDetailQuery.data.module_roles)
          .map((role) => role.role_id)
          .filter((roleID): roleID is string => Boolean(roleID)),
      ),
    );

    setSelectedRoleID(uniqueRoleIDs.length === 1 ? uniqueRoleIDs[0] : "");
    setSelectedIsSuperAdmin(userDetailQuery.data.is_super_admin);
  }, [userDetailQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async (payload: {
      userID: string;
      moduleRoles: SetUserModuleRolePayload[];
      superAdminEnabled: boolean;
      currentDetail: AdminUserDetail;
    }) => {
      await updateAdminUserModuleRoles(
        payload.userID,
        payload.moduleRoles,
      );

      if (
        isSuperAdmin &&
        payload.currentDetail.is_super_admin !== payload.superAdminEnabled
      ) {
        await toggleAdminUserSuperAdmin(payload.userID, payload.superAdminEnabled);
      }
    },
    onSuccess: async (_, variables) => {
      if (variables.userID === user?.id && session) {
        try {
          const profile = await fetchSessionProfile();
          setSession({
            ...session,
            user: profile.user,
            module_roles: profile.module_roles,
            permissions: profile.permissions,
            is_super_admin: profile.is_super_admin,
          });
        } catch {
          // User update already succeeded; leave a stale session refresh to the next layout sync.
        }
      }
      toast.success("Role pengguna berhasil diperbarui");
      void queryClient.invalidateQueries({ queryKey: adminRbacKeys.users() });
      setEditingUserID(null);
    },
    onError: (error) => {
      toast.error(
        "Gagal memperbarui akses pengguna",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  const toggleActiveMutation = useMutation({
    mutationFn: ({ userID, active }: { userID: string; active: boolean }) =>
      toggleAdminUserActive(userID, active),
    onSuccess: () => {
      toast.success("Status pengguna berhasil diperbarui");
      void queryClient.invalidateQueries({ queryKey: adminRbacKeys.users() });
    },
    onError: (error) => {
      toast.error(
        "Gagal memperbarui status pengguna",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  const columns: Array<DataTableColumn<AdminUserSummary>> = [
    {
      id: "user",
      header: "User",
      cell: (item) => (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-medium text-text-primary">{item.user.full_name}</span>
            {item.is_super_admin ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-error-light px-2 py-0.5 text-[11px] font-semibold text-error">
                <ShieldCheck className="h-3 w-3" />
                Super Admin
              </span>
            ) : null}
          </div>
          <p className="text-xs text-text-secondary">{item.user.email}</p>
        </div>
      ),
    },
    {
      id: "module_roles",
      header: "Module Roles",
      cell: (item) => (
        <div className="flex flex-wrap gap-2">
          {renderModuleBadges(item.module_roles)}
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (item) => (
        <StatusBadge status={item.user.is_active ? "active" : "inactive"} />
      ),
    },
  ];

  if (canManageUsers) {
    columns.push({
      id: "actions",
      header: "Aksi",
      align: "right",
      cell: (item) => (
        <div className="flex items-center justify-end gap-1">
          <Button
            onClick={() => setEditingUserID(item.user.id)}
            size="xs"
            type="button"
            variant="ghost"
          >
            <Pencil className="h-4 w-4" />
          </Button>
          <Button
            onClick={() =>
              toggleActiveMutation.mutate({
                userID: item.user.id,
                active: !item.user.is_active,
              })
            }
            size="xs"
            type="button"
            variant="ghost"
          >
            {item.user.is_active ? (
              <ShieldOff className="h-4 w-4 text-error" />
            ) : (
              <Shield className="h-4 w-4 text-success" />
            )}
          </Button>
        </div>
      ),
    });
  }

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
          .map((permissionID) => permissionModuleMap.get(permissionID))
          .filter((moduleID): moduleID is string => Boolean(moduleID)),
      ),
    );
  }, [permissionModuleMap, selectedRoleQuery.data]);

  const currentAssignedRoleIDs = useMemo(
    () =>
      Array.from(
        new Set(
          Object.values(userDetailQuery.data?.module_roles ?? {})
            .map((role) => role.role_id)
            .filter((roleID): roleID is string => Boolean(roleID)),
        ),
      ),
    [userDetailQuery.data],
  );

  const hasMixedLegacyAssignments = currentAssignedRoleIDs.length > 1;

  const effectivePermissionPreview = useMemo(() => {
    if (selectedIsSuperAdmin) {
      return ["super_admin:bypass"];
    }

    return [...(selectedRoleQuery.data?.permission_ids ?? [])].sort();
  }, [selectedIsSuperAdmin, selectedRoleQuery.data]);

  const moduleOptions = modulesQuery.data ?? [];
  const roleOptions = (rolesQuery.data ?? []).filter((role) => role.slug !== "super_admin");
  const users = usersQuery.data?.items ?? [];

  return (
    <div className="space-y-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.08em] text-error">
          Admin
        </p>
        <h1 className="mt-2 font-display text-[28px] font-[700] tracking-[-0.02em] text-text-primary">
          Users
        </h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
          Assign role berbeda per modul, cabut akses per modul tanpa menghapus
          akun, dan kelola status super admin secara terpisah.
        </p>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_200px_220px]">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary" />
          <Input
            className="pl-10"
            onChange={(event) => setSearchInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                setFilters((current) => ({
                  ...current,
                  page: 1,
                  search: searchInput.trim(),
                }));
              }
            }}
            placeholder="Cari nama atau email"
            value={searchInput}
          />
        </div>
        <select
          className="h-10 rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
          onChange={(event) =>
            setFilters((current) => ({
              ...current,
              page: 1,
              moduleId: event.target.value,
            }))
          }
          value={filters.moduleId ?? ""}
        >
          <option value="">Semua Modul</option>
          {moduleOptions.map((module) => (
            <option key={module.id} value={module.id}>
              {module.name}
            </option>
          ))}
        </select>
        <select
          className="h-10 rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
          onChange={(event) =>
            setFilters((current) => ({
              ...current,
              page: 1,
              superAdmin:
                event.target.value === ""
                  ? null
                  : event.target.value === "true",
            }))
          }
          value={filters.superAdmin == null ? "" : String(filters.superAdmin)}
        >
          <option value="">Semua Tipe Akun</option>
          <option value="true">Super Admin</option>
          <option value="false">Non Super Admin</option>
        </select>
      </div>

      <DataTable
        columns={columns}
        data={users}
        emptyDescription="Belum ada user yang cocok dengan filter aktif."
        emptyTitle="User tidak ditemukan"
        getRowId={(item) => item.user.id}
        loading={usersQuery.isLoading}
        pagination={
          usersQuery.data?.meta
            ? {
                page: usersQuery.data.meta.page,
                perPage: usersQuery.data.meta.per_page,
                total: usersQuery.data.meta.total,
                onPageChange: (page) =>
                  setFilters((current) => ({
                    ...current,
                    page,
                  })),
              }
            : undefined
        }
      />

      <FormModal
        isLoading={saveMutation.isPending}
        isOpen={canManageUsers && Boolean(editingUserID)}
        onClose={() => setEditingUserID(null)}
        onSubmit={(event) => {
          event.preventDefault();
          if (!editingUserID || !userDetailQuery.data) {
            return;
          }
          saveMutation.mutate({
            userID: editingUserID,
            moduleRoles: moduleOptions.map((module) => ({
              module_id: module.id,
              role_id:
                selectedRoleID && selectedRoleModules.includes(module.id)
                  ? selectedRoleID
                  : null,
            })),
            superAdminEnabled: selectedIsSuperAdmin,
            currentDetail: userDetailQuery.data,
          });
        }}
        size="lg"
        submitLabel="Simpan Perubahan"
        subtitle="Pilih satu role. Sistem akan mengaktifkan modul mengikuti permission yang ada di role tersebut."
        title="Edit Akses Pengguna"
      >
        {userDetailQuery.isLoading || !userDetailQuery.data ? (
          <div className="space-y-3">
            <div className="h-20 animate-pulse rounded-md bg-surface-muted" />
            <div className="h-32 animate-pulse rounded-md bg-surface-muted" />
          </div>
        ) : (
          <div className="space-y-5">
            <div className="rounded-md border border-border bg-surface-muted/60 p-4">
              <div className="flex items-center gap-3">
                <div className="flex h-11 w-11 items-center justify-center rounded-full bg-error-light text-sm font-semibold text-error">
                  {userDetailQuery.data.user.full_name.slice(0, 1).toUpperCase()}
                </div>
                <div>
                  <p className="font-medium text-text-primary">
                    {userDetailQuery.data.user.full_name}
                  </p>
                  <p className="text-sm text-text-secondary">
                    {userDetailQuery.data.user.email}
                  </p>
                </div>
              </div>
            </div>

            <div className="space-y-4">
              <div className="space-y-1.5">
                <label
                  className="text-[13px] font-[600] text-text-primary"
                  htmlFor="user-role"
                >
                  Role
                </label>
                <select
                  className="h-10 w-full rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
                  id="user-role"
                  onChange={(event) => setSelectedRoleID(event.target.value)}
                  value={selectedRoleID}
                >
                  <option value="">Tidak ada akses</option>
                  {roleOptions.map((role) => (
                    <option key={role.id} value={role.id}>
                      {role.name} ({role.slug})
                    </option>
                  ))}
                </select>
                <p className="text-xs text-text-secondary">
                  Assignment lama lintas modul akan diganti dengan satu role ini.
                </p>
              </div>

              {hasMixedLegacyAssignments ? (
                <div className="rounded-md border border-warning/30 bg-warning-light px-4 py-3 text-sm text-warning-dark">
                  Pengguna ini masih punya assignment campuran dari beberapa role. Saat
                  disimpan, semua assignment tersebut akan diganti mengikuti role yang
                  dipilih di atas.
                </div>
              ) : null}

              <div className="rounded-md border border-border p-4">
                <p className="text-sm font-semibold text-text-primary">Modul yang akan aktif</p>
                <p className="mt-1 text-xs text-text-secondary">
                  Modul diaktifkan otomatis berdasarkan permission yang ada di role.
                </p>
                {selectedRoleID && selectedRoleQuery.isLoading ? (
                  <div className="mt-3 h-10 animate-pulse rounded-md bg-surface-muted" />
                ) : selectedRoleModules.length === 0 ? (
                  <p className="mt-3 text-sm text-text-secondary">
                    Tidak ada modul aktif. Pilih role untuk memberi akses, atau kosongkan untuk mencabut semua akses.
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

            {isSuperAdmin && user?.id !== userDetailQuery.data.user.id ? (
              <div className="rounded-md border border-border bg-surface-muted/60 p-4">
                <label className="flex items-center justify-between gap-4">
                  <div>
                    <p className="text-sm font-semibold text-text-primary">
                      Super Admin
                    </p>
                    <p className="text-xs text-text-secondary">
                      Bypass semua permission dan module access.
                    </p>
                  </div>
                  <input
                    checked={selectedIsSuperAdmin}
                    className="h-5 w-5 accent-[var(--module-primary)]"
                    onChange={(event) => setSelectedIsSuperAdmin(event.target.checked)}
                    type="checkbox"
                  />
                </label>
              </div>
            ) : null}

            <div className="space-y-2 rounded-md border border-border p-4">
              <div className="flex items-center gap-2">
                <UserCog className="h-4 w-4 text-error" />
                <p className="text-sm font-semibold text-text-primary">
                  Effective Permissions
                </p>
              </div>
              {selectedIsSuperAdmin ? (
                <p className="text-sm text-text-secondary">
                  User ini akan bypass seluruh permission check sebagai super admin.
                </p>
              ) : effectivePermissionPreview.length === 0 ? (
                <p className="text-sm text-text-secondary">
                  Belum ada permission efektif karena semua modul disetel tanpa akses.
                </p>
              ) : (
                <div className="max-h-52 overflow-y-auto rounded-md bg-surface-muted/60 p-3">
                  <div className="flex flex-wrap gap-2">
                    {effectivePermissionPreview.map((permissionID) => (
                      <code
                        className="rounded-full bg-surface px-2 py-1 font-mono text-[11px] text-text-secondary"
                        key={permissionID}
                      >
                        {permissionID}
                      </code>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}
      </FormModal>
    </div>
  );
}

function renderModuleBadges(moduleRoles: AdminUserSummary["module_roles"]) {
  const orderedModules = ["operational", "hris", "marketing", "admin"] as const;

  return orderedModules
    .filter((moduleID) => moduleRoles[moduleID])
    .map((moduleID) => {
      const role = moduleRoles[moduleID];
      const label = moduleID === "operational"
        ? "OPS"
        : moduleID === "hris"
          ? "HRIS"
          : moduleID === "marketing"
            ? "MKT"
            : "ADMIN";

      return (
        <span
          className={cn(
            "inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.08em]",
            moduleID === "operational"
              ? "bg-ops-light text-ops"
              : moduleID === "hris"
                ? "bg-hr-light text-hr"
                : moduleID === "marketing"
                  ? "bg-mkt-light text-mkt"
                  : "bg-error-light text-error",
          )}
          key={moduleID}
        >
          {label}: {role.role_slug}
        </span>
      );
    });
}

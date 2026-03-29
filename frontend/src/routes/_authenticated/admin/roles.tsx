import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";
import {
  Copy,
  Lock,
  Pencil,
  Plus,
  Power,
  Search,
  Shield,
  Trash2,
} from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { EmptyState } from "@/components/shared/empty-state";
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
  createRole,
  deleteRole,
  duplicateRole,
  getRole,
  listPermissionGroups,
  listRoles,
  toggleRole,
  updateRole,
} from "@/services/admin-rbac";
import { toast } from "@/stores/toast-store";
import type {
  AdminRoleSummary,
  ListRolesFilters,
  PermissionItem,
  PermissionModuleGroup,
  UpsertRolePayload,
} from "@/types/admin";

const roleSchema = z.object({
  name: z.string().min(3, "Nama role minimal 3 karakter").max(100),
  slug: z
    .string()
    .min(3, "Slug minimal 3 karakter")
    .max(50)
    .regex(/^[a-z0-9_-]+$/, "Slug hanya boleh huruf kecil, angka, dash, dan underscore"),
  description: z.string().max(500),
});

type RoleFormValues = z.infer<typeof roleSchema>;

type RoleModalState =
  | { mode: "create" }
  | { mode: "edit"; roleId: string }
  | null;

type PermissionMatrix = {
  actions: string[];
  rows: Array<{
    resource: string;
    permissionsByAction: Record<string, PermissionItem | undefined>;
  }>;
};

export const Route = createFileRoute("/_authenticated/admin/roles")({
  beforeLoad: async () => {
    await ensureModuleAccess("admin");
    await ensurePermission(permissions.adminRolesView);
  },
  component: AdminRolesPage,
});

function AdminRolesPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canManageRoles = hasPermission(permissions.adminRolesManage);
  const [filters, setFilters] = useState<ListRolesFilters>({
    search: "",
    isActive: null,
    isSystem: null,
  });
  const [searchInput, setSearchInput] = useState("");
  const [modalState, setModalState] = useState<RoleModalState>(null);
  const [selectedPermissionIDs, setSelectedPermissionIDs] = useState<string[]>([]);
  const [deleteTarget, setDeleteTarget] = useState<AdminRoleSummary | null>(null);

  const form = useForm<RoleFormValues>({
    resolver: zodResolver(roleSchema),
    defaultValues: {
      name: "",
      slug: "",
      description: "",
    },
  });

  const rolesQuery = useQuery({
    queryKey: adminRbacKeys.roleList(filters),
    queryFn: () => listRoles(filters),
  });
  const permissionsQuery = useQuery({
    queryKey: adminRbacKeys.permissions(),
    queryFn: listPermissionGroups,
  });
  const selectedRoleQuery = useQuery({
    queryKey: adminRbacKeys.roleDetail(
      modalState?.mode === "edit" ? modalState.roleId : "new",
    ),
    queryFn: () => getRole(modalState?.mode === "edit" ? modalState.roleId : ""),
    enabled: modalState?.mode === "edit",
  });

  const createMutation = useMutation({
    mutationFn: createRole,
    onSuccess: async () => {
      toast.success("Role berhasil dibuat");
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.roles() });
      setModalState(null);
    },
    onError: (error) => {
      toast.error("Gagal membuat role", error instanceof Error ? error.message : undefined);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ roleId, payload }: { roleId: string; payload: UpsertRolePayload }) =>
      updateRole(roleId, payload),
    onSuccess: async (_, variables) => {
      toast.success("Role berhasil diperbarui");
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.roles() });
      await queryClient.invalidateQueries({
        queryKey: adminRbacKeys.roleDetail(variables.roleId),
      });
      setModalState(null);
    },
    onError: (error) => {
      toast.error("Gagal memperbarui role", error instanceof Error ? error.message : undefined);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteRole,
    onSuccess: async () => {
      toast.success("Role berhasil dihapus");
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.roles() });
      setDeleteTarget(null);
    },
    onError: (error) => {
      toast.error("Gagal menghapus role", error instanceof Error ? error.message : undefined);
    },
  });

  const toggleMutation = useMutation({
    mutationFn: toggleRole,
    onSuccess: async () => {
      toast.success("Status role diperbarui");
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.roles() });
    },
    onError: (error) => {
      toast.error("Gagal mengubah status role", error instanceof Error ? error.message : undefined);
    },
  });

  const duplicateMutation = useMutation({
    mutationFn: duplicateRole,
    onSuccess: async () => {
      toast.success("Role berhasil diduplikasi");
      await queryClient.invalidateQueries({ queryKey: adminRbacKeys.roles() });
    },
    onError: (error) => {
      toast.error("Gagal menduplikasi role", error instanceof Error ? error.message : undefined);
    },
  });

  const isSubmitting = createMutation.isPending || updateMutation.isPending;
  const selectedRole = selectedRoleQuery.data ?? null;

  const permissionMatrices = useMemo(
    () =>
      (permissionsQuery.data ?? []).map((group) => ({
        group,
        matrix: buildPermissionMatrix(group),
      })),
    [permissionsQuery.data],
  );

  useEffect(() => {
    if (!modalState) {
      form.reset({
        name: "",
        slug: "",
        description: "",
      });
      setSelectedPermissionIDs([]);
      return;
    }

    if (modalState.mode === "create") {
      form.reset({
        name: "",
        slug: "",
        description: "",
      });
      setSelectedPermissionIDs([]);
      return;
    }

    if (!selectedRole) {
      return;
    }

    form.reset({
      name: selectedRole.name,
      slug: selectedRole.slug,
      description: selectedRole.description,
    });
    setSelectedPermissionIDs(selectedRole.permission_ids);
  }, [form, modalState, selectedRole]);

  const watchedName = form.watch("name");
  const watchedSlug = form.watch("slug");

  useEffect(() => {
    if (modalState?.mode !== "create") {
      return;
    }
    if (form.formState.dirtyFields.slug) {
      return;
    }
    const nextSlug = slugify(watchedName);
    if (nextSlug !== watchedSlug) {
      form.setValue("slug", nextSlug, { shouldDirty: false });
    }
  }, [form, modalState, watchedName, watchedSlug]);

  const columns: Array<DataTableColumn<AdminRoleSummary>> = [
    {
      id: "name",
      header: "Role",
      sortable: true,
      cell: (role) => (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-medium text-text-primary">{role.name}</span>
            {role.is_system ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-surface-muted px-2 py-0.5 text-[11px] font-semibold text-text-secondary">
                <Lock className="h-3 w-3" />
                System
              </span>
            ) : null}
          </div>
          <code className="font-mono text-xs text-text-secondary">{role.slug}</code>
          {role.description ? (
            <p className="text-xs leading-5 text-text-secondary">{role.description}</p>
          ) : null}
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      sortable: true,
      accessor: "is_active",
      cell: (role) => <StatusBadge status={role.is_active ? "active" : "inactive"} />,
    },
    {
      id: "permissions_count",
      header: "Permissions",
      accessor: "permissions_count",
      sortable: true,
      numeric: true,
      align: "right",
    },
    {
      id: "users_count",
      header: "Users",
      accessor: "users_count",
      sortable: true,
      numeric: true,
      align: "right",
    },
  ];

  if (canManageRoles) {
    columns.push({
      id: "actions",
      header: "Aksi",
      align: "right",
      cell: (role) => (
        <div className="flex items-center justify-end gap-1">
          <Button
            size="xs"
            type="button"
            variant="ghost"
            onClick={() => setModalState({ mode: "edit", roleId: role.id })}
          >
            <Pencil className="h-4 w-4" />
          </Button>
          <Button
            size="xs"
            type="button"
            variant="ghost"
            onClick={() => duplicateMutation.mutate(role.id)}
          >
            <Copy className="h-4 w-4" />
          </Button>
          <Button
            size="xs"
            type="button"
            variant="ghost"
            onClick={() => toggleMutation.mutate(role.id)}
          >
            <Power className="h-4 w-4" />
          </Button>
          <Button
            size="xs"
            type="button"
            variant="ghost"
            disabled={role.is_system || role.users_count > 0}
            onClick={() => setDeleteTarget(role)}
          >
            <Trash2 className="h-4 w-4 text-error" />
          </Button>
        </div>
      ),
    });
  }

  const submitRoleForm = form.handleSubmit((values) => {
    const payload: UpsertRolePayload = {
      ...values,
      description: values.description,
      hierarchy_level: selectedRole?.hierarchy_level ?? 50,
      permission_ids: selectedPermissionIDs,
    };

    if (modalState?.mode === "edit") {
      updateMutation.mutate({ roleId: modalState.roleId, payload });
      return;
    }

    createMutation.mutate(payload);
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-error">
            Admin
          </p>
          <h1 className="mt-2 font-display text-[28px] font-[700] tracking-[-0.02em] text-text-primary">
            Roles
          </h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
            Buat custom role, edit permission system role, dan kelola akses lintas
            modul tanpa kembali ke role flat lama.
          </p>
        </div>
        {canManageRoles ? (
          <Button onClick={() => setModalState({ mode: "create" })} type="button">
            <Plus className="h-4 w-4" />
            Buat Role
          </Button>
        ) : null}
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_180px_180px]">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary" />
          <Input
            className="pl-10"
            onChange={(event) => setSearchInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                setFilters((current) => ({
                  ...current,
                  search: searchInput.trim(),
                }));
              }
            }}
            placeholder="Cari nama role atau slug"
            value={searchInput}
          />
        </div>
        <select
          className="h-10 rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
          onChange={(event) =>
            setFilters((current) => ({
              ...current,
              isSystem:
                event.target.value === ""
                  ? null
                  : event.target.value === "true",
            }))
          }
          value={filters.isSystem == null ? "" : String(filters.isSystem)}
        >
          <option value="">Semua Tipe</option>
          <option value="true">System</option>
          <option value="false">Custom</option>
        </select>
        <select
          className="h-10 rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
          onChange={(event) =>
            setFilters((current) => ({
              ...current,
              isActive:
                event.target.value === ""
                  ? null
                  : event.target.value === "true",
            }))
          }
          value={filters.isActive == null ? "" : String(filters.isActive)}
        >
          <option value="">Semua Status</option>
          <option value="true">Aktif</option>
          <option value="false">Nonaktif</option>
        </select>
      </div>

      <DataTable
        columns={columns}
        data={rolesQuery.data ?? []}
        emptyActionLabel={canManageRoles ? "Buat Role" : undefined}
        emptyDescription="Belum ada role yang cocok dengan filter aktif."
        emptyTitle="Role tidak ditemukan"
        getRowId={(role) => role.id}
        loading={rolesQuery.isLoading}
        onEmptyAction={canManageRoles ? () => setModalState({ mode: "create" }) : undefined}
      />

      <FormModal
        error={selectedPermissionIDs.length === 0 ? "Pilih minimal satu permission." : null}
        isLoading={isSubmitting}
        isOpen={canManageRoles && modalState !== null}
        onClose={() => setModalState(null)}
        onSubmit={submitRoleForm}
        size="xl"
        submitDisabled={
          selectedPermissionIDs.length === 0 ||
          (modalState?.mode === "edit" && selectedRoleQuery.isLoading)
        }
        submitLabel={modalState?.mode === "edit" ? "Simpan Perubahan" : "Buat Role"}
        subtitle="System role tidak bisa dihapus, tetapi permission-nya tetap bisa diperbarui."
        title={modalState?.mode === "edit" ? "Edit Role" : "Buat Role Baru"}
      >
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-1.5">
            <label className="text-[13px] font-[600] text-text-primary" htmlFor="role-name">
              Nama Role<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input
              id="role-name"
              disabled={selectedRole?.is_system}
              placeholder="Finance Officer"
              {...form.register("name")}
            />
            {form.formState.errors.name ? (
              <p className="text-xs text-error">{form.formState.errors.name.message}</p>
            ) : null}
          </div>

          <div className="space-y-1.5">
            <label className="text-[13px] font-[600] text-text-primary" htmlFor="role-slug">
              Slug<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input
              id="role-slug"
              disabled={selectedRole?.is_system}
              placeholder="finance_officer"
              {...form.register("slug")}
            />
            {form.formState.errors.slug ? (
              <p className="text-xs text-error">{form.formState.errors.slug.message}</p>
            ) : null}
          </div>
        </div>

        <div className="space-y-1.5">
          <label
            className="text-[13px] font-[600] text-text-primary"
            htmlFor="role-description"
          >
            Deskripsi
          </label>
          <textarea
            className="min-h-28 w-full rounded-[6px] border-[1.5px] border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary outline-none transition-all duration-150 placeholder:text-text-tertiary focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
            id="role-description"
            placeholder="Role khusus untuk approval reimbursement dan finance."
            {...form.register("description")}
          />
        </div>

        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-[13px] font-[600] text-text-primary">Permission Picker</p>
              <p className="text-xs text-text-secondary">
                {selectedPermissionIDs.length} permission dipilih
              </p>
            </div>
          </div>

          {permissionsQuery.isLoading ? (
            <div className="grid gap-3">
              {Array.from({ length: 3 }).map((_, index) => (
                <div
                  className="h-40 animate-pulse rounded-md border border-border bg-surface-muted"
                  key={index}
                />
              ))}
            </div>
          ) : permissionMatrices.length === 0 ? (
            <EmptyState
              description="Belum ada permission yang bisa dipilih."
              icon={Shield}
              title="Permission kosong"
            />
          ) : (
            <div className="space-y-4">
              {permissionMatrices.map(({ group, matrix }) => {
                const groupPermissionIDs = group.permissions.map((permission) => permission.id);
                const allModuleSelected = groupPermissionIDs.every((permissionID) =>
                  selectedPermissionIDs.includes(permissionID),
                );

                return (
                  <div className="rounded-md border border-border" key={group.id}>
                    <div className="flex flex-col gap-3 border-b border-border px-4 py-4 md:flex-row md:items-center md:justify-between">
                      <div>
                        <p className="text-sm font-semibold text-text-primary">{group.name}</p>
                        <p className="text-xs text-text-secondary">{group.description}</p>
                      </div>
                      <Button
                        onClick={() =>
                          setSelectedPermissionIDs((current) =>
                            toggleMany(current, groupPermissionIDs, !allModuleSelected),
                          )
                        }
                        size="xs"
                        type="button"
                        variant="secondary"
                      >
                        {allModuleSelected ? "Batalkan Modul" : "Pilih Semua Modul"}
                      </Button>
                    </div>
                    <div className="overflow-x-auto">
                      <table className="min-w-full border-collapse">
                        <thead className="bg-surface-muted">
                          <tr>
                            <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.08em] text-text-secondary">
                              Resource
                            </th>
                            {matrix.actions.map((action) => (
                              <th
                                className="px-3 py-3 text-center text-xs font-semibold uppercase tracking-[0.08em] text-text-secondary"
                                key={action}
                              >
                                {action}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {matrix.rows.map((row) => {
                            const rowPermissionIDs = Object.values(row.permissionsByAction)
                              .filter(Boolean)
                              .map((permission) => permission!.id);
                            const allRowSelected = rowPermissionIDs.every((permissionID) =>
                              selectedPermissionIDs.includes(permissionID),
                            );

                            return (
                              <tr className="border-b border-border last:border-b-0" key={row.resource}>
                                <td className="px-4 py-3 align-top">
                                  <div className="space-y-1">
                                    <button
                                      className="text-left text-sm font-medium text-text-primary transition hover:text-module"
                                      onClick={() =>
                                        setSelectedPermissionIDs((current) =>
                                          toggleMany(current, rowPermissionIDs, !allRowSelected),
                                        )
                                      }
                                      type="button"
                                    >
                                      {row.resource}
                                    </button>
                                    <p className="text-xs text-text-secondary">
                                      {allRowSelected ? "Semua action dipilih" : "Pilih per action atau per baris"}
                                    </p>
                                  </div>
                                </td>
                                {matrix.actions.map((action) => {
                                  const permission = row.permissionsByAction[action];
                                  if (!permission) {
                                    return (
                                      <td
                                        className="px-3 py-3 text-center text-sm text-text-tertiary"
                                        key={action}
                                      >
                                        -
                                      </td>
                                    );
                                  }

                                  const checked = selectedPermissionIDs.includes(permission.id);
                                  return (
                                    <td className="px-3 py-3 text-center" key={action}>
                                      <button
                                        aria-pressed={checked}
                                        className={cn(
                                          "inline-flex min-h-16 min-w-20 items-center justify-center rounded-md border",
                                          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-module focus-visible:ring-offset-2 focus-visible:ring-offset-surface",
                                          checked
                                            ? "border-module bg-module-light text-module"
                                            : "border-border bg-surface text-text-secondary hover:border-module/40",
                                        )}
                                        onClick={() =>
                                          setSelectedPermissionIDs((current) =>
                                            toggleOne(current, permission.id),
                                          )
                                        }
                                        type="button"
                                      >
                                        <span className="inline-flex flex-col items-center justify-center gap-2 px-2 py-2 text-xs">
                                          <span className="font-semibold uppercase tracking-[0.08em]">
                                            {permission.action}
                                          </span>
                                          {permission.is_sensitive ? (
                                            <Lock className="h-3.5 w-3.5" />
                                          ) : null}
                                        </span>
                                      </button>
                                    </td>
                                  );
                                })}
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </FormModal>

      <ConfirmDialog
        confirmLabel="Hapus"
        description={
          deleteTarget?.users_count
            ? `Role ini masih dipakai ${deleteTarget.users_count} user. Reassign dulu sebelum menghapus.`
            : `Role ${deleteTarget?.name ?? ""} akan dihapus permanen.`
        }
        isLoading={deleteMutation.isPending}
        isOpen={canManageRoles && deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
        title={
          deleteTarget?.is_system
            ? "System role tidak bisa dihapus"
            : `Hapus role ${deleteTarget?.name ?? ""}?`
        }
      />
    </div>
  );
}

function buildPermissionMatrix(group: PermissionModuleGroup): PermissionMatrix {
  const actions = Array.from(
    new Set(group.permissions.map((permission) => permission.action)),
  );
  const resourceMap = new Map<string, Record<string, PermissionItem | undefined>>();

  for (const permission of group.permissions) {
    const current = resourceMap.get(permission.resource) ?? {};
    current[permission.action] = permission;
    resourceMap.set(permission.resource, current);
  }

  return {
    actions,
    rows: Array.from(resourceMap.entries()).map(([resource, permissionsByAction]) => ({
      resource,
      permissionsByAction,
    })),
  };
}

function slugify(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9\s_-]/g, "")
    .replace(/\s+/g, "_")
    .replace(/_+/g, "_");
}

function toggleOne(current: string[], value: string) {
  return current.includes(value)
    ? current.filter((item) => item !== value)
    : [...current, value];
}

function toggleMany(current: string[], values: string[], enable: boolean) {
  if (!enable) {
    return current.filter((item) => !values.includes(item));
  }

  const next = new Set(current);
  for (const value of values) {
    next.add(value);
  }
  return Array.from(next);
}

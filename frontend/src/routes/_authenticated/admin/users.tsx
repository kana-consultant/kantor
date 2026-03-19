import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Info,
  Search,
  Shield,
  ShieldOff,
  UserCog,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { ensureAuthenticated } from "@/services/auth";
import { hasRole } from "@/lib/rbac";
import {
  adminUsersKeys,
  listUsers,
  toggleUserActive,
  updateUserRoles,
} from "@/services/admin-users";
import type { AdminUser, AdminUserFilters, RoleKeyDTO } from "@/types/admin";

export const Route = createFileRoute("/_authenticated/admin/users")({
  beforeLoad: async () => {
    const session = await ensureAuthenticated();
    if (!session) throw new Error("Not authenticated");
    if (!hasRole(session, "super_admin")) {
      const { redirect } = await import("@tanstack/react-router");
      throw redirect({ to: "/forbidden" });
    }
  },
  component: AdminUsersPage,
});

interface RoleGroup {
  module: string;
  label: string;
  description: string;
  color: string;
  bgColor: string;
  roles: Array<{ key: RoleKeyDTO; label: string; description: string }>;
}

const ROLE_GROUPS: RoleGroup[] = [
  {
    module: "",
    label: "Global",
    description: "Akses ke seluruh modul dan pengaturan sistem",
    color: "text-purple-600 dark:text-purple-400",
    bgColor: "bg-purple-50 dark:bg-purple-900/20",
    roles: [
      {
        key: { name: "super_admin", module: "" },
        label: "Super Admin",
        description: "Akses penuh ke semua modul, pengguna, dan konfigurasi",
      },
    ],
  },
  {
    module: "operational",
    label: "Operasional",
    description: "Project management, kanban board, dan task tracking",
    color: "text-ops",
    bgColor: "bg-ops-light",
    roles: [
      {
        key: { name: "admin", module: "operational" },
        label: "Admin",
        description: "CRUD penuh: buat, edit, hapus project dan task",
      },
      {
        key: { name: "manager", module: "operational" },
        label: "Manager",
        description: "Lihat semua, edit task, approve perubahan",
      },
      {
        key: { name: "staff", module: "operational" },
        label: "Staff",
        description: "Buat dan edit task sendiri, tidak bisa hapus",
      },
      {
        key: { name: "viewer", module: "operational" },
        label: "Viewer",
        description: "Hanya lihat project dan task (read-only)",
      },
    ],
  },
  {
    module: "hris",
    label: "HRIS",
    description: "Data karyawan, gaji, reimbursement, dan langganan",
    color: "text-hr",
    bgColor: "bg-hr-light",
    roles: [
      {
        key: { name: "admin", module: "hris" },
        label: "Admin",
        description: "CRUD penuh: kelola employee, gaji, dan semua data HR",
      },
      {
        key: { name: "manager", module: "hris" },
        label: "Manager",
        description: "Lihat semua, approve reimbursement dan bonus",
      },
      {
        key: { name: "staff", module: "hris" },
        label: "Staff",
        description: "Edit data sendiri, ajukan reimbursement (tanpa akses gaji/bonus)",
      },
      {
        key: { name: "viewer", module: "hris" },
        label: "Viewer",
        description: "Lihat data karyawan dan departemen (tanpa gaji/bonus)",
      },
    ],
  },
  {
    module: "marketing",
    label: "Marketing",
    description: "Campaign, ads metrics, dan lead management",
    color: "text-mkt",
    bgColor: "bg-mkt-light",
    roles: [
      {
        key: { name: "admin", module: "marketing" },
        label: "Admin",
        description: "CRUD penuh: kelola campaign, ads, dan leads",
      },
      {
        key: { name: "manager", module: "marketing" },
        label: "Manager",
        description: "Lihat semua, edit campaign dan metrics",
      },
      {
        key: { name: "staff", module: "marketing" },
        label: "Staff",
        description: "Buat dan edit campaign, input ads metrics",
      },
      {
        key: { name: "viewer", module: "marketing" },
        label: "Viewer",
        description: "Hanya lihat data marketing (read-only)",
      },
    ],
  },
];

function roleKeyStr(r: RoleKeyDTO) {
  return r.module ? `${r.name}:${r.module}` : r.name;
}

function parseRoleStr(s: string): RoleKeyDTO {
  const [name, module = ""] = s.split(":");
  return { name, module };
}

function friendlyRoleName(roleStr: string): { label: string; color: string } {
  for (const group of ROLE_GROUPS) {
    for (const role of group.roles) {
      if (roleKeyStr(role.key) === roleStr) {
        return { label: `${role.label} ${group.label}`.trim(), color: group.color };
      }
    }
  }
  return { label: roleStr, color: "text-text-secondary" };
}

function AdminUsersPage() {
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<AdminUserFilters>({
    page: 1,
    perPage: 20,
    search: "",
  });
  const [searchInput, setSearchInput] = useState("");
  const [editingUser, setEditingUser] = useState<AdminUser | null>(null);
  const [selectedRoles, setSelectedRoles] = useState<string[]>([]);

  const usersQuery = useQuery({
    queryKey: adminUsersKeys.list(filters),
    queryFn: () => listUsers(filters),
  });

  const rolesMutation = useMutation({
    mutationFn: ({ userId, roles }: { userId: string; roles: RoleKeyDTO[] }) =>
      updateUserRoles(userId, roles),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: adminUsersKeys.all });
      setEditingUser(null);
    },
  });

  const activeMutation = useMutation({
    mutationFn: ({ userId, active }: { userId: string; active: boolean }) =>
      toggleUserActive(userId, active),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: adminUsersKeys.all });
    },
  });

  function openRoleEditor(user: AdminUser) {
    setEditingUser(user);
    setSelectedRoles([...user.roles]);
  }

  function toggleRole(roleStr: string) {
    setSelectedRoles((prev) =>
      prev.includes(roleStr) ? prev.filter((r) => r !== roleStr) : [...prev, roleStr],
    );
  }

  function saveRoles() {
    if (!editingUser) return;
    rolesMutation.mutate({
      userId: editingUser.user.id,
      roles: selectedRoles.map(parseRoleStr),
    });
  }

  const users = usersQuery.data?.items ?? [];
  const meta = usersQuery.data?.meta;
  const totalPages = meta ? Math.ceil(meta.total / meta.per_page) : 1;

  return (
    <div className="mx-auto max-w-5xl space-y-6 p-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.08em] text-purple-500 dark:text-purple-400">
          Admin
        </p>
        <h1 className="mt-2 text-[28px] font-bold tracking-tight text-text-primary">
          Kelola Pengguna
        </h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-text-secondary">
          Kelola akun, role, dan status pengguna. Setiap user yang mendaftar otomatis menjadi
          employee dan mendapat role <strong>Viewer</strong> di semua modul.
        </p>
      </div>

      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary" />
        <Input
          className="pl-10"
          placeholder="Cari nama atau email..."
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") setFilters((f) => ({ ...f, search: searchInput, page: 1 }));
          }}
        />
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-surface-muted text-left text-xs font-medium uppercase tracking-wider text-text-tertiary">
              <th className="px-4 py-3">Pengguna</th>
              <th className="px-4 py-3">Roles</th>
              <th className="px-4 py-3">Status</th>
              <th className="px-4 py-3 text-right">Aksi</th>
            </tr>
          </thead>
          <tbody>
            {usersQuery.isLoading && (
              <tr>
                <td colSpan={4} className="px-4 py-10 text-center text-text-tertiary">
                  Memuat...
                </td>
              </tr>
            )}
            {!usersQuery.isLoading && users.length === 0 && (
              <tr>
                <td colSpan={4} className="px-4 py-10 text-center text-text-tertiary">
                  Tidak ada pengguna ditemukan
                </td>
              </tr>
            )}
            {users.map((item) => (
              <tr key={item.user.id} className="border-b last:border-0 hover:bg-surface-muted/50">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-3">
                    <div className="flex h-9 w-9 items-center justify-center rounded-full bg-primary/10 text-sm font-semibold text-primary">
                      {item.user.full_name[0]?.toUpperCase()}
                    </div>
                    <div>
                      <p className="font-medium text-text-primary">{item.user.full_name}</p>
                      <p className="text-xs text-text-tertiary">{item.user.email}</p>
                    </div>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-1">
                    {item.roles.map((role) => {
                      const { label, color } = friendlyRoleName(role);
                      return (
                        <span
                          key={role}
                          className={cn(
                            "inline-block rounded-full px-2 py-0.5 text-[11px] font-medium",
                            role === "super_admin"
                              ? "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400"
                              : role.includes("operational")
                                ? "bg-ops-light text-ops"
                                : role.includes("hris")
                                  ? "bg-hr-light text-hr"
                                  : role.includes("marketing")
                                    ? "bg-mkt-light text-mkt"
                                    : "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
                          )}
                        >
                          {label}
                        </span>
                      );
                    })}
                  </div>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={cn(
                      "inline-block rounded-full px-2.5 py-0.5 text-xs font-medium",
                      item.user.is_active
                        ? "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400"
                        : "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
                    )}
                  >
                    {item.user.is_active ? "Aktif" : "Nonaktif"}
                  </span>
                </td>
                <td className="px-4 py-3 text-right">
                  <div className="flex items-center justify-end gap-1">
                    <Button variant="ghost" size="sm" onClick={() => openRoleEditor(item)} title="Edit role">
                      <UserCog className="h-4 w-4" />
                    </Button>
                    {item.user.is_active ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => activeMutation.mutate({ userId: item.user.id, active: false })}
                        title="Nonaktifkan"
                        disabled={activeMutation.isPending}
                      >
                        <ShieldOff className="h-4 w-4 text-red-500" />
                      </Button>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => activeMutation.mutate({ userId: item.user.id, active: true })}
                        title="Aktifkan"
                        disabled={activeMutation.isPending}
                      >
                        <Shield className="h-4 w-4 text-emerald-500" />
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between text-sm text-text-tertiary">
          <span>
            Halaman {filters.page} dari {totalPages} ({meta?.total ?? 0} pengguna)
          </span>
          <div className="flex gap-1">
            <Button
              variant="outline"
              size="sm"
              disabled={filters.page <= 1}
              onClick={() => setFilters((f) => ({ ...f, page: f.page - 1 }))}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={filters.page >= totalPages}
              onClick={() => setFilters((f) => ({ ...f, page: f.page + 1 }))}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}

      {/* Role editor dialog */}
      {editingUser && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="mx-4 w-full max-w-lg rounded-xl border bg-card p-6 shadow-xl">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-text-primary">
                Edit Role — {editingUser.user.full_name}
              </h3>
              <button onClick={() => setEditingUser(null)} className="text-text-tertiary hover:text-text-primary">
                <X className="h-5 w-5" />
              </button>
            </div>
            <p className="mt-1 text-sm text-text-tertiary">{editingUser.user.email}</p>

            <div className="mt-4 max-h-[28rem] space-y-4 overflow-y-auto pr-1">
              {ROLE_GROUPS.map((group) => (
                <div key={group.module || "global"}>
                  <div className="mb-2">
                    <p className={cn("text-xs font-semibold uppercase tracking-[0.08em]", group.color)}>
                      {group.label}
                    </p>
                    <p className="text-xs text-text-tertiary">{group.description}</p>
                  </div>
                  <div className="space-y-1">
                    {group.roles.map((role) => {
                      const key = roleKeyStr(role.key);
                      const checked = selectedRoles.includes(key);
                      return (
                        <label
                          key={key}
                          className={cn(
                            "flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-2.5 text-sm transition-colors",
                            checked
                              ? cn("border-current/20", group.bgColor)
                              : "border-transparent hover:bg-surface-muted",
                          )}
                          onClick={() => toggleRole(key)}
                        >
                          <div
                            className={cn(
                              "mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded border",
                              checked ? "border-primary bg-primary text-white" : "border-border",
                            )}
                          >
                            {checked && <Check className="h-3.5 w-3.5" />}
                          </div>
                          <div className="min-w-0">
                            <span className="font-medium text-text-primary">{role.label}</span>
                            <p className="mt-0.5 text-xs text-text-tertiary">{role.description}</p>
                          </div>
                        </label>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>

            <div className="mt-4 flex items-center gap-2 rounded-lg bg-surface-muted px-3 py-2">
              <Info className="h-4 w-4 shrink-0 text-text-tertiary" />
              <p className="text-xs text-text-tertiary">
                User bisa memiliki lebih dari satu role. Role menentukan akses ke fitur di setiap modul.
                Super Admin memiliki akses ke semua modul.
              </p>
            </div>

            {rolesMutation.isError && (
              <p className="mt-3 text-sm text-red-500">Gagal menyimpan role. Coba lagi.</p>
            )}

            <div className="mt-5 flex justify-end gap-2">
              <Button variant="outline" size="sm" onClick={() => setEditingUser(null)}>
                Batal
              </Button>
              <Button
                size="sm"
                onClick={saveRoles}
                disabled={rolesMutation.isPending || selectedRoles.length === 0}
              >
                {rolesMutation.isPending ? "Menyimpan..." : "Simpan Role"}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

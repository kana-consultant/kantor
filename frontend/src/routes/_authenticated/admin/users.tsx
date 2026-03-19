import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  ChevronLeft,
  ChevronRight,
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

// All possible roles in the system
const AVAILABLE_ROLES: RoleKeyDTO[] = [
  { name: "super_admin", module: "" },
  { name: "admin", module: "operational" },
  { name: "manager", module: "operational" },
  { name: "staff", module: "operational" },
  { name: "viewer", module: "operational" },
  { name: "admin", module: "hris" },
  { name: "manager", module: "hris" },
  { name: "staff", module: "hris" },
  { name: "viewer", module: "hris" },
  { name: "admin", module: "marketing" },
  { name: "manager", module: "marketing" },
  { name: "staff", module: "marketing" },
  { name: "viewer", module: "marketing" },
];

function roleLabel(r: RoleKeyDTO) {
  if (r.name === "super_admin") return "Super Admin";
  return `${r.name.charAt(0).toUpperCase() + r.name.slice(1)} — ${r.module}`;
}

function roleKeyStr(r: RoleKeyDTO) {
  return r.module ? `${r.name}:${r.module}` : r.name;
}

function parseRoleStr(s: string): RoleKeyDTO {
  const [name, module = ""] = s.split(":");
  return { name, module };
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
        <h1 className="text-2xl font-semibold text-text-primary">Kelola Pengguna</h1>
        <p className="text-sm text-text-tertiary">Kelola akun, role, dan status pengguna</p>
      </div>

      {/* Search */}
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

      {/* Table */}
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
                    {item.roles.map((role) => (
                      <span
                        key={role}
                        className={cn(
                          "inline-block rounded-full px-2 py-0.5 text-[11px] font-medium",
                          role === "super_admin"
                            ? "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400"
                            : "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
                        )}
                      >
                        {role}
                      </span>
                    ))}
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

      {/* Pagination */}
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
          <div className="mx-4 w-full max-w-md rounded-xl border bg-card p-6 shadow-xl">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-text-primary">
                Edit Role — {editingUser.user.full_name}
              </h3>
              <button onClick={() => setEditingUser(null)} className="text-text-tertiary hover:text-text-primary">
                <X className="h-5 w-5" />
              </button>
            </div>
            <p className="mt-1 text-sm text-text-tertiary">{editingUser.user.email}</p>

            <div className="mt-4 max-h-72 space-y-1.5 overflow-y-auto">
              {AVAILABLE_ROLES.map((role) => {
                const key = roleKeyStr(role);
                const checked = selectedRoles.includes(key);
                return (
                  <label
                    key={key}
                    className={cn(
                      "flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2 text-sm transition-colors",
                      checked
                        ? "border-primary/30 bg-primary/5"
                        : "border-transparent hover:bg-surface-muted",
                    )}
                  >
                    <div
                      className={cn(
                        "flex h-5 w-5 items-center justify-center rounded border",
                        checked ? "border-primary bg-primary text-white" : "border-border",
                      )}
                      onClick={() => toggleRole(key)}
                    >
                      {checked && <Check className="h-3.5 w-3.5" />}
                    </div>
                    <span className="text-text-primary" onClick={() => toggleRole(key)}>
                      {roleLabel(role)}
                    </span>
                  </label>
                );
              })}
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

import { useAuthStore } from "@/stores/auth-store";
import type { AuthModuleRole } from "@/types/auth";

export function useAuth() {
  const session = useAuthStore((state) => state.session);
  const setSession = useAuthStore((state) => state.setSession);
  const clearSession = useAuthStore((state) => state.clearSession);

  const moduleRoles = session?.module_roles ?? {};
  const isSuperAdmin = session?.is_super_admin ?? false;
  const roles = isSuperAdmin
    ? ["super_admin", ...Object.entries(moduleRoles).map(([moduleID, role]) => `${role.role_slug}:${moduleID}`)]
    : Object.entries(moduleRoles).map(([moduleID, role]) => `${role.role_slug}:${moduleID}`);
  const roleLabels = isSuperAdmin
    ? ["Super Admin", ...formatModuleRoleLabels(moduleRoles)]
    : formatModuleRoleLabels(moduleRoles);
  const roleSummary = isSuperAdmin
    ? "Super Admin"
    : roleLabels.length === 0
      ? "Tanpa akses modul"
      : roleLabels.length <= 2
        ? roleLabels.join(" | ")
        : `${roleLabels.slice(0, 2).join(" | ")} +${roleLabels.length - 2}`;

  return {
    session,
    user: session?.user ?? null,
    roles,
    roleLabels,
    roleSummary,
    moduleRoles,
    permissions: session?.permissions ?? [],
    isSuperAdmin,
    isAuthenticated: Boolean(session?.tokens.access_token),
    setSession,
    clearSession,
  };
}

function formatModuleRoleLabels(moduleRoles: Record<string, AuthModuleRole>) {
  const order = ["operational", "hris", "marketing", "admin"];
  const moduleNames: Record<string, string> = {
    operational: "OPS",
    hris: "HRIS",
    marketing: "MKT",
    admin: "ADMIN",
  };

  return order
    .filter((moduleID) => moduleRoles[moduleID])
    .map((moduleID) => `${moduleNames[moduleID] ?? moduleID.toUpperCase()} ${moduleRoles[moduleID]!.role_slug}`);
}

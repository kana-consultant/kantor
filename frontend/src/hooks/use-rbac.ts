import { useAuthStore } from "@/stores/auth-store";
import {
  canAccess,
  getModuleRole,
  hasAllPermissions,
  hasAnyPermission,
  hasModuleAccess,
  hasPermission,
  hasRole,
} from "@/lib/rbac";

export function useRBAC() {
  const session = useAuthStore((state) => state.session);

  return {
    hasPermission: (permission: string) => hasPermission(session, permission),
    hasAnyPermission: (...permissions: string[]) =>
      hasAnyPermission(session, ...permissions),
    hasAllPermissions: (...permissions: string[]) =>
      hasAllPermissions(session, ...permissions),
    hasRole: (role: string, module?: string) => hasRole(session, role, module),
    hasModuleAccess: (module: string) => hasModuleAccess(session, module),
    getModuleRole: (module: string) => getModuleRole(session, module),
    isSuperAdmin: session?.is_super_admin ?? false,
    canAccess: (options: {
      permission?: string;
      role?: string;
      module?: string;
    }) => canAccess(session, options),
  };
}

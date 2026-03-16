import { useAuthStore } from "@/stores/auth-store";
import { canAccess, hasPermission, hasRole } from "@/lib/rbac";

export function useRBAC() {
  const session = useAuthStore((state) => state.session);

  return {
    hasPermission: (permission: string) => hasPermission(session, permission),
    hasRole: (role: string, module?: string) => hasRole(session, role, module),
    canAccess: (options: {
      permission?: string;
      role?: string;
      module?: string;
    }) => canAccess(session, options),
  };
}

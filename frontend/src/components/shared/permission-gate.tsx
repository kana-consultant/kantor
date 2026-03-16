import type { PropsWithChildren, ReactNode } from "react";

import { useRBAC } from "@/hooks/use-rbac";

interface PermissionGateProps extends PropsWithChildren {
  permission?: string;
  role?: string;
  module?: string;
  fallback?: ReactNode;
}

export function PermissionGate({
  permission,
  role,
  module,
  fallback = null,
  children,
}: PermissionGateProps) {
  const { canAccess } = useRBAC();

  if (!canAccess({ permission, role, module })) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}

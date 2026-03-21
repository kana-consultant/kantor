import type { PropsWithChildren, ReactNode } from "react";

import { useRBAC } from "@/hooks/use-rbac";

interface ModuleGateProps extends PropsWithChildren {
  module: string;
  fallback?: ReactNode;
}

export function ModuleGate({
  module,
  fallback = null,
  children,
}: ModuleGateProps) {
  const { hasModuleAccess } = useRBAC();

  if (!hasModuleAccess(module)) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}

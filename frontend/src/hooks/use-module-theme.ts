import { useRouterState } from "@tanstack/react-router";

import { resolveBreadcrumb, resolveModuleTheme } from "@/lib/module-theme";

export function useModuleTheme() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });

  return {
    pathname,
    module: resolveModuleTheme(pathname),
    breadcrumb: resolveBreadcrumb(pathname),
  };
}

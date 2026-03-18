import type { PropsWithChildren } from "react";
import { useRouterState } from "@tanstack/react-router";

import { Sidebar } from "@/components/layout/sidebar";
import { Topbar } from "@/components/layout/topbar";
import { resolveModuleTheme } from "@/lib/module-theme";
import { cn } from "@/lib/utils";
import { useSidebarStore } from "@/stores/sidebar-store";

export function PageShell({ children }: PropsWithChildren) {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const {
    isDesktopCollapsed,
    isMobileOpen,
    setMobileOpen,
    toggleDesktopCollapsed,
  } = useSidebarStore();
  const module = resolveModuleTheme(pathname);

  return (
    <div
      className="mx-auto flex min-h-screen w-full max-w-[1760px] gap-2 bg-background p-3 lg:gap-3 lg:p-4"
      data-module={module.key}
    >
      <div
        className={cn(
          "hidden shrink-0 lg:block",
          isDesktopCollapsed ? "w-[4.25rem]" : "w-64",
        )}
      >
        <Sidebar collapsed={isDesktopCollapsed} onToggleCollapse={toggleDesktopCollapsed} />
      </div>

      <div
        className={cn(
          "fixed inset-0 z-40 bg-black/35 backdrop-blur-sm transition lg:hidden",
          isMobileOpen ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0",
        )}
        onClick={() => setMobileOpen(false)}
      />
      <div
        className={cn(
          "fixed inset-y-0 left-0 z-50 w-64 max-w-[88vw] p-3 transition-transform lg:hidden",
          isMobileOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <Sidebar mobile onNavigate={() => setMobileOpen(false)} />
      </div>

      <main className="flex min-h-[calc(100vh-1.5rem)] flex-1 flex-col gap-3 lg:min-h-[calc(100vh-2rem)]">
        <Topbar />
        <section className="page-transition flex-1" key={pathname}>
          {children}
        </section>
      </main>
    </div>
  );
}

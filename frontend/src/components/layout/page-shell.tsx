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
      className="relative mx-auto flex min-h-screen w-full max-w-[1760px] gap-2 overflow-x-clip bg-background p-2 sm:p-3 lg:gap-3 lg:p-4"
      data-module={module.key}
    >
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute left-[-10%] top-[-10%] h-64 w-64 rounded-full bg-module/8 blur-3xl sm:h-80 sm:w-80" />
        <div className="absolute bottom-[-14%] right-[-8%] h-72 w-72 rounded-full bg-module/10 blur-3xl sm:h-96 sm:w-96" />
        <div className="absolute inset-x-0 top-0 h-48 bg-gradient-to-b from-white/40 to-transparent dark:from-white/[0.03]" />
      </div>

      <div
        className={cn(
          "relative z-10 hidden shrink-0 lg:block",
          isDesktopCollapsed ? "w-[4.25rem]" : "w-64",
        )}
      >
        <Sidebar collapsed={isDesktopCollapsed} onToggleCollapse={toggleDesktopCollapsed} />
      </div>

      <div
        className={cn(
          "fixed inset-0 z-40 bg-slate-950/58 transition lg:hidden",
          isMobileOpen ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0",
        )}
        onClick={() => setMobileOpen(false)}
      />
      <div
        className={cn(
          "fixed inset-y-0 left-0 z-50 w-[min(92vw,360px)] transition-transform lg:hidden",
          isMobileOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <Sidebar mobile onNavigate={() => setMobileOpen(false)} />
      </div>

      <main className="relative z-10 flex min-h-[calc(100vh-1rem)] min-w-0 flex-1 flex-col gap-3 lg:min-h-[calc(100vh-2rem)]">
        <Topbar />
        <section className="page-transition min-w-0 flex-1" key={pathname}>
          {children}
        </section>
      </main>
    </div>
  );
}

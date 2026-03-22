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
      className="relative mx-auto flex h-[100dvh] w-full max-w-[1760px] gap-2 overflow-hidden bg-background px-2 pb-2 pt-1 sm:px-3 sm:pb-3 sm:pt-1.5 lg:gap-3 lg:px-4 lg:pb-4 lg:pt-2"
      data-module={module.key}
    >
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute left-[-10%] top-[-10%] h-64 w-64 rounded-full bg-module/8 blur-3xl sm:h-80 sm:w-80" />
        <div className="absolute bottom-[-14%] right-[-8%] h-72 w-72 rounded-full bg-module/10 blur-3xl sm:h-96 sm:w-96" />
        <div className="absolute inset-x-0 top-0 h-48 bg-gradient-to-b from-white/40 to-transparent dark:from-white/[0.03]" />
      </div>

      <div
        className={cn(
          "relative z-10 hidden min-h-0 shrink-0 lg:block",
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
          "fixed inset-y-2 left-2 z-50 w-[min(calc(100vw-1rem),380px)] transition-transform lg:hidden",
          isMobileOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <Sidebar mobile onNavigate={() => setMobileOpen(false)} />
      </div>

      <main className="relative z-10 flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-hidden lg:gap-5">
        <Topbar />
        <section className="page-transition min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain pb-2" key={pathname}>
          {children}
        </section>
      </main>
    </div>
  );
}

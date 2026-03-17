import type { PropsWithChildren } from "react";

import { Sidebar } from "@/components/layout/sidebar";
import { Topbar } from "@/components/layout/topbar";
import { cn } from "@/lib/utils";
import { useSidebarStore } from "@/stores/sidebar-store";

export function PageShell({ children }: PropsWithChildren) {
  const {
    isDesktopCollapsed,
    isMobileOpen,
    setMobileOpen,
    toggleDesktopCollapsed,
  } = useSidebarStore();

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-[1760px] gap-6 p-4 lg:p-6">
      <div
        className={cn(
          "hidden shrink-0 lg:block",
          isDesktopCollapsed ? "w-[6rem]" : "w-[18rem] xl:w-[20rem]",
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
          "fixed inset-y-0 left-0 z-50 w-[18rem] max-w-[88vw] p-4 transition-transform lg:hidden",
          isMobileOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <Sidebar mobile onNavigate={() => setMobileOpen(false)} />
      </div>

      <main className="flex min-h-[calc(100vh-2rem)] flex-1 flex-col gap-6">
        <Topbar />
        <section className="flex-1">{children}</section>
      </main>
    </div>
  );
}

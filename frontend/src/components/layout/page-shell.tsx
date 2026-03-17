import type { PropsWithChildren } from "react";

import { Sidebar } from "@/components/layout/sidebar";
import { Topbar } from "@/components/layout/topbar";

export function PageShell({ children }: PropsWithChildren) {
  return (
    <div className="mx-auto flex min-h-screen w-full max-w-[1760px] gap-6 p-4 lg:p-6">
      <div className="hidden w-80 shrink-0 xl:block">
        <Sidebar />
      </div>

      <main className="flex min-h-[calc(100vh-2rem)] flex-1 flex-col gap-6">
        <Topbar />
        <section className="flex-1">{children}</section>
      </main>
    </div>
  );
}

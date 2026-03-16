import { Outlet, createRootRouteWithContext } from "@tanstack/react-router";

import type { RouterContext } from "@/router";

function RootComponent() {
  return <Outlet />;
}

function NotFoundComponent() {
  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="rounded-[28px] border border-border bg-card/80 p-8 text-center shadow-panel">
        <p className="text-sm uppercase tracking-[0.3em] text-muted-foreground">
          404
        </p>
        <h1 className="mt-3 text-3xl font-bold">Page not found</h1>
      </div>
    </div>
  );
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootComponent,
  notFoundComponent: NotFoundComponent,
});

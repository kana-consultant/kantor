import {
  Outlet,
  createRootRouteWithContext,
  type ErrorComponentProps,
} from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
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
        <h1 className="mt-3 text-3xl font-bold">Halaman tidak ditemukan</h1>
      </div>
    </div>
  );
}

function RootErrorComponent({ error }: ErrorComponentProps) {
  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="rounded-[28px] border border-border bg-card/80 p-8 text-center shadow-panel max-w-md">
        <p className="text-sm uppercase tracking-[0.3em] text-destructive">
          Galat
        </p>
        <h1 className="mt-3 text-2xl font-bold">Terjadi masalah</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {import.meta.env.DEV
            ? error.message || "Terjadi kesalahan yang tidak terduga"
            : "Terjadi kesalahan yang tidak terduga"}
        </p>
        <Button
          variant="outline"
          className="mt-6"
          onClick={() => window.location.reload()}
        >
          Muat Ulang
        </Button>
      </div>
    </div>
  );
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootComponent,
  notFoundComponent: NotFoundComponent,
  errorComponent: RootErrorComponent,
});

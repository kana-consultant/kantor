import { QueryClient } from "@tanstack/react-query";
import { createRouter } from "@tanstack/react-router";

import { routeTree } from "@/routeTree.gen";

export interface RouterContext {
  queryClient: QueryClient;
}

export function createAppRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree,
    context: {
      queryClient,
    },
    defaultPreload: "intent",
    defaultPendingMinMs: 150,
    scrollRestoration: true,
    defaultErrorComponent: ({ error }) => (
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="text-center">
          <p className="text-sm uppercase tracking-[0.3em] text-destructive">
            Galat
          </p>
          <h1 className="mt-3 text-2xl font-bold">Terjadi masalah</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {error.message || "Terjadi kesalahan yang tidak terduga"}
          </p>
        </div>
      </div>
    ),
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}

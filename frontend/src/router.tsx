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
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}

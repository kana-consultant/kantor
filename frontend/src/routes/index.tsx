import { createFileRoute, redirect } from "@tanstack/react-router";

import { ensureAuthenticated } from "@/services/auth";

export const Route = createFileRoute("/")({
  beforeLoad: async () => {
    const session = await ensureAuthenticated();

    throw redirect({
      to: session ? "/operational" : "/login",
    });
  },
});

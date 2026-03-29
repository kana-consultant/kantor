import { createFileRoute, redirect } from "@tanstack/react-router";

import { getDefaultAuthorizedPath } from "@/lib/rbac";
import { ensureAuthenticated } from "@/services/auth";

export const Route = createFileRoute("/")({
  beforeLoad: async () => {
    const session = await ensureAuthenticated();

    throw redirect({
      to: getDefaultAuthorizedPath(session),
    });
  },
});

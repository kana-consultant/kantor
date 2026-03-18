import { createFileRoute, redirect } from "@tanstack/react-router";

import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";

export const Route = createFileRoute("/_authenticated/operational/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.operationalOverview);
    throw redirect({ to: "/operational/overview" });
  },
  component: () => null,
});

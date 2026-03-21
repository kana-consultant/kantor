import { createFileRoute, redirect } from "@tanstack/react-router";

import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";

export const Route = createFileRoute("/_authenticated/marketing/")({
  beforeLoad: async () => {
    await ensureModuleAccess("marketing");
    await ensurePermission(permissions.marketingOverview);
    throw redirect({ to: "/marketing/overview" });
  },
  component: () => null,
});

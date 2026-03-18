import { createFileRoute, redirect } from "@tanstack/react-router";

import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";

export const Route = createFileRoute("/_authenticated/hris/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.hrisOverview);
    throw redirect({ to: "/hris/overview" });
  },
  component: () => null,
});

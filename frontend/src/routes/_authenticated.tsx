import { Outlet, createFileRoute, redirect } from "@tanstack/react-router";

import { PageShell } from "@/components/layout/page-shell";
import { ensureAuthenticated } from "@/services/auth";

export const Route = createFileRoute("/_authenticated")({
  beforeLoad: async () => {
    const session = await ensureAuthenticated();

    if (!session) {
      throw redirect({
        to: "/login",
      });
    }
  },
  component: AuthenticatedLayout,
});

function AuthenticatedLayout() {
  return (
    <PageShell>
      <Outlet />
    </PageShell>
  );
}

import { useEffect } from "react";
import {
  Outlet,
  createFileRoute,
  redirect,
  useRouter,
  type ErrorComponentProps,
} from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { PageShell } from "@/components/layout/page-shell";
import { ensureAuthenticated } from "@/services/auth";
import { fetchSessionProfile } from "@/services/foundation";
import { useAuthStore } from "@/stores/auth-store";

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
  errorComponent: AuthenticatedErrorComponent,
});

function AuthenticatedLayout() {
  const session = useAuthStore((state) => state.session);
  const setSession = useAuthStore((state) => state.setSession);

  useEffect(() => {
    if (!session) {
      return;
    }

    let active = true;
    void fetchSessionProfile()
      .then((profile) => {
        if (!active) {
          return;
        }

        setSession({
          ...session,
          user: profile.user,
          module_roles: profile.module_roles,
          permissions: profile.permissions,
          is_super_admin: profile.is_super_admin,
        });
      })
      .catch(() => {
        // Route guards already handle auth loss; ignore profile sync errors here.
      });

    return () => {
      active = false;
    };
  }, [session?.tokens.access_token, setSession]);

  return (
    <PageShell>
      <Outlet />
    </PageShell>
  );
}

function AuthenticatedErrorComponent({ error, reset }: ErrorComponentProps) {
  const router = useRouter();

  return (
    <PageShell>
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="rounded-[28px] border border-border bg-card/80 p-8 text-center shadow-panel max-w-md">
          <p className="text-sm uppercase tracking-[0.3em] text-destructive">
            Error
          </p>
          <h1 className="mt-3 text-2xl font-bold">Something went wrong</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {error.message || "An unexpected error occurred"}
          </p>
          <div className="mt-6 flex items-center justify-center gap-3">
            <Button
              variant="outline"
              onClick={() => {
                reset();
                router.invalidate();
              }}
            >
              Try again
            </Button>
            <Button variant="outline" onClick={() => router.navigate({ to: "/" })}>
              Go home
            </Button>
          </div>
        </div>
      </div>
    </PageShell>
  );
}

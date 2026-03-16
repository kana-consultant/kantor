import { useMutation, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { PermissionGate } from "@/components/shared/permission-gate";
import { useAuth } from "@/hooks/use-auth";
import { fetchApiHealth, fetchModuleOverview, fetchSessionProfile } from "@/services/foundation";
import { refreshSession } from "@/services/auth";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";

export const Route = createFileRoute("/_authenticated/operational/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.operationalOverview);
  },
  component: OperationalPage,
});

function OperationalPage() {
  const { user, roles, permissions: grantedPermissions } = useAuth();
  const healthQuery = useQuery({
    queryKey: ["system", "health"],
    queryFn: fetchApiHealth,
  });
  const sessionQuery = useQuery({
    queryKey: ["auth", "session-profile"],
    queryFn: fetchSessionProfile,
  });
  const operationalQuery = useQuery({
    queryKey: ["operational", "overview"],
    queryFn: () => fetchModuleOverview("operational"),
  });
  const hrisQuery = useQuery({
    queryKey: ["hris", "overview"],
    queryFn: () => fetchModuleOverview("hris"),
  });
  const marketingQuery = useQuery({
    queryKey: ["marketing", "overview"],
    queryFn: () => fetchModuleOverview("marketing"),
  });
  const refreshMutation = useMutation({
    mutationFn: refreshSession,
    onSuccess: () => {
      void sessionQuery.refetch();
    },
  });

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
          Foundation verification
        </p>
        <h3 className="mt-3 text-3xl font-bold">UI test cockpit</h3>
        <p className="mt-3 max-w-3xl text-muted-foreground">
          Halaman ini sengaja dipakai untuk mengetes auth, refresh token, seed
          superadmin, protected endpoint, dan permission rendering langsung dari
          UI.
        </p>
        <div className="mt-6 flex flex-wrap gap-3">
          <Button
            disabled={refreshMutation.isPending}
            onClick={() => refreshMutation.mutate()}
            variant="outline"
          >
            {refreshMutation.isPending ? "Refreshing..." : "Refresh session"}
          </Button>
          <Button onClick={() => void sessionQuery.refetch()} variant="ghost">
            Reload auth profile
          </Button>
        </div>
      </Card>

      <div className="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
        <Card className="p-8">
          <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
            Seeded account
          </p>
          <h4 className="mt-3 text-2xl font-bold">
            {user?.full_name ?? "Unknown user"}
          </h4>
          <div className="mt-5 grid gap-4 md:grid-cols-2">
            <Metric label="Email" value={user?.email ?? "-"} />
            <Metric label="Primary role" value={roles[0] ?? "-"} />
            <Metric label="Role count" value={String(roles.length)} />
            <Metric label="Permission count" value={String(grantedPermissions.length)} />
          </div>
          <PermissionGate fallback={null} permission="operational:project:create">
            <div className="mt-6 rounded-3xl border border-emerald-200 bg-emerald-50/80 p-4 text-sm text-emerald-800">
              PermissionGate aktif: blok ini hanya terlihat jika user punya
              permission create project.
            </div>
          </PermissionGate>
        </Card>

        <Card className="p-8">
          <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
            Backend session
          </p>
          <h4 className="mt-3 text-2xl font-bold">`GET /api/v1/auth/me`</h4>
          <StatusBlock
            description="Endpoint auth middleware yang membaca JWT aktif dan mengembalikan profile server-side."
            error={sessionQuery.error instanceof Error ? sessionQuery.error.message : null}
            loading={sessionQuery.isLoading}
            title={sessionQuery.data?.user?.email ?? "Waiting for response"}
          />
        </Card>
      </div>

      <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-4">
        <QueryCard
          description="Public API health endpoint"
          error={healthQuery.error instanceof Error ? healthQuery.error.message : null}
          loading={healthQuery.isLoading}
          status={healthQuery.data?.status}
          title="API Health"
        />
        <QueryCard
          description="Protected Operational overview"
          error={operationalQuery.error instanceof Error ? operationalQuery.error.message : null}
          loading={operationalQuery.isLoading}
          status={operationalQuery.data?.message}
          title="Operational Overview"
        />
        <QueryCard
          description="Protected HRIS overview"
          error={hrisQuery.error instanceof Error ? hrisQuery.error.message : null}
          loading={hrisQuery.isLoading}
          status={hrisQuery.data?.message}
          title="HRIS Overview"
        />
        <QueryCard
          description="Protected Marketing overview"
          error={marketingQuery.error instanceof Error ? marketingQuery.error.message : null}
          loading={marketingQuery.isLoading}
          status={marketingQuery.data?.message}
          title="Marketing Overview"
        />
      </div>

      <Card className="p-8">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
          JWT claims snapshot
        </p>
        <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {grantedPermissions.map((permission) => (
            <div
              className="rounded-2xl border border-border/70 bg-background/70 px-4 py-3 text-sm text-muted-foreground"
              key={permission}
            >
              {permission}
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}

interface MetricProps {
  label: string;
  value: string;
}

function Metric({ label, value }: MetricProps) {
  return (
    <div className="rounded-3xl border border-border/70 bg-background/70 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-2 text-base font-semibold">{value}</p>
    </div>
  );
}

interface QueryCardProps {
  title: string;
  description: string;
  status?: string;
  loading: boolean;
  error: string | null;
}

function QueryCard({ title, description, status, loading, error }: QueryCardProps) {
  return (
    <Card className="p-6">
      <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">
        {title}
      </p>
      <p className="mt-3 text-sm text-muted-foreground">{description}</p>
      <p className="mt-4 text-lg font-semibold">
        {loading ? "Loading..." : error ? "Failed" : "Success"}
      </p>
      <p className="mt-2 text-sm text-muted-foreground">{error ?? status ?? "-"}</p>
    </Card>
  );
}

interface StatusBlockProps {
  title: string;
  description: string;
  loading: boolean;
  error: string | null;
}

function StatusBlock({ title, description, loading, error }: StatusBlockProps) {
  return (
    <div className="mt-5 rounded-3xl border border-border/70 bg-background/70 p-5">
      <p className="text-sm text-muted-foreground">{description}</p>
      <p className="mt-4 text-lg font-semibold">
        {loading ? "Loading..." : error ? "Failed" : title}
      </p>
      {error ? <p className="mt-2 text-sm text-red-600">{error}</p> : null}
    </div>
  );
}

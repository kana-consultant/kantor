import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { Card } from "@/components/ui/card";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { fetchModuleOverview } from "@/services/foundation";

export const Route = createFileRoute("/_authenticated/marketing/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.marketingOverview);
  },
  component: MarketingPage,
});

function MarketingPage() {
  const overviewQuery = useQuery({
    queryKey: ["marketing", "overview", "page"],
    queryFn: () => fetchModuleOverview("marketing"),
  });

  return (
    <Card className="p-8">
      <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
        Marketing
      </p>
      <h3 className="mt-3 text-3xl font-bold">Campaign and lead pipeline</h3>
      <p className="mt-3 max-w-2xl text-muted-foreground">
        Placeholder page untuk campaign kanban, ads metrics, dan leads
        management.
      </p>
      <div className="mt-6 rounded-3xl border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
        {overviewQuery.isLoading
          ? "Memuat protected overview..."
          : overviewQuery.error instanceof Error
            ? overviewQuery.error.message
            : overviewQuery.data?.message}
      </div>
    </Card>
  );
}

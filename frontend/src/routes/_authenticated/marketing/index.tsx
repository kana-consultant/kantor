import { BarChart3, FolderKanban, LayoutDashboard, Users } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";

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
    <div className="space-y-6">
      <Card className="p-8">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
          Marketing
        </p>
        <h3 className="mt-3 text-3xl font-bold">Campaign workspace</h3>
        <p className="mt-3 max-w-2xl text-muted-foreground">
          Gunakan board campaigns sebagai pusat eksekusi, lalu lanjut ke ads metrics
          dan leads pipeline pada step berikutnya.
        </p>
        <div className="mt-6 rounded-3xl border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
          {overviewQuery.isLoading
            ? "Memuat protected overview..."
            : overviewQuery.error instanceof Error
              ? overviewQuery.error.message
              : overviewQuery.data?.message}
        </div>
      </Card>

      <div className="grid gap-4 xl:grid-cols-4">
        <HubCard
          caption="Satu layar untuk status campaign, ads spend, funnel leads, dan top ROAS."
          icon={LayoutDashboard}
          title="Dashboard"
          to="/marketing/dashboard"
        />
        <HubCard
          caption="Board utama campaign dengan drag-and-drop stage."
          icon={FolderKanban}
          title="Campaigns"
          to="/marketing/campaigns"
        />
        <HubCard
          caption="Input spent, revenue, CTR, dan dashboard performa."
          icon={BarChart3}
          title="Ads Metrics"
          to="/marketing/ads-metrics"
        />
        <HubCard
          caption="Pipeline leads dari source masuk sampai closed won/lost."
          icon={Users}
          title="Leads"
          to="/marketing/leads"
        />
      </div>
    </div>
  );
}

function HubCard({
  title,
  caption,
  icon: Icon,
  to,
}: {
  title: string;
  caption: string;
  icon: typeof FolderKanban;
  to: "/marketing" | "/marketing/dashboard" | "/marketing/campaigns" | "/marketing/ads-metrics" | "/marketing/leads";
}) {
  return (
    <Link
      className="rounded-[28px] border border-border/70 bg-card/85 p-6 transition hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-panel"
      to={to}
    >
      <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary/15 text-primary">
        <Icon className="h-5 w-5" />
      </div>
      <h4 className="mt-5 text-xl font-bold">{title}</h4>
      <p className="mt-2 text-sm text-muted-foreground">{caption}</p>
    </Link>
  );
}

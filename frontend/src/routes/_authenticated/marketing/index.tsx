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
      <Card className="p-8 border-none bg-gradient-to-br from-mkt/5 to-surface shadow-md">
        <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-mkt mb-2">
          Marketing
        </p>
        <h3 className="mt-2 text-[32px] font-[700] leading-tight text-text-primary">Campaign workspace</h3>
        <p className="mt-4 max-w-2xl text-[14px] text-text-secondary leading-relaxed">
          Gunakan board campaigns sebagai pusat eksekusi, lalu lanjut ke ads metrics
          dan leads pipeline pada step berikutnya.
        </p>
        <div className="mt-6 rounded-[12px] border border-border bg-surface-muted p-4 text-[13px] font-[500] text-text-tertiary">
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
      className="group rounded-[12px] border border-border bg-surface p-6 transition-all hover:-translate-y-0.5 hover:border-mkt/30 hover:shadow-card flex flex-col items-start"
      to={to}
    >
      <div className="flex h-10 w-10 items-center justify-center rounded-[8px] bg-mkt/10 text-mkt">
        <Icon className="h-5 w-5" />
      </div>
      <h4 className="mt-5 text-[20px] font-[700] text-text-primary group-hover:text-mkt transition-colors">{title}</h4>
      <p className="mt-2 text-[13px] text-text-secondary leading-relaxed">{caption}</p>
    </Link>
  );
}

import { useMemo } from "react";

import { useQuery } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  AlertTriangle,
  BarChart3,
  CircleDollarSign,
  Megaphone,
  Target,
} from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { EmptyState } from "@/components/shared/empty-state";
import { OverviewSkeleton } from "@/components/shared/skeletons";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { formatIDR } from "@/lib/currency";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { getMarketingOverview, overviewKeys } from "@/services/overview";
import type { MarketingTopCampaign } from "@/types/overview";

const pipelineColors: Record<string, string> = {
  new: "#00B8D9",
  contacted: "#4C9AFF",
  qualified: "#6554C0",
  proposal: "#FF8B00",
  negotiation: "#FFAB00",
  won: "#36B37E",
  lost: "#97A0AF",
};

export const Route = createFileRoute("/_authenticated/marketing/overview")({
  beforeLoad: async () => {
    await ensurePermission(permissions.marketingOverview);
  },
  component: MarketingOverviewPage,
});

function MarketingOverviewPage() {
  const navigate = useNavigate();
  const { hasPermission } = useRBAC();
  const canManageCampaigns = hasPermission(permissions.marketingCampaignCreate);
  const canManageAds = hasPermission(permissions.marketingAdsMetricsCreate);
  const overviewQuery = useQuery({
    queryKey: overviewKeys.marketing(),
    queryFn: getMarketingOverview,
  });

  const topCampaignColumns = useMemo<Array<DataTableColumn<MarketingTopCampaign>>>(
    () => [
      {
        id: "campaign",
        header: "Campaign",
        cell: (row) => (
          <div>
            <p className="font-semibold text-text-primary">{row.campaign_name}</p>
            <div className="mt-2">
              <StatusBadge status={row.status} variant="campaign-status" />
            </div>
          </div>
        ),
      },
      {
        id: "spent",
        header: "Spent",
        accessor: "total_spent",
        align: "right",
        numeric: true,
        cell: (row) => formatIDR(row.total_spent),
      },
      {
        id: "roas",
        header: "ROAS",
        accessor: "roas",
        align: "right",
        numeric: true,
        cell: (row) => formatRoas(row.roas),
      },
    ],
    [],
  );

  if (overviewQuery.isLoading) {
    return <OverviewSkeleton />;
  }

  if (overviewQuery.isError || !overviewQuery.data) {
    return (
      <EmptyState
        actionLabel="Open campaigns"
        description="Data overview marketing belum bisa dimuat. Anda masih bisa lanjut dari halaman campaigns, ads metrics, atau leads."
        icon={AlertTriangle}
        onAction={() => void navigate({ to: "/marketing/campaigns" })}
        title="Overview tidak tersedia"
      />
    );
  }

  const overview = overviewQuery.data;
  const roasTone =
    overview.overall_roas == null
      ? "info"
      : overview.overall_roas > 3
        ? "success"
        : overview.overall_roas >= 1
          ? "warning"
          : "error";

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-mkt">
            Marketing
          </p>
          <h1 className="mt-2 text-[28px] font-bold tracking-tight text-text-primary">
            Campaign and funnel overview
          </h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
            Pantau campaign aktif, ads spend bulan ini, ROAS trend, dan kondisi funnel leads tanpa pindah halaman.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => void navigate({ to: "/marketing/campaigns" })}>
            {canManageCampaigns ? "Manage Campaigns" : "View Campaigns"}
          </Button>
          {canManageAds && (
            <Button
              onClick={() => void navigate({ to: "/marketing/ads-metrics" })}
              variant="secondary"
            >
              Open Ads Metrics
            </Button>
          )}
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-4">
        <StatCard
          helper="Campaign dengan status ideation sampai live."
          icon={Megaphone}
          label="Active Campaigns"
          tone="mkt"
          value={overview.active_campaigns.toLocaleString("id-ID")}
        />
        <StatCard
          helper="Akumulasi amount spent bulan berjalan."
          icon={CircleDollarSign}
          label="Total Ads Spent"
          mono
          tone="mkt"
          value={formatIDR(overview.total_ads_spent)}
        />
        <StatCard
          helper="Revenue dibandingkan dengan total spend bulan ini."
          icon={BarChart3}
          label="Overall ROAS"
          tone={roasTone}
          value={formatRoas(overview.overall_roas)}
        />
        <StatCard
          helper={`${overview.conversion_rate.toFixed(2)}% conversion rate`}
          icon={Target}
          label="Total Leads"
          tone="info"
          value={overview.total_leads.toLocaleString("id-ID")}
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-2">
        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-mkt">
              Ads Performance
            </p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">
              ROAS trend in 6 months
            </h2>
          </div>
          <div className="mt-6 h-[320px]">
            <ResponsiveContainer height="100%" width="100%">
              <LineChart data={overview.roas_trend}>
                <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="4 4" vertical={false} />
                <XAxis dataKey="label" stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} />
                <YAxis stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} />
                <Tooltip
                  contentStyle={{
                    backgroundColor: "hsl(var(--surface))",
                    border: "1px solid hsl(var(--border))",
                    borderRadius: 8,
                    boxShadow: "0 4px 8px -2px rgba(23,43,77,0.08), 0 2px 4px -2px rgba(23,43,77,0.06)",
                  }}
                  formatter={(value) => formatRoas(Number(value))}
                />
                <Line
                  dataKey="roas"
                  dot={{ fill: "#FF5630", r: 4 }}
                  stroke="#FF5630"
                  strokeWidth={2}
                  type="monotone"
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </Card>

        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-mkt">
              Leads Pipeline
            </p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">
              Current stage distribution
            </h2>
          </div>
          <div className="mt-6 h-[320px]">
            {overview.leads_by_stage.some((item) => item.lead_count > 0) ? (
              <ResponsiveContainer height="100%" width="100%">
                <BarChart data={overview.leads_by_stage} layout="vertical" margin={{ left: 24 }}>
                  <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="4 4" horizontal={false} />
                  <XAxis allowDecimals={false} stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} type="number" />
                  <YAxis dataKey="label" stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} type="category" width={96} />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "hsl(var(--surface))",
                      border: "1px solid hsl(var(--border))",
                      borderRadius: 8,
                      boxShadow: "0 4px 8px -2px rgba(23,43,77,0.08), 0 2px 4px -2px rgba(23,43,77,0.06)",
                    }}
                  />
                  <Bar dataKey="lead_count" radius={[0, 4, 4, 0]}>
                    {overview.leads_by_stage.map((item) => (
                      <Cell fill={pipelineColors[item.status] ?? "#97A0AF"} key={item.status} />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <EmptyState
                className="h-full"
                description="Pipeline akan muncul saat lead mulai masuk dari channel marketing."
                icon={Target}
                title="Belum ada leads"
              />
            )}
          </div>
        </Card>
      </div>

      <Card className="p-6">
        <div className="border-b border-border pb-4">
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-mkt">
            Top Campaigns
          </p>
          <h2 className="mt-2 text-[22px] font-bold text-text-primary">
            Best ROAS this month
          </h2>
        </div>
        <div className="mt-6">
          <DataTable
            columns={topCampaignColumns}
            data={overview.top_campaigns}
            emptyDescription="Campaign dengan ROAS terbaik akan muncul saat ads metrics bulan ini sudah terisi."
            emptyTitle="Belum ada campaign ranking"
            getRowId={(row) => row.campaign_id}
          />
        </div>
      </Card>
    </div>
  );
}

function formatRoas(value?: number | null) {
  if (value == null) {
    return "-";
  }

  return `${value.toFixed(2)}x`;
}

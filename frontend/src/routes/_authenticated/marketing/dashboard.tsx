import { useMemo } from "react";

import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
} from "recharts";

import { Card } from "@/components/ui/card";
import { formatIDR } from "@/lib/currency";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  adsMetricsKeys,
  getAdsMetricsSummary,
} from "@/services/marketing-ads-metrics";
import { campaignsKeys, listCampaignKanban } from "@/services/marketing-campaigns";
import { getLeadSummary, leadsKeys } from "@/services/marketing-leads";

const chartColors = ["#2563eb", "#16a34a", "#ea580c", "#7c3aed", "#ca8a04", "#0f766e", "#dc2626"];

export const Route = createFileRoute("/_authenticated/marketing/dashboard")({
  beforeLoad: async () => {
    await ensureModuleAccess("marketing");
    await ensurePermission(permissions.marketingOverview);
  },
  component: MarketingDashboardPage,
});

function MarketingDashboardPage() {
  const monthRange = getMonthRange(0);
  const previousMonthRange = getMonthRange(-1);

  const campaignsQuery = useQuery({
    queryKey: campaignsKeys.kanban(),
    queryFn: listCampaignKanban,
  });

  const thisMonthMetricsQuery = useQuery({
    queryKey: adsMetricsKeys.summary("campaign", monthRange.dateFrom, monthRange.dateTo),
    queryFn: () => getAdsMetricsSummary("campaign", monthRange.dateFrom, monthRange.dateTo),
  });

  const lastMonthMetricsQuery = useQuery({
    queryKey: adsMetricsKeys.summary("campaign", previousMonthRange.dateFrom, previousMonthRange.dateTo),
    queryFn: () => getAdsMetricsSummary("campaign", previousMonthRange.dateFrom, previousMonthRange.dateTo),
  });

  const leadsSummaryQuery = useQuery({
    queryKey: leadsKeys.summary(),
    queryFn: getLeadSummary,
  });

  const campaignStatusRows = useMemo(
    () =>
      (campaignsQuery.data ?? []).map((column) => ({
        label: column.name,
        value: column.campaigns?.length ?? 0,
      })),
    [campaignsQuery.data],
  );

  const currentCampaignMetrics = thisMonthMetricsQuery.data?.items ?? [];
  const currentSpent = currentCampaignMetrics.reduce((total, row) => total + row.total_spent, 0);
  const previousSpent = (lastMonthMetricsQuery.data?.items ?? []).reduce((total, row) => total + row.total_spent, 0);
  const topCampaigns = [...currentCampaignMetrics].sort((left, right) => (right.roas ?? 0) - (left.roas ?? 0)).slice(0, 5);

  const leadSummary = leadsSummaryQuery.data;
  const totalLeads = leadSummary?.total_leads ?? 0;
  const funnelRows = leadSummary?.by_status ?? [];

  return (
    <div className="space-y-6">
      <Card className="border-mkt/20 bg-gradient-to-br from-mkt/10 via-background to-background p-8">
        <p className="text-sm font-semibold uppercase tracking-[0.24em] text-mkt">Marketing dashboard</p>
        <h3 className="mt-2 text-3xl font-bold tracking-tight text-foreground">Overview lintas campaign, ads, dan leads</h3>
        <p className="mt-3 max-w-3xl text-muted-foreground">
          Satu layar untuk melihat distribusi status campaign, perbandingan spend bulan ini,
          funnel leads, dan campaign dengan ROAS terbaik.
        </p>
      </Card>

      <div className="grid gap-4 lg:grid-cols-4">
        <SummaryCard label="Campaign aktif" value={String((campaignsQuery.data ?? []).reduce((total, column) => total + (column.campaigns?.length ?? 0), 0))} />
        <SummaryCard label="Ads spent bulan ini" value={formatIDR(currentSpent)} />
        <SummaryCard label="Ads spent bulan lalu" value={formatIDR(previousSpent)} />
        <SummaryCard label="Conversion rate leads" value={`${(leadSummary?.conversion_rate ?? 0).toFixed(2)}%`} />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1.2fr,1fr]">
        <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
          <p className="text-sm font-semibold uppercase tracking-[0.24em] text-mkt">Campaign status</p>
          <h4 className="mt-2 text-2xl font-bold tracking-tight text-foreground">Status distribution</h4>
          <div className="mt-6 h-[300px]">
            <ResponsiveContainer height="100%" width="100%">
              <PieChart>
                <Pie data={campaignStatusRows} dataKey="value" innerRadius={70} outerRadius={110} paddingAngle={4}>
                  {campaignStatusRows.map((row, index) => (
                    <Cell fill={chartColors[index % chartColors.length]} key={row.label} />
                  ))}
                </Pie>
                <Tooltip formatter={(value) => Number(value).toLocaleString("id-ID")} />
              </PieChart>
            </ResponsiveContainer>
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            {campaignStatusRows.map((row, index) => (
              <div className="flex items-center justify-between rounded-[18px] border border-border/70 bg-background/70 px-4 py-3" key={row.label}>
                <div className="flex items-center gap-3">
                  <span className="h-3 w-3 rounded-full" style={{ backgroundColor: chartColors[index % chartColors.length] }} />
                  <span className="text-sm font-medium">{row.label}</span>
                </div>
                <span className="text-sm text-muted-foreground">{row.value}</span>
              </div>
            ))}
          </div>
        </Card>

        <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
          <p className="text-sm font-semibold uppercase tracking-[0.24em] text-mkt">Top campaigns</p>
          <h4 className="mt-2 text-2xl font-bold tracking-tight text-foreground">Top 5 by ROAS</h4>
          <div className="mt-5 space-y-3">
            {topCampaigns.length > 0 ? (
              topCampaigns.map((row) => (
                <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={row.group_key}>
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="font-semibold">{row.group_label}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {formatIDR(row.total_spent)} spent | {formatIDR(row.total_revenue)} revenue
                      </p>
                    </div>
                    <div className={`text-right text-sm font-semibold ${metricTone(row.roas)}`}>
                      {formatRatio(row.roas)}
                    </div>
                  </div>
                </div>
              ))
            ) : (
              <p className="text-sm text-muted-foreground">Belum ada data ROAS bulan ini.</p>
            )}
          </div>
        </Card>
      </div>

      <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
        <p className="text-sm font-semibold uppercase tracking-[0.24em] text-mkt">Leads funnel</p>
        <h4 className="mt-2 text-2xl font-bold tracking-tight text-foreground">Pipeline from new to won</h4>
        <div className="mt-5 space-y-3">
          {funnelRows.map((row) => {
            const percentage = totalLeads > 0 ? (row.lead_count / totalLeads) * 100 : 0;
            return (
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={row.status}>
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <p className="font-semibold">{row.label}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {row.lead_count.toLocaleString("id-ID")} leads | {formatIDR(row.estimated_value)}
                    </p>
                  </div>
                  <div className="text-right text-sm font-semibold">{percentage.toFixed(1)}%</div>
                </div>
                <div className="mt-3 h-3 rounded-full bg-muted">
                  <div
                    className="h-3 rounded-full bg-primary transition-all"
                    style={{ width: `${Math.max(percentage, percentage > 0 ? 6 : 0)}%` }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </Card>
    </div>
  );
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm transition-all hover:border-mkt/30 hover:shadow-sm">
      <p className="text-sm font-medium uppercase tracking-[0.16em] text-muted-foreground">{label}</p>
      <p className="mt-4 text-3xl font-bold tracking-tight text-foreground">{value}</p>
    </Card>
  );
}

function getMonthRange(offset: number) {
  const now = new Date();
  const start = new Date(now.getFullYear(), now.getMonth() + offset, 1);
  const end = new Date(now.getFullYear(), now.getMonth() + offset + 1, 0);

  return {
    dateFrom: start.toISOString().slice(0, 10),
    dateTo: end.toISOString().slice(0, 10),
  };
}

function formatRatio(value?: number | null) {
  if (value === undefined || value === null) {
    return "-";
  }
  return `${value.toFixed(2)}x`;
}

function metricTone(value?: number | null) {
  if (value === undefined || value === null) {
    return "text-muted-foreground";
  }
  if (value > 3) {
    return "text-success";
  }
  if (value >= 1) {
    return "text-warning";
  }
  return "text-error";
}

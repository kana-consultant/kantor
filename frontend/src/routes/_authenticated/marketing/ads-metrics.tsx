import type { Dispatch, SetStateAction } from "react";
import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Controller, useForm } from "react-hook-form";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { z } from "zod";

import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { formatIDR } from "@/lib/currency";
import { adsMetricPlatformOptions, adsPlatformMeta } from "@/lib/marketing";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import {
  adsMetricsKeys,
  batchCreateAdsMetrics,
  createAdsMetric,
  deleteAdsMetric,
  exportAdsMetricsCSV,
  getAdsMetricsSummary,
  listAdsMetrics,
  updateAdsMetric,
} from "@/services/marketing-ads-metrics";
import { campaignsKeys, listCampaigns } from "@/services/marketing-campaigns";
import type { AdsMetric, AdsMetricFilters, AdsMetricFormValues } from "@/types/marketing";

const metricSchema = z
  .object({
    campaign_id: z.string().min(1, "Campaign is required"),
    platform: z.enum(["instagram", "facebook", "google_ads", "tiktok", "youtube", "other"]),
    period_start: z.string().min(1, "Start date is required"),
    period_end: z.string().min(1, "End date is required"),
    amount_spent: z.number().min(0),
    impressions: z.number().min(0),
    clicks: z.number().min(0),
    conversions: z.number().min(0),
    revenue: z.number().min(0),
    notes: z.string(),
  })
  .refine((value) => value.period_end >= value.period_start, {
    message: "Period end must be on or after period start",
    path: ["period_end"],
  });

const defaultMetricForm: AdsMetricFormValues = {
  campaign_id: "",
  platform: "instagram",
  period_start: new Date().toISOString().slice(0, 10),
  period_end: new Date().toISOString().slice(0, 10),
  amount_spent: 0,
  impressions: 0,
  clicks: 0,
  conversions: 0,
  revenue: 0,
  notes: "",
};

const defaultFilters: AdsMetricFilters = {
  page: 1,
  perPage: 20,
  campaignId: "",
  platform: "",
  dateFrom: "",
  dateTo: "",
};

const summaryColors = ["#2563eb", "#16a34a", "#ea580c", "#7c3aed", "#ca8a04", "#0f766e"];

export const Route = createFileRoute("/_authenticated/marketing/ads-metrics")({
  beforeLoad: async () => {
    await ensurePermission(permissions.marketingAdsMetricsView);
  },
  component: AdsMetricsPage,
});

function AdsMetricsPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [activeTab, setActiveTab] = useState<"input" | "dashboard">("input");
  const [filters, setFilters] = useState<AdsMetricFilters>(defaultFilters);
  const [dashboardRange, setDashboardRange] = useState({
    dateFrom: `${new Date().getFullYear()}-01-01`,
    dateTo: new Date().toISOString().slice(0, 10),
  });
  const [showForm, setShowForm] = useState(false);
  const [showBatchForm, setShowBatchForm] = useState(false);
  const [editingMetric, setEditingMetric] = useState<AdsMetric | null>(null);
  const [batchRows, setBatchRows] = useState<AdsMetricFormValues[]>([{ ...defaultMetricForm }]);

  const campaignsQuery = useQuery({
    queryKey: campaignsKeys.list({
      page: 1,
      perPage: 100,
      search: "",
      channel: "",
      status: "",
      pic: "",
      dateFrom: "",
      dateTo: "",
    }),
    queryFn: () =>
      listCampaigns({
        page: 1,
        perPage: 100,
        search: "",
        channel: "",
        status: "",
        pic: "",
        dateFrom: "",
        dateTo: "",
      }),
  });

  const metricsQuery = useQuery({
    queryKey: adsMetricsKeys.list(filters),
    queryFn: () => listAdsMetrics(filters),
  });

  const campaignSummaryQuery = useQuery({
    queryKey: adsMetricsKeys.summary("campaign", dashboardRange.dateFrom, dashboardRange.dateTo),
    queryFn: () => getAdsMetricsSummary("campaign", dashboardRange.dateFrom, dashboardRange.dateTo),
  });

  const platformSummaryQuery = useQuery({
    queryKey: adsMetricsKeys.summary("platform", dashboardRange.dateFrom, dashboardRange.dateTo),
    queryFn: () => getAdsMetricsSummary("platform", dashboardRange.dateFrom, dashboardRange.dateTo),
  });

  const monthlySummaryQuery = useQuery({
    queryKey: adsMetricsKeys.summary("month", dashboardRange.dateFrom, dashboardRange.dateTo),
    queryFn: () => getAdsMetricsSummary("month", dashboardRange.dateFrom, dashboardRange.dateTo),
  });

  const form = useForm<AdsMetricFormValues>({
    resolver: zodResolver(metricSchema),
    defaultValues: defaultMetricForm,
  });

  const createMutation = useMutation({
    mutationFn: createAdsMetric,
    onSuccess: async () => {
      resetMetricForm(form);
      setShowForm(false);
      await invalidateAdsMetrics(queryClient);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (payload: { metricId: string; values: AdsMetricFormValues }) =>
      updateAdsMetric(payload.metricId, payload.values),
    onSuccess: async () => {
      resetMetricForm(form);
      setEditingMetric(null);
      setShowForm(false);
      await invalidateAdsMetrics(queryClient);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteAdsMetric,
    onSuccess: async () => {
      await invalidateAdsMetrics(queryClient);
    },
  });

  const batchMutation = useMutation({
    mutationFn: batchCreateAdsMetrics,
    onSuccess: async () => {
      setBatchRows([{ ...defaultMetricForm }]);
      setShowBatchForm(false);
      await invalidateAdsMetrics(queryClient);
    },
  });

  const campaigns = campaignsQuery.data?.items ?? [];
  const monthlyRows = monthlySummaryQuery.data?.items ?? [];
  const campaignRows = [...(campaignSummaryQuery.data?.items ?? [])].sort(
    (left, right) => (right.roas ?? 0) - (left.roas ?? 0),
  );
  const platformRows = platformSummaryQuery.data?.items ?? [];
  const metrics = metricsQuery.data?.items ?? [];

  const totals = monthlyRows.reduce(
    (accumulator, row) => ({
      spent: accumulator.spent + row.total_spent,
      revenue: accumulator.revenue + row.total_revenue,
      impressions: accumulator.impressions + row.total_impressions,
      clicks: accumulator.clicks + row.total_clicks,
    }),
    { spent: 0, revenue: 0, impressions: 0, clicks: 0 },
  );

  const overallROAS = totals.spent > 0 ? totals.revenue / totals.spent : null;
  const overallCTR = totals.impressions > 0 ? (totals.clicks / totals.impressions) * 100 : null;

  const handleSubmitMetric = form.handleSubmit((values) => {
    if (editingMetric) {
      updateMutation.mutate({ metricId: editingMetric.id, values });
      return;
    }
    createMutation.mutate(values);
  });

  const handleExport = async () => {
    const blob = await exportAdsMetricsCSV(dashboardRange.dateFrom, dashboardRange.dateTo);
    const url = window.URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `ads-metrics-${dashboardRange.dateFrom || "all"}-${dashboardRange.dateTo || "all"}.csv`;
    anchor.click();
    window.URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">Marketing analytics</p>
            <h3 className="mt-2 text-3xl font-bold">Ads spent and performance metrics</h3>
            <p className="mt-2 max-w-3xl text-muted-foreground">
              Input performa campaign manual, pantau ROAS dan CTR per periode, lalu export data tanpa pindah tool.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Button onClick={() => setActiveTab("input")} variant={activeTab === "input" ? "default" : "outline"}>
              Input Data
            </Button>
            <Button onClick={() => setActiveTab("dashboard")} variant={activeTab === "dashboard" ? "default" : "outline"}>
              Dashboard
            </Button>
          </div>
        </div>
      </Card>

      {activeTab === "input" ? (
        <div className="space-y-6">
          <Card className="p-6">
            <div className="grid gap-3 lg:grid-cols-5">
              <select
                className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm"
                onChange={(event) => setFilters((previous) => ({ ...previous, campaignId: event.target.value, page: 1 }))}
                value={filters.campaignId}
              >
                <option value="">All campaigns</option>
                {campaigns.map((campaign) => (
                  <option key={campaign.id} value={campaign.id}>
                    {campaign.name}
                  </option>
                ))}
              </select>
              <select
                className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm"
                onChange={(event) => setFilters((previous) => ({ ...previous, platform: event.target.value, page: 1 }))}
                value={filters.platform}
              >
                <option value="">All platforms</option>
                {adsMetricPlatformOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              <Input onChange={(event) => setFilters((previous) => ({ ...previous, dateFrom: event.target.value, page: 1 }))} type="date" value={filters.dateFrom} />
              <Input onChange={(event) => setFilters((previous) => ({ ...previous, dateTo: event.target.value, page: 1 }))} type="date" value={filters.dateTo} />
              <Button
                onClick={() => setFilters((previous) => ({ ...previous, campaignId: "", platform: "", dateFrom: "", dateTo: "", page: 1 }))}
                variant="outline"
              >
                Clear
              </Button>
            </div>
            <div className="mt-4 flex flex-wrap gap-3">
              <PermissionGate permission={permissions.marketingAdsMetricsCreate}>
                <Button
                  onClick={() => {
                    setEditingMetric(null);
                    resetMetricForm(form);
                    setShowForm((value) => !value);
                    setShowBatchForm(false);
                  }}
                >
                  {showForm ? "Close form" : "New entry"}
                </Button>
                <Button
                  onClick={() => {
                    setShowBatchForm((value) => !value);
                    setShowForm(false);
                    setEditingMetric(null);
                    resetMetricForm(form);
                  }}
                  variant="outline"
                >
                  {showBatchForm ? "Close bulk input" : "Bulk input"}
                </Button>
              </PermissionGate>
            </div>
          </Card>

          {showForm ? (
            <Card className="p-6">
              <div className="mb-5">
                <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Single entry</p>
                <h4 className="mt-2 text-2xl font-bold">{editingMetric ? "Edit ads metric" : "Add ads metric"}</h4>
              </div>
              <form className="grid gap-4 lg:grid-cols-2" onSubmit={handleSubmitMetric}>
                <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...form.register("campaign_id")}>
                  <option value="">Select campaign</option>
                  {campaigns.map((campaign) => (
                    <option key={campaign.id} value={campaign.id}>
                      {campaign.name}
                    </option>
                  ))}
                </select>
                <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...form.register("platform")}>
                  {adsMetricPlatformOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
                <Input {...form.register("period_start")} type="date" />
                <Input {...form.register("period_end")} type="date" />
                <Controller control={form.control} name="amount_spent" render={({ field }) => <CurrencyInput onValueChange={field.onChange} value={field.value} />} />
                <Controller control={form.control} name="revenue" render={({ field }) => <CurrencyInput onValueChange={field.onChange} value={field.value} />} />
                <Input {...form.register("impressions", { valueAsNumber: true })} min={0} placeholder="Impressions" type="number" />
                <Input {...form.register("clicks", { valueAsNumber: true })} min={0} placeholder="Clicks" type="number" />
                <Input {...form.register("conversions", { valueAsNumber: true })} min={0} placeholder="Conversions" type="number" />
                <Input className="lg:col-span-2" {...form.register("notes")} placeholder="Notes" />
                {form.formState.errors.period_end ? (
                  <p className="text-sm text-red-700 lg:col-span-2">{form.formState.errors.period_end.message}</p>
                ) : null}
                <div className="flex flex-wrap gap-3 lg:col-span-2">
                  <Button disabled={createMutation.isPending || updateMutation.isPending} type="submit">
                    {editingMetric ? "Save changes" : "Save entry"}
                  </Button>
                  <Button
                    onClick={() => {
                      setEditingMetric(null);
                      resetMetricForm(form);
                      setShowForm(false);
                    }}
                    type="button"
                    variant="outline"
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            </Card>
          ) : null}

          {showBatchForm ? (
            <Card className="p-6">
              <div className="mb-5 flex items-start justify-between gap-4">
                <div>
                  <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Bulk input</p>
                  <h4 className="mt-2 text-2xl font-bold">Multiple ads metric rows</h4>
                </div>
                <Button onClick={() => setBatchRows((previous) => [...previous, { ...defaultMetricForm }])} variant="outline">
                  Add row
                </Button>
              </div>
              <div className="space-y-4">
                {batchRows.map((row, index) => (
                  <div className="grid gap-3 rounded-[24px] border border-border/70 bg-background/70 p-4 lg:grid-cols-5" key={`${index}-${row.campaign_id}-${row.platform}`}>
                    <select className="h-11 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => updateBatchRow(setBatchRows, index, "campaign_id", event.target.value)} value={row.campaign_id}>
                      <option value="">Select campaign</option>
                      {campaigns.map((campaign) => (
                        <option key={campaign.id} value={campaign.id}>
                          {campaign.name}
                        </option>
                      ))}
                    </select>
                    <select className="h-11 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => updateBatchRow(setBatchRows, index, "platform", event.target.value)} value={row.platform}>
                      {adsMetricPlatformOptions.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                    <Input onChange={(event) => updateBatchRow(setBatchRows, index, "period_start", event.target.value)} type="date" value={row.period_start} />
                    <Input onChange={(event) => updateBatchRow(setBatchRows, index, "period_end", event.target.value)} type="date" value={row.period_end} />
                    <Button onClick={() => removeBatchRow(setBatchRows, index)} type="button" variant="ghost">
                      Remove
                    </Button>
                    <CurrencyInput onValueChange={(value) => updateBatchRow(setBatchRows, index, "amount_spent", value)} value={row.amount_spent} />
                    <CurrencyInput onValueChange={(value) => updateBatchRow(setBatchRows, index, "revenue", value)} value={row.revenue} />
                    <Input onChange={(event) => updateBatchRow(setBatchRows, index, "impressions", Number(event.target.value))} placeholder="Impressions" type="number" value={row.impressions} />
                    <Input onChange={(event) => updateBatchRow(setBatchRows, index, "clicks", Number(event.target.value))} placeholder="Clicks" type="number" value={row.clicks} />
                    <Input onChange={(event) => updateBatchRow(setBatchRows, index, "conversions", Number(event.target.value))} placeholder="Conversions" type="number" value={row.conversions} />
                    <Input className="lg:col-span-5" onChange={(event) => updateBatchRow(setBatchRows, index, "notes", event.target.value)} placeholder="Notes" value={row.notes} />
                  </div>
                ))}
              </div>
              <div className="mt-5 flex flex-wrap gap-3">
                <Button disabled={batchMutation.isPending} onClick={() => batchMutation.mutate(batchRows)} type="button">
                  Submit batch
                </Button>
                <Button
                  onClick={() => {
                    setBatchRows([{ ...defaultMetricForm }]);
                    setShowBatchForm(false);
                  }}
                  type="button"
                  variant="outline"
                >
                  Cancel
                </Button>
              </div>
            </Card>
          ) : null}

          <Card className="overflow-hidden">
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="bg-muted/60 text-left">
                  <tr>
                    <th className="px-4 py-3 font-semibold">Campaign</th>
                    <th className="px-4 py-3 font-semibold">Platform</th>
                    <th className="px-4 py-3 font-semibold">Period</th>
                    <th className="px-4 py-3 font-semibold">Spent</th>
                    <th className="px-4 py-3 font-semibold">Revenue</th>
                    <th className="px-4 py-3 font-semibold">CPR</th>
                    <th className="px-4 py-3 font-semibold">ROAS</th>
                    <th className="px-4 py-3 font-semibold">CTR</th>
                    <th className="px-4 py-3 font-semibold">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {metrics.map((item) => {
                    const platform = adsPlatformMeta(item.platform);
                    const PlatformIcon = platform.icon;
                    return (
                      <tr className="border-t border-border/70" key={item.id}>
                        <td className="px-4 py-3">
                          <div>
                            <p className="font-semibold">{item.campaign_name ?? "Unknown campaign"}</p>
                            <p className="text-xs text-muted-foreground">
                              {item.impressions.toLocaleString("id-ID")} impressions | {item.clicks.toLocaleString("id-ID")} clicks
                            </p>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <span className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold ${platform.badgeClassName}`}>
                            <PlatformIcon className="h-3.5 w-3.5" />
                            {platform.label}
                          </span>
                        </td>
                        <td className="px-4 py-3 text-muted-foreground">
                          {formatShortDate(item.period_start)} - {formatShortDate(item.period_end)}
                        </td>
                        <td className="px-4 py-3 font-semibold">{formatIDR(item.amount_spent)}</td>
                        <td className="px-4 py-3 font-semibold">{formatIDR(item.revenue)}</td>
                        <td className="px-4 py-3">{formatMetricCurrency(item.cpr)}</td>
                        <td className={`px-4 py-3 font-semibold ${metricTone(item.roas, "roas")}`}>{formatMetricRatio(item.roas)}</td>
                        <td className={`px-4 py-3 font-semibold ${metricTone(item.ctr, "ctr")}`}>{formatMetricPercent(item.ctr)}</td>
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap gap-2">
                            <PermissionGate permission={permissions.marketingAdsMetricsEdit}>
                              <Button
                                onClick={() => {
                                  setEditingMetric(item);
                                  setShowForm(true);
                                  setShowBatchForm(false);
                                  form.reset({
                                    campaign_id: item.campaign_id,
                                    platform: item.platform,
                                    period_start: item.period_start.slice(0, 10),
                                    period_end: item.period_end.slice(0, 10),
                                    amount_spent: item.amount_spent,
                                    impressions: item.impressions,
                                    clicks: item.clicks,
                                    conversions: item.conversions,
                                    revenue: item.revenue,
                                    notes: item.notes ?? "",
                                  });
                                }}
                                size="sm"
                                variant="outline"
                              >
                                Edit
                              </Button>
                            </PermissionGate>
                            <PermissionGate permission={permissions.marketingAdsMetricsDelete}>
                              <Button disabled={deleteMutation.isPending && deleteMutation.variables === item.id} onClick={() => deleteMutation.mutate(item.id)} size="sm" variant="ghost">
                                Delete
                              </Button>
                            </PermissionGate>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </Card>

          {metrics.length === 0 ? (
            <Card className="p-8 text-center text-sm text-muted-foreground">Belum ada ads metrics yang dicatat untuk filter ini.</Card>
          ) : null}

          <div className="flex items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">
              Showing page {metricsQuery.data?.meta.page ?? filters.page} of {Math.max(1, Math.ceil((metricsQuery.data?.meta.total ?? 0) / (metricsQuery.data?.meta.per_page ?? filters.perPage)))}
            </p>
            <div className="flex gap-2">
              <Button disabled={filters.page <= 1} onClick={() => setFilters((previous) => ({ ...previous, page: previous.page - 1 }))} variant="outline">
                Previous
              </Button>
              <Button disabled={(metricsQuery.data?.meta.total ?? 0) <= filters.page * filters.perPage} onClick={() => setFilters((previous) => ({ ...previous, page: previous.page + 1 }))} variant="outline">
                Next
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <div className="space-y-6">
          <Card className="p-6">
            <div className="grid gap-3 lg:grid-cols-4">
              <Input onChange={(event) => setDashboardRange((previous) => ({ ...previous, dateFrom: event.target.value }))} type="date" value={dashboardRange.dateFrom} />
              <Input onChange={(event) => setDashboardRange((previous) => ({ ...previous, dateTo: event.target.value }))} type="date" value={dashboardRange.dateTo} />
              <div className="flex flex-wrap gap-3 lg:col-span-2">
                <Button onClick={() => void handleExport()} variant="outline">
                  Export CSV
                </Button>
              </div>
            </div>
          </Card>

          <div className="grid gap-4 lg:grid-cols-4">
            <SummaryMetricCard label="Total spent" value={formatIDR(totals.spent)} />
            <SummaryMetricCard label="Total revenue" value={formatIDR(totals.revenue)} />
            <SummaryMetricCard label="Overall ROAS" toneClass={metricTone(overallROAS, "roas")} value={formatMetricRatio(overallROAS)} />
            <SummaryMetricCard label="Overall CTR" toneClass={metricTone(overallCTR, "ctr")} value={formatMetricPercent(overallCTR)} />
          </div>

          <div className="grid gap-6 xl:grid-cols-[2fr,1fr]">
            <Card className="p-6">
              <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Campaign comparison</p>
              <h4 className="mt-2 text-2xl font-bold">Spent vs revenue per campaign</h4>
              <div className="mt-6 h-[320px]">
                <ResponsiveContainer height="100%" width="100%">
                  <BarChart data={campaignRows.slice(0, 8)}>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} />
                    <XAxis dataKey="group_label" tick={{ fontSize: 12 }} />
                    <YAxis tickFormatter={(value) => `${Math.round(Number(value) / 1000000)} jt`} />
                    <Tooltip formatter={(value: number) => formatIDR(value)} />
                    <Bar dataKey="total_spent" fill="#2563eb" radius={[8, 8, 0, 0]} />
                    <Bar dataKey="total_revenue" fill="#16a34a" radius={[8, 8, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </Card>

            <Card className="p-6">
              <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Platform mix</p>
              <h4 className="mt-2 text-2xl font-bold">Spent breakdown</h4>
              <div className="mt-6 h-[260px]">
                <ResponsiveContainer height="100%" width="100%">
                  <PieChart>
                    <Pie cx="50%" cy="50%" data={platformRows} dataKey="total_spent" innerRadius={55} outerRadius={90} paddingAngle={3}>
                      {platformRows.map((row, index) => (
                        <Cell fill={summaryColors[index % summaryColors.length]} key={row.group_key} />
                      ))}
                    </Pie>
                    <Tooltip formatter={(value: number) => formatIDR(value)} />
                  </PieChart>
                </ResponsiveContainer>
              </div>
              <div className="space-y-2">
                {platformRows.map((row, index) => (
                  <div className="flex items-center justify-between gap-3 rounded-[18px] border border-border/70 bg-background/70 px-4 py-3" key={row.group_key}>
                    <div className="flex items-center gap-3">
                      <span className="h-3 w-3 rounded-full" style={{ backgroundColor: summaryColors[index % summaryColors.length] }} />
                      <span className="text-sm font-medium">{row.group_label}</span>
                    </div>
                    <span className="text-sm text-muted-foreground">{formatIDR(row.total_spent)}</span>
                  </div>
                ))}
              </div>
            </Card>
          </div>

          <div className="grid gap-6 xl:grid-cols-[2fr,1fr]">
            <Card className="p-6">
              <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Trendline</p>
              <h4 className="mt-2 text-2xl font-bold">Monthly CTR and ROAS</h4>
              <div className="mt-6 h-[320px]">
                <ResponsiveContainer height="100%" width="100%">
                  <LineChart data={monthlyRows}>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} />
                    <XAxis dataKey="group_label" />
                    <YAxis tickFormatter={(value) => `${Number(value).toFixed(1)}`} yAxisId="left" />
                    <YAxis orientation="right" tickFormatter={(value) => `${Number(value).toFixed(1)}%`} yAxisId="right" />
                    <Tooltip formatter={(value: number, name: string) => (name === "ctr" ? `${Number(value).toFixed(2)}%` : Number(value).toFixed(2))} />
                    <Line dataKey="roas" name="roas" stroke="#16a34a" strokeWidth={3} type="monotone" yAxisId="left" />
                    <Line dataKey="ctr" name="ctr" stroke="#ea580c" strokeWidth={3} type="monotone" yAxisId="right" />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </Card>

            <Card className="p-6">
              <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Ranking</p>
              <h4 className="mt-2 text-2xl font-bold">Best ROAS campaigns</h4>
              <div className="mt-5 space-y-3">
                {campaignRows.slice(0, 8).map((row) => (
                  <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={row.group_key}>
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <p className="font-semibold">{row.group_label}</p>
                        <p className="mt-1 text-xs text-muted-foreground">{formatIDR(row.total_spent)} spent | {formatIDR(row.total_revenue)} revenue</p>
                      </div>
                      <div className={`text-right text-sm font-semibold ${metricTone(row.roas, "roas")}`}>{formatMetricRatio(row.roas)}</div>
                    </div>
                  </div>
                ))}
              </div>
            </Card>
          </div>
        </div>
      )}

      {!hasPermission(permissions.marketingAdsMetricsCreate) ? (
        <Card className="p-5 text-sm text-muted-foreground">
          Akun ini hanya punya akses view untuk ads metrics. Form input dan aksi edit otomatis disembunyikan oleh permission gate.
        </Card>
      ) : null}
    </div>
  );
}

function SummaryMetricCard({ label, value, toneClass }: { label: string; value: string; toneClass?: string }) {
  return (
    <Card className="p-6">
      <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <h4 className={`mt-3 text-2xl font-bold ${toneClass ?? ""}`}>{value}</h4>
    </Card>
  );
}

function resetMetricForm(form: ReturnType<typeof useForm<AdsMetricFormValues>>) {
  form.reset({ ...defaultMetricForm });
}

async function invalidateAdsMetrics(queryClient: ReturnType<typeof useQueryClient>) {
  await queryClient.invalidateQueries({ queryKey: adsMetricsKeys.all });
}

function formatShortDate(value: string) {
  return new Date(value).toLocaleDateString("id-ID");
}

function formatMetricCurrency(value?: number | null) {
  if (value === undefined || value === null) {
    return "-";
  }
  return formatIDR(Math.round(value));
}

function formatMetricRatio(value?: number | null) {
  if (value === undefined || value === null) {
    return "-";
  }
  return `${value.toFixed(2)}x`;
}

function formatMetricPercent(value?: number | null) {
  if (value === undefined || value === null) {
    return "-";
  }
  return `${value.toFixed(2)}%`;
}

function metricTone(value: number | null | undefined, metric: "roas" | "ctr") {
  if (value === undefined || value === null) {
    return "text-muted-foreground";
  }

  if (metric === "roas") {
    if (value > 3) {
      return "text-emerald-700";
    }
    if (value >= 1) {
      return "text-amber-700";
    }
    return "text-red-700";
  }

  if (value > 2) {
    return "text-emerald-700";
  }
  if (value >= 0.5) {
    return "text-amber-700";
  }
  return "text-red-700";
}

function updateBatchRow(
  setRows: Dispatch<SetStateAction<AdsMetricFormValues[]>>,
  index: number,
  field: keyof AdsMetricFormValues,
  value: string | number,
) {
  setRows((previous) =>
    previous.map((row, rowIndex) =>
      rowIndex === index ? { ...row, [field]: value } : row,
    ),
  );
}

function removeBatchRow(setRows: Dispatch<SetStateAction<AdsMetricFormValues[]>>, index: number) {
  setRows((previous) => {
    if (previous.length === 1) {
      return [{ ...defaultMetricForm }];
    }
    return previous.filter((_, rowIndex) => rowIndex !== index);
  });
}

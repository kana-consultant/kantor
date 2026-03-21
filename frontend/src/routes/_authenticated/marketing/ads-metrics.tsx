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
import { BarChart3, CircleDollarSign, Plus, Ratio, TrendingUp } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { ExportButton } from "@/components/shared/export-button";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { StatCard } from "@/components/shared/stat-card";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { formatIDR } from "@/lib/currency";
import { adsMetricPlatformOptions, adsPlatformMeta } from "@/lib/marketing";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  adsMetricsKeys,
  batchCreateAdsMetrics,
  createAdsMetric,
  deleteAdsMetric,
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

const summaryColors = ["#FF5630", "#36B37E", "#0065FF", "#6554C0", "#FF8B00", "#00B8D9"];

export const Route = createFileRoute("/_authenticated/marketing/ads-metrics")({
  beforeLoad: async () => {
    await ensureModuleAccess("marketing");
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
  const [metricToDelete, setMetricToDelete] = useState<AdsMetric | null>(null);
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
      setMetricToDelete(null);
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
  const meta = metricsQuery.data?.meta;

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

  const metricColumns: Array<DataTableColumn<AdsMetric>> = [
    {
      id: "campaign",
      header: "Campaign",
      accessor: "campaign_name",
      sortable: true,
      cell: (item) => (
        <div className="space-y-1">
          <p className="font-semibold text-text-primary">{item.campaign_name ?? "Unknown campaign"}</p>
          <p className="text-[13px] text-text-secondary">
            {item.impressions.toLocaleString("id-ID")} impressions | {item.clicks.toLocaleString("id-ID")} clicks
          </p>
        </div>
      ),
    },
    {
      id: "platform",
      header: "Platform",
      accessor: "platform",
      sortable: true,
      cell: (item) => {
        const platform = adsPlatformMeta(item.platform);
        const PlatformIcon = platform.icon;
        return (
          <span className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold ${platform.badgeClassName}`}>
            <PlatformIcon className="h-3.5 w-3.5" />
            {platform.label}
          </span>
        );
      },
    },
    {
      id: "period",
      header: "Period",
      accessor: "period_start",
      sortable: true,
      cell: (item) => (
        <span className="text-sm text-text-secondary">
          {formatShortDate(item.period_start)} - {formatShortDate(item.period_end)}
        </span>
      ),
    },
    {
      id: "spent",
      header: "Spent",
      accessor: "amount_spent",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (item) => <span className="font-mono tabular-nums">{formatIDR(item.amount_spent)}</span>,
    },
    {
      id: "revenue",
      header: "Revenue",
      accessor: "revenue",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (item) => <span className="font-mono tabular-nums">{formatIDR(item.revenue)}</span>,
    },
    {
      id: "cpr",
      header: "CPR",
      accessor: "cpr",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (item) => <span className="font-mono tabular-nums">{formatMetricCurrency(item.cpr)}</span>,
    },
    {
      id: "roas",
      header: "ROAS",
      accessor: "roas",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (item) => (
        <span className={`font-mono tabular-nums ${metricTone(item.roas, "roas")}`}>{formatMetricRatio(item.roas)}</span>
      ),
    },
    {
      id: "ctr",
      header: "CTR",
      accessor: "ctr",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (item) => (
        <span className={`font-mono tabular-nums ${metricTone(item.ctr, "ctr")}`}>{formatMetricPercent(item.ctr)}</span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (item) => (
        <div className="flex justify-end gap-2">
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
              type="button"
              variant="outline"
            >
              Edit
            </Button>
          </PermissionGate>
          <PermissionGate permission={permissions.marketingAdsMetricsDelete}>
            <Button
              disabled={deleteMutation.isPending}
              onClick={() => setMetricToDelete(item)}
              size="sm"
              type="button"
              variant="ghost"
            >
              Delete
            </Button>
          </PermissionGate>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 border-b border-border pb-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-mkt">
              Marketing analytics
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">Ads spent and performance metrics</h3>
            <p className="mt-2 max-w-3xl text-[14px] leading-relaxed text-text-secondary">
              Capture paid media performance, compare ROAS across campaigns, and export clean reporting data.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Button onClick={() => setActiveTab("input")} variant={activeTab === "input" ? undefined : "outline"}>
              Input data
            </Button>
            <Button onClick={() => setActiveTab("dashboard")} variant={activeTab === "dashboard" ? undefined : "outline"}>
              Dashboard
            </Button>
            <PermissionGate permission={permissions.marketingAdsMetricsView}>
              <ExportButton
                endpoint="/marketing/ads-metrics/export"
                filename="ads-metrics-report"
                filters={
                  activeTab === "input"
                    ? {
                        campaign_id: filters.campaignId,
                        date_from: filters.dateFrom,
                        date_to: filters.dateTo,
                        platform: filters.platform,
                      }
                    : {
                        date_from: dashboardRange.dateFrom,
                        date_to: dashboardRange.dateTo,
                      }
                }
                formats={["csv", "pdf", "xlsx"]}
              />
            </PermissionGate>
          </div>
        </div>
      </Card>

      {activeTab === "input" ? (
        <div className="space-y-6">
          <Card className="p-6">
            <div className="grid gap-3 lg:grid-cols-5">
              <select
                className="field-select"
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
                className="field-select"
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
                type="button"
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
                    setShowForm(true);
                    setShowBatchForm(false);
                  }}
                  type="button"
                >
                  <Plus className="h-4 w-4" />
                  New entry
                </Button>
                <Button
                  onClick={() => {
                    setShowBatchForm(true);
                    setShowForm(false);
                    setEditingMetric(null);
                    resetMetricForm(form);
                  }}
                  type="button"
                  variant="outline"
                >
                  Bulk input
                </Button>
              </PermissionGate>
            </div>
          </Card>

          <DataTable
            columns={metricColumns}
            data={metrics}
            emptyDescription="No ads metrics have been recorded for the current filter."
            emptyTitle="No ads metrics found"
            getRowId={(item) => item.id}
            loading={metricsQuery.isLoading}
            loadingRows={6}
            pagination={
              meta
                ? {
                    page: meta.page,
                    perPage: meta.per_page,
                    total: meta.total,
                    onPageChange: (page) => setFilters((previous) => ({ ...previous, page })),
                  }
                : undefined
            }
          />

          <FormModal
            isLoading={createMutation.isPending || updateMutation.isPending}
            isOpen={showForm}
            onClose={() => {
              setEditingMetric(null);
              resetMetricForm(form);
              setShowForm(false);
            }}
            onSubmit={handleSubmitMetric}
            size="lg"
            submitLabel={editingMetric ? "Save metric" : "Save entry"}
            title={editingMetric ? "Edit ads metric" : "Add ads metric"}
            subtitle="Capture platform performance for a single campaign entry without shifting the metrics table below."
          >
            <div className="grid gap-4 lg:grid-cols-2">
              <select className="field-select" {...form.register("campaign_id")}>
                <option value="">Select campaign</option>
                {campaigns.map((campaign) => (
                  <option key={campaign.id} value={campaign.id}>
                    {campaign.name}
                  </option>
                ))}
              </select>
              <select className="field-select" {...form.register("platform")}>
                {adsMetricPlatformOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              <Input {...form.register("period_start")} type="date" />
              <Input {...form.register("period_end")} type="date" />
              <Controller
                control={form.control}
                name="amount_spent"
                render={({ field }) => <CurrencyInput onValueChange={field.onChange} value={field.value} />}
              />
              <Controller
                control={form.control}
                name="revenue"
                render={({ field }) => <CurrencyInput onValueChange={field.onChange} value={field.value} />}
              />
              <Input {...form.register("impressions", { valueAsNumber: true })} min={0} placeholder="Impressions" type="number" />
              <Input {...form.register("clicks", { valueAsNumber: true })} min={0} placeholder="Clicks" type="number" />
              <Input {...form.register("conversions", { valueAsNumber: true })} min={0} placeholder="Conversions" type="number" />
              <Input className="lg:col-span-2" {...form.register("notes")} placeholder="Notes" />
              {form.formState.errors.period_end ? (
                <p className="text-sm text-error lg:col-span-2">{form.formState.errors.period_end.message}</p>
              ) : null}
            </div>
          </FormModal>

          <FormModal
            isLoading={batchMutation.isPending}
            isOpen={showBatchForm}
            onClose={() => {
              setBatchRows([{ ...defaultMetricForm }]);
              setShowBatchForm(false);
            }}
            onSubmit={(event) => {
              event.preventDefault();
              batchMutation.mutate(batchRows);
            }}
            size="xl"
            submitLabel="Submit batch"
            title="Bulk ads metrics input"
            subtitle="Enter multiple metric rows in one pass without pushing the list view down."
          >
            <div className="flex justify-end">
              <Button onClick={() => setBatchRows((previous) => [...previous, { ...defaultMetricForm }])} type="button" variant="outline">
                Add row
              </Button>
            </div>
            <div className="space-y-4">
              {batchRows.map((row, index) => (
                <div className="grid gap-3 rounded-md border border-border bg-surface-muted p-4 lg:grid-cols-5" key={`${index}-${row.campaign_id}-${row.platform}`}>
                  <select className="field-select" onChange={(event) => updateBatchRow(setBatchRows, index, "campaign_id", event.target.value)} value={row.campaign_id}>
                    <option value="">Select campaign</option>
                    {campaigns.map((campaign) => (
                      <option key={campaign.id} value={campaign.id}>
                        {campaign.name}
                      </option>
                    ))}
                  </select>
                  <select className="field-select" onChange={(event) => updateBatchRow(setBatchRows, index, "platform", event.target.value)} value={row.platform}>
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
          </FormModal>

          <ConfirmDialog
            confirmLabel="Delete metric"
            description={metricToDelete ? `The metric entry for "${metricToDelete.campaign_name ?? "Unknown campaign"}" will be removed.` : ""}
            isLoading={deleteMutation.isPending}
            isOpen={Boolean(metricToDelete)}
            onClose={() => setMetricToDelete(null)}
            onConfirm={() => {
              if (metricToDelete) {
                deleteMutation.mutate(metricToDelete.id);
              }
            }}
            title={metricToDelete ? "Delete ads metric?" : "Delete ads metric?"}
          />
        </div>
      ) : (
        <div className="space-y-6">
          <Card className="p-6">
            <div className="grid gap-3 lg:grid-cols-4">
              <Input onChange={(event) => setDashboardRange((previous) => ({ ...previous, dateFrom: event.target.value }))} type="date" value={dashboardRange.dateFrom} />
              <Input onChange={(event) => setDashboardRange((previous) => ({ ...previous, dateTo: event.target.value }))} type="date" value={dashboardRange.dateTo} />
              <div className="flex flex-wrap gap-3 lg:col-span-2">
              </div>
            </div>
          </Card>

          <div className="grid gap-4 lg:grid-cols-4">
            <StatCard
              helper="Spend across the selected period"
              icon={CircleDollarSign}
              label="Total spent"
              mono
              tone="mkt"
              value={formatIDR(totals.spent)}
            />
            <StatCard
              helper="Revenue attributed to paid traffic"
              icon={TrendingUp}
              label="Total revenue"
              mono
              tone="success"
              value={formatIDR(totals.revenue)}
            />
            <StatCard
              helper="Return on ad spend"
              icon={Ratio}
              label="Overall ROAS"
              tone={metricTone(overallROAS, "roas").includes("success") ? "success" : metricTone(overallROAS, "roas").includes("warning") ? "warning" : "error"}
              value={formatMetricRatio(overallROAS)}
            />
            <StatCard
              helper="Click-through rate"
              icon={BarChart3}
              label="Overall CTR"
              tone={metricTone(overallCTR, "ctr").includes("success") ? "success" : metricTone(overallCTR, "ctr").includes("warning") ? "warning" : "error"}
              value={formatMetricPercent(overallCTR)}
            />
          </div>

          <div className="grid gap-6 xl:grid-cols-[2fr,1fr]">
            <Card className="p-6">
              <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
                Campaign comparison
              </p>
              <h4 className="text-[20px] font-[700] text-text-primary">Spent vs revenue per campaign</h4>
              <div className="mt-6 h-[320px]">
                <ResponsiveContainer height="100%" minHeight={240} minWidth={1} width="100%">
                  <BarChart data={campaignRows.slice(0, 8)}>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} />
                    <XAxis dataKey="group_label" tick={{ fontSize: 12 }} />
                    <YAxis tickFormatter={(value) => `${Math.round(Number(value) / 1000000)} jt`} />
                    <Tooltip formatter={(value) => formatIDR(Number(value))} />
                    <Bar dataKey="total_spent" fill="#FF5630" radius={[4, 4, 0, 0]} />
                    <Bar dataKey="total_revenue" fill="#36B37E" radius={[4, 4, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </Card>

            <Card className="p-6">
              <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
                Platform mix
              </p>
              <h4 className="text-[20px] font-[700] text-text-primary">Spent breakdown</h4>
              <div className="mt-6 h-[260px]">
                <ResponsiveContainer height="100%" minHeight={240} minWidth={1} width="100%">
                  <PieChart>
                    <Pie cx="50%" cy="50%" data={platformRows} dataKey="total_spent" innerRadius={55} outerRadius={90} paddingAngle={3}>
                      {platformRows.map((row, index) => (
                        <Cell fill={summaryColors[index % summaryColors.length]} key={row.group_key} />
                      ))}
                    </Pie>
                    <Tooltip formatter={(value) => formatIDR(Number(value))} />
                  </PieChart>
                </ResponsiveContainer>
              </div>
              <div className="space-y-2">
                {platformRows.map((row, index) => (
                  <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface-muted px-4 py-3" key={row.group_key}>
                    <div className="flex items-center gap-3">
                      <span className="h-3 w-3 rounded-full" style={{ backgroundColor: summaryColors[index % summaryColors.length] }} />
                      <span className="text-sm font-medium text-text-primary">{row.group_label}</span>
                    </div>
                    <span className="font-mono text-sm tabular-nums text-text-secondary">{formatIDR(row.total_spent)}</span>
                  </div>
                ))}
              </div>
            </Card>
          </div>

          <div className="grid gap-6 xl:grid-cols-[2fr,1fr]">
            <Card className="p-6">
              <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
                Trendline
              </p>
              <h4 className="text-[20px] font-[700] text-text-primary">Monthly CTR and ROAS</h4>
              <div className="mt-6 h-[320px]">
                <ResponsiveContainer height="100%" minHeight={240} minWidth={1} width="100%">
                  <LineChart data={monthlyRows}>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} />
                    <XAxis dataKey="group_label" />
                    <YAxis tickFormatter={(value) => `${Number(value).toFixed(1)}`} yAxisId="left" />
                    <YAxis orientation="right" tickFormatter={(value) => `${Number(value).toFixed(1)}%`} yAxisId="right" />
                    <Tooltip formatter={(value, name) => (name === "ctr" ? `${Number(value).toFixed(2)}%` : Number(value).toFixed(2))} />
                    <Line dataKey="roas" name="roas" stroke="#36B37E" strokeWidth={2} type="monotone" yAxisId="left" />
                    <Line dataKey="ctr" name="ctr" stroke="#FF5630" strokeWidth={2} type="monotone" yAxisId="right" />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </Card>

            <Card className="p-6">
              <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
                Ranking
              </p>
              <h4 className="text-[20px] font-[700] text-text-primary">Best ROAS campaigns</h4>
              <div className="mt-5 space-y-3">
                {campaignRows.slice(0, 8).map((row) => (
                  <div className="rounded-md border border-border bg-surface-muted p-4" key={row.group_key}>
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <p className="font-semibold text-text-primary">{row.group_label}</p>
                        <p className="mt-1 text-xs text-text-secondary">
                          {formatIDR(row.total_spent)} spent | {formatIDR(row.total_revenue)} revenue
                        </p>
                      </div>
                      <div className={`text-right font-mono text-sm tabular-nums ${metricTone(row.roas, "roas")}`}>
                        {formatMetricRatio(row.roas)}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </Card>
          </div>
        </div>
      )}

      {!hasPermission(permissions.marketingAdsMetricsCreate) ? (
        <Card className="p-5 text-sm text-text-secondary">
          This account has view-only access to ads metrics.
        </Card>
      ) : null}
    </div>
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
    return "text-text-tertiary";
  }

  if (metric === "roas") {
    if (value > 3) {
      return "text-success";
    }
    if (value >= 1) {
      return "text-warning";
    }
    return "text-error";
  }

  if (value > 2) {
    return "text-success";
  }
  if (value >= 0.5) {
    return "text-warning";
  }
  return "text-error";
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

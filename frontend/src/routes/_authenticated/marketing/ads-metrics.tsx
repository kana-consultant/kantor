import type { Dispatch, SetStateAction } from "react";
import { useEffect, useState } from "react";

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
import { BarChart3, CircleDollarSign, Info, Plus, Ratio, TrendingUp } from "lucide-react";

import { ChannelBadge, PlatformMismatchNote } from "@/components/shared/channel-badge";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { ExportButton } from "@/components/shared/export-button";
import { FieldError, RequiredMark, formLabelClassName, formSubLabelClassName } from "@/components/shared/form-field";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { StatCard } from "@/components/shared/stat-card";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { formatIDR } from "@/lib/currency";
import { extractDateInputValue, formatCalendarDate, formatDateInputValue } from "@/lib/date";
import {
  adsMetricPlatformOptions,
  adsMetricPlatforms,
  adsPlatformMeta,
  channelMeta,
  defaultPlatformForChannel,
  formatMetricPercent,
  formatMetricRatio,
  isAdsMetricPlatform,
  marketingErrorMessage,
  platformMismatchMessage,
} from "@/lib/marketing";
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
import { campaignsKeys, listAllCampaigns } from "@/services/marketing-campaigns";
import { toast } from "@/stores/toast-store";
import type { AdsMetric, AdsMetricFilters, AdsMetricFormValues } from "@/types/marketing";

const metricSchema = z
  .object({
    campaign_id: z.string().min(1, "Campaign wajib dipilih"),
    platform: z.enum(adsMetricPlatforms, { message: "Pilih platform" }),
    period_start: z.string().min(1, "Tanggal mulai wajib diisi"),
    period_end: z.string().min(1, "Tanggal selesai wajib diisi"),
    amount_spent: z.number({ message: "Isi angka, 0 jika belum ada" }).min(0, "Belanja minimal 0"),
    impressions: z.number({ message: "Isi angka, 0 jika belum ada" }).min(0, "Impresi minimal 0"),
    clicks: z.number({ message: "Isi angka, 0 jika belum ada" }).min(0, "Klik minimal 0"),
    conversions: z.number({ message: "Isi angka, 0 jika belum ada" }).min(0, "Konversi minimal 0"),
    revenue: z.number({ message: "Isi angka, 0 jika belum ada" }).min(0, "Pendapatan minimal 0"),
    notes: z.string(),
  })
  .refine((value) => value.period_end >= value.period_start, {
    message: "Tanggal selesai tidak boleh sebelum tanggal mulai",
    path: ["period_end"],
  });

const defaultMetricForm: AdsMetricFormValues = {
  campaign_id: "",
  platform: "meta_ads",
  period_start: formatDateInputValue(),
  period_end: formatDateInputValue(),
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

// ?campaign=<id> is the table's campaign filter, kept in the URL so a link to
// the plain page (the sidebar) clears it and a reload keeps it. A campaign's
// "Catat metrik" adds &new=true, which opens the new-metric form for it once.
const searchSchema = z.object({
  campaign: z.string().optional().catch(undefined),
  new: z.boolean().optional().catch(undefined),
});

export const Route = createFileRoute("/_authenticated/marketing/ads-metrics")({
  validateSearch: searchSchema,
  beforeLoad: async () => {
    await ensureModuleAccess("marketing");
    await ensurePermission(permissions.marketingAdsMetricsView);
  },
  component: AdsMetricsPage,
});

function AdsMetricsPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const [activeTab, setActiveTab] = useState<"input" | "dashboard">("input");
  const [filters, setFilters] = useState<AdsMetricFilters>(() => ({
    ...defaultFilters,
    campaignId: search.campaign ?? "",
  }));
  const [dashboardRange, setDashboardRange] = useState({
    dateFrom: `${new Date().getFullYear()}-01-01`,
    dateTo: formatDateInputValue(),
  });
  const [showForm, setShowForm] = useState(false);
  const [showBatchForm, setShowBatchForm] = useState(false);
  const [editingMetric, setEditingMetric] = useState<AdsMetric | null>(null);
  const [metricToDelete, setMetricToDelete] = useState<AdsMetric | null>(null);
  const [batchRows, setBatchRows] = useState<AdsMetricFormValues[]>([{ ...defaultMetricForm }]);
  // Whether the user picked the platform themselves. Until then, choosing a
  // campaign sets the platform from its channel; after that nothing the user
  // chose is overwritten. Editing starts touched, so a saved value stays.
  const [platformTouched, setPlatformTouched] = useState(false);
  const [batchPlatformTouched, setBatchPlatformTouched] = useState<boolean[]>([false]);

  // Every campaign (all pages): the selects must hold any campaign a metric
  // or a "Catat metrik" link can point at, not only the first 100.
  const campaignsQuery = useQuery({
    queryKey: campaignsKeys.allOptions(),
    queryFn: listAllCampaigns,
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

  // Form submits show their error in the dialog (FormModal banner) and keep
  // the values; actions without a form use a toast.
  const createMutation = useMutation({
    mutationFn: createAdsMetric,
    onSuccess: async () => {
      resetSingleForm();
      setShowForm(false);
      toast.success("Metrik iklan disimpan");
      await invalidateAdsMetrics(queryClient);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (payload: { metricId: string; values: AdsMetricFormValues }) =>
      updateAdsMetric(payload.metricId, payload.values),
    onSuccess: async () => {
      resetSingleForm();
      setEditingMetric(null);
      setShowForm(false);
      toast.success("Perubahan metrik disimpan");
      await invalidateAdsMetrics(queryClient);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteAdsMetric,
    onSuccess: async () => {
      setMetricToDelete(null);
      toast.success("Metrik iklan dihapus");
      await invalidateAdsMetrics(queryClient);
    },
    onError: (error) => {
      // Close the confirm dialog first: the toast layer sits under the
      // dialog's backdrop and could not be read while it is open.
      setMetricToDelete(null);
      toast.error("Gagal menghapus metrik iklan", marketingErrorMessage(error));
    },
  });

  const batchMutation = useMutation({
    mutationFn: batchCreateAdsMetrics,
    onSuccess: async (_, rows) => {
      resetBatchRows();
      setShowBatchForm(false);
      toast.success("Metrik iklan disimpan", `${rows.length} baris`);
      await invalidateAdsMetrics(queryClient);
    },
  });

  const campaigns = campaignsQuery.data ?? [];
  const channelOfCampaign = (campaignId: string) =>
    campaigns.find((campaign) => campaign.id === campaignId)?.channel ?? null;
  // The platform a campaign starts with: its channel when that is an ads
  // platform (Meta Ads -> Meta Ads), otherwise "Lainnya" (e.g. Email).
  const platformForCampaign = (campaignId: string) => {
    const channel = channelOfCampaign(campaignId);
    return channel ? defaultPlatformForChannel(channel) : null;
  };
  // Info hint (never blocking) when the platform does not fit the campaign's
  // channel; a multi-platform campaign can legitimately differ.
  const mismatchFor = (campaignId: string, platform: string) => {
    const channel = channelOfCampaign(campaignId);
    return channel ? platformMismatchMessage(channel, platform) : null;
  };
  const watchedCampaignId = form.watch("campaign_id");
  const watchedPlatform = form.watch("platform");
  const singleMismatch = mismatchFor(watchedCampaignId, watchedPlatform);
  const singleFormError = editingMetric ? updateMutation.error : createMutation.error;
  function resetSingleForm() {
    resetMetricForm(form);
    setPlatformTouched(false);
  }
  function resetBatchRows() {
    setBatchRows([{ ...defaultMetricForm }]);
    setBatchPlatformTouched([false]);
  }
  const closeSingleForm = () => {
    setEditingMetric(null);
    resetSingleForm();
    setShowForm(false);
    createMutation.reset();
    updateMutation.reset();
  };

  // The campaign filter follows the URL (see searchSchema): opening the
  // plain page, going back or following a link updates it.
  const linkedCampaignId = search.campaign ?? "";
  useEffect(() => {
    setFilters((previous) =>
      previous.campaignId === linkedCampaignId ? previous : { ...previous, campaignId: linkedCampaignId, page: 1 },
    );
  }, [linkedCampaignId]);
  const setCampaignFilter = (campaignId: string) => {
    setFilters((previous) => ({ ...previous, campaignId, page: 1 }));
    void navigate({ replace: true, search: (previous) => ({ ...previous, campaign: campaignId || undefined }) });
  };

  // Arriving from a campaign ("Catat metrik", &new=true): open the form for
  // it, then drop `new` so a reload does not open the form again.
  const openNewForm = search.new === true;
  const canCreateMetric = hasPermission(permissions.marketingAdsMetricsCreate);
  // Wait for the campaign list: the native select can only show the
  // campaign once its option exists.
  const campaignsReady = campaignsQuery.isFetched;
  useEffect(() => {
    if (!openNewForm || !campaignsReady) {
      return;
    }
    setActiveTab("input");
    if (canCreateMetric && linkedCampaignId) {
      setEditingMetric(null);
      setPlatformTouched(false);
      form.reset({ ...defaultMetricForm, campaign_id: linkedCampaignId });
      setShowBatchForm(false);
      setShowForm(true);
    }
    void navigate({ replace: true, search: (previous) => ({ ...previous, new: undefined }) });
  }, [campaignsReady, canCreateMetric, form, linkedCampaignId, navigate, openNewForm]);

  // A form opened with a campaign already set gets that campaign's platform
  // as soon as the campaign list has loaded (unless the user picked one).
  const autoPlatform = showForm && !editingMetric && !platformTouched ? platformForCampaign(watchedCampaignId) : null;
  useEffect(() => {
    if (autoPlatform && form.getValues("platform") !== autoPlatform) {
      form.setValue("platform", autoPlatform);
    }
  }, [autoPlatform, form]);
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

  const errors = form.formState.errors;
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
          <p className="font-semibold text-text-primary">{item.campaign_name ?? "Campaign tidak ditemukan"}</p>
          <p className="text-[13px] text-text-secondary">
            {item.impressions.toLocaleString("id-ID")} impresi · {item.clicks.toLocaleString("id-ID")} klik
          </p>
        </div>
      ),
    },
    {
      id: "platform",
      header: "Platform",
      accessor: "platform",
      sortable: true,
      // Same note as the campaign drawer when the platform does not fit the
      // campaign's channel (looked up in the loaded campaign list).
      cell: (item) => (
        <div>
          <ChannelBadge channel={item.platform} kind="platform" />
          <PlatformMismatchNote channel={channelOfCampaign(item.campaign_id)} platform={item.platform} />
        </div>
      ),
    },
    {
      id: "period",
      header: "Periode",
      accessor: "period_start",
      sortable: true,
      // The range may wrap between the two dates (as it always did, so the
      // table keeps fitting at 1440), but never inside a date.
      cell: (item) => (
        <span className="text-sm text-text-secondary">
          <span className="whitespace-nowrap">{formatShortDate(item.period_start)} -</span>{" "}
          <span className="whitespace-nowrap">{formatShortDate(item.period_end)}</span>
        </span>
      ),
    },
    {
      id: "spent",
      header: "Belanja",
      accessor: "amount_spent",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (item) => <span className="font-mono tabular-nums">{formatIDR(item.amount_spent)}</span>,
    },
    {
      id: "revenue",
      header: "Pendapatan",
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
      header: "Aksi",
      align: "right",
      cell: (item) => (
        <div className="flex justify-end gap-2">
          <PermissionGate permission={permissions.marketingAdsMetricsEdit}>
            <Button
              onClick={() => {
                setEditingMetric(item);
                setPlatformTouched(true);
                createMutation.reset();
                updateMutation.reset();
                setShowForm(true);
                setShowBatchForm(false);
                form.reset({
                  campaign_id: item.campaign_id,
                  platform: item.platform,
                  period_start: extractDateInputValue(item.period_start),
                  period_end: extractDateInputValue(item.period_end),
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
              Hapus
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
              Analitik marketing
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">Metrik iklan</h3>
            <p className="mt-2 max-w-3xl text-[14px] leading-relaxed text-text-secondary">
              Catat belanja dan hasil iklan, bandingkan ROAS antar campaign, lalu ekspor laporannya.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Button onClick={() => setActiveTab("input")} variant={activeTab === "input" ? undefined : "outline"}>
              Input
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
                aria-label="Filter campaign"
                className="field-select w-full min-w-0"
                onChange={(event) => setCampaignFilter(event.target.value)}
                value={filters.campaignId}
              >
                <option value="">Semua campaign</option>
                {filters.campaignId && campaignsQuery.isSuccess && !campaigns.some((campaign) => campaign.id === filters.campaignId) ? (
                  // A link to a campaign that no longer exists: say so rather
                  // than show "Semua campaign" over a filtered table.
                  <option value={filters.campaignId}>Campaign tidak ditemukan</option>
                ) : null}
                {campaigns.map((campaign) => (
                  <option key={campaign.id} value={campaign.id}>
                    {campaignOptionLabel(campaign)}
                  </option>
                ))}
              </select>
              <select
                aria-label="Filter platform"
                className="field-select w-full min-w-0"
                onChange={(event) => setFilters((previous) => ({ ...previous, platform: event.target.value, page: 1 }))}
                value={filters.platform}
              >
                <option value="">Semua platform</option>
                {adsMetricPlatformOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              <Input aria-label="Dari tanggal" onChange={(event) => setFilters((previous) => ({ ...previous, dateFrom: event.target.value, page: 1 }))} type="date" value={filters.dateFrom} />
              <Input aria-label="Sampai tanggal" onChange={(event) => setFilters((previous) => ({ ...previous, dateTo: event.target.value, page: 1 }))} type="date" value={filters.dateTo} />
              <Button
                onClick={() => {
                  setFilters((previous) => ({ ...previous, platform: "", dateFrom: "", dateTo: "", page: 1 }));
                  setCampaignFilter("");
                }}
                type="button"
                variant="outline"
              >
                Reset filter
              </Button>
            </div>
            <div className="mt-4 flex flex-wrap gap-3">
              <PermissionGate permission={permissions.marketingAdsMetricsCreate}>
                <Button
                  onClick={() => {
                    setEditingMetric(null);
                    resetSingleForm();
                    createMutation.reset();
                    updateMutation.reset();
                    setShowForm(true);
                    setShowBatchForm(false);
                  }}
                  type="button"
                >
                  <Plus className="h-4 w-4" />
                  Metrik baru
                </Button>
                <Button
                  onClick={() => {
                    batchMutation.reset();
                    setShowBatchForm(true);
                    setShowForm(false);
                    setEditingMetric(null);
                    resetSingleForm();
                  }}
                  type="button"
                  variant="outline"
                >
                  Input massal
                </Button>
              </PermissionGate>
            </div>
          </Card>

          <DataTable
            columns={metricColumns}
            data={metrics}
            emptyDescription="Belum ada metrik iklan untuk filter ini"
            emptyTitle="Belum ada metrik iklan"
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
            error={singleFormError ? marketingErrorMessage(singleFormError) : null}
            isLoading={createMutation.isPending || updateMutation.isPending}
            isOpen={showForm}
            onClose={closeSingleForm}
            onSubmit={handleSubmitMetric}
            size="lg"
            submitLabel="Simpan metrik"
            title={editingMetric ? "Edit metrik iklan" : "Tambah metrik iklan"}
            subtitle="Catat belanja dan hasil iklan untuk satu campaign dan satu periode."
          >
            <div className="grid gap-4 lg:grid-cols-2">
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-campaign">
                  Campaign<RequiredMark />
                </label>
                <select
                  aria-describedby={errors.campaign_id ? "ads-metric-campaign-error" : undefined}
                  aria-invalid={Boolean(errors.campaign_id) || undefined}
                  className={fieldSelectClass}
                  id="ads-metric-campaign"
                  {...form.register("campaign_id", {
                    onChange: (event: { target: { value: string } }) => {
                      const platform = platformTouched ? null : platformForCampaign(event.target.value);
                      if (platform) {
                        form.setValue("platform", platform);
                      }
                    },
                  })}
                >
                  <option value="">Pilih campaign</option>
                  {campaigns.map((campaign) => (
                    <option key={campaign.id} value={campaign.id}>
                      {campaignOptionLabel(campaign)}
                    </option>
                  ))}
                </select>
                <FieldError id="ads-metric-campaign-error" message={errors.campaign_id?.message} />
              </div>
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-platform">
                  Platform<RequiredMark />
                </label>
                <select
                  aria-describedby={
                    [errors.platform ? "ads-metric-platform-error" : "", singleMismatch ? "ads-metric-platform-hint" : ""]
                      .filter(Boolean)
                      .join(" ") || undefined
                  }
                  aria-invalid={Boolean(errors.platform) || undefined}
                  className={fieldSelectClass}
                  id="ads-metric-platform"
                  {...form.register("platform", { onChange: () => setPlatformTouched(true) })}
                >
                  {adsMetricPlatformOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
                <FieldError id="ads-metric-platform-error" message={errors.platform?.message} />
              </div>
              {singleMismatch ? (
                <div className="lg:col-span-2">
                  <PlatformMismatchHint id="ads-metric-platform-hint" message={singleMismatch} />
                </div>
              ) : null}
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-start">
                  Tanggal mulai<RequiredMark />
                </label>
                <Input aria-describedby={errors.period_start ? "ads-metric-start-error" : undefined} aria-invalid={Boolean(errors.period_start) || undefined} className={inputErrorClass} id="ads-metric-start" {...form.register("period_start")} type="date" />
                <FieldError id="ads-metric-start-error" message={errors.period_start?.message} />
              </div>
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-end">
                  Tanggal selesai<RequiredMark />
                </label>
                <Input aria-describedby={errors.period_end ? "ads-metric-end-error" : undefined} aria-invalid={Boolean(errors.period_end) || undefined} className={inputErrorClass} id="ads-metric-end" {...form.register("period_end")} type="date" />
                <FieldError id="ads-metric-end-error" message={errors.period_end?.message} />
              </div>
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-spent">Belanja</label>
                <Controller
                  control={form.control}
                  name="amount_spent"
                  render={({ field }) => (
                    <CurrencyInput
                      aria-describedby={errors.amount_spent ? "ads-metric-spent-error" : undefined}
                      aria-invalid={Boolean(errors.amount_spent) || undefined}
                      className={inputErrorClass}
                      id="ads-metric-spent"
                      onValueChange={field.onChange}
                      value={field.value}
                      variant="field"
                    />
                  )}
                />
                <FieldError id="ads-metric-spent-error" message={errors.amount_spent?.message} />
              </div>
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-revenue">Pendapatan</label>
                <Controller
                  control={form.control}
                  name="revenue"
                  render={({ field }) => (
                    <CurrencyInput
                      aria-describedby={errors.revenue ? "ads-metric-revenue-error" : undefined}
                      aria-invalid={Boolean(errors.revenue) || undefined}
                      className={inputErrorClass}
                      id="ads-metric-revenue"
                      onValueChange={field.onChange}
                      value={field.value}
                      variant="field"
                    />
                  )}
                />
                <FieldError id="ads-metric-revenue-error" message={errors.revenue?.message} />
              </div>
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-impressions">Impresi</label>
                <Input aria-describedby={errors.impressions ? "ads-metric-impressions-error" : undefined} aria-invalid={Boolean(errors.impressions) || undefined} className={inputErrorClass} id="ads-metric-impressions" {...form.register("impressions", { valueAsNumber: true })} min={0} placeholder="0" type="number" />
                <FieldError id="ads-metric-impressions-error" message={errors.impressions?.message} />
              </div>
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-clicks">Klik</label>
                <Input aria-describedby={errors.clicks ? "ads-metric-clicks-error" : undefined} aria-invalid={Boolean(errors.clicks) || undefined} className={inputErrorClass} id="ads-metric-clicks" {...form.register("clicks", { valueAsNumber: true })} min={0} placeholder="0" type="number" />
                <FieldError id="ads-metric-clicks-error" message={errors.clicks?.message} />
              </div>
              <div className="min-w-0">
                <label className={formLabelClass} htmlFor="ads-metric-conversions">Konversi</label>
                <Input aria-describedby={errors.conversions ? "ads-metric-conversions-error" : undefined} aria-invalid={Boolean(errors.conversions) || undefined} className={inputErrorClass} id="ads-metric-conversions" {...form.register("conversions", { valueAsNumber: true })} min={0} placeholder="0" type="number" />
                <FieldError id="ads-metric-conversions-error" message={errors.conversions?.message} />
              </div>
              <div className="min-w-0 lg:col-span-2">
                <label className={formLabelClass} htmlFor="ads-metric-notes">Catatan</label>
                <Input className={inputErrorClass} id="ads-metric-notes" {...form.register("notes")} placeholder="Opsional" />
              </div>
            </div>
          </FormModal>

          <FormModal
            error={batchMutation.error ? marketingErrorMessage(batchMutation.error) : null}
            isLoading={batchMutation.isPending}
            isOpen={showBatchForm}
            onClose={() => {
              resetBatchRows();
              setShowBatchForm(false);
              batchMutation.reset();
            }}
            onSubmit={(event) => {
              event.preventDefault();
              batchMutation.mutate(batchRows);
            }}
            size="xl"
            submitLabel="Simpan semua"
            title="Input metrik massal"
            subtitle="Isi beberapa baris metrik sekaligus, satu baris per campaign dan periode."
          >
            <div className="flex justify-end">
              <Button
                onClick={() => {
                  setBatchRows((previous) => [...previous, { ...defaultMetricForm }]);
                  setBatchPlatformTouched((previous) => [...previous, false]);
                }}
                type="button"
                variant="outline"
              >
                <Plus className="h-4 w-4" />
                Tambah baris
              </Button>
            </div>
            <div className="space-y-3">
              {batchRows.map((row, index) => {
                const rowMismatch = mismatchFor(row.campaign_id, row.platform);
                const fieldId = (name: string) => `bulk-${index}-${name}`;
                return (
                  <div
                    aria-label={`Baris ${index + 1}`}
                    className="grid gap-3 rounded-xl border border-border bg-surface p-4 lg:grid-cols-5"
                    key={index}
                    role="group"
                  >
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("campaign")}>Campaign</label>
                      <select
                        className={fieldSelectClass}
                        id={fieldId("campaign")}
                        onChange={(event) => {
                          updateBatchRow(setBatchRows, index, "campaign_id", event.target.value);
                          const platform = batchPlatformTouched[index] ? null : platformForCampaign(event.target.value);
                          if (platform) {
                            updateBatchRow(setBatchRows, index, "platform", platform);
                          }
                        }}
                        value={row.campaign_id}
                      >
                        <option value="">Pilih campaign</option>
                        {campaigns.map((campaign) => (
                          <option key={campaign.id} value={campaign.id}>
                            {campaignOptionLabel(campaign)}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("platform")}>Platform</label>
                      <select
                        aria-describedby={rowMismatch ? fieldId("hint") : undefined}
                        className={fieldSelectClass}
                        id={fieldId("platform")}
                        onChange={(event) => {
                          updateBatchRow(setBatchRows, index, "platform", event.target.value);
                          setBatchPlatformTouched((previous) => previous.map((touched, rowIndex) => (rowIndex === index ? true : touched)));
                        }}
                        value={row.platform}
                      >
                        {adsMetricPlatformOptions.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("start")}>Mulai</label>
                      <Input id={fieldId("start")} onChange={(event) => updateBatchRow(setBatchRows, index, "period_start", event.target.value)} type="date" value={row.period_start} />
                    </div>
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("end")}>Selesai</label>
                      <Input id={fieldId("end")} onChange={(event) => updateBatchRow(setBatchRows, index, "period_end", event.target.value)} type="date" value={row.period_end} />
                    </div>
                    <div className="flex items-end">
                      <Button
                        className="w-full"
                        onClick={() => {
                          removeBatchRow(setBatchRows, index);
                          setBatchPlatformTouched((previous) =>
                            previous.length === 1 ? [false] : previous.filter((_, rowIndex) => rowIndex !== index),
                          );
                        }}
                        type="button"
                        variant="ghost"
                      >
                        Hapus baris
                      </Button>
                    </div>
                    {rowMismatch ? (
                      <div className="lg:col-span-5">
                        <PlatformMismatchHint id={fieldId("hint")} message={rowMismatch} />
                      </div>
                    ) : null}
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("spent")}>Belanja</label>
                      <CurrencyInput id={fieldId("spent")} variant="field" onValueChange={(value) => updateBatchRow(setBatchRows, index, "amount_spent", value)} value={row.amount_spent} />
                    </div>
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("revenue")}>Pendapatan</label>
                      <CurrencyInput id={fieldId("revenue")} variant="field" onValueChange={(value) => updateBatchRow(setBatchRows, index, "revenue", value)} value={row.revenue} />
                    </div>
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("impressions")}>Impresi</label>
                      <Input id={fieldId("impressions")} min={0} onChange={(event) => updateBatchRow(setBatchRows, index, "impressions", Number(event.target.value))} type="number" value={row.impressions} />
                    </div>
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("clicks")}>Klik</label>
                      <Input id={fieldId("clicks")} min={0} onChange={(event) => updateBatchRow(setBatchRows, index, "clicks", Number(event.target.value))} type="number" value={row.clicks} />
                    </div>
                    <div className="min-w-0">
                      <label className={bulkLabelClass} htmlFor={fieldId("conversions")}>Konversi</label>
                      <Input id={fieldId("conversions")} min={0} onChange={(event) => updateBatchRow(setBatchRows, index, "conversions", Number(event.target.value))} type="number" value={row.conversions} />
                    </div>
                    <div className="min-w-0 lg:col-span-5">
                      <label className={bulkLabelClass} htmlFor={fieldId("notes")}>Catatan</label>
                      <Input id={fieldId("notes")} onChange={(event) => updateBatchRow(setBatchRows, index, "notes", event.target.value)} placeholder="Opsional" value={row.notes} />
                    </div>
                  </div>
                );
              })}
            </div>
          </FormModal>

          <ConfirmDialog
            confirmLabel="Hapus metrik"
            description={metricToDelete ? `Metrik iklan untuk "${metricToDelete.campaign_name ?? "Campaign tidak ditemukan"}" akan dihapus.` : ""}
            isLoading={deleteMutation.isPending}
            isOpen={Boolean(metricToDelete)}
            onClose={() => setMetricToDelete(null)}
            onConfirm={() => {
              if (metricToDelete) {
                deleteMutation.mutate(metricToDelete.id);
              }
            }}
            title="Hapus metrik iklan?"
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
                    <Pie cx="50%" cy="50%" data={platformRows.map((row) => ({ ...row, group_label: platformLabel(row.group_key, row.group_label) }))} dataKey="total_spent" nameKey="group_label" innerRadius={55} outerRadius={90} paddingAngle={3}>
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
                      <span className="text-sm font-medium text-text-primary">{platformLabel(row.group_key, row.group_label)}</span>
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
          Akun ini hanya bisa melihat metrik iklan.
        </Card>
      ) : null}
    </div>
  );
}

// The native selects keep the global .field-select look; only the error
// border is added. Labels share the campaign form's typography.
const fieldSelectClass = "field-select w-full min-w-0 aria-[invalid=true]:border-error";
const inputErrorClass = "aria-[invalid=true]:border-error";
const formLabelClass = `mb-1.5 block ${formLabelClassName}`;
const bulkLabelClass = `mb-1 block ${formSubLabelClassName}`;

// "Name · Meta Ads": campaigns can share a name, and the channel tells the
// user which platform to expect before the mismatch hint does.
function campaignOptionLabel(campaign: { name: string; channel: string }) {
  return `${campaign.name} · ${channelMeta(campaign.channel).label}`;
}

function PlatformMismatchHint({ id, message }: { id: string; message: string }) {
  return (
    <p className="flex gap-2 rounded-xl border border-info/30 bg-info-light px-3 py-2 text-[13px] text-text-primary" id={id} role="status">
      <Info aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-info" />
      <span>{message}</span>
    </p>
  );
}

// The API labels platforms by humanising the key ("Tiktok"); show the same
// label as everywhere else ("TikTok") for the platforms the app knows.
function platformLabel(key: string, fallback: string) {
  return isAdsMetricPlatform(key) ? adsPlatformMeta(key).label : fallback;
}

function resetMetricForm(form: ReturnType<typeof useForm<AdsMetricFormValues>>) {
  form.reset({ ...defaultMetricForm });
}

async function invalidateAdsMetrics(queryClient: ReturnType<typeof useQueryClient>) {
  await queryClient.invalidateQueries({ queryKey: adsMetricsKeys.all });
}

function formatShortDate(value: string) {
  return formatCalendarDate(value);
}

function formatMetricCurrency(value?: number | null) {
  if (value === undefined || value === null) {
    return "-";
  }
  return formatIDR(Math.round(value));
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

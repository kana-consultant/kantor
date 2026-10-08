import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { useEffect, useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { AlertTriangle, Download, FolderKanban, LayoutList, Paperclip, Plus, Search, SearchX, SlidersHorizontal } from "lucide-react";

import { CampaignForm } from "@/components/shared/campaign-form";
import { ChannelBadge, PlatformMismatchNote } from "@/components/shared/channel-badge";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import {
  Drawer,
  DrawerBody,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@/components/shared/drawer";
import { EmptyState } from "@/components/shared/empty-state";
import { ExportButton } from "@/components/shared/export-button";
import { MarketingCampaignBoard } from "@/components/shared/marketing-campaign-board";
import { PermissionGate } from "@/components/shared/permission-gate";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useRBAC } from "@/hooks/use-rbac";
import {
  campaignChannelOptions,
  campaignMatchesFilters,
  campaignStageOptions,
  campaignStatusLabel,
  channelMeta,
  formatMetricPercent,
  formatMetricRatio,
  formatShortPeriod,
  marketingErrorMessage,
  splitTextLinks,
} from "@/lib/marketing";
import { formatIDR } from "@/lib/currency";
import { extractDateInputValue } from "@/lib/date";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { getProtectedFileName, openProtectedFile } from "@/services/files";
import {
  adsMetricsKeys,
  listAdsMetrics,
} from "@/services/marketing-ads-metrics";
import {
  campaignsKeys,
  createCampaign,
  deleteCampaign,
  deleteCampaignAttachment,
  getCampaign,
  listCampaignActivities,
  listCampaignKanban,
  listCampaignPICOptions,
  listCampaigns,
  moveCampaign,
  updateCampaign,
  uploadCampaignAttachments,
} from "@/services/marketing-campaigns";
import { leadsKeys, listLeads } from "@/services/marketing-leads";
import { cn } from "@/lib/utils";
import { toast } from "@/stores/toast-store";
import type {
  AdsMetric,
  Campaign,
  CampaignActivity,
  CampaignAttachment,
  CampaignColumn,
  CampaignFilters,
  CampaignFormValues,
  CampaignStatus,
  Lead,
} from "@/types/marketing";

const searchSchema = z.object({
  view: z.enum(["kanban", "table"]).optional().catch("kanban"),
});

// Campaign ids are UUIDs (8-4-4-4-12 hex, as Postgres prints them).
const CAMPAIGN_ID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

type DetailTab = "overview" | "attachments" | "metrics" | "leads" | "activity";

const detailTabLabels: Record<DetailTab, string> = {
  overview: "Ringkasan",
  attachments: "Lampiran",
  metrics: "Metrik iklan",
  leads: "Leads",
  activity: "Aktivitas",
};

const defaultFilters: CampaignFilters = {
  page: 1,
  perPage: 20,
  search: "",
  channel: "",
  status: "",
  pic: "",
  dateFrom: "",
  dateTo: "",
};

export const Route = createFileRoute("/_authenticated/marketing/campaigns")({
  validateSearch: searchSchema,
  beforeLoad: async () => {
    await ensureModuleAccess("marketing");
    await ensurePermission(permissions.marketingCampaignView);
  },
  component: MarketingCampaignsPage,
});

function MarketingCampaignsPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const activeView = search.view ?? "kanban";
  const canCreate = hasPermission(permissions.marketingCampaignCreate);
  const canEdit = hasPermission(permissions.marketingCampaignEdit);
  const canDelete = hasPermission(permissions.marketingCampaignDelete);
  const canViewMetrics = hasPermission(permissions.marketingAdsMetricsView);
  const canCreateMetrics = hasPermission(permissions.marketingAdsMetricsCreate);
  const canViewLeads = hasPermission(permissions.marketingLeadsView);

  const [filters, setFilters] = useState<CampaignFilters>(defaultFilters);
  const [searchInput, setSearchInput] = useState("");
  const debouncedSearch = useDebouncedValue(searchInput, 350);
  const [isComposerOpen, setIsComposerOpen] = useState(false);
  // Stage of the lane a campaign is created from ("+" in a lane header).
  const [composerStage, setComposerStage] = useState<CampaignStatus | undefined>(undefined);
  // Below md the filters fold behind a "Filter" button.
  const [isFilterPanelOpen, setIsFilterPanelOpen] = useState(false);
  const [editingCampaign, setEditingCampaign] = useState<Campaign | null>(null);
  const [selectedCampaignId, setSelectedCampaignId] = useState<string | null>(null);
  const [detailTab, setDetailTab] = useState<DetailTab>("overview");
  const [campaignToDelete, setCampaignToDelete] = useState<Campaign | null>(null);
  const [attachmentToDelete, setAttachmentToDelete] = useState<CampaignAttachment | null>(null);

  useEffect(() => {
    if (typeof window === "undefined" || search.view) {
      return;
    }

    if (window.innerWidth < 768) {
      void navigate({ replace: true, search: { view: "table" } });
    }
  }, [navigate, search.view]);

  // Deep link from a notification (#campaign:<id>), also when the page is
  // already open: window.location.assign with only a new hash fires hashchange.
  useEffect(() => {
    if (typeof window === "undefined") {
      return undefined;
    }

    const openFromHash = () => {
      const hash = window.location.hash;
      if (!hash.startsWith("#campaign:")) {
        return;
      }

      // The id ends up in API paths, so only a real campaign id (a UUID) is
      // accepted; a crafted or broken hash is ignored.
      let campaignId = "";
      try {
        campaignId = decodeURIComponent(hash.slice("#campaign:".length)).trim();
      } catch {
        return;
      }
      if (!CAMPAIGN_ID_PATTERN.test(campaignId)) {
        return;
      }

      setEditingCampaign(null);
      setSelectedCampaignId(campaignId);
      setDetailTab("overview");
      window.history.replaceState(
        window.history.state,
        document.title,
        `${window.location.pathname}${window.location.search}`,
      );
    };

    openFromHash();
    window.addEventListener("hashchange", openFromHash);
    return () => window.removeEventListener("hashchange", openFromHash);
  }, []);

  const appliedFilters = useMemo(
    () => ({ ...filters, search: debouncedSearch.trim() }),
    [debouncedSearch, filters],
  );
  const isFiltering = Boolean(
    appliedFilters.search || filters.channel || filters.status || filters.pic || filters.dateFrom || filters.dateTo,
  );

  const picOptionsQuery = useQuery({
    queryKey: campaignsKeys.picOptions(),
    queryFn: listCampaignPICOptions,
    staleTime: 60_000,
  });

  const campaignsQuery = useQuery({
    queryKey: campaignsKeys.list(appliedFilters),
    queryFn: () => listCampaigns(appliedFilters),
    placeholderData: keepPreviousData,
  });

  const kanbanQuery = useQuery({
    queryKey: campaignsKeys.kanban(),
    queryFn: listCampaignKanban,
  });

  const detailQuery = useQuery({
    enabled: Boolean(selectedCampaignId),
    queryKey: selectedCampaignId ? campaignsKeys.detail(selectedCampaignId) : [...campaignsKeys.all, "detail", "empty"],
    queryFn: () => getCampaign(selectedCampaignId!),
    retry: false,
  });

  const metricsQuery = useQuery({
    enabled: Boolean(selectedCampaignId) && canViewMetrics,
    queryKey: selectedCampaignId
      ? adsMetricsKeys.list({
          page: 1,
          perPage: 8,
          campaignId: selectedCampaignId,
          platform: "",
          dateFrom: "",
          dateTo: "",
        })
      : [...adsMetricsKeys.all, "related", "empty"],
    queryFn: () =>
      listAdsMetrics({
        page: 1,
        perPage: 8,
        campaignId: selectedCampaignId!,
        platform: "",
        dateFrom: "",
        dateTo: "",
      }),
  });

  const relatedLeadsQuery = useQuery({
    enabled: Boolean(selectedCampaignId) && canViewLeads,
    queryKey: selectedCampaignId
      ? leadsKeys.list({
          page: 1,
          perPage: 8,
          pipelineStatus: "",
          sourceChannel: "",
          campaignId: selectedCampaignId,
          assignedTo: "",
          dateFrom: "",
          dateTo: "",
          search: "",
        })
      : [...leadsKeys.all, "related", "empty"],
    queryFn: () =>
      listLeads({
        page: 1,
        perPage: 8,
        pipelineStatus: "",
        sourceChannel: "",
        campaignId: selectedCampaignId!,
        assignedTo: "",
        dateFrom: "",
        dateTo: "",
        search: "",
      }),
  });

  const activitiesQuery = useQuery({
    enabled: Boolean(selectedCampaignId),
    queryKey: selectedCampaignId ? campaignsKeys.activities(selectedCampaignId) : [...campaignsKeys.all, "activities", "empty"],
    queryFn: () => listCampaignActivities(selectedCampaignId!),
  });

  const kanbanColumns = kanbanQuery.data;

  const createMutation = useMutation({
    mutationFn: createCampaign,
    onSuccess: async (detail) => {
      setIsComposerOpen(false);
      toast.success("Campaign dibuat", detail.campaign.name);
      // Open the new campaign so it is visible even when a filter hides its card.
      setDetailTab("overview");
      setSelectedCampaignId(detail.campaign.id);
      await invalidateCampaignQueries(queryClient);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ campaignId, values }: { campaignId: string; values: CampaignFormValues }) =>
      updateCampaign(campaignId, values),
    onSuccess: async (detail, variables) => {
      setEditingCampaign(null);
      setSelectedCampaignId(variables.campaignId);
      toast.success("Perubahan disimpan", detail.campaign.name);
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
  });

  // Stage change from the drawer: a move to the end of the lane that carries
  // the stage, as a drag on the board does. A move touches only the lane and
  // the status, so a colleague's edit made since the drawer loaded (name,
  // budget, description ...) is never overwritten with the drawer's copy.
  const stageMutation = useMutation({
    mutationFn: async ({ campaign, status }: { campaign: Campaign; status: CampaignStatus }) => {
      const lane = kanbanColumns?.find((column) => column.stage === status);
      if (lane) {
        const cards = lane.campaigns ?? [];
        const end = cards.reduce((highest, card) => Math.max(highest, card.column_position ?? 0), cards.length) + 1;
        return moveCampaign(campaign.id, lane.id, end);
      }
      // No lane for the stage on the board as loaded (the server creates a
      // missing stage lane on update): update from a fresh copy instead.
      const fresh = await getCampaign(campaign.id);
      return updateCampaign(campaign.id, { ...toCampaignFormValues(fresh.campaign), status });
    },
    onSuccess: async (detail, variables) => {
      toast.success("Tahap diubah", campaignStatusLabel(variables.status, kanbanColumns));
      await invalidateCampaignQueries(queryClient, detail.campaign.id);
    },
    onError: (error) => {
      toast.error("Gagal mengubah tahap", marketingErrorMessage(error, kanbanColumns));
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (campaign: Campaign) => deleteCampaign(campaign.id),
    onSuccess: async (_, campaign) => {
      setCampaignToDelete(null);
      if (selectedCampaignId === campaign.id) {
        setSelectedCampaignId(null);
      }
      toast.success("Campaign dihapus", campaign.name);
      await invalidateCampaignQueries(queryClient, campaign.id);
    },
    onError: (error) => {
      // Close the confirm dialog first: the toast layer sits under the
      // dialog's backdrop and could not be read while it is open.
      setCampaignToDelete(null);
      toast.error("Gagal menghapus campaign", marketingErrorMessage(error, kanbanColumns));
    },
  });

  const moveMutation = useMutation({
    mutationFn: ({ campaignId, columnId, position }: { campaignId: string; columnId: string; position: number }) =>
      moveCampaign(campaignId, columnId, position),
    onSuccess: async (_, variables) => {
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
    onError: async (error, variables) => {
      toast.error("Gagal memindahkan campaign", marketingErrorMessage(error, kanbanColumns));
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
  });

  const uploadMutation = useMutation({
    mutationFn: ({ campaignId, files }: { campaignId: string; files: File[] }) =>
      uploadCampaignAttachments(campaignId, files),
    onSuccess: async (_, variables) => {
      toast.success("Lampiran diunggah", `${variables.files.length} file`);
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
    onError: (error) => {
      toast.error("Gagal mengunggah lampiran", marketingErrorMessage(error, kanbanColumns));
    },
  });

  const deleteAttachmentMutation = useMutation({
    mutationFn: ({ campaignId, attachmentId }: { campaignId: string; attachmentId: string }) =>
      deleteCampaignAttachment(campaignId, attachmentId),
    onSuccess: async (_, variables) => {
      setAttachmentToDelete(null);
      toast.success("Lampiran dihapus");
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
    onError: (error) => {
      // As for a campaign delete: the toast must not end up under the dialog.
      setAttachmentToDelete(null);
      toast.error("Gagal menghapus lampiran", marketingErrorMessage(error, kanbanColumns));
    },
  });

  const picOptions = picOptionsQuery.data ?? [];
  // The server already applied the filters to the table page.
  const tableItems = campaignsQuery.data?.items ?? [];
  const filteredColumns = useMemo(
    () =>
      (kanbanColumns ?? []).map((column) => ({
        ...column,
        campaigns: (column.campaigns ?? []).filter((campaign) => campaignMatchesFilters(campaign, appliedFilters)),
      })),
    [appliedFilters, kanbanColumns],
  );
  // While filtering, the board sends a real position: past the last card of
  // the target lane on the unfiltered board. A deleted campaign leaves a gap
  // in its lane, so this is the highest position + 1, not the count + 1 (the
  // server clamps anything past the end to the end).
  const laneEndPositions = useMemo(
    () =>
      isFiltering && kanbanColumns
        ? Object.fromEntries(
            kanbanColumns.map((column) => {
              const cards = column.campaigns ?? [];
              const last = cards.reduce((highest, campaign) => Math.max(highest, campaign.column_position ?? 0), cards.length);
              return [column.id, last + 1];
            }),
          )
        : null,
    [isFiltering, kanbanColumns],
  );

  // The kanban response holds every campaign that matches the filters (it is
  // not paginated), so the tiles count from it in both views.
  const boardCampaigns = useMemo(() => filteredColumns.flatMap((column) => column.campaigns ?? []), [filteredColumns]);

  const selectedCampaign = detailQuery.data?.campaign ?? null;
  const selectedAttachments = detailQuery.data?.attachments ?? [];
  const relatedMetrics = metricsQuery.data?.items ?? [];
  const relatedLeads = relatedLeadsQuery.data?.items ?? [];
  const activities = activitiesQuery.data ?? [];
  const editValues = useMemo(() => (editingCampaign ? toCampaignFormValues(editingCampaign) : undefined), [editingCampaign]);

  const updateFilters = (patch: Partial<CampaignFilters>) => setFilters((current) => ({ ...current, ...patch, page: 1 }));
  const resetFilters = () => {
    setSearchInput("");
    setFilters(defaultFilters);
  };

  const openComposer = (stage?: CampaignStatus) => {
    createMutation.reset();
    setComposerStage(stage);
    setIsComposerOpen(true);
  };
  const activeFilterCount = [
    appliedFilters.search,
    filters.channel,
    filters.status,
    filters.pic,
    filters.dateFrom || filters.dateTo,
  ].filter(Boolean).length;
  const liveCount = boardCampaigns.filter((campaign) => campaign.status === "live").length;
  const totalBudget = boardCampaigns.reduce((total, campaign) => total + campaign.budget_amount, 0);

  const tableColumns: Array<DataTableColumn<Campaign>> = [
    {
      id: "campaign",
      header: "Campaign",
      accessor: "name",
      sortable: true,
      mobilePrimary: true,
      cell: (campaign) => (
        <button
          className="max-w-[28rem] rounded-[6px] text-left outline-none focus-visible:shadow-focus"
          onClick={(event) => {
            event.stopPropagation();
            openCampaign(campaign.id);
          }}
          type="button"
        >
          <span className="block font-semibold text-text-primary [overflow-wrap:anywhere]">{campaign.name}</span>
          {campaign.description ? (
            <span className="mt-0.5 line-clamp-1 text-[13px] text-text-secondary">{campaign.description}</span>
          ) : null}
        </button>
      ),
    },
    {
      id: "channel",
      header: "Kanal",
      accessor: "channel",
      sortable: true,
      cell: (campaign) => <ChannelBadge channel={campaign.channel} />,
    },
    {
      id: "budget",
      header: "Anggaran",
      accessor: "budget_amount",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (campaign) => <span className="whitespace-nowrap font-mono tabular-nums">{formatIDR(campaign.budget_amount)}</span>,
    },
    {
      id: "pic",
      header: "PIC",
      accessor: "pic_employee_name",
      sortable: true,
      cell: (campaign) =>
        campaign.pic_employee_name ? (
          <span className="whitespace-nowrap text-sm text-text-primary">{campaign.pic_employee_name}</span>
        ) : (
          <span className="whitespace-nowrap text-sm text-text-secondary">Belum ada PIC</span>
        ),
    },
    {
      id: "timeline",
      header: "Periode",
      accessor: "start_date",
      sortable: true,
      hideOnMobile: true,
      cell: (campaign) => (
        <span className="whitespace-nowrap text-sm text-text-secondary">
          {formatShortPeriod(campaign.start_date, campaign.end_date)}
        </span>
      ),
    },
    {
      id: "status",
      header: "Tahap",
      accessor: "status",
      sortable: true,
      cell: (campaign) => (
        <StatusBadge
          className="whitespace-nowrap text-text-primary"
          label={campaignStatusLabel(campaign.status, kanbanColumns)}
          status={campaign.status}
          variant="campaign-status"
        />
      ),
    },
    {
      id: "assets",
      header: "Lampiran",
      accessor: "attachment_count",
      numeric: true,
      align: "right",
      sortable: true,
      hideOnMobile: true,
      cell: (campaign) => <span className="font-mono tabular-nums text-text-secondary">{campaign.attachment_count}</span>,
    },
  ];

  function openCampaign(campaignId: string) {
    setDetailTab("overview");
    setSelectedCampaignId(campaignId);
  }

  const viewOptions = [
    { value: "kanban" as const, label: "Kanban", icon: FolderKanban },
    { value: "table" as const, label: "Tabel", icon: LayoutList },
  ];

  return (
    <div className="space-y-4">
      <Card className="p-4 shadow-sm hover:border-border/80 hover:shadow-sm sm:p-5">
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
          <div className="min-w-0">
            <h1 className="font-sans text-[24px] font-bold leading-tight tracking-tight text-text-primary">Campaigns</h1>
            <p className="mt-1 text-sm text-text-secondary">Rencanakan, jalankan, dan pantau campaign marketing di satu board</p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <div aria-label="Tampilan" className="inline-flex rounded-xl bg-surface-muted p-1" role="group">
              {viewOptions.map((option) => {
                const isActive = activeView === option.value;
                const Icon = option.icon;
                return (
                  <button
                    aria-pressed={isActive}
                    className={cn(
                      "inline-flex h-8 items-center gap-1.5 rounded-lg px-3 text-[13px] font-medium outline-none transition-colors duration-150 focus-visible:shadow-focus",
                      isActive ? "bg-surface text-text-primary shadow-xs" : "text-text-secondary hover:text-text-primary",
                    )}
                    key={option.value}
                    onClick={() => void navigate({ search: { view: option.value } })}
                    type="button"
                  >
                    <Icon aria-hidden="true" className="h-4 w-4" />
                    {option.label}
                  </button>
                );
              })}
            </div>
            <PermissionGate permission={permissions.marketingCampaignView}>
              <ExportButton
                endpoint="/marketing/campaigns/export"
                filename="campaigns-report"
                filters={{
                  channel: filters.channel,
                  date_from: filters.dateFrom,
                  date_to: filters.dateTo,
                  pic: filters.pic,
                  search: appliedFilters.search,
                  status: filters.status,
                }}
                formats={["pdf", "xlsx"]}
              />
            </PermissionGate>
            <PermissionGate permission={permissions.marketingCampaignCreate}>
              <Button onClick={() => openComposer()} variant="mkt">
                <Plus className="h-4 w-4" />
                Campaign baru
              </Button>
            </PermissionGate>
          </div>
        </div>

        <div className="mt-4 flex items-center gap-2 md:hidden">
          <Button
            aria-controls="campaign-filters"
            aria-expanded={isFilterPanelOpen}
            onClick={() => setIsFilterPanelOpen((open) => !open)}
            size="sm"
            type="button"
            variant="outline"
          >
            <SlidersHorizontal className="h-4 w-4" />
            Filter
            {activeFilterCount > 0 ? (
              <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-mkt px-1.5 text-[12px] font-semibold leading-none text-white">
                <span className="sr-only">filter aktif: </span>
                {activeFilterCount}
              </span>
            ) : null}
          </Button>
          {isFiltering ? (
            <Button onClick={resetFilters} size="sm" type="button" variant="ghost">
              Reset filter
            </Button>
          ) : null}
        </div>

        <div
          className={cn(
            "mt-3 flex-col gap-2 md:mt-4 md:flex md:flex-row md:flex-wrap md:items-center",
            isFilterPanelOpen ? "flex" : "hidden",
          )}
          id="campaign-filters"
        >
          <div className="relative md:min-w-[160px] md:flex-1">
            <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-secondary" />
            <Input
              aria-label="Cari campaign"
              className="h-10 pl-9 placeholder:text-text-secondary"
              onChange={(event) => {
                setSearchInput(event.target.value);
                setFilters((current) => ({ ...current, page: 1 }));
              }}
              placeholder="Cari nama campaign"
              type="search"
              value={searchInput}
            />
          </div>
          <Select
            aria-label="Filter kanal"
            className="md:w-[152px]"
            onValueChange={(value) => updateFilters({ channel: value })}
            options={[{ value: "", label: "Semua kanal" }, ...campaignChannelOptions]}
            triggerClassName="h-10"
            value={filters.channel}
          />
          <Select
            aria-label="Filter tahap"
            className="md:w-[152px]"
            onValueChange={(value) => updateFilters({ status: value })}
            options={[{ value: "", label: "Semua tahap" }, ...campaignStageOptions(kanbanColumns)]}
            triggerClassName="h-10"
            value={filters.status}
          />
          <Select
            aria-label="Filter PIC"
            className="md:w-[168px]"
            onValueChange={(value) => updateFilters({ pic: value })}
            options={[
              { value: "", label: "Semua PIC" },
              ...picOptions.map((option) => ({
                value: option.id,
                label: option.full_name,
                description: option.position || undefined,
              })),
            ]}
            triggerClassName="h-10"
            value={filters.pic}
          />
          <div aria-label="Periode" className="grid grid-cols-[1fr_auto_1fr] items-center gap-1.5 md:flex" role="group">
            <Input
              aria-label="Dari tanggal"
              className="h-10 md:w-[140px]"
              onChange={(event) => updateFilters({ dateFrom: event.target.value })}
              title="Dari tanggal"
              type="date"
              value={filters.dateFrom}
            />
            <span aria-hidden="true" className="text-text-secondary">–</span>
            <Input
              aria-label="Sampai tanggal"
              className="h-10 md:w-[140px]"
              min={filters.dateFrom || undefined}
              onChange={(event) => updateFilters({ dateTo: event.target.value })}
              title="Sampai tanggal"
              type="date"
              value={filters.dateTo}
            />
          </div>
          {isFiltering ? (
            <Button className="hidden md:inline-flex" onClick={resetFilters} size="xs" type="button" variant="ghost">
              Reset filter
            </Button>
          ) : null}
        </div>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
          <p aria-live="polite" className="text-sm text-text-secondary">
            {kanbanColumns ? (
              <>
                <span className="font-semibold tabular-nums text-text-primary">{boardCampaigns.length}</span> campaign
                <span aria-hidden="true"> · </span>
                <span className="font-semibold tabular-nums text-text-primary">{liveCount}</span> live
                <span aria-hidden="true"> · </span>
                <span className="font-semibold tabular-nums text-text-primary">{formatIDR(totalBudget)}</span> anggaran
              </>
            ) : kanbanQuery.isLoading ? (
              "Memuat ringkasan…"
            ) : (
              "Ringkasan belum tersedia"
            )}
          </p>
          {activeView === "kanban" && isFiltering && canEdit ? (
            <p className="text-[12px] text-text-secondary">Saat filter aktif, kartu yang dipindah masuk ke akhir kolom</p>
          ) : null}
        </div>
      </Card>

      <CampaignForm
        columns={kanbanColumns}
        description="Isi kanal, tahap, anggaran, PIC, periode, dan brief inti agar campaign langsung muncul di board."
        error={createMutation.error}
        isOpen={isComposerOpen}
        defaultStatus={composerStage}
        isSubmitting={createMutation.isPending}
        onCancel={() => {
          setIsComposerOpen(false);
          createMutation.reset();
        }}
        onSubmit={(values) => createMutation.mutate(values)}
        picOptions={picOptions}
        picOptionsUnavailable={picOptionsQuery.isError}
        submitLabel="Buat campaign"
        title="Campaign baru"
      />

      {editingCampaign ? (
        <CampaignForm
          columns={kanbanColumns}
          currentPic={
            editingCampaign.pic_employee_id
              ? { id: editingCampaign.pic_employee_id, name: editingCampaign.pic_employee_name ?? "PIC saat ini" }
              : null
          }
          defaultValues={editValues}
          description="Perbarui detail utama campaign tanpa meninggalkan workspace marketing."
          error={updateMutation.error}
          isOpen
          isSubmitting={updateMutation.isPending}
          key={editingCampaign.id}
          onCancel={() => {
            setEditingCampaign(null);
            updateMutation.reset();
          }}
          onSubmit={(values) => updateMutation.mutate({ campaignId: editingCampaign.id, values })}
          picOptions={picOptions}
          picOptionsUnavailable={picOptionsQuery.isError}
          submitLabel="Simpan perubahan"
          title={`Edit ${editingCampaign.name}`}
        />
      ) : null}

      {activeView === "kanban" ? (
        kanbanQuery.isLoading ? (
          <div aria-busy="true" className="-mx-1 overflow-hidden px-1 pb-3">
            <div className="flex gap-3">
              {[0, 1, 2, 3].map((index) => (
                <Skeleton className="h-[360px] w-[min(82vw,288px)] shrink-0 rounded-xl md:w-[288px]" key={index} />
              ))}
            </div>
          </div>
        ) : kanbanQuery.isError && !kanbanColumns ? (
          <ErrorPanel
            message={marketingErrorMessage(kanbanQuery.error)}
            onRetry={() => void kanbanQuery.refetch()}
            title="Board gagal dimuat"
          />
        ) : (kanbanColumns ?? []).length === 0 ? (
          <EmptyState
            actionLabel="Muat ulang"
            description="Muat ulang halaman; jika board masih kosong, hubungi admin marketing."
            icon={FolderKanban}
            onAction={() => void kanbanQuery.refetch()}
            title="Board belum punya tahap"
          />
        ) : isFiltering && boardCampaigns.length === 0 ? (
          <EmptyState
            actionLabel="Reset filter"
            actionProps={{ variant: "outline" }}
            description="Ubah atau reset filter untuk melihat semua campaign"
            icon={SearchX}
            onAction={resetFilters}
            title="Tidak ada campaign yang cocok"
          />
        ) : (
          <>
            {kanbanQuery.isError ? (
              // A background refetch failed: keep the board that was loaded
              // (the summary above shows the same data) and say it may be stale.
              <p className="mb-3 flex flex-wrap items-center gap-x-2 text-[13px] text-text-secondary" role="status">
                <AlertTriangle aria-hidden="true" className="h-4 w-4 text-warning" />
                Board belum bisa diperbarui: {marketingErrorMessage(kanbanQuery.error)}
                <button
                  className="font-semibold text-mkt-dark underline-offset-2 hover:underline dark:text-mkt"
                  onClick={() => void kanbanQuery.refetch()}
                  type="button"
                >
                  Coba lagi
                </button>
              </p>
            ) : null}
            <MarketingCampaignBoard
              canEdit={canEdit}
              columns={filteredColumns}
              laneEndPositions={laneEndPositions}
              onCampaignOpen={(campaign) => openCampaign(campaign.id)}
              onCreateInLane={canCreate ? (stage) => openComposer(stage) : undefined}
              onMoveCampaign={(campaignId, columnId, position) =>
                moveMutation.mutateAsync({ campaignId, columnId, position }).then(() => undefined)
              }
            />
          </>
        )
      ) : campaignsQuery.isError && !campaignsQuery.data ? (
        // Nothing loaded: the panel alone. An empty table under it would say
        // "Belum ada campaign" and offer to create the first one.
        <ErrorPanel
          message={marketingErrorMessage(campaignsQuery.error)}
          onRetry={() => void campaignsQuery.refetch()}
          title="Daftar campaign gagal dimuat"
        />
      ) : (
        <>
          {campaignsQuery.isError ? (
            <ErrorPanel
              message={marketingErrorMessage(campaignsQuery.error)}
              onRetry={() => void campaignsQuery.refetch()}
              title="Daftar campaign gagal dimuat"
            />
          ) : null}
          <DataTable
            columns={tableColumns}
            data={tableItems}
            emptyActionLabel={isFiltering ? "Reset filter" : canCreate ? "Campaign baru" : undefined}
            emptyDescription={
              isFiltering
                ? "Ubah atau reset filter untuk melihat semua campaign"
                : "Buat campaign pertama untuk mulai merencanakan"
            }
            emptyTitle={isFiltering ? "Tidak ada campaign yang cocok" : "Belum ada campaign"}
            getRowId={(campaign) => campaign.id}
            loading={campaignsQuery.isLoading}
            onEmptyAction={isFiltering ? resetFilters : canCreate ? () => openComposer() : undefined}
            onRowClick={(campaign) => openCampaign(campaign.id)}
            pagination={
              campaignsQuery.data?.meta
                ? {
                    page: campaignsQuery.data.meta.page,
                    perPage: campaignsQuery.data.meta.per_page,
                    total: campaignsQuery.data.meta.total,
                    onPageChange: (page) => setFilters((current) => ({ ...current, page })),
                  }
                : undefined
            }
            selectedRowId={selectedCampaignId}
          />
        </>
      )}

      {selectedCampaignId && !editingCampaign ? (
        <CampaignDetailDrawer
          attachments={selectedAttachments}
          campaign={selectedCampaign}
          canDelete={canDelete}
          canEdit={canEdit}
          canViewLeads={canViewLeads}
          canViewMetrics={canViewMetrics}
          columns={kanbanColumns}
          detailTab={detailTab}
          activities={activities}
          error={detailQuery.isError ? marketingErrorMessage(detailQuery.error) : null}
          leads={relatedLeads}
          metrics={relatedMetrics}
          isLoadingActivities={activitiesQuery.isLoading}
          isDeletingAttachment={deleteAttachmentMutation.isPending}
          isLoading={detailQuery.isLoading}
          isLoadingLeads={relatedLeadsQuery.isLoading}
          isLoadingMetrics={metricsQuery.isLoading}
          isChangingStage={stageMutation.isPending}
          isUploading={uploadMutation.isPending}
          onClose={() => setSelectedCampaignId(null)}
          onLogMetric={
            canCreateMetrics && selectedCampaign
              ? () => void navigate({ to: "/marketing/ads-metrics", search: { campaign: selectedCampaign.id, new: true } })
              : undefined
          }
          onDelete={() => {
            if (selectedCampaign) {
              setCampaignToDelete(selectedCampaign);
            }
          }}
          onDeleteAttachment={(attachmentId) => {
            if (selectedCampaign) {
              const targetAttachment = selectedAttachments.find((attachment) => attachment.id === attachmentId);
              if (targetAttachment) {
                setAttachmentToDelete(targetAttachment);
              }
            }
          }}
          onEdit={() => {
            if (selectedCampaign) {
              // The drawer hides while the form is open and comes back on
              // cancel, because selectedCampaignId is kept.
              updateMutation.reset();
              setEditingCampaign(selectedCampaign);
            }
          }}
          onRetry={() => void detailQuery.refetch()}
          onStageChange={(status) => {
            // The Tahap field stays focusable while a change saves (a
            // disabled field would drop keyboard focus); ignore picks then.
            if (selectedCampaign && status !== selectedCampaign.status && !stageMutation.isPending) {
              stageMutation.mutate({ campaign: selectedCampaign, status: status as CampaignStatus });
            }
          }}
          onTabChange={setDetailTab}
          onUpload={(files) => {
            if (selectedCampaign) {
              uploadMutation.mutate({ campaignId: selectedCampaign.id, files });
            }
          }}
        />
      ) : null}

      <ConfirmDialog
        confirmLabel="Hapus campaign"
        description={
          campaignToDelete
            ? `Campaign "${campaignToDelete.name}" akan dihapus permanen beserta lampiran dan data ads metrics-nya. Lead yang tertaut tetap ada, tetapi tidak lagi terhubung ke campaign ini.`
            : ""
        }
        isLoading={deleteMutation.isPending}
        isOpen={Boolean(campaignToDelete)}
        onClose={() => setCampaignToDelete(null)}
        onConfirm={() => {
          if (campaignToDelete) {
            deleteMutation.mutate(campaignToDelete);
          }
        }}
        title={campaignToDelete ? `Hapus ${campaignToDelete.name}?` : "Hapus campaign?"}
      />

      <ConfirmDialog
        confirmLabel="Hapus lampiran"
        description={attachmentToDelete ? `Lampiran "${attachmentToDelete.file_name}" akan dihapus dari campaign ini.` : ""}
        isLoading={deleteAttachmentMutation.isPending}
        isOpen={Boolean(attachmentToDelete)}
        onClose={() => setAttachmentToDelete(null)}
        onConfirm={() => {
          if (selectedCampaign && attachmentToDelete) {
            deleteAttachmentMutation.mutate({ attachmentId: attachmentToDelete.id, campaignId: selectedCampaign.id });
          }
        }}
        title="Hapus lampiran?"
      />
    </div>
  );
}

// Plain text with its line breaks kept and http(s) URLs as links.
function LinkifiedText({ text }: { text: string }) {
  return (
    <>
      {splitTextLinks(text).map((part, index) =>
        part.type === "link" ? (
          <a
            className="text-mkt-dark underline underline-offset-2 hover:no-underline dark:text-mkt"
            href={part.value}
            key={index}
            rel="noopener noreferrer"
            target="_blank"
          >
            {part.value}
          </a>
        ) : (
          <span key={index}>{part.value}</span>
        ),
      )}
    </>
  );
}

// A failed load: a light-red panel with the reason and a retry, in place of
// the content that could not load.
function ErrorPanel({ title, message, onRetry }: { title: string; message: string; onRetry: () => void }) {
  return (
    <div
      className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-error/30 bg-error-light px-4 py-3 text-sm"
      role="alert"
    >
      <p className="flex min-w-0 items-start gap-2 text-text-primary">
        <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-error" />
        <span>
          <span className="font-semibold">{title}.</span> {message}
        </span>
      </p>
      <Button onClick={onRetry} size="sm" type="button" variant="outline">
        Coba lagi
      </Button>
    </div>
  );
}

function CampaignDetailDrawer({
  campaign,
  attachments,
  metrics,
  leads,
  activities,
  columns,
  detailTab,
  canEdit,
  canDelete,
  canViewMetrics,
  canViewLeads,
  error,
  isLoading,
  isLoadingMetrics,
  isLoadingLeads,
  isLoadingActivities,
  isChangingStage,
  isUploading,
  isDeletingAttachment,
  onClose,
  onEdit,
  onDelete,
  onLogMetric,
  onRetry,
  onStageChange,
  onTabChange,
  onUpload,
  onDeleteAttachment,
}: {
  campaign: Campaign | null;
  attachments: CampaignAttachment[];
  metrics: AdsMetric[];
  leads: Lead[];
  activities: CampaignActivity[];
  columns?: CampaignColumn[];
  detailTab: DetailTab;
  canEdit: boolean;
  canDelete: boolean;
  canViewMetrics: boolean;
  canViewLeads: boolean;
  error: string | null;
  isLoading: boolean;
  isLoadingMetrics: boolean;
  isLoadingLeads: boolean;
  isLoadingActivities: boolean;
  isChangingStage: boolean;
  isUploading: boolean;
  isDeletingAttachment: boolean;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
  // Set when the user may log ads metrics: opens Ads Metrics for this campaign.
  onLogMetric?: () => void;
  onRetry: () => void;
  onStageChange: (status: string) => void;
  onTabChange: (tab: DetailTab) => void;
  onUpload: (files: File[]) => void;
  onDeleteAttachment: (attachmentId: string) => void;
}) {
  const tabs = (Object.keys(detailTabLabels) as DetailTab[]).filter(
    (tab) => (tab !== "metrics" || canViewMetrics) && (tab !== "leads" || canViewLeads),
  );
  const activeTab = tabs.includes(detailTab) ? detailTab : "overview";

  const handleTabKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    const index = tabs.indexOf(activeTab);
    let next: DetailTab | undefined;
    if (event.key === "ArrowRight") {
      next = tabs[(index + 1) % tabs.length];
    } else if (event.key === "ArrowLeft") {
      next = tabs[(index - 1 + tabs.length) % tabs.length];
    } else if (event.key === "Home") {
      next = tabs[0];
    } else if (event.key === "End") {
      next = tabs[tabs.length - 1];
    }
    if (!next) {
      return;
    }
    event.preventDefault();
    onTabChange(next);
    document.getElementById(`campaign-tab-${next}`)?.focus();
  };

  const sectionHeading = "font-sans text-sm font-semibold tracking-normal text-text-primary";
  const listSkeleton = (
    <div aria-busy="true" className="space-y-2">
      <Skeleton className="h-12 rounded-lg" />
      <Skeleton className="h-12 rounded-lg" />
    </div>
  );

  return (
    <Drawer onOpenChange={(nextOpen) => (!nextOpen ? onClose() : undefined)} open>
      {/* Capped at the viewport so nothing pushes the drawer wider than a phone screen. */}
      <DrawerContent className="max-w-[min(640px,100vw)]" size="lg">
        <DrawerHeader className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <DrawerTitle className="break-words font-sans tracking-normal [overflow-wrap:anywhere]">
              {campaign ? campaign.name : isLoading ? "Memuat campaign…" : "Detail campaign"}
            </DrawerTitle>
            {campaign ? (
              <DrawerDescription className="mt-1">
                {campaignStatusLabel(campaign.status, columns)} · {channelMeta(campaign.channel).label}
              </DrawerDescription>
            ) : null}
          </div>
          <DrawerClose />
        </DrawerHeader>

        <DrawerBody className="space-y-5">
          {isLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-6 w-2/3 rounded-lg" />
              <Skeleton className="h-24 rounded-lg" />
              <Skeleton className="h-24 rounded-lg" />
            </div>
          ) : null}

          {!isLoading && !campaign && error ? (
            <div className="space-y-3">
              <EmptyState
                actionLabel="Coba lagi"
                description={error}
                icon={AlertTriangle}
                onAction={onRetry}
                title="Campaign tidak ditemukan atau gagal dimuat"
              />
              <div className="flex justify-center">
                <Button onClick={onClose} size="sm" type="button" variant="ghost">
                  Tutup
                </Button>
              </div>
            </div>
          ) : null}

          {campaign && canEdit ? (
            <div className="flex flex-wrap gap-2">
              <Button onClick={onEdit} size="sm" type="button" variant="outline">
                Edit campaign
              </Button>
            </div>
          ) : null}

          {campaign ? (
            // The tabs wrap rather than scroll, so every tab stays visible at
            // phone width and no scroll box clips the focus ring.
            <div
              aria-label="Bagian detail campaign"
              className="flex flex-wrap gap-x-3 border-b border-border sm:gap-x-4"
              role="tablist"
            >
              {tabs.map((tab) => {
                const isActive = activeTab === tab;
                return (
                  <button
                    aria-controls="campaign-tab-panel"
                    aria-selected={isActive}
                    className={cn(
                      "-mb-px whitespace-nowrap border-b-2 px-0.5 pb-2.5 pt-1 text-sm font-medium outline-none transition-colors duration-150 focus-visible:shadow-focus",
                      isActive
                        ? "border-mkt text-text-primary"
                        : "border-transparent text-text-secondary hover:text-text-primary",
                    )}
                    id={`campaign-tab-${tab}`}
                    key={tab}
                    onClick={() => onTabChange(tab)}
                    onKeyDown={handleTabKeyDown}
                    role="tab"
                    tabIndex={isActive ? 0 : -1}
                    type="button"
                  >
                    {detailTabLabels[tab]}
                  </button>
                );
              })}
            </div>
          ) : null}

          {campaign ? (
            <div aria-labelledby={`campaign-tab-${activeTab}`} id="campaign-tab-panel" role="tabpanel">
              {activeTab === "overview" ? (
                <div className="space-y-6">
                  <dl className="grid grid-cols-[96px_minmax(0,1fr)] items-center gap-x-4 gap-y-3 sm:grid-cols-[120px_minmax(0,1fr)]">
                    <dt className="text-sm text-text-secondary" id="campaign-detail-stage-label">Tahap</dt>
                    <dd className="min-w-0 text-sm text-text-primary">
                      {canEdit ? (
                        <Select
                          aria-busy={isChangingStage}
                          aria-labelledby="campaign-detail-stage-label"
                          className="max-w-[240px]"
                          onValueChange={onStageChange}
                          options={campaignStageOptions(columns)}
                          triggerClassName="h-9"
                          value={campaign.status}
                        />
                      ) : (
                        <StatusBadge
                          className="text-text-primary"
                          label={campaignStatusLabel(campaign.status, columns)}
                          status={campaign.status}
                          variant="campaign-status"
                        />
                      )}
                    </dd>
                    <dt className="text-sm text-text-secondary">Kanal</dt>
                    <dd className="min-w-0 text-sm text-text-primary">
                      <ChannelBadge channel={campaign.channel} />
                    </dd>
                    <dt className="text-sm text-text-secondary">Anggaran</dt>
                    <dd className="min-w-0 font-mono text-sm tabular-nums text-text-primary">{formatIDR(campaign.budget_amount)}</dd>
                    <dt className="text-sm text-text-secondary">PIC</dt>
                    <dd className={cn("min-w-0 text-sm", campaign.pic_employee_name ? "text-text-primary" : "text-text-secondary")}>
                      {campaign.pic_employee_name ?? "Belum ada PIC"}
                    </dd>
                    <dt className="text-sm text-text-secondary">Periode</dt>
                    <dd className="min-w-0 text-sm text-text-primary">{formatShortPeriod(campaign.start_date, campaign.end_date)}</dd>
                  </dl>

                  <section>
                    <h3 className={sectionHeading}>Deskripsi</h3>
                    <p className={cn("mt-2 whitespace-pre-wrap break-words text-sm leading-6", campaign.description ? "text-text-primary" : "text-text-secondary")}>
                      {campaign.description ? <LinkifiedText text={campaign.description} /> : "Belum ada deskripsi"}
                    </p>
                  </section>
                  <section>
                    <h3 className={sectionHeading}>Brief</h3>
                    <p className={cn("mt-2 whitespace-pre-wrap break-words text-sm leading-6", campaign.brief_text ? "text-text-primary" : "text-text-secondary")}>
                      {campaign.brief_text ? <LinkifiedText text={campaign.brief_text} /> : "Belum ada brief"}
                    </p>
                  </section>

                  {canDelete ? (
                    <div className="border-t border-border pt-4">
                      {/* -ml-4 cancels the button padding, so the label lines up with the content above. */}
                      <Button
                        className="-ml-4 text-error-strong hover:bg-error-light hover:text-error-strong dark:hover:bg-error-light dark:hover:text-error-strong"
                        onClick={onDelete}
                        size="sm"
                        type="button"
                        variant="ghost"
                      >
                        Hapus campaign
                      </Button>
                    </div>
                  ) : null}
                </div>
              ) : null}

              {activeTab === "attachments" ? (
                <div className="space-y-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <p className="text-sm text-text-secondary">Brief, aset desain, atau dokumen pendukung</p>
                    {canEdit ? (
                      <label
                        className={cn(
                          buttonVariants({ variant: "mkt", size: "sm" }),
                          "focus-within:shadow-focus",
                          isUploading ? "cursor-wait opacity-70" : "cursor-pointer",
                        )}
                      >
                        <Paperclip className="h-4 w-4" />
                        {isUploading ? "Mengunggah…" : "Unggah file"}
                        <input
                          className="sr-only"
                          disabled={isUploading}
                          multiple
                          onChange={(event) => {
                            const files = Array.from(event.target.files ?? []);
                            if (files.length > 0) {
                              onUpload(files);
                            }
                            event.target.value = "";
                          }}
                          type="file"
                        />
                      </label>
                    ) : null}
                  </div>

                  {attachments.length > 0 ? (
                    <ul className="divide-y divide-border rounded-xl border border-border">
                      {attachments.map((attachment) => (
                        <li className="flex flex-wrap items-center justify-between gap-3 px-4 py-3" key={attachment.id}>
                          <div className="min-w-0">
                            <p className="break-words text-sm font-semibold text-text-primary [overflow-wrap:anywhere]">{attachment.file_name}</p>
                            <p className="mt-0.5 text-[12px] text-text-secondary">
                              {formatFileSize(attachment.file_size)} · {new Date(attachment.created_at).toLocaleString("id-ID")}
                            </p>
                          </div>
                          <div className="flex gap-2">
                            <Button
                              onClick={() => {
                                openProtectedFile("campaigns", campaign.id, getProtectedFileName(attachment.file_path)).catch((error: unknown) => {
                                  toast.error("Gagal membuka lampiran", marketingErrorMessage(error));
                                });
                              }}
                              size="sm"
                              type="button"
                              variant="outline"
                            >
                              <Download className="h-4 w-4" />
                              Buka
                            </Button>
                            {canDelete ? (
                              <Button disabled={isDeletingAttachment} onClick={() => onDeleteAttachment(attachment.id)} size="sm" type="button" variant="ghost">
                                Hapus
                              </Button>
                            ) : null}
                          </div>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <EmptyState
                      className="border-border/70"
                      description={canEdit ? "Unggah brief, paket aset, atau file pendukung agar materi campaign ada di satu tempat" : "Lampiran campaign akan muncul di sini"}
                      icon={Paperclip}
                      title="Belum ada lampiran"
                    />
                  )}
                </div>
              ) : null}

              {activeTab === "metrics" ? (
                <div className="space-y-3">
                  {isLoadingMetrics ? listSkeleton : null}
                  {metrics.length > 0 ? (
                    <div className="overflow-x-auto rounded-xl border border-border">
                      <table className="w-full min-w-0 text-sm">
                        <thead className="bg-surface-muted text-left text-[12px] font-medium text-text-secondary">
                          <tr>
                            <th className="px-3 py-2 font-medium" scope="col">Platform</th>
                            <th className="hidden px-3 py-2 font-medium sm:table-cell" scope="col">Periode</th>
                            <th className="px-3 py-2 text-right font-medium" scope="col">Belanja</th>
                            <th className="px-3 py-2 text-right font-medium" scope="col">ROAS</th>
                            <th className="hidden px-3 py-2 text-right font-medium sm:table-cell" scope="col">CTR</th>
                            <th className="hidden px-3 py-2 text-right font-medium sm:table-cell" scope="col">Konversi</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                          {metrics.map((metric) => (
                            <tr key={metric.id}>
                              <td className="px-3 py-2.5 align-top">
                                <ChannelBadge channel={metric.platform} kind="platform" />
                                <PlatformMismatchNote channel={campaign.channel} platform={metric.platform} />
                                <p className="mt-1 whitespace-nowrap text-[12px] text-text-secondary sm:hidden">
                                  {formatShortPeriod(metric.period_start, metric.period_end)}
                                </p>
                              </td>
                              <td className="hidden whitespace-nowrap px-3 py-2.5 align-top text-text-secondary sm:table-cell">
                                {formatShortPeriod(metric.period_start, metric.period_end)}
                              </td>
                              <td className="whitespace-nowrap px-3 py-2.5 text-right align-top font-mono tabular-nums text-text-primary">
                                {formatIDR(metric.amount_spent)}
                              </td>
                              <td className="whitespace-nowrap px-3 py-2.5 text-right align-top font-mono tabular-nums text-text-primary">
                                {formatMetricRatio(metric.roas)}
                              </td>
                              <td className="hidden whitespace-nowrap px-3 py-2.5 text-right align-top font-mono tabular-nums text-text-primary sm:table-cell">
                                {formatMetricPercent(metric.ctr)}
                              </td>
                              <td className="hidden whitespace-nowrap px-3 py-2.5 text-right align-top font-mono tabular-nums text-text-primary sm:table-cell">
                                {metric.conversions.toLocaleString("id-ID")}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : !isLoadingMetrics ? (
                    <EmptyState
                      actionLabel={onLogMetric ? "Catat metrik" : undefined}
                      actionProps={{ size: "sm", variant: "outline" }}
                      className="border-border/70"
                      description={
                        onLogMetric
                          ? "Catat belanja dan hasil iklan pertama campaign ini di Ads Metrics"
                          : "Metrik iklan yang ditautkan ke campaign ini muncul setelah data performa pertama dicatat"
                      }
                      icon={LayoutList}
                      onAction={onLogMetric}
                      title="Belum ada metrik"
                    />
                  ) : null}
                </div>
              ) : null}

              {activeTab === "leads" ? (
                <div className="space-y-3">
                  {isLoadingLeads ? listSkeleton : null}
                  {leads.length > 0 ? (
                    <ul className="divide-y divide-border rounded-xl border border-border">
                      {leads.map((lead) => (
                        <li className="flex items-start justify-between gap-3 px-4 py-3" key={lead.id}>
                          <div className="min-w-0">
                            <p className="text-sm font-semibold text-text-primary">{lead.name}</p>
                            <p className="mt-0.5 text-[12px] text-text-secondary">
                              {lead.phone ?? lead.email ?? "Tanpa kontak"} · {lead.assigned_to_name ?? "Belum ditugaskan"}
                            </p>
                          </div>
                          <div className="flex shrink-0 flex-col items-end gap-1">
                            <StatusBadge status={lead.pipeline_status} variant="lead-status" />
                            <p className="font-mono text-[12px] tabular-nums text-text-secondary">{formatIDR(lead.estimated_value)}</p>
                          </div>
                        </li>
                      ))}
                    </ul>
                  ) : !isLoadingLeads ? (
                    <EmptyState
                      className="border-border/70"
                      description="Lead yang ditautkan akan muncul di sini setelah ada peluang dari campaign ini"
                      icon={LayoutList}
                      title="Belum ada lead terkait"
                    />
                  ) : null}
                </div>
              ) : null}

              {activeTab === "activity" ? (
                <div className="space-y-3">
                  {isLoadingActivities ? listSkeleton : null}
                  {!isLoadingActivities && activities.length === 0 ? (
                    <EmptyState
                      className="border-border/70"
                      description="Aktivitas muncul setelah campaign diubah, dipindah, atau diberi lampiran"
                      icon={LayoutList}
                      title="Belum ada aktivitas"
                    />
                  ) : null}
                  {activities.length > 0 ? (
                    <ul className="divide-y divide-border rounded-xl border border-border">
                      {activities.map((activity) => (
                        <li className="flex items-start justify-between gap-4 px-4 py-3" key={activity.id}>
                          <div className="min-w-0">
                            <p className="text-sm font-medium text-text-primary">{activity.description}</p>
                            <p className="mt-0.5 text-[12px] text-text-secondary">{activity.actor_name ?? "Sistem"}</p>
                          </div>
                          <p className="shrink-0 text-[12px] text-text-secondary">{new Date(activity.created_at).toLocaleString("id-ID")}</p>
                        </li>
                      ))}
                    </ul>
                  ) : null}
                </div>
              ) : null}
            </div>
          ) : null}
        </DrawerBody>
      </DrawerContent>
    </Drawer>
  );
}

function formatFileSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "-";
  }
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toLocaleString("id-ID", { maximumFractionDigits: 1 })} KB`;
  }
  return `${(bytes / (1024 * 1024)).toLocaleString("id-ID", { maximumFractionDigits: 1 })} MB`;
}

function toCampaignFormValues(campaign: Campaign): CampaignFormValues {
  return {
    name: campaign.name,
    description: campaign.description ?? "",
    channel: campaign.channel,
    budget_amount: campaign.budget_amount,
    budget_currency: campaign.budget_currency,
    pic_employee_id: campaign.pic_employee_id ?? "",
    start_date: extractDateInputValue(campaign.start_date),
    end_date: extractDateInputValue(campaign.end_date),
    brief_text: campaign.brief_text ?? "",
    status: campaign.status,
  };
}

async function invalidateCampaignQueries(queryClient: ReturnType<typeof useQueryClient>, campaignId?: string) {
  await queryClient.invalidateQueries({ queryKey: campaignsKeys.all });
  if (campaignId) {
    await queryClient.invalidateQueries({ queryKey: campaignsKeys.detail(campaignId) });
  }
}

import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { Download, FolderKanban, LayoutList, Paperclip, Plus } from "lucide-react";

import { CampaignForm } from "@/components/shared/campaign-form";
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
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useRBAC } from "@/hooks/use-rbac";
import {
  campaignMatchesFilters,
  channelMeta,
} from "@/lib/marketing";
import { formatIDR } from "@/lib/currency";
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
  listCampaigns,
  moveCampaign,
  updateCampaign,
  uploadCampaignAttachments,
} from "@/services/marketing-campaigns";
import { leadsKeys, listLeads } from "@/services/marketing-leads";
import { employeesKeys, listEmployees } from "@/services/hris-employees";
import type { AdsMetric, Campaign, CampaignActivity, CampaignAttachment, CampaignFilters, CampaignFormValues, Lead } from "@/types/marketing";
import type { EmployeeFilters } from "@/types/hris";

const searchSchema = z.object({
  view: z.enum(["kanban", "table"]).optional().catch("kanban"),
});

const employeeFilters: EmployeeFilters = {
  page: 1,
  perPage: 100,
  search: "",
  department: "",
  status: "",
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

  useEffect(() => {
    if (typeof window === "undefined" || search.view) {
      return;
    }

    if (window.innerWidth < 768) {
      void navigate({ replace: true, search: { view: "table" } });
    }
  }, [navigate, search.view]);

  const [filters, setFilters] = useState<CampaignFilters>(defaultFilters);
  const [isComposerOpen, setIsComposerOpen] = useState(false);
  const [editingCampaign, setEditingCampaign] = useState<Campaign | null>(null);
  const [selectedCampaignId, setSelectedCampaignId] = useState<string | null>(null);
  const [detailTab, setDetailTab] = useState<"overview" | "attachments" | "metrics" | "leads" | "activity">("overview");
  const [campaignToDelete, setCampaignToDelete] = useState<Campaign | null>(null);
  const [attachmentToDelete, setAttachmentToDelete] = useState<CampaignAttachment | null>(null);

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list(employeeFilters),
    queryFn: () => listEmployees(employeeFilters),
  });

  const campaignsQuery = useQuery({
    queryKey: campaignsKeys.list(filters),
    queryFn: () => listCampaigns(filters),
  });

  const kanbanQuery = useQuery({
    queryKey: campaignsKeys.kanban(),
    queryFn: listCampaignKanban,
  });

  const detailQuery = useQuery({
    enabled: Boolean(selectedCampaignId),
    queryKey: selectedCampaignId ? campaignsKeys.detail(selectedCampaignId) : [...campaignsKeys.all, "detail", "empty"],
    queryFn: () => getCampaign(selectedCampaignId!),
  });

  const metricsQuery = useQuery({
    enabled: Boolean(selectedCampaignId),
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
    enabled: Boolean(selectedCampaignId),
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

  const createMutation = useMutation({
    mutationFn: createCampaign,
    onSuccess: async () => {
      setIsComposerOpen(false);
      await invalidateCampaignQueries(queryClient);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ campaignId, values }: { campaignId: string; values: CampaignFormValues }) =>
      updateCampaign(campaignId, values),
    onSuccess: async (_, variables) => {
      setEditingCampaign(null);
      setSelectedCampaignId(variables.campaignId);
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteCampaign,
    onSuccess: async (_, campaignId) => {
      setCampaignToDelete(null);
      if (selectedCampaignId === campaignId) {
        setSelectedCampaignId(null);
      }
      await invalidateCampaignQueries(queryClient, campaignId);
    },
  });

  const moveMutation = useMutation({
    mutationFn: ({ campaignId, columnId, position }: { campaignId: string; columnId: string; position: number }) =>
      moveCampaign(campaignId, columnId, position),
    onSuccess: async (_, variables) => {
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
  });

  const uploadMutation = useMutation({
    mutationFn: ({ campaignId, files }: { campaignId: string; files: File[] }) =>
      uploadCampaignAttachments(campaignId, files),
    onSuccess: async (_, variables) => {
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
  });

  const deleteAttachmentMutation = useMutation({
    mutationFn: ({ campaignId, attachmentId }: { campaignId: string; attachmentId: string }) =>
      deleteCampaignAttachment(campaignId, attachmentId),
    onSuccess: async (_, variables) => {
      setAttachmentToDelete(null);
      await invalidateCampaignQueries(queryClient, variables.campaignId);
    },
  });

  const employees = employeesQuery.data?.items ?? [];
  const tableItems = useMemo(
    () => (campaignsQuery.data?.items ?? []).filter((campaign) => campaignMatchesFilters(campaign, filters)),
    [campaignsQuery.data?.items, filters],
  );
  const filteredColumns = useMemo(
    () =>
      (kanbanQuery.data ?? []).map((column) => ({
        ...column,
        campaigns: (column.campaigns ?? []).filter((campaign) => campaignMatchesFilters(campaign, filters)),
      })),
    [filters, kanbanQuery.data],
  );

  const selectedCampaign = detailQuery.data?.campaign ?? null;
  const selectedAttachments = detailQuery.data?.attachments ?? [];
  const relatedMetrics = metricsQuery.data?.items ?? [];
  const relatedLeads = relatedLeadsQuery.data?.items ?? [];
  const activities = activitiesQuery.data ?? [];
  const defaultEditValues = editingCampaign ? toCampaignFormValues(editingCampaign) : undefined;

  const tableColumns: Array<DataTableColumn<Campaign>> = [
    {
      id: "campaign",
      header: "Campaign",
      accessor: "name",
      sortable: true,
      cell: (campaign) => (
        <button className="space-y-1 text-left" onClick={() => setSelectedCampaignId(campaign.id)} type="button">
          <p className="font-semibold text-text-primary">{campaign.name}</p>
          <p className="line-clamp-1 text-sm text-text-secondary">{campaign.description || "Open detail drawer for brief and attachments."}</p>
        </button>
      ),
    },
    {
      id: "channel",
      header: "Channel",
      accessor: "channel",
      sortable: true,
      cell: (campaign) => {
        const channel = channelMeta(campaign.channel);
        const ChannelIcon = channel.icon;
        return (
          <span className={`inline-flex items-center gap-1.5 rounded-[6px] border px-2 py-0.5 text-[11px] font-[700] uppercase tracking-wider ${channel.badgeClassName}`}>
            <ChannelIcon className="h-3.5 w-3.5" />
            {channel.label}
          </span>
        );
      },
    },
    {
      id: "budget",
      header: "Budget",
      accessor: "budget_amount",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (campaign) => <span className="font-mono tabular-nums">{formatIDR(campaign.budget_amount)}</span>,
    },
    {
      id: "pic",
      header: "PIC",
      accessor: "pic_employee_name",
      sortable: true,
      cell: (campaign) => <span className="text-sm text-text-primary">{campaign.pic_employee_name ?? "Unassigned"}</span>,
    },
    {
      id: "timeline",
      header: "Timeline",
      accessor: "start_date",
      sortable: true,
      cell: (campaign) => (
        <span className="text-sm text-text-secondary">
          {new Date(campaign.start_date).toLocaleDateString("id-ID")} - {new Date(campaign.end_date).toLocaleDateString("id-ID")}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      accessor: "status",
      sortable: true,
      cell: (campaign) => <StatusBadge status={campaign.status} variant="campaign-status" />,
    },
    {
      id: "assets",
      header: "Assets",
      accessor: "attachment_count",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (campaign) => <span className="font-mono tabular-nums text-text-secondary">{campaign.attachment_count}</span>,
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (campaign) => (
        <div className="flex justify-end gap-2">
          <Button onClick={() => setSelectedCampaignId(campaign.id)} size="sm" type="button" variant="outline">
            Open
          </Button>
          {hasPermission(permissions.marketingCampaignDelete) ? (
            <Button
              disabled={deleteMutation.isPending && deleteMutation.variables === campaign.id}
              onClick={() => setCampaignToDelete(campaign)}
              size="sm"
              type="button"
              variant="ghost"
            >
              Delete
            </Button>
          ) : null}
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <Card className="p-5 sm:p-6 lg:p-7">
        <div className="flex flex-col gap-4 border-b border-border/80 pb-4 xl:flex-row xl:items-end xl:justify-between">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-mkt mb-1">Marketing workspace</p>
            <h3 className="text-[24px] font-[700] text-text-primary sm:text-[28px]">Campaigns</h3>
            <p className="mt-2 max-w-3xl text-[14px] text-text-secondary leading-relaxed">
              Kelola campaign dari board utama atau table view, pindahkan antar stage,
              dan buka drawer detail untuk brief, attachment, dan context eksekusi.
            </p>
          </div>

          <div className="flex w-full flex-col gap-3 xl:w-auto">
            <div className="grid grid-cols-2 gap-2 sm:inline-flex sm:w-auto">
              <Button className="w-full sm:w-auto" onClick={() => void navigate({ search: { view: "kanban" } })} variant={activeView === "kanban" ? "default" : "outline"}>
              <FolderKanban className="mr-2 h-4 w-4" />
              Kanban view
              </Button>
              <Button className="w-full sm:w-auto" onClick={() => void navigate({ search: { view: "table" } })} variant={activeView === "table" ? "default" : "outline"}>
              <LayoutList className="mr-2 h-4 w-4" />
              Table view
              </Button>
            </div>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-[auto_auto]">
              <PermissionGate permission={permissions.marketingCampaignView}>
                <ExportButton
                  className="w-full sm:w-auto"
                  endpoint="/marketing/campaigns/export"
                  filename="campaigns-report"
                  filters={{
                    channel: filters.channel,
                    date_from: filters.dateFrom,
                    date_to: filters.dateTo,
                    pic: filters.pic,
                    search: filters.search,
                    status: filters.status,
                  }}
                  formats={["pdf", "xlsx"]}
                />
              </PermissionGate>
              <PermissionGate permission={permissions.marketingCampaignCreate}>
                <Button className="w-full sm:w-auto" onClick={() => setIsComposerOpen(true)} variant="mkt">
                <Plus className="mr-2 h-4 w-4" />
                New campaign
                </Button>
              </PermissionGate>
            </div>
          </div>
        </div>
      </Card>

      <CampaignForm
        description="Susun campaign baru dengan channel, budget, PIC, timeline, dan brief inti agar board langsung siap dipakai."
        employees={employees}
        isOpen={isComposerOpen}
        isSubmitting={createMutation.isPending}
        onCancel={() => setIsComposerOpen(false)}
        onSubmit={(values) => createMutation.mutate(values)}
        submitLabel="Create campaign"
        title="New campaign"
      />

      {editingCampaign ? (
        <CampaignForm
          defaultValues={defaultEditValues}
          description="Perbarui detail utama campaign tanpa meninggalkan workspace marketing."
          employees={employees}
          isOpen={Boolean(editingCampaign)}
          isSubmitting={updateMutation.isPending}
          onCancel={() => setEditingCampaign(null)}
          onSubmit={(values) => updateMutation.mutate({ campaignId: editingCampaign.id, values })}
          submitLabel="Save changes"
          title={`Edit ${editingCampaign.name}`}
        />
      ) : null}

      <Card className="p-4 sm:p-5">
        <div className="grid gap-3 xl:grid-cols-[1.3fr_repeat(4,minmax(0,1fr))]">
          <Input
            onChange={(event) => setFilters((current) => ({ ...current, page: 1, search: event.target.value }))}
            placeholder="Search by campaign name"
            value={filters.search}
          />
          <select
            className="field-select"
            onChange={(event) => setFilters((current) => ({ ...current, channel: event.target.value }))}
            value={filters.channel}
          >
            <option value="">All channels</option>
            <option value="instagram">Instagram</option>
            <option value="facebook">Facebook</option>
            <option value="google_ads">Google Ads</option>
            <option value="tiktok">TikTok</option>
            <option value="youtube">YouTube</option>
            <option value="email">Email</option>
            <option value="other">Other</option>
          </select>
          <select
            className="field-select"
            onChange={(event) => setFilters((current) => ({ ...current, pic: event.target.value }))}
            value={filters.pic}
          >
            <option value="">All PIC</option>
            {employees.map((employee) => (
              <option key={employee.id} value={employee.id}>
                {employee.full_name}
              </option>
            ))}
          </select>
          <Input onChange={(event) => setFilters((current) => ({ ...current, dateFrom: event.target.value }))} type="date" value={filters.dateFrom} />
          <Input onChange={(event) => setFilters((current) => ({ ...current, dateTo: event.target.value }))} type="date" value={filters.dateTo} />
        </div>
      </Card>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <SummaryCard label="Campaigns on page" value={String(tableItems.length)} />
        <SummaryCard label="Live campaigns" value={String(tableItems.filter((campaign) => campaign.status === "live").length)} />
        <SummaryCard label="Tracked budget" value={formatIDR(tableItems.reduce((total, campaign) => total + campaign.budget_amount, 0))} />
      </div>

      {campaignsQuery.error instanceof Error ? <Card className="p-6 text-sm text-error">{campaignsQuery.error.message}</Card> : null}
      {kanbanQuery.error instanceof Error ? <Card className="p-6 text-sm text-error">{kanbanQuery.error.message}</Card> : null}

      {activeView === "kanban" ? (
        <PermissionGate permission={permissions.marketingCampaignView}>
          <MarketingCampaignBoard
            columns={filteredColumns}
            onCampaignOpen={(campaign) => {
              setDetailTab("overview");
              setSelectedCampaignId(campaign.id);
            }}
            onMoveCampaign={(campaignId, columnId, position) =>
              moveMutation.mutateAsync({ campaignId, columnId, position }).then(() => undefined)
            }
          />
        </PermissionGate>
      ) : (
        <DataTable
          columns={tableColumns}
          data={tableItems}
          emptyDescription="No campaigns match the current filters."
          emptyTitle="No campaigns found"
          getRowId={(campaign) => campaign.id}
          loading={campaignsQuery.isLoading}
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
      )}

      {activeView === "kanban" ? (
        <Card className="flex flex-col gap-4 p-6 md:flex-row md:items-center md:justify-between">
          <p className="text-sm text-muted-foreground">
            Page {campaignsQuery.data?.meta.page ?? filters.page} of{" "}
            {campaignsQuery.data?.meta ? Math.max(1, Math.ceil(campaignsQuery.data.meta.total / campaignsQuery.data.meta.per_page)) : 1} | Total{" "}
            {campaignsQuery.data?.meta.total ?? 0} campaigns
          </p>
          <div className="flex gap-3">
            <Button disabled={(campaignsQuery.data?.meta.page ?? filters.page) <= 1} onClick={() => setFilters((current) => ({ ...current, page: Math.max(1, current.page - 1) }))} variant="outline">
              Previous
            </Button>
            <Button
              disabled={campaignsQuery.data?.meta ? campaignsQuery.data.meta.page * campaignsQuery.data.meta.per_page >= campaignsQuery.data.meta.total : true}
              onClick={() => setFilters((current) => ({ ...current, page: current.page + 1 }))}
              variant="outline"
            >
              Next
            </Button>
          </div>
        </Card>
      ) : null}

      {selectedCampaignId ? (
        <CampaignDetailDrawer
          attachments={selectedAttachments}
          campaign={selectedCampaign}
          canDelete={hasPermission(permissions.marketingCampaignDelete)}
          detailTab={detailTab}
          activities={activities}
          leads={relatedLeads}
          metrics={relatedMetrics}
          isLoadingActivities={activitiesQuery.isLoading}
          isDeletingAttachment={deleteAttachmentMutation.isPending}
          isLoading={detailQuery.isLoading}
          isLoadingLeads={relatedLeadsQuery.isLoading}
          isLoadingMetrics={metricsQuery.isLoading}
          isUploading={uploadMutation.isPending}
          onClose={() => setSelectedCampaignId(null)}
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
              setEditingCampaign(selectedCampaign);
              setSelectedCampaignId(null);
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
        confirmLabel="Delete campaign"
        description={campaignToDelete ? `Campaign "${campaignToDelete.name}" and its workflow history will be removed.` : ""}
        isLoading={deleteMutation.isPending}
        isOpen={Boolean(campaignToDelete)}
        onClose={() => setCampaignToDelete(null)}
        onConfirm={() => {
          if (campaignToDelete) {
            deleteMutation.mutate(campaignToDelete.id);
          }
        }}
        title={campaignToDelete ? `Delete ${campaignToDelete.name}?` : "Delete campaign?"}
      />

      <ConfirmDialog
        confirmLabel="Delete attachment"
        description={attachmentToDelete ? `Attachment "${attachmentToDelete.file_name}" will be removed from this campaign.` : ""}
        isLoading={deleteAttachmentMutation.isPending}
        isOpen={Boolean(attachmentToDelete)}
        onClose={() => setAttachmentToDelete(null)}
        onConfirm={() => {
          if (selectedCampaign && attachmentToDelete) {
            deleteAttachmentMutation.mutate({ attachmentId: attachmentToDelete.id, campaignId: selectedCampaign.id });
          }
        }}
        title={attachmentToDelete ? "Delete attachment?" : "Delete attachment?"}
      />
    </div>
  );
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-5 border border-border shadow-sm bg-surface">
      <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">{label}</p>
      <p className="mt-2 text-[24px] font-[700] text-text-primary leading-none">{value}</p>
    </Card>
  );
}

function CampaignDetailDrawer({
  campaign,
  attachments,
  metrics,
  leads,
  activities,
  detailTab,
  canDelete,
  isLoading,
  isLoadingMetrics,
  isLoadingLeads,
  isLoadingActivities,
  isUploading,
  isDeletingAttachment,
  onClose,
  onEdit,
  onDelete,
  onTabChange,
  onUpload,
  onDeleteAttachment,
}: {
  campaign: Campaign | null;
  attachments: CampaignAttachment[];
  metrics: AdsMetric[];
  leads: Lead[];
  activities: CampaignActivity[];
  detailTab: "overview" | "attachments" | "metrics" | "leads" | "activity";
  canDelete: boolean;
  isLoading: boolean;
  isLoadingMetrics: boolean;
  isLoadingLeads: boolean;
  isLoadingActivities: boolean;
  isUploading: boolean;
  isDeletingAttachment: boolean;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onTabChange: (tab: "overview" | "attachments" | "metrics" | "leads" | "activity") => void;
  onUpload: (files: File[]) => void;
  onDeleteAttachment: (attachmentId: string) => void;
}) {
  return (
    <Drawer onOpenChange={(nextOpen) => (!nextOpen ? onClose() : undefined)} open={Boolean(campaign) || isLoading}>
      <DrawerContent size="lg">
        <DrawerHeader className="flex items-start justify-between gap-4">
          <div>
            <DrawerTitle>{campaign ? campaign.name : isLoading ? "Loading campaign..." : "Campaign detail"}</DrawerTitle>
            <DrawerDescription>
              Review campaign scope, attached assets, related metrics, leads, and movement history without leaving the board.
            </DrawerDescription>
          </div>
          <DrawerClose />
        </DrawerHeader>

        <DrawerBody className="space-y-6">
          {campaign ? (
            <div className="flex flex-wrap gap-3">
              <Button onClick={onEdit} size="sm" type="button" variant="outline">
                Edit campaign
              </Button>
              {canDelete ? (
                <Button onClick={onDelete} size="sm" type="button" variant="ghost">
                  Delete campaign
                </Button>
              ) : null}
            </div>
          ) : null}

          <div className="flex flex-wrap gap-3">
            {(["overview", "attachments", "metrics", "leads", "activity"] as const).map((tab) => (
              <Button key={tab} onClick={() => onTabChange(tab)} size="sm" type="button" variant={detailTab === tab ? "default" : "outline"}>
                {tab}
              </Button>
            ))}
          </div>

          {isLoading ? (
            <div className="space-y-4">
              <Skeleton className="h-20 rounded-lg" />
              <Skeleton className="h-20 rounded-lg" />
              <Skeleton className="h-20 rounded-lg" />
            </div>
          ) : null}

          {campaign && detailTab === "overview" ? (
            <div className="space-y-4">
              <InfoRow label="Channel" value={channelMeta(campaign.channel).label} />
              <InfoRow label="Budget" value={formatIDR(campaign.budget_amount)} />
              <InfoRow label="PIC" value={campaign.pic_employee_name ?? "Unassigned"} />
              <InfoRow label="Status" value={<StatusBadge status={campaign.status} variant="campaign-status" />} />
              <InfoRow label="Timeline" value={`${new Date(campaign.start_date).toLocaleDateString()} - ${new Date(campaign.end_date).toLocaleDateString()}`} />
              <Card className="p-4">
                <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Description</p>
                <p className="mt-3 text-sm text-muted-foreground">{campaign.description || "No campaign description yet."}</p>
              </Card>
              <Card className="p-4">
                <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Brief</p>
                <p className="mt-3 whitespace-pre-wrap text-sm text-muted-foreground">{campaign.brief_text || "No campaign brief yet."}</p>
              </Card>
            </div>
          ) : null}

          {campaign && detailTab === "attachments" ? (
            <div className="space-y-4">
              <Card className="p-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Attachments</p>
                    <p className="mt-2 text-sm text-muted-foreground">Upload brief, design assets, or supporting documents.</p>
                  </div>
                  <label className="inline-flex cursor-pointer items-center gap-2 rounded-full bg-primary px-4 py-2 text-sm font-medium text-primary-foreground">
                    <Paperclip className="h-4 w-4" />
                    {isUploading ? "Uploading..." : "Upload files"}
                    <input
                      className="hidden"
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
                </div>
              </Card>

              {attachments.length > 0 ? (
                attachments.map((attachment) => (
                  <Card className="p-4" key={attachment.id}>
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="font-semibold">{attachment.file_name}</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {attachment.file_type} | {new Date(attachment.created_at).toLocaleString()}
                        </p>
                      </div>
                      <div className="flex gap-3">
                        <button
                          className="inline-flex items-center gap-2 rounded-full border border-border px-3 py-1.5 text-sm font-medium transition hover:bg-muted"
                          onClick={() => void openProtectedFile("campaigns", campaign.id, getProtectedFileName(attachment.file_path))}
                          type="button"
                        >
                          <Download className="h-4 w-4" />
                          Open
                        </button>
                        <Button disabled={isDeletingAttachment} onClick={() => onDeleteAttachment(attachment.id)} size="sm" type="button" variant="ghost">
                          Delete
                        </Button>
                      </div>
                    </div>
                  </Card>
                ))
              ) : (
                <EmptyState
                  className="border-border/70"
                  description="Upload a brief, asset pack, or supporting file to keep campaign materials in one place."
                  icon={Paperclip}
                  title="No attachments yet"
                />
              )}
            </div>
          ) : null}

          {campaign && detailTab === "metrics" ? (
            <div className="space-y-4">
              <Card className="p-4">
                <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Related ads metrics</p>
                <p className="mt-2 text-sm text-muted-foreground">Ringkasan performa terbaru untuk campaign ini.</p>
              </Card>
              {isLoadingMetrics ? <Card className="p-6 text-sm text-muted-foreground">Loading metrics...</Card> : null}
              {metrics.length > 0 ? (
                metrics.map((metric) => (
                  <Card className="p-4" key={metric.id}>
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="font-semibold">{metric.platform.replaceAll("_", " ")}</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {new Date(metric.period_start).toLocaleDateString("id-ID")} - {new Date(metric.period_end).toLocaleDateString("id-ID")}
                        </p>
                      </div>
                      <div className="text-right text-sm">
                        <p className="font-semibold">{formatIDR(metric.amount_spent)}</p>
                        <p className="text-muted-foreground">ROAS {formatMetricRatio(metric.roas)}</p>
                      </div>
                    </div>
                    <div className="mt-4 grid gap-3 md:grid-cols-3">
                      <MiniMetric label="Revenue" value={formatIDR(metric.revenue)} />
                      <MiniMetric label="CTR" value={formatMetricPercent(metric.ctr)} />
                      <MiniMetric label="Conversions" value={metric.conversions.toLocaleString("id-ID")} />
                    </div>
                  </Card>
                ))
              ) : (
                <EmptyState
                  className="border-border/70"
                  description="Ads metrics linked to this campaign will appear here after the first performance entry is recorded."
                  icon={LayoutList}
                  title="No metrics linked yet"
                />
              )}
            </div>
          ) : null}

          {campaign && detailTab === "leads" ? (
            <div className="space-y-4">
              <Card className="p-4">
                <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Related leads</p>
                <p className="mt-2 text-sm text-muted-foreground">Lead yang sudah ditautkan ke campaign ini akan muncul di sini.</p>
              </Card>
              {isLoadingLeads ? <Card className="p-6 text-sm text-muted-foreground">Loading leads...</Card> : null}
              {leads.length > 0 ? (
                leads.map((lead) => (
                  <Card className="p-4" key={lead.id}>
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="font-semibold">{lead.name}</p>
                        <p className="mt-1 text-xs text-muted-foreground">{lead.phone ?? lead.email ?? "No contact"}</p>
                      </div>
                      <div className="text-right">
                        <StatusBadge status={lead.pipeline_status} variant="lead-status" />
                        <p className="text-xs text-muted-foreground">{formatIDR(lead.estimated_value)}</p>
                      </div>
                    </div>
                    <div className="mt-3 flex items-center justify-between gap-3 text-xs text-muted-foreground">
                      <span>{lead.assigned_to_name ?? "Unassigned"}</span>
                      <span>{new Date(lead.updated_at).toLocaleDateString("id-ID")}</span>
                    </div>
                  </Card>
                ))
              ) : (
                <EmptyState
                  className="border-border/70"
                  description="Linked leads will appear here once campaign-attributed opportunities start coming in."
                  icon={LayoutList}
                  title="No linked leads"
                />
              )}
            </div>
          ) : null}

          {campaign && detailTab === "activity" ? (
            <div className="space-y-4">
              <Card className="p-4">
                <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Campaign activity</p>
                <p className="mt-2 text-sm text-muted-foreground">Recent movement and asset upload activity for this campaign.</p>
              </Card>
              {isLoadingActivities ? <Card className="p-6 text-sm text-muted-foreground">Loading activity...</Card> : null}
              {!isLoadingActivities && activities.length === 0 ? (
                <EmptyState
                  className="border-border/70"
                  description="Activity will appear here after the campaign is moved or attachments are uploaded."
                  icon={LayoutList}
                  title="No activity yet"
                />
              ) : null}
              {activities.map((activity) => (
                <Card className="p-4" key={activity.id}>
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <p className="font-semibold text-text-primary">{activity.description}</p>
                      <p className="mt-1 text-sm text-text-secondary">{activity.actor_name ?? "System"}</p>
                    </div>
                    <p className="text-xs text-text-secondary">{new Date(activity.created_at).toLocaleString("id-ID")}</p>
                  </div>
                </Card>
              ))}
            </div>
          ) : null}
        </DrawerBody>
      </DrawerContent>
    </Drawer>
  );
}

function InfoRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-[22px] border border-border/70 bg-background/80 px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <div className="text-sm font-semibold capitalize">{value}</div>
    </div>
  );
}

function MiniMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[18px] border border-border/70 bg-background/80 px-4 py-3">
      <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">{label}</p>
      <p className="mt-2 text-sm font-semibold">{value}</p>
    </div>
  );
}

function toCampaignFormValues(campaign: Campaign): CampaignFormValues {
  return {
    name: campaign.name,
    description: campaign.description ?? "",
    channel: campaign.channel,
    budget_amount: campaign.budget_amount,
    budget_currency: campaign.budget_currency,
    pic_employee_id: campaign.pic_employee_id ?? "",
    start_date: campaign.start_date.slice(0, 10),
    end_date: campaign.end_date.slice(0, 10),
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



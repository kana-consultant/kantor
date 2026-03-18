import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { z } from "zod";
import { Download, FolderKanban, LayoutList, Paperclip, Plus } from "lucide-react";

import { CampaignForm } from "@/components/shared/campaign-form";
import { MarketingCampaignBoard } from "@/components/shared/marketing-campaign-board";
import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useRBAC } from "@/hooks/use-rbac";
import {
  campaignMatchesFilters,
  channelMeta,
  formatCampaignStatus,
  uploadsURL,
} from "@/lib/marketing";
import { formatIDR } from "@/lib/currency";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
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
  listCampaignKanban,
  listCampaigns,
  moveCampaign,
  updateCampaign,
  uploadCampaignAttachments,
} from "@/services/marketing-campaigns";
import { leadsKeys, listLeads } from "@/services/marketing-leads";
import { employeesKeys, listEmployees } from "@/services/hris-employees";
import type { AdsMetric, Campaign, CampaignAttachment, CampaignFilters, CampaignFormValues, Lead } from "@/types/marketing";
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
    await ensurePermission(permissions.marketingCampaignView);
  },
  component: MarketingCampaignsPage,
});

function MarketingCampaignsPage() {
  const search = Route.useSearch();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const activeView = search.view ?? "kanban";

  const [filters, setFilters] = useState<CampaignFilters>(defaultFilters);
  const [isComposerOpen, setIsComposerOpen] = useState(false);
  const [editingCampaign, setEditingCampaign] = useState<Campaign | null>(null);
  const [selectedCampaignId, setSelectedCampaignId] = useState<string | null>(null);
  const [detailTab, setDetailTab] = useState<"overview" | "attachments" | "metrics" | "leads" | "activity">("overview");

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
  const defaultEditValues = editingCampaign ? toCampaignFormValues(editingCampaign) : undefined;

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between border-b border-border pb-4">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-mkt mb-1">Marketing workspace</p>
            <h3 className="text-[28px] font-[700] text-text-primary">Campaigns</h3>
            <p className="mt-2 max-w-3xl text-[14px] text-text-secondary leading-relaxed">
              Kelola campaign dari board utama atau table view, pindahkan antar stage,
              dan buka drawer detail untuk brief, attachment, dan context eksekusi.
            </p>
          </div>

          <div className="flex flex-wrap gap-3">
            <Button onClick={() => void navigate({ search: { view: "kanban" } })} variant={activeView === "kanban" ? "default" : "outline"}>
              <FolderKanban className="mr-2 h-4 w-4" />
              Kanban view
            </Button>
            <Button onClick={() => void navigate({ search: { view: "table" } })} variant={activeView === "table" ? "default" : "outline"}>
              <LayoutList className="mr-2 h-4 w-4" />
              Table view
            </Button>
            <PermissionGate permission={permissions.marketingCampaignCreate}>
              <Button onClick={() => setIsComposerOpen((value) => !value)} variant="mkt">
                <Plus className="mr-2 h-4 w-4" />
                {isComposerOpen ? "Close composer" : "New campaign"}
              </Button>
            </PermissionGate>
          </div>
        </div>
      </Card>

      {isComposerOpen ? (
        <CampaignForm
          description="Susun campaign baru dengan channel, budget, PIC, timeline, dan brief inti agar board langsung siap dipakai."
          employees={employees}
          isSubmitting={createMutation.isPending}
          onCancel={() => setIsComposerOpen(false)}
          onSubmit={(values) => createMutation.mutate(values)}
          submitLabel="Create campaign"
          title="New campaign"
        />
      ) : null}

      {editingCampaign ? (
        <CampaignForm
          defaultValues={defaultEditValues}
          description="Perbarui detail utama campaign tanpa meninggalkan workspace marketing."
          employees={employees}
          isSubmitting={updateMutation.isPending}
          onCancel={() => setEditingCampaign(null)}
          onSubmit={(values) => updateMutation.mutate({ campaignId: editingCampaign.id, values })}
          submitLabel="Save changes"
          title={`Edit ${editingCampaign.name}`}
        />
      ) : null}

      <Card className="p-6">
        <div className="grid gap-4 xl:grid-cols-[1.3fr_repeat(4,minmax(0,1fr))]">
          <Input
            onChange={(event) => setFilters((current) => ({ ...current, page: 1, search: event.target.value }))}
            placeholder="Search by campaign name"
            value={filters.search}
          />
          <select
            className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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
            className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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

      <div className="grid gap-4 md:grid-cols-3">
        <SummaryCard label="Campaigns on page" value={String(tableItems.length)} />
        <SummaryCard label="Live campaigns" value={String(tableItems.filter((campaign) => campaign.status === "live").length)} />
        <SummaryCard label="Tracked budget" value={formatIDR(tableItems.reduce((total, campaign) => total + campaign.budget_amount, 0))} />
      </div>

      {campaignsQuery.error instanceof Error ? <Card className="p-6 text-sm text-red-700">{campaignsQuery.error.message}</Card> : null}
      {kanbanQuery.error instanceof Error ? <Card className="p-6 text-sm text-red-700">{kanbanQuery.error.message}</Card> : null}

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
      ) : campaignsQuery.isLoading ? (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-[14px]">
              <thead className="border-b border-border bg-surface-muted text-[12px] font-[600] text-text-tertiary uppercase tracking-wider">
                <tr>
                  <th className="px-5 py-4 font-medium h-[45px]">Campaign</th>
                  <th className="px-5 py-4 font-medium h-[45px]">Channel</th>
                  <th className="px-5 py-4 font-medium h-[45px]">Budget</th>
                  <th className="px-5 py-4 font-medium h-[45px]">PIC</th>
                  <th className="px-5 py-4 font-medium h-[45px]">Timeline</th>
                  <th className="px-5 py-4 font-medium h-[45px]">Status</th>
                  <th className="px-5 py-4 font-medium h-[45px]">Assets</th>
                  <th className="px-5 py-4 font-medium h-[45px]">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border bg-surface">
                {[1, 2, 3, 4, 5].map((i) => (
                  <tr className="transition-colors hover:bg-surface-muted/50 group" key={i}>
                    <td className="px-5 py-4 space-y-2">
                       <Skeleton className="h-4 w-[160px] bg-muted/60" />
                       <Skeleton className="h-3 w-[200px] bg-muted/60" />
                    </td>
                    <td className="px-5 py-4">
                       <Skeleton className="h-6 w-[80px] rounded-full bg-muted/60" />
                    </td>
                    <td className="px-5 py-4">
                       <Skeleton className="h-4 w-[100px] bg-muted/60" />
                    </td>
                    <td className="px-5 py-4">
                       <Skeleton className="h-4 w-[90px] bg-muted/60" />
                    </td>
                    <td className="px-5 py-4">
                       <Skeleton className="h-4 w-[150px] bg-muted/60" />
                    </td>
                    <td className="px-5 py-4">
                       <Skeleton className="h-5 w-[70px] rounded-[6px] bg-muted/60" />
                    </td>
                    <td className="px-5 py-4">
                       <Skeleton className="h-4 w-[20px] bg-muted/60" />
                    </td>
                    <td className="px-5 py-4 flex gap-2">
                       <Skeleton className="h-8 w-[60px] rounded-[6px] bg-muted/60" />
                       <Skeleton className="h-8 w-[60px] rounded-[6px] bg-muted/60" />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      ) : (
        <CampaignsTable
          campaigns={tableItems}
          canDelete={hasPermission(permissions.marketingCampaignDelete)}
          deletingId={deleteMutation.isPending ? deleteMutation.variables ?? null : null}
          onDelete={(campaignId) => deleteMutation.mutate(campaignId)}
          onOpen={(campaign) => {
            setDetailTab("overview");
            setSelectedCampaignId(campaign.id);
          }}
        />
      )}

      <Card className="flex flex-col gap-4 p-6 md:flex-row md:items-center md:justify-between">
        <p className="text-sm text-muted-foreground">
          Page {campaignsQuery.data?.meta.page ?? filters.page} of{" "}
          {campaignsQuery.data?.meta ? Math.max(1, Math.ceil(campaignsQuery.data.meta.total / campaignsQuery.data.meta.per_page)) : 1} · Total{" "}
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

      {selectedCampaignId ? (
        <CampaignDetailDrawer
          attachments={selectedAttachments}
          campaign={selectedCampaign}
          canDelete={hasPermission(permissions.marketingCampaignDelete)}
          detailTab={detailTab}
          leads={relatedLeads}
          metrics={relatedMetrics}
          isDeletingAttachment={deleteAttachmentMutation.isPending}
          isLoading={detailQuery.isLoading}
          isLoadingLeads={relatedLeadsQuery.isLoading}
          isLoadingMetrics={metricsQuery.isLoading}
          isUploading={uploadMutation.isPending}
          onClose={() => setSelectedCampaignId(null)}
          onDelete={() => {
            if (selectedCampaign && window.confirm(`Delete "${selectedCampaign.name}"?`)) {
              deleteMutation.mutate(selectedCampaign.id);
            }
          }}
          onDeleteAttachment={(attachmentId) => {
            if (selectedCampaign) {
              deleteAttachmentMutation.mutate({ attachmentId, campaignId: selectedCampaign.id });
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

function CampaignsTable({
  campaigns,
  canDelete,
  deletingId,
  onOpen,
  onDelete,
}: {
  campaigns: Campaign[];
  canDelete: boolean;
  deletingId: string | null;
  onOpen: (campaign: Campaign) => void;
  onDelete: (campaignId: string) => void;
}) {
  return (
    <Card className="overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-[14px]">
          <thead className="border-b border-border bg-surface-muted text-[12px] font-[600] text-text-tertiary uppercase tracking-wider">
            <tr>
              <th className="px-5 py-4 font-medium">Campaign</th>
              <th className="px-5 py-4 font-medium">Channel</th>
              <th className="px-5 py-4 font-medium">Budget</th>
              <th className="px-5 py-4 font-medium">PIC</th>
              <th className="px-5 py-4 font-medium">Timeline</th>
              <th className="px-5 py-4 font-medium">Status</th>
              <th className="px-5 py-4 font-medium">Assets</th>
              <th className="px-5 py-4 font-medium">Action</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border bg-surface">
            {campaigns.map((campaign) => {
              const channel = channelMeta(campaign.channel);
              const ChannelIcon = channel.icon;
              return (
                <tr className="transition-colors hover:bg-surface-muted/50 group" key={campaign.id}>
                  <td className="px-5 py-4 align-top">
                    <button className="text-left" onClick={() => onOpen(campaign)} type="button">
                      <p className="font-[600] text-[15px] text-text-primary group-hover:text-mkt transition-colors">{campaign.name}</p>
                      <p className="mt-1 line-clamp-1 text-[13px] text-text-secondary w-[16rem]">{campaign.description || "Open detail drawer for attachments and brief."}</p>
                    </button>
                  </td>
                  <td className="px-5 py-4 align-top">
                    <span className={`inline-flex items-center gap-1.5 rounded-[6px] border px-2 py-0.5 text-[11px] font-[700] uppercase tracking-wider ${channel.badgeClassName}`}>
                      <ChannelIcon className="h-3.5 w-3.5" />
                      {channel.label}
                    </span>
                  </td>
                  <td className="px-5 py-4 align-top font-[600] text-[14px] text-text-primary">{formatIDR(campaign.budget_amount)}</td>
                  <td className="px-5 py-4 align-top text-[13px] font-[500] text-text-primary">{campaign.pic_employee_name ?? "Unassigned"}</td>
                  <td className="px-5 py-4 align-top text-[13px] text-text-secondary">{new Date(campaign.start_date).toLocaleDateString()} - {new Date(campaign.end_date).toLocaleDateString()}</td>
                  <td className="px-5 py-4 align-top">
                    <span className="rounded-[6px] border border-border bg-surface-muted px-2 py-0.5 text-[10px] font-[700] uppercase tracking-wider text-text-secondary">
                      {formatCampaignStatus(campaign.status)}
                    </span>
                  </td>
                  <td className="px-5 py-4 align-top text-[13px] font-[500] text-text-primary">{campaign.attachment_count}</td>
                  <td className="px-5 py-4 align-top">
                    <div className="flex gap-2">
                      <Button onClick={() => onOpen(campaign)} size="sm" variant="outline">
                        Open
                      </Button>
                      {canDelete ? (
                        <Button disabled={deletingId === campaign.id} onClick={() => onDelete(campaign.id)} size="sm" variant="ghost">
                           {deletingId === campaign.id ? "Deleting..." : "Delete"}
                        </Button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              );
            })}
            {campaigns.length === 0 ? (
              <tr>
                <td className="px-5 py-10 text-center text-muted-foreground" colSpan={8}>
                  No campaigns found for the current filter.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

function CampaignDetailDrawer({
  campaign,
  attachments,
  metrics,
  leads,
  detailTab,
  canDelete,
  isLoading,
  isLoadingMetrics,
  isLoadingLeads,
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
  detailTab: "overview" | "attachments" | "metrics" | "leads" | "activity";
  canDelete: boolean;
  isLoading: boolean;
  isLoadingMetrics: boolean;
  isLoadingLeads: boolean;
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
    <div className="fixed inset-0 z-50 bg-foreground/25 backdrop-blur-sm">
      <button aria-label="Close campaign drawer" className="absolute inset-0 h-full w-full cursor-default" onClick={onClose} type="button" />
      <Card className="absolute inset-y-0 right-0 z-10 flex w-full max-w-2xl flex-col rounded-none border-l border-border/80 p-6 shadow-2xl">
        <div className="flex items-start justify-between gap-4 border-b border-border/70 pb-5">
          <div>
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Campaign detail</p>
            <h4 className="mt-2 text-2xl font-bold">{campaign ? campaign.name : isLoading ? "Loading..." : "Campaign"}</h4>
          </div>
          <Button onClick={onClose} size="sm" variant="outline">
            Close
          </Button>
        </div>

        {campaign ? (
          <div className="mt-5 flex flex-wrap gap-3">
            <Button onClick={onEdit} size="sm" variant="outline">
              Edit campaign
            </Button>
            {canDelete ? (
              <Button onClick={onDelete} size="sm" variant="ghost">
                Delete campaign
              </Button>
            ) : null}
          </div>
        ) : null}

        <div className="mt-5 flex flex-wrap gap-3">
          {(["overview", "attachments", "metrics", "leads", "activity"] as const).map((tab) => (
            <Button key={tab} onClick={() => onTabChange(tab)} size="sm" variant={detailTab === tab ? "default" : "outline"}>
              {tab}
            </Button>
          ))}
        </div>

        <div className="mt-6 flex-1 overflow-y-auto pr-1">
          {isLoading ? <p className="text-sm text-muted-foreground">Loading campaign detail...</p> : null}

          {campaign && detailTab === "overview" ? (
            <div className="space-y-4">
              <InfoRow label="Channel" value={channelMeta(campaign.channel).label} />
              <InfoRow label="Budget" value={formatIDR(campaign.budget_amount)} />
              <InfoRow label="PIC" value={campaign.pic_employee_name ?? "Unassigned"} />
              <InfoRow label="Status" value={formatCampaignStatus(campaign.status)} />
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
                        <p className="mt-1 text-xs text-muted-foreground">{attachment.file_type} · {new Date(attachment.created_at).toLocaleString()}</p>
                      </div>
                      <div className="flex gap-3">
                        <a className="inline-flex items-center gap-2 rounded-full border border-border px-3 py-1.5 text-sm font-medium transition hover:bg-muted" href={uploadsURL(attachment.file_path)} rel="noreferrer" target="_blank">
                          <Download className="h-4 w-4" />
                          Open
                        </a>
                        <Button disabled={isDeletingAttachment} onClick={() => onDeleteAttachment(attachment.id)} size="sm" variant="ghost">
                          Delete
                        </Button>
                      </div>
                    </div>
                  </Card>
                ))
              ) : (
                <Card className="p-6 text-sm text-muted-foreground">No attachments yet.</Card>
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
                <Card className="p-6 text-sm text-muted-foreground">Belum ada ads metrics yang dikaitkan ke campaign ini.</Card>
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
                        <p className="text-sm font-semibold">{formatCampaignStatusLabel(lead.pipeline_status)}</p>
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
                <Card className="p-6 text-sm text-muted-foreground">Belum ada lead yang terhubung ke campaign ini.</Card>
              )}
            </div>
          ) : null}

          {campaign && detailTab === "activity" ? (
            <Card className="p-6 text-sm text-muted-foreground">
              Activity log akan dipakai untuk history perpindahan stage dan update campaign. Untuk step ini, audit visual masih placeholder.
            </Card>
          ) : null}
        </div>
      </Card>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-[22px] border border-border/70 bg-background/80 px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-semibold capitalize">{value}</span>
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

function formatCampaignStatusLabel(value: string) {
  return value.replaceAll("_", " ");
}

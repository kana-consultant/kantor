import type { ReactNode } from "react";
import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import Papa from "papaparse";
import { Controller, useForm } from "react-hook-form";
import { Download, FolderKanban, LayoutList, Plus } from "lucide-react";
import { z } from "zod";

import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { EmptyState } from "@/components/shared/empty-state";
import { MarketingLeadsPipeline } from "@/components/shared/marketing-leads-pipeline";
import { PermissionGate } from "@/components/shared/permission-gate";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { formatIDR } from "@/lib/currency";
import { formatLeadStatus, leadSourceMeta, leadSourceOptions, leadStatusOptions } from "@/lib/marketing";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import {
  createLead,
  createLeadActivity,
  deleteLead,
  getLead,
  getLeadSummary,
  importLeadsCSV,
  leadsKeys,
  listLeadActivities,
  listLeadPipeline,
  listLeads,
  moveLeadStatus,
  updateLead,
} from "@/services/marketing-leads";
import { campaignsKeys, listCampaigns } from "@/services/marketing-campaigns";
import { employeesKeys, listEmployees } from "@/services/hris-employees";
import type { EmployeeFilters } from "@/types/hris";
import type { Lead, LeadFilters, LeadFormValues, LeadImportSummary } from "@/types/marketing";

const leadSchema = z.object({
  name: z.string().trim().min(2).max(180),
  phone: z.string().trim().regex(/^(\+62|08)\d{8,13}$/).or(z.literal("")),
  email: z.string().trim().email().or(z.literal("")),
  source_channel: z.enum(["whatsapp", "email", "instagram", "facebook", "website", "referral", "other"]),
  pipeline_status: z.enum(["new", "contacted", "qualified", "proposal", "negotiation", "won", "lost"]),
  campaign_id: z.string(),
  assigned_to: z.string(),
  notes: z.string(),
  company_name: z.string(),
  estimated_value: z.number().min(0),
}).refine((value) => value.phone !== "" || value.email !== "", {
  message: "Phone or email is required",
  path: ["phone"],
});

const defaultLeadForm: LeadFormValues = {
  name: "",
  phone: "",
  email: "",
  source_channel: "whatsapp",
  pipeline_status: "new",
  campaign_id: "",
  assigned_to: "",
  notes: "",
  company_name: "",
  estimated_value: 0,
};

const defaultFilters: LeadFilters = {
  page: 1,
  perPage: 20,
  pipelineStatus: "",
  sourceChannel: "",
  campaignId: "",
  assignedTo: "",
  dateFrom: "",
  dateTo: "",
  search: "",
};

const employeeFilters: EmployeeFilters = {
  page: 1,
  perPage: 100,
  search: "",
  department: "",
  status: "",
};

const leadImportHeaders = [
  "name",
  "phone",
  "email",
  "source_channel",
  "pipeline_status",
  "assigned_to",
  "notes",
  "company_name",
  "estimated_value",
] as const;

type LeadImportPreviewRow = {
  id: string;
  rowNumber: number;
  name: string;
  phone: string;
  email: string;
  source_channel: string;
  pipeline_status: string;
  assigned_to: string;
  notes: string;
  company_name: string;
  estimated_value: string;
  errors: string[];
};

export const Route = createFileRoute("/_authenticated/marketing/leads")({
  beforeLoad: async () => {
    await ensurePermission(permissions.marketingLeadsView);
  },
  component: MarketingLeadsPage,
});

function MarketingLeadsPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [activeView, setActiveView] = useState<"pipeline" | "table">("pipeline");
  const [filters, setFilters] = useState<LeadFilters>(defaultFilters);
  const [showForm, setShowForm] = useState(false);
  const [showImport, setShowImport] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importPreviewRows, setImportPreviewRows] = useState<LeadImportPreviewRow[]>([]);
  const [importPreviewError, setImportPreviewError] = useState<string | null>(null);
  const [importResult, setImportResult] = useState<LeadImportSummary | null>(null);
  const [isParsingImport, setIsParsingImport] = useState(false);
  const [editingLead, setEditingLead] = useState<Lead | null>(null);
  const [selectedLeadId, setSelectedLeadId] = useState<string | null>(null);
  const [activityNote, setActivityNote] = useState("");

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list(employeeFilters),
    queryFn: () => listEmployees(employeeFilters),
  });

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

  const leadsQuery = useQuery({
    queryKey: leadsKeys.list(filters),
    queryFn: () => listLeads(filters),
  });

  const pipelineQuery = useQuery({
    queryKey: leadsKeys.pipeline(),
    queryFn: listLeadPipeline,
  });

  const summaryQuery = useQuery({
    queryKey: leadsKeys.summary(),
    queryFn: getLeadSummary,
  });

  const leadDetailQuery = useQuery({
    enabled: Boolean(selectedLeadId),
    queryKey: selectedLeadId ? leadsKeys.detail(selectedLeadId) : [...leadsKeys.all, "detail", "empty"],
    queryFn: () => getLead(selectedLeadId!),
  });

  const activitiesQuery = useQuery({
    enabled: Boolean(selectedLeadId),
    queryKey: selectedLeadId ? leadsKeys.activities(selectedLeadId) : [...leadsKeys.all, "activities", "empty"],
    queryFn: () => listLeadActivities(selectedLeadId!),
  });

  const form = useForm<LeadFormValues>({
    resolver: zodResolver(leadSchema),
    defaultValues: defaultLeadForm,
  });

  const createMutation = useMutation({
    mutationFn: createLead,
    onSuccess: async () => {
      resetLeadForm(form);
      setShowForm(false);
      await invalidateLeads(queryClient);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (payload: { leadId: string; values: LeadFormValues }) =>
      updateLead(payload.leadId, payload.values),
    onSuccess: async () => {
      resetLeadForm(form);
      setEditingLead(null);
      setShowForm(false);
      await invalidateLeads(queryClient);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteLead,
    onSuccess: async () => {
      setSelectedLeadId(null);
      await invalidateLeads(queryClient);
    },
  });

  const moveMutation = useMutation({
    mutationFn: ({ leadId, status }: { leadId: string; status: string }) => moveLeadStatus(leadId, status),
    onSuccess: async () => {
      await invalidateLeads(queryClient);
    },
  });

  const activityMutation = useMutation({
    mutationFn: ({ leadId, description }: { leadId: string; description: string }) =>
      createLeadActivity(leadId, { activity_type: "follow_up", description }),
    onSuccess: async () => {
      setActivityNote("");
      await invalidateLeads(queryClient);
    },
  });

  const importMutation = useMutation({
    mutationFn: importLeadsCSV,
    onSuccess: async (summary) => {
      setImportResult(summary);
      await invalidateLeads(queryClient);
    },
  });

  const employees = employeesQuery.data?.items ?? [];
  const campaigns = campaignsQuery.data?.items ?? [];
  const tableItems = leadsQuery.data?.items ?? [];
  const pipelineColumns = pipelineQuery.data ?? [];
  const selectedLead = leadDetailQuery.data ?? null;
  const leadActivities = activitiesQuery.data ?? [];
  const meta = leadsQuery.data?.meta;

  const leadColumns: Array<DataTableColumn<Lead>> = [
    {
      id: "lead",
      header: "Lead",
      accessor: "name",
      sortable: true,
      cell: (lead) => (
        <button className="space-y-1 text-left" onClick={() => setSelectedLeadId(lead.id)} type="button">
          <p className="font-semibold text-text-primary">{lead.name}</p>
          <p className="text-[13px] text-text-secondary">{lead.phone || lead.email || "No contact"}</p>
        </button>
      ),
    },
    {
      id: "source",
      header: "Source",
      accessor: "source_channel",
      sortable: true,
      cell: (lead) => {
        const source = leadSourceMeta(lead.source_channel);
        const SourceIcon = source.icon;
        return (
          <span className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold ${source.badgeClassName}`}>
            <SourceIcon className="h-3.5 w-3.5" />
            {source.label}
          </span>
        );
      },
    },
    {
      id: "campaign",
      header: "Campaign",
      accessor: "campaign_name",
      sortable: true,
      cell: (lead) => <span className="text-sm text-text-secondary">{lead.campaign_name ?? "Not linked"}</span>,
    },
    {
      id: "assigned",
      header: "Assigned",
      accessor: "assigned_to_name",
      sortable: true,
      cell: (lead) => <span className="text-sm text-text-primary">{lead.assigned_to_name ?? "Unassigned"}</span>,
    },
    {
      id: "value",
      header: "Estimated value",
      accessor: "estimated_value",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (lead) => <span className="font-mono tabular-nums">{formatIDR(lead.estimated_value)}</span>,
    },
    {
      id: "status",
      header: "Status",
      accessor: "pipeline_status",
      sortable: true,
      cell: (lead) => <StatusBadge status={lead.pipeline_status} variant="lead-status" />,
    },
    {
      id: "updated",
      header: "Updated",
      accessor: "updated_at",
      sortable: true,
      cell: (lead) => (
        <span className="text-sm text-text-secondary">
          {new Date(lead.updated_at).toLocaleDateString("id-ID")}
        </span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (lead) => (
        <div className="flex justify-end gap-2">
          <Button onClick={() => setSelectedLeadId(lead.id)} size="sm" type="button" variant="outline">
            Open
          </Button>
          <PermissionGate permission={permissions.marketingLeadsDelete}>
            <Button
              disabled={deleteMutation.isPending && deleteMutation.variables === lead.id}
              onClick={() => deleteMutation.mutate(lead.id)}
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

  const importPreviewColumns: Array<DataTableColumn<LeadImportPreviewRow>> = [
    {
      id: "row",
      header: "Row",
      accessor: "rowNumber",
      sortable: true,
      widthClassName: "w-20",
      cell: (row) => <span className="font-mono tabular-nums">{row.rowNumber}</span>,
    },
    { id: "name", header: "Name", accessor: "name", sortable: true },
    { id: "phone", header: "Phone", accessor: "phone", sortable: true },
    { id: "email", header: "Email", accessor: "email", sortable: true },
    { id: "source", header: "Source", accessor: "source_channel", sortable: true },
    { id: "status", header: "Status", accessor: "pipeline_status", sortable: true },
    {
      id: "errors",
      header: "Validation",
      cell: (row) =>
        row.errors.length > 0 ? (
          <div className="space-y-1 text-xs text-error">
            {row.errors.map((error) => (
              <p key={`${row.id}-${error}`}>{error}</p>
            ))}
          </div>
        ) : (
          <span className="text-sm text-success">Ready</span>
        ),
    },
  ];

  const handleSubmit = form.handleSubmit((values) => {
    if (editingLead) {
      updateMutation.mutate({ leadId: editingLead.id, values });
      return;
    }
    createMutation.mutate(values);
  });

  const handleImportFileChange = (file: File | null) => {
    setImportFile(file);
    setImportResult(null);
    setImportPreviewRows([]);
    setImportPreviewError(null);

    if (!file) {
      return;
    }

    setIsParsingImport(true);
    Papa.parse<Record<string, string>>(file, {
      header: true,
      skipEmptyLines: true,
      complete: (results) => {
        setIsParsingImport(false);
        const fields = results.meta.fields?.map((field) => field.trim()) ?? [];
        const missingHeaders = leadImportHeaders.filter((header) => !fields.includes(header));
        if (missingHeaders.length > 0) {
          setImportPreviewError(`Missing headers: ${missingHeaders.join(", ")}`);
          return;
        }

        const rows = (results.data ?? []).map((row, index) => buildLeadImportPreviewRow(row, index));
        setImportPreviewRows(rows);
      },
      error: (error) => {
        setIsParsingImport(false);
        setImportPreviewError(error.message);
      },
    });
  };

  const handleConfirmImport = () => {
    if (!importFile || importMutation.isPending) {
      return;
    }

    importMutation.mutate(importFile);
  };

  return (
    <div className="space-y-6">
      <Card className="border-mkt/20 bg-gradient-to-br from-mkt/10 via-background to-background p-8">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div>
            <p className="text-sm font-semibold uppercase tracking-[0.28em] text-mkt">Marketing CRM</p>
            <h3 className="mt-2 text-3xl font-bold tracking-tight text-foreground">Leads pipeline</h3>
            <p className="mt-2 max-w-3xl text-muted-foreground">
              Lacak lead dari kontak pertama sampai won atau lost, lengkap dengan assigned sales, nilai estimasi, dan history follow-up.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Button onClick={() => setActiveView("pipeline")} variant={activeView === "pipeline" ? "mkt" : "outline"}>
              <FolderKanban className="mr-2 h-4 w-4" />
              Pipeline
            </Button>
            <Button onClick={() => setActiveView("table")} variant={activeView === "table" ? "mkt" : "outline"}>
              <LayoutList className="mr-2 h-4 w-4" />
              Table view
            </Button>
            <PermissionGate permission={permissions.marketingLeadsCreate}>
              <Button onClick={() => setShowForm((value) => !value)} variant="mkt">
                <Plus className="mr-2 h-4 w-4" />
                {showForm ? "Close form" : "New lead"}
              </Button>
              <Button onClick={() => setShowImport((value) => !value)} variant="outline">
                <Download className="mr-2 h-4 w-4" />
                Import CSV
              </Button>
            </PermissionGate>
          </div>
        </div>
      </Card>

      <div className="grid gap-4 lg:grid-cols-4">
        <SummaryCard label="Total leads" value={String(summaryQuery.data?.total_leads ?? 0)} />
        <SummaryCard label="Won leads" value={String(summaryQuery.data?.won_leads ?? 0)} />
        <SummaryCard label="Conversion rate" value={`${(summaryQuery.data?.conversion_rate ?? 0).toFixed(2)}%`} />
        <SummaryCard
          label="Pipeline value"
          value={formatIDR((summaryQuery.data?.by_status ?? []).reduce((total, row) => total + row.estimated_value, 0))}
        />
      </div>

      <Card className="p-6">
        <div className="grid gap-3 lg:grid-cols-6">
          <Input onChange={(event) => setFilters((current) => ({ ...current, search: event.target.value, page: 1 }))} placeholder="Search name, phone, or email" value={filters.search} />
          <select className="field-select" onChange={(event) => setFilters((current) => ({ ...current, pipelineStatus: event.target.value, page: 1 }))} value={filters.pipelineStatus}>
            <option value="">All statuses</option>
            {leadStatusOptions.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
          <select className="field-select" onChange={(event) => setFilters((current) => ({ ...current, sourceChannel: event.target.value, page: 1 }))} value={filters.sourceChannel}>
            <option value="">All sources</option>
            {leadSourceOptions.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
          <select className="field-select" onChange={(event) => setFilters((current) => ({ ...current, campaignId: event.target.value, page: 1 }))} value={filters.campaignId}>
            <option value="">All campaigns</option>
            {campaigns.map((campaign) => (
              <option key={campaign.id} value={campaign.id}>{campaign.name}</option>
            ))}
          </select>
          <select className="field-select" onChange={(event) => setFilters((current) => ({ ...current, assignedTo: event.target.value, page: 1 }))} value={filters.assignedTo}>
            <option value="">All assigned sales</option>
            {employees.map((employee) => (
              <option key={employee.id} value={employee.id}>{employee.full_name}</option>
            ))}
          </select>
          <div className="grid gap-3 lg:grid-cols-2 lg:col-span-2">
            <Input onChange={(event) => setFilters((current) => ({ ...current, dateFrom: event.target.value, page: 1 }))} type="date" value={filters.dateFrom} />
            <Input onChange={(event) => setFilters((current) => ({ ...current, dateTo: event.target.value, page: 1 }))} type="date" value={filters.dateTo} />
          </div>
        </div>
      </Card>

      {showForm ? (
        <Card className="p-6">
          <div className="mb-5">
            <p className="text-sm font-semibold uppercase tracking-[0.24em] text-mkt">Lead form</p>
            <h4 className="mt-2 text-2xl font-bold tracking-tight text-foreground">{editingLead ? "Edit lead" : "Create lead"}</h4>
          </div>
          <form className="grid gap-4 lg:grid-cols-2" onSubmit={handleSubmit}>
            <Input {...form.register("name")} placeholder="Lead name" />
            <Controller control={form.control} name="estimated_value" render={({ field }) => <CurrencyInput onValueChange={field.onChange} value={field.value} />} />
              <Input {...form.register("phone")} placeholder="+628123456789" />
              <Input {...form.register("email")} placeholder="lead@example.com" />
              <select className="field-select" {...form.register("source_channel")}>
                {leadSourceOptions.map((option) => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
              <select className="field-select" {...form.register("pipeline_status")}>
                {leadStatusOptions.map((option) => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
              <select className="field-select" {...form.register("campaign_id")}>
                <option value="">No linked campaign</option>
                {campaigns.map((campaign) => (
                  <option key={campaign.id} value={campaign.id}>{campaign.name}</option>
                ))}
              </select>
              <select className="field-select" {...form.register("assigned_to")}>
                <option value="">Unassigned</option>
                {employees.map((employee) => (
                  <option key={employee.id} value={employee.id}>{employee.full_name}</option>
                ))}
            </select>
            <Input {...form.register("company_name")} placeholder="Company name" />
            <Input className="lg:col-span-2" {...form.register("notes")} placeholder="Lead notes or qualification summary" />
            <div className="flex flex-wrap gap-3 lg:col-span-2">
              <Button disabled={createMutation.isPending || updateMutation.isPending} type="submit" variant="mkt">
                {editingLead ? "Save changes" : "Create lead"}
              </Button>
              <Button
                onClick={() => {
                  resetLeadForm(form);
                  setEditingLead(null);
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

      {showImport ? (
        <Card className="p-6">
          <div className="mb-5">
            <p className="text-sm font-semibold uppercase tracking-[0.24em] text-mkt">Bulk import</p>
            <h4 className="mt-2 text-2xl font-bold tracking-tight text-foreground">Import leads from CSV</h4>
            <p className="mt-2 text-sm text-muted-foreground">
              Format kolom: `name, phone, email, source_channel, pipeline_status, assigned_to, notes, company_name, estimated_value`
            </p>
          </div>

          <div className="space-y-4">
            <input
              accept=".csv,text/csv"
              onChange={(event) => {
                handleImportFileChange(event.target.files?.[0] ?? null);
                event.target.value = "";
              }}
              type="file"
            />

            {isParsingImport ? <p className="text-sm text-text-secondary">Parsing CSV preview...</p> : null}
            {importPreviewError ? <Card className="p-4 text-sm text-error">{importPreviewError}</Card> : null}

            {importFile && importPreviewRows.length > 0 ? (
              <div className="space-y-4">
                <Card className="p-4">
                  <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                    <div>
                      <p className="text-sm font-semibold text-text-primary">{importFile.name}</p>
                      <p className="mt-1 text-sm text-text-secondary">
                        Showing the first {Math.min(importPreviewRows.length, 10)} rows. Rows with validation issues are highlighted before import.
                      </p>
                    </div>
                    <div className="flex flex-wrap items-center gap-3">
                      <span className="text-sm text-text-secondary">
                        {importPreviewRows.filter((row) => row.errors.length === 0).length} ready
                      </span>
                      <span className="text-sm text-error">
                        {importPreviewRows.filter((row) => row.errors.length > 0).length} flagged
                      </span>
                    </div>
                  </div>
                </Card>

                <DataTable
                  columns={importPreviewColumns}
                  data={importPreviewRows.slice(0, 10)}
                  emptyDescription="Upload a CSV file to preview import rows."
                  emptyTitle="No preview rows"
                  getRowClassName={(row) => (row.errors.length > 0 ? "bg-error-light hover:bg-error-light" : undefined)}
                  getRowId={(row) => row.id}
                  loading={false}
                />

                <div className="flex flex-wrap gap-3">
                  <Button disabled={Boolean(importPreviewError) || importMutation.isPending} onClick={handleConfirmImport} type="button" variant="mkt">
                    {importMutation.isPending ? "Importing..." : "Confirm import"}
                  </Button>
                  <Button
                    onClick={() => {
                      setImportFile(null);
                      setImportPreviewRows([]);
                      setImportPreviewError(null);
                      setImportResult(null);
                    }}
                    type="button"
                    variant="outline"
                  >
                    Reset file
                  </Button>
                </div>
              </div>
            ) : null}

            {importResult ? (
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4 text-sm">
                <p>Success: {importResult.success_count}</p>
                <p>Failed: {importResult.failed_count}</p>
                {importResult.errors.length > 0 ? (
                  <div className="mt-3 space-y-1 text-error">
                    {importResult.errors.map((error) => (
                      <p key={`${error.row}-${error.message}`}>Row {error.row}: {error.message}</p>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        </Card>
      ) : null}

      {activeView === "pipeline" ? (
        <MarketingLeadsPipeline
          columns={pipelineColumns}
          onLeadOpen={(lead) => setSelectedLeadId(lead.id)}
          onMoveLead={(leadId, pipelineStatus) => moveMutation.mutateAsync({ leadId, status: pipelineStatus }).then(() => undefined)}
        />
      ) : (
        <DataTable
          columns={leadColumns}
          data={tableItems}
          emptyActionLabel={hasPermission(permissions.marketingLeadsCreate) ? "New lead" : undefined}
          emptyDescription="No leads match the current filters."
          emptyTitle="No leads found"
          getRowId={(lead) => lead.id}
          loading={leadsQuery.isLoading}
          loadingRows={6}
          onEmptyAction={hasPermission(permissions.marketingLeadsCreate) ? () => setShowForm(true) : undefined}
          pagination={
            meta
              ? {
                  page: meta.page,
                  perPage: meta.per_page,
                  total: meta.total,
                  onPageChange: (page) => setFilters((current) => ({ ...current, page })),
                }
              : undefined
          }
          selectedRowId={selectedLeadId}
        />
      )}

      {selectedLeadId ? (
        <LeadDetailDrawer
          activities={leadActivities}
          activityNote={activityNote}
          lead={selectedLead}
          onActivityNoteChange={setActivityNote}
          onAddActivity={() => {
            if (selectedLeadId && activityNote.trim()) {
              activityMutation.mutate({ leadId: selectedLeadId, description: activityNote.trim() });
            }
          }}
          onClose={() => setSelectedLeadId(null)}
          onDelete={() => {
            if (selectedLead) {
              deleteMutation.mutate(selectedLead.id);
            }
          }}
          onEdit={() => {
            if (selectedLead) {
              setEditingLead(selectedLead);
              form.reset(toLeadForm(selectedLead));
              setShowForm(true);
              setSelectedLeadId(null);
            }
          }}
        />
      ) : null}
    </div>
  );
}

function LeadDetailDrawer({
  lead,
  activities,
  activityNote,
  onActivityNoteChange,
  onAddActivity,
  onEdit,
  onDelete,
  onClose,
}: {
  lead: Lead | null;
  activities: { id: string; description: string; created_by_name?: string | null; created_at: string; activity_type: string }[];
  activityNote: string;
  onActivityNoteChange: (value: string) => void;
  onAddActivity: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 bg-foreground/25 backdrop-blur-sm">
      <button className="absolute inset-0 h-full w-full cursor-default" onClick={onClose} type="button" />
      <Card className="absolute inset-y-0 right-0 z-10 flex w-full max-w-2xl flex-col rounded-none border-l border-border/80 bg-surface p-6 shadow-xl">
        <div className="flex items-start justify-between gap-4 border-b border-border/70 pb-5">
          <div>
            <p className="text-sm font-semibold uppercase tracking-[0.24em] text-mkt">Lead detail</p>
            <h4 className="mt-2 text-2xl font-bold tracking-tight text-foreground">{lead?.name ?? "Loading..."}</h4>
          </div>
          <Button onClick={onClose} size="sm" variant="outline">Close</Button>
        </div>

        {lead ? (
          <div className="mt-5 flex flex-wrap gap-3">
            <Button onClick={onEdit} size="sm" variant="outline">Edit lead</Button>
            <Button onClick={onDelete} size="sm" variant="ghost">Delete lead</Button>
          </div>
        ) : null}

        <div className="mt-6 flex-1 space-y-4 overflow-y-auto pr-1">
          {lead ? (
            <>
              <DetailRow label="Source" value={leadSourceMeta(lead.source_channel).label} />
              <DetailRow label="Status" value={<StatusBadge status={lead.pipeline_status} variant="lead-status" />} />
              <DetailRow label="Campaign" value={lead.campaign_name ?? "Not linked"} />
              <DetailRow label="Assigned" value={lead.assigned_to_name ?? "Unassigned"} />
              <DetailRow label="Estimated value" value={formatIDR(lead.estimated_value)} />
              <DetailRow label="Contact" value={lead.phone || lead.email || "No contact"} />
              <Card className="p-6">
                <p className="text-sm font-semibold uppercase tracking-[0.18em] text-mkt">Notes</p>
                <p className="mt-3 text-sm text-muted-foreground">{lead.notes || "No notes yet."}</p>
              </Card>
            </>
          ) : null}

          <Card className="p-6">
            <p className="text-sm font-semibold uppercase tracking-[0.18em] text-mkt">Add follow-up</p>
            <div className="mt-4 flex gap-3">
              <Input onChange={(event) => onActivityNoteChange(event.target.value)} placeholder="Follow-up note" value={activityNote} />
              <Button onClick={onAddActivity} variant="mkt">Add note</Button>
            </div>
          </Card>

          <Card className="p-6">
            <p className="text-sm font-semibold uppercase tracking-[0.18em] text-mkt">Activity timeline</p>
            <div className="mt-5 space-y-3">
              {activities.map((activity) => (
                <div className="rounded-[18px] border border-border/70 bg-background/70 p-3" key={activity.id}>
                  <p className="text-sm font-semibold">{activity.description}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {activity.created_by_name ?? "System"} | {new Date(activity.created_at).toLocaleString("id-ID")} | {activity.activity_type}
                  </p>
                </div>
              ))}
              {activities.length === 0 ? (
                <EmptyState
                  className="border-border/70 bg-transparent px-4 py-8"
                  description="Timeline events and follow-up notes will appear here once the lead starts moving through the pipeline."
                  icon={LayoutList}
                  title="No activities yet"
                />
              ) : null}
            </div>
          </Card>
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

function DetailRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-[18px] border border-border/70 bg-background/70 px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <div className="text-sm font-semibold">{value}</div>
    </div>
  );
}

function resetLeadForm(form: ReturnType<typeof useForm<LeadFormValues>>) {
  form.reset({ ...defaultLeadForm });
}

function buildLeadImportPreviewRow(row: Record<string, string>, index: number): LeadImportPreviewRow {
  const previewRow: LeadImportPreviewRow = {
    id: `import-row-${index + 2}`,
    rowNumber: index + 2,
    name: normalizeImportValue(row.name),
    phone: normalizeImportValue(row.phone),
    email: normalizeImportValue(row.email),
    source_channel: normalizeImportValue(row.source_channel),
    pipeline_status: normalizeImportValue(row.pipeline_status),
    assigned_to: normalizeImportValue(row.assigned_to),
    notes: normalizeImportValue(row.notes),
    company_name: normalizeImportValue(row.company_name),
    estimated_value: normalizeImportValue(row.estimated_value),
    errors: [],
  };

  if (!previewRow.name) {
    previewRow.errors.push("Name is required");
  }
  if (previewRow.phone && !/^(\+62|08)\d{8,13}$/.test(previewRow.phone)) {
    previewRow.errors.push("Phone must use +62 or 08xx format");
  }
  if (previewRow.email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(previewRow.email)) {
    previewRow.errors.push("Email format is invalid");
  }
  if (!previewRow.phone && !previewRow.email) {
    previewRow.errors.push("Phone or email is required");
  }

  return previewRow;
}

function normalizeImportValue(value: string | undefined) {
  return (value ?? "").trim();
}

function toLeadForm(lead: Lead): LeadFormValues {
  return {
    name: lead.name,
    phone: lead.phone ?? "",
    email: lead.email ?? "",
    source_channel: lead.source_channel,
    pipeline_status: lead.pipeline_status,
    campaign_id: lead.campaign_id ?? "",
    assigned_to: lead.assigned_to ?? "",
    notes: lead.notes ?? "",
    company_name: lead.company_name ?? "",
    estimated_value: lead.estimated_value,
  };
}

async function invalidateLeads(queryClient: ReturnType<typeof useQueryClient>) {
  await queryClient.invalidateQueries({ queryKey: leadsKeys.all });
}

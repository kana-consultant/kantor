import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Controller, useForm } from "react-hook-form";
import { Download, FolderKanban, LayoutList, Plus } from "lucide-react";
import { z } from "zod";

import { MarketingLeadsPipeline } from "@/components/shared/marketing-leads-pipeline";
import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
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
import type { Lead, LeadFilters, LeadFormValues } from "@/types/marketing";

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

export const Route = createFileRoute("/_authenticated/marketing/leads")({
  beforeLoad: async () => {
    await ensurePermission(permissions.marketingLeadsView);
  },
  component: MarketingLeadsPage,
});

function MarketingLeadsPage() {
  const queryClient = useQueryClient();
  const [activeView, setActiveView] = useState<"pipeline" | "table">("pipeline");
  const [filters, setFilters] = useState<LeadFilters>(defaultFilters);
  const [showForm, setShowForm] = useState(false);
  const [showImport, setShowImport] = useState(false);
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
    onSuccess: async () => {
      await invalidateLeads(queryClient);
    },
  });

  const employees = employeesQuery.data?.items ?? [];
  const campaigns = campaignsQuery.data?.items ?? [];
  const tableItems = leadsQuery.data?.items ?? [];
  const pipelineColumns = pipelineQuery.data ?? [];
  const selectedLead = leadDetailQuery.data ?? null;
  const leadActivities = activitiesQuery.data ?? [];

  const handleSubmit = form.handleSubmit((values) => {
    if (editingLead) {
      updateMutation.mutate({ leadId: editingLead.id, values });
      return;
    }
    createMutation.mutate(values);
  });

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">Marketing CRM</p>
            <h3 className="mt-2 text-3xl font-bold">Leads pipeline</h3>
            <p className="mt-2 max-w-3xl text-muted-foreground">
              Lacak lead dari kontak pertama sampai won atau lost, lengkap dengan assigned sales, nilai estimasi, dan history follow-up.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Button onClick={() => setActiveView("pipeline")} variant={activeView === "pipeline" ? "default" : "outline"}>
              <FolderKanban className="mr-2 h-4 w-4" />
              Pipeline
            </Button>
            <Button onClick={() => setActiveView("table")} variant={activeView === "table" ? "default" : "outline"}>
              <LayoutList className="mr-2 h-4 w-4" />
              Table view
            </Button>
            <PermissionGate permission={permissions.marketingLeadsCreate}>
              <Button onClick={() => setShowForm((value) => !value)}>
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
          <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((current) => ({ ...current, pipelineStatus: event.target.value, page: 1 }))} value={filters.pipelineStatus}>
            <option value="">All statuses</option>
            {leadStatusOptions.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
          <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((current) => ({ ...current, sourceChannel: event.target.value, page: 1 }))} value={filters.sourceChannel}>
            <option value="">All sources</option>
            {leadSourceOptions.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
          <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((current) => ({ ...current, campaignId: event.target.value, page: 1 }))} value={filters.campaignId}>
            <option value="">All campaigns</option>
            {campaigns.map((campaign) => (
              <option key={campaign.id} value={campaign.id}>{campaign.name}</option>
            ))}
          </select>
          <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((current) => ({ ...current, assignedTo: event.target.value, page: 1 }))} value={filters.assignedTo}>
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
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Lead form</p>
            <h4 className="mt-2 text-2xl font-bold">{editingLead ? "Edit lead" : "Create lead"}</h4>
          </div>
          <form className="grid gap-4 lg:grid-cols-2" onSubmit={handleSubmit}>
            <Input {...form.register("name")} placeholder="Lead name" />
            <Controller control={form.control} name="estimated_value" render={({ field }) => <CurrencyInput onValueChange={field.onChange} value={field.value} />} />
              <Input {...form.register("phone")} placeholder="+628123456789" />
              <Input {...form.register("email")} placeholder="lead@example.com" />
              <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...form.register("source_channel")}>
                {leadSourceOptions.map((option) => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
              <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...form.register("pipeline_status")}>
                {leadStatusOptions.map((option) => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
              <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...form.register("campaign_id")}>
                <option value="">No linked campaign</option>
                {campaigns.map((campaign) => (
                  <option key={campaign.id} value={campaign.id}>{campaign.name}</option>
                ))}
              </select>
              <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...form.register("assigned_to")}>
                <option value="">Unassigned</option>
                {employees.map((employee) => (
                  <option key={employee.id} value={employee.id}>{employee.full_name}</option>
                ))}
            </select>
            <Input {...form.register("company_name")} placeholder="Company name" />
            <Input className="lg:col-span-2" {...form.register("notes")} placeholder="Lead notes or qualification summary" />
            <div className="flex flex-wrap gap-3 lg:col-span-2">
              <Button disabled={createMutation.isPending || updateMutation.isPending} type="submit">
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
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Bulk import</p>
            <h4 className="mt-2 text-2xl font-bold">Import leads from CSV</h4>
            <p className="mt-2 text-sm text-muted-foreground">
              Format kolom: `name, phone, email, source_channel, pipeline_status, assigned_to, notes, company_name, estimated_value`
            </p>
          </div>
          <input
            accept=".csv,text/csv"
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (file) {
                importMutation.mutate(file);
              }
              event.target.value = "";
            }}
            type="file"
          />
          {importMutation.data ? (
            <div className="mt-4 rounded-[22px] border border-border/70 bg-background/70 p-4 text-sm">
              <p>Success: {importMutation.data.success_count}</p>
              <p>Failed: {importMutation.data.failed_count}</p>
              {importMutation.data.errors.length > 0 ? (
                <div className="mt-3 space-y-1 text-red-700">
                  {importMutation.data.errors.map((error) => (
                    <p key={`${error.row}-${error.message}`}>Row {error.row}: {error.message}</p>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}
        </Card>
      ) : null}

      {activeView === "pipeline" ? (
        <MarketingLeadsPipeline
          columns={pipelineColumns}
          onLeadOpen={(lead) => setSelectedLeadId(lead.id)}
          onMoveLead={(leadId, pipelineStatus) => moveMutation.mutateAsync({ leadId, status: pipelineStatus }).then(() => undefined)}
        />
      ) : (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="bg-muted/60 text-left">
                <tr>
                  <th className="px-4 py-3 font-semibold">Lead</th>
                  <th className="px-4 py-3 font-semibold">Source</th>
                  <th className="px-4 py-3 font-semibold">Campaign</th>
                  <th className="px-4 py-3 font-semibold">Assigned</th>
                  <th className="px-4 py-3 font-semibold">Estimated value</th>
                  <th className="px-4 py-3 font-semibold">Status</th>
                  <th className="px-4 py-3 font-semibold">Updated</th>
                  <th className="px-4 py-3 font-semibold">Actions</th>
                </tr>
              </thead>
              <tbody>
                {tableItems.map((lead) => {
                  const source = leadSourceMeta(lead.source_channel);
                  const SourceIcon = source.icon;
                  return (
                    <tr className="border-t border-border/70" key={lead.id}>
                      <td className="px-4 py-3">
                        <button className="text-left" onClick={() => setSelectedLeadId(lead.id)} type="button">
                          <p className="font-semibold">{lead.name}</p>
                          <p className="text-xs text-muted-foreground">{lead.phone || lead.email || "No contact"}</p>
                        </button>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold ${source.badgeClassName}`}>
                          <SourceIcon className="h-3.5 w-3.5" />
                          {source.label}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">{lead.campaign_name ?? "Not linked"}</td>
                      <td className="px-4 py-3">{lead.assigned_to_name ?? "Unassigned"}</td>
                      <td className="px-4 py-3 font-semibold">{formatIDR(lead.estimated_value)}</td>
                      <td className="px-4 py-3">
                        <span className="rounded-full bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-secondary-foreground">
                          {formatLeadStatus(lead.pipeline_status)}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">{new Date(lead.updated_at).toLocaleDateString("id-ID")}</td>
                      <td className="px-4 py-3">
                        <div className="flex gap-2">
                          <Button onClick={() => setSelectedLeadId(lead.id)} size="sm" variant="outline">Open</Button>
                          <PermissionGate permission={permissions.marketingLeadsDelete}>
                            <Button disabled={deleteMutation.isPending && deleteMutation.variables === lead.id} onClick={() => deleteMutation.mutate(lead.id)} size="sm" variant="ghost">Delete</Button>
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
      <Card className="absolute inset-y-0 right-0 z-10 flex w-full max-w-2xl flex-col rounded-none border-l border-border/80 p-6 shadow-2xl">
        <div className="flex items-start justify-between gap-4 border-b border-border/70 pb-5">
          <div>
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Lead detail</p>
            <h4 className="mt-2 text-2xl font-bold">{lead?.name ?? "Loading..."}</h4>
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
              <DetailRow label="Status" value={formatLeadStatus(lead.pipeline_status)} />
              <DetailRow label="Campaign" value={lead.campaign_name ?? "Not linked"} />
              <DetailRow label="Assigned" value={lead.assigned_to_name ?? "Unassigned"} />
              <DetailRow label="Estimated value" value={formatIDR(lead.estimated_value)} />
              <DetailRow label="Contact" value={lead.phone || lead.email || "No contact"} />
              <Card className="p-4">
                <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Notes</p>
                <p className="mt-3 text-sm text-muted-foreground">{lead.notes || "No notes yet."}</p>
              </Card>
            </>
          ) : null}

          <Card className="p-4">
            <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Add follow-up</p>
            <div className="mt-3 flex gap-3">
              <Input onChange={(event) => onActivityNoteChange(event.target.value)} placeholder="Follow-up note" value={activityNote} />
              <Button onClick={onAddActivity}>Add note</Button>
            </div>
          </Card>

          <Card className="p-4">
            <p className="text-sm uppercase tracking-[0.18em] text-muted-foreground">Activity timeline</p>
            <div className="mt-4 space-y-3">
              {activities.map((activity) => (
                <div className="rounded-[18px] border border-border/70 bg-background/70 p-3" key={activity.id}>
                  <p className="text-sm font-semibold">{activity.description}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {activity.created_by_name ?? "System"} | {new Date(activity.created_at).toLocaleString("id-ID")} | {activity.activity_type}
                  </p>
                </div>
              ))}
              {activities.length === 0 ? <p className="text-sm text-muted-foreground">No activities yet.</p> : null}
            </div>
          </Card>
        </div>
      </Card>
    </div>
  );
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-5">
      <p className="text-sm uppercase tracking-[0.2em] text-muted-foreground">{label}</p>
      <p className="mt-3 text-2xl font-bold">{value}</p>
    </Card>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-[18px] border border-border/70 bg-background/70 px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-semibold">{value}</span>
    </div>
  );
}

function resetLeadForm(form: ReturnType<typeof useForm<LeadFormValues>>) {
  form.reset({ ...defaultLeadForm });
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

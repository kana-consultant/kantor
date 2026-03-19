import { authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
import type {
  Lead,
  LeadActivity,
  LeadFilters,
  LeadFormValues,
  LeadImportSummary,
  LeadPipelineColumn,
  LeadsListResponse,
  LeadSummary,
} from "@/types/marketing";
import type { PaginationMeta } from "@/types/project";

export const leadsKeys = {
  all: ["marketing", "leads"] as const,
  list: (filters: LeadFilters) => [...leadsKeys.all, "list", { ...filters }] as const,
  detail: (leadId: string) => [...leadsKeys.all, "detail", leadId] as const,
  pipeline: () => [...leadsKeys.all, "pipeline"] as const,
  activities: (leadId: string) => [...leadsKeys.all, "activities", leadId] as const,
  summary: () => [...leadsKeys.all, "summary"] as const,
};

export async function listLeads(filters: LeadFilters): Promise<LeadsListResponse> {
  const params = new URLSearchParams();
  params.set("page", String(filters.page));
  params.set("per_page", String(filters.perPage));
  if (filters.pipelineStatus) {
    params.set("pipeline_status", filters.pipelineStatus);
  }
  if (filters.sourceChannel) {
    params.set("source_channel", filters.sourceChannel);
  }
  if (filters.campaignId) {
    params.set("campaign_id", filters.campaignId);
  }
  if (filters.assignedTo) {
    params.set("assigned_to", filters.assignedTo);
  }
  if (filters.dateFrom) {
    params.set("date_from", filters.dateFrom);
  }
  if (filters.dateTo) {
    params.set("date_to", filters.dateTo);
  }
  if (filters.search) {
    params.set("search", filters.search);
  }

  const payload = await authRequestEnvelope<LeadsListResponse["items"]>(
    `/marketing/leads?${params.toString()}`,
    { method: "GET" },
  );

  return {
    items: payload.data,
    meta: (payload.meta as PaginationMeta | undefined) ?? {
      page: filters.page,
      per_page: filters.perPage,
      total: 0,
    },
  };
}

export async function getLead(leadId: string) {
  return authRequestJSON<Lead>(`/marketing/leads/${leadId}`, { method: "GET" });
}

export async function createLead(input: LeadFormValues) {
  return authRequestJSON<Lead>(
    "/marketing/leads",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeLead(input)),
    },
  );
}

export async function updateLead(leadId: string, input: LeadFormValues) {
  return authRequestJSON<Lead>(
    `/marketing/leads/${leadId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeLead(input)),
    },
  );
}

export async function deleteLead(leadId: string) {
  return authRequestJSON<{ message: string }>(`/marketing/leads/${leadId}`, { method: "DELETE" });
}

export async function listLeadPipeline() {
  return authRequestJSON<LeadPipelineColumn[]>("/marketing/leads/pipeline", { method: "GET" });
}

export async function moveLeadStatus(leadId: string, pipelineStatus: string) {
  return authRequestJSON<Lead>(
    `/marketing/leads/${leadId}/status`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pipeline_status: pipelineStatus }),
    },
  );
}

export async function listLeadActivities(leadId: string) {
  return authRequestJSON<LeadActivity[]>(`/marketing/leads/${leadId}/activities`, { method: "GET" });
}

export async function createLeadActivity(leadId: string, payload: { activity_type: string; description: string }) {
  return authRequestJSON<LeadActivity>(
    `/marketing/leads/${leadId}/activities`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    },
  );
}

export async function importLeadsCSV(file: File) {
  const body = new FormData();
  body.append("file", file);

  return authRequestJSON<LeadImportSummary>(
    "/marketing/leads/import",
    {
      method: "POST",
      body,
    },
  );
}

export async function getLeadSummary() {
  return authRequestJSON<LeadSummary>("/marketing/leads/summary", { method: "GET" });
}

function serializeLead(input: LeadFormValues) {
  return {
    name: input.name.trim(),
    phone: input.phone.trim() || null,
    email: input.email.trim() || null,
    source_channel: input.source_channel,
    pipeline_status: input.pipeline_status,
    campaign_id: input.campaign_id.trim() || null,
    assigned_to: input.assigned_to.trim() || null,
    notes: input.notes.trim() || null,
    company_name: input.company_name.trim() || null,
    estimated_value: input.estimated_value,
  };
}

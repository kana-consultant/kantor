import { ApiError, requestEnvelope, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
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
  const token = await requireAccessToken();
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

  const payload = await requestEnvelope<LeadsListResponse["items"]>(
    `/marketing/leads?${params.toString()}`,
    { method: "GET" },
    token,
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
  const token = await requireAccessToken();
  return requestJSON<Lead>(`/marketing/leads/${leadId}`, { method: "GET" }, token);
}

export async function createLead(input: LeadFormValues) {
  const token = await requireAccessToken();
  return requestJSON<Lead>(
    "/marketing/leads",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeLead(input)),
    },
    token,
  );
}

export async function updateLead(leadId: string, input: LeadFormValues) {
  const token = await requireAccessToken();
  return requestJSON<Lead>(
    `/marketing/leads/${leadId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeLead(input)),
    },
    token,
  );
}

export async function deleteLead(leadId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(`/marketing/leads/${leadId}`, { method: "DELETE" }, token);
}

export async function listLeadPipeline() {
  const token = await requireAccessToken();
  return requestJSON<LeadPipelineColumn[]>("/marketing/leads/pipeline", { method: "GET" }, token);
}

export async function moveLeadStatus(leadId: string, pipelineStatus: string) {
  const token = await requireAccessToken();
  return requestJSON<Lead>(
    `/marketing/leads/${leadId}/status`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pipeline_status: pipelineStatus }),
    },
    token,
  );
}

export async function listLeadActivities(leadId: string) {
  const token = await requireAccessToken();
  return requestJSON<LeadActivity[]>(`/marketing/leads/${leadId}/activities`, { method: "GET" }, token);
}

export async function createLeadActivity(leadId: string, payload: { activity_type: string; description: string }) {
  const token = await requireAccessToken();
  return requestJSON<LeadActivity>(
    `/marketing/leads/${leadId}/activities`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    },
    token,
  );
}

export async function importLeadsCSV(file: File) {
  const token = await requireAccessToken();
  const body = new FormData();
  body.append("file", file);

  return requestJSON<LeadImportSummary>(
    "/marketing/leads/import",
    {
      method: "POST",
      body,
    },
    token,
  );
}

export async function getLeadSummary() {
  const token = await requireAccessToken();
  return requestJSON<LeadSummary>("/marketing/leads/summary", { method: "GET" }, token);
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

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }
  return session.tokens.access_token;
}

import { ApiError, requestEnvelope, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type {
  CampaignActivity,
  CampaignColumn,
  CampaignDetail,
  CampaignFilters,
  CampaignFormValues,
  CampaignsListResponse,
} from "@/types/marketing";
import type { PaginationMeta } from "@/types/project";

export const campaignsKeys = {
  all: ["marketing", "campaigns"] as const,
  list: (filters: CampaignFilters) => [...campaignsKeys.all, "list", { ...filters }] as const,
  detail: (campaignId: string) => [...campaignsKeys.all, "detail", campaignId] as const,
  activities: (campaignId: string) => [...campaignsKeys.all, "activities", campaignId] as const,
  kanban: () => [...campaignsKeys.all, "kanban"] as const,
  columns: () => [...campaignsKeys.all, "columns"] as const,
};

export async function listCampaigns(filters: CampaignFilters): Promise<CampaignsListResponse> {
  const token = await requireAccessToken();
  const params = new URLSearchParams();

  params.set("page", String(filters.page));
  params.set("per_page", String(filters.perPage));
  if (filters.search) {
    params.set("search", filters.search);
  }
  if (filters.channel) {
    params.set("channel", filters.channel);
  }
  if (filters.status) {
    params.set("status", filters.status);
  }
  if (filters.pic) {
    params.set("pic", filters.pic);
  }
  if (filters.dateFrom) {
    params.set("date_from", filters.dateFrom);
  }
  if (filters.dateTo) {
    params.set("date_to", filters.dateTo);
  }

  const payload = await requestEnvelope<CampaignsListResponse["items"]>(
    `/marketing/campaigns?${params.toString()}`,
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

export async function listCampaignKanban() {
  const token = await requireAccessToken();
  return requestJSON<CampaignColumn[]>("/marketing/campaigns/kanban", { method: "GET" }, token);
}

export async function getCampaign(campaignId: string) {
  const token = await requireAccessToken();
  return requestJSON<CampaignDetail>(`/marketing/campaigns/${campaignId}`, { method: "GET" }, token);
}

export async function listCampaignActivities(campaignId: string) {
  const token = await requireAccessToken();
  return requestJSON<CampaignActivity[]>(`/marketing/campaigns/${campaignId}/activities`, { method: "GET" }, token);
}

export async function createCampaign(input: CampaignFormValues) {
  const token = await requireAccessToken();
  return requestJSON<CampaignDetail>(
    "/marketing/campaigns",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeCampaignForm(input)),
    },
    token,
  );
}

export async function updateCampaign(campaignId: string, input: CampaignFormValues) {
  const token = await requireAccessToken();
  return requestJSON<CampaignDetail>(
    `/marketing/campaigns/${campaignId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeCampaignForm(input)),
    },
    token,
  );
}

export async function deleteCampaign(campaignId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(
    `/marketing/campaigns/${campaignId}`,
    { method: "DELETE" },
    token,
  );
}

export async function moveCampaign(campaignId: string, columnId: string, position: number) {
  const token = await requireAccessToken();
  return requestJSON<CampaignDetail>(
    `/marketing/campaigns/${campaignId}/move`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        column_id: columnId,
        position,
      }),
    },
    token,
  );
}

export async function listCampaignColumns() {
  const token = await requireAccessToken();
  return requestJSON<CampaignColumn[]>("/marketing/columns", { method: "GET" }, token);
}

export async function createCampaignColumn(input: { name: string; color?: string; position?: number }) {
  const token = await requireAccessToken();
  return requestJSON<CampaignColumn>(
    "/marketing/columns",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
    token,
  );
}

export async function updateCampaignColumn(columnId: string, input: { name: string; color?: string }) {
  const token = await requireAccessToken();
  return requestJSON<CampaignColumn>(
    `/marketing/columns/${columnId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
    token,
  );
}

export async function deleteCampaignColumn(columnId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(
    `/marketing/columns/${columnId}`,
    { method: "DELETE" },
    token,
  );
}

export async function reorderCampaignColumns(columnIds: string[]) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(
    "/marketing/columns/reorder",
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ column_ids: columnIds }),
    },
    token,
  );
}

export async function uploadCampaignAttachments(campaignId: string, files: File[]) {
  const token = await requireAccessToken();
  const body = new FormData();
  for (const file of files) {
    body.append("files", file);
  }

  return requestJSON<CampaignDetail>(
    `/marketing/campaigns/${campaignId}/attachments`,
    {
      method: "POST",
      body,
    },
    token,
  );
}

export async function listCampaignAttachments(campaignId: string) {
  const token = await requireAccessToken();
  return requestJSON<CampaignDetail["attachments"]>(
    `/marketing/campaigns/${campaignId}/attachments`,
    { method: "GET" },
    token,
  );
}

export async function deleteCampaignAttachment(campaignId: string, attachmentId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(
    `/marketing/campaigns/${campaignId}/attachments/${attachmentId}`,
    { method: "DELETE" },
    token,
  );
}

function serializeCampaignForm(input: CampaignFormValues) {
  return {
    name: input.name.trim(),
    description: input.description.trim() || null,
    channel: input.channel,
    budget_amount: input.budget_amount,
    budget_currency: input.budget_currency.trim() || "IDR",
    pic_employee_id: input.pic_employee_id.trim() || null,
    start_date: new Date(input.start_date).toISOString(),
    end_date: new Date(input.end_date).toISOString(),
    brief_text: input.brief_text.trim() || null,
    status: input.status,
  };
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

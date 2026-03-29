import { authRequestEnvelope, authRequestJSON } from "@/lib/api-client";
import { toUTCDateOnlyISOString } from "@/lib/date";
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

  const payload = await authRequestEnvelope<CampaignsListResponse["items"]>(
    `/marketing/campaigns?${params.toString()}`,
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

export async function listCampaignKanban() {
  return authRequestJSON<CampaignColumn[]>("/marketing/campaigns/kanban", { method: "GET" });
}

export async function getCampaign(campaignId: string) {
  return authRequestJSON<CampaignDetail>(`/marketing/campaigns/${campaignId}`, { method: "GET" });
}

export async function listCampaignActivities(campaignId: string) {
  return authRequestJSON<CampaignActivity[]>(`/marketing/campaigns/${campaignId}/activities`, { method: "GET" });
}

export async function createCampaign(input: CampaignFormValues) {
  return authRequestJSON<CampaignDetail>(
    "/marketing/campaigns",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeCampaignForm(input)),
    },
  );
}

export async function updateCampaign(campaignId: string, input: CampaignFormValues) {
  return authRequestJSON<CampaignDetail>(
    `/marketing/campaigns/${campaignId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeCampaignForm(input)),
    },
  );
}

export async function deleteCampaign(campaignId: string) {
  return authRequestJSON<{ message: string }>(
    `/marketing/campaigns/${campaignId}`,
    { method: "DELETE" },
  );
}

export async function moveCampaign(campaignId: string, columnId: string, position: number) {
  return authRequestJSON<CampaignDetail>(
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
  );
}

export async function listCampaignColumns() {
  return authRequestJSON<CampaignColumn[]>("/marketing/columns", { method: "GET" });
}

export async function createCampaignColumn(input: { name: string; color?: string; position?: number }) {
  return authRequestJSON<CampaignColumn>(
    "/marketing/columns",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
  );
}

export async function updateCampaignColumn(columnId: string, input: { name: string; color?: string }) {
  return authRequestJSON<CampaignColumn>(
    `/marketing/columns/${columnId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
  );
}

export async function deleteCampaignColumn(columnId: string) {
  return authRequestJSON<{ message: string }>(
    `/marketing/columns/${columnId}`,
    { method: "DELETE" },
  );
}

export async function reorderCampaignColumns(columnIds: string[]) {
  return authRequestJSON<{ message: string }>(
    "/marketing/columns/reorder",
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ column_ids: columnIds }),
    },
  );
}

export async function uploadCampaignAttachments(campaignId: string, files: File[]) {
  const body = new FormData();
  for (const file of files) {
    body.append("files", file);
  }

  return authRequestJSON<CampaignDetail>(
    `/marketing/campaigns/${campaignId}/attachments`,
    {
      method: "POST",
      body,
    },
  );
}

export async function listCampaignAttachments(campaignId: string) {
  return authRequestJSON<CampaignDetail["attachments"]>(
    `/marketing/campaigns/${campaignId}/attachments`,
    { method: "GET" },
  );
}

export async function deleteCampaignAttachment(campaignId: string, attachmentId: string) {
  return authRequestJSON<{ message: string }>(
    `/marketing/campaigns/${campaignId}/attachments/${attachmentId}`,
    { method: "DELETE" },
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
    start_date: toUTCDateOnlyISOString(input.start_date),
    end_date: toUTCDateOnlyISOString(input.end_date),
    brief_text: input.brief_text.trim() || null,
    status: input.status,
  };
}

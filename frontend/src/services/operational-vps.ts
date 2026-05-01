import { authRequestJSON } from "@/lib/api-client";
import type {
  VPSAppFormValues,
  VPSCheckFormValues,
  VPSDetail,
  VPSFormValues,
  VPSHealthCheck,
  VPSApp,
  VPSListFilters,
  VPSServer,
  VPSServerSummary,
} from "@/types/vps";

export const vpsKeys = {
  all: ["operational", "vps"] as const,
  list: (filters: VPSListFilters) => [...vpsKeys.all, "list", { ...filters }] as const,
  detail: (vpsID: string) => [...vpsKeys.all, "detail", vpsID] as const,
};

export async function listVPS(filters: VPSListFilters): Promise<VPSServerSummary[]> {
  const params = new URLSearchParams();
  if (filters.search) params.set("search", filters.search);
  if (filters.status) params.set("status", filters.status);
  if (filters.provider) params.set("provider", filters.provider);
  if (filters.tag) params.set("tag", filters.tag);
  const qs = params.toString();
  return authRequestJSON<VPSServerSummary[]>(`/operational/vps${qs ? `?${qs}` : ""}`, { method: "GET" });
}

export async function getVPS(vpsID: string): Promise<VPSDetail> {
  return authRequestJSON<VPSDetail>(`/operational/vps/${vpsID}`, { method: "GET" });
}

export async function createVPS(input: VPSFormValues): Promise<VPSServer> {
  return authRequestJSON<VPSServer>(`/operational/vps`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateVPS(vpsID: string, input: VPSFormValues): Promise<VPSServer> {
  return authRequestJSON<VPSServer>(`/operational/vps/${vpsID}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteVPS(vpsID: string): Promise<void> {
  await authRequestJSON<{ message: string }>(`/operational/vps/${vpsID}`, { method: "DELETE" });
}

export async function createVPSCheck(vpsID: string, input: VPSCheckFormValues): Promise<VPSHealthCheck> {
  return authRequestJSON<VPSHealthCheck>(`/operational/vps/${vpsID}/checks`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateVPSCheck(vpsID: string, checkID: string, input: VPSCheckFormValues): Promise<VPSHealthCheck> {
  return authRequestJSON<VPSHealthCheck>(`/operational/vps/${vpsID}/checks/${checkID}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteVPSCheck(vpsID: string, checkID: string): Promise<void> {
  await authRequestJSON<{ message: string }>(`/operational/vps/${vpsID}/checks/${checkID}`, { method: "DELETE" });
}

export async function createVPSApp(vpsID: string, input: VPSAppFormValues): Promise<VPSApp> {
  return authRequestJSON<VPSApp>(`/operational/vps/${vpsID}/apps`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateVPSApp(vpsID: string, appID: string, input: VPSAppFormValues): Promise<VPSApp> {
  return authRequestJSON<VPSApp>(`/operational/vps/${vpsID}/apps/${appID}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteVPSApp(vpsID: string, appID: string): Promise<void> {
  await authRequestJSON<{ message: string }>(`/operational/vps/${vpsID}/apps/${appID}`, { method: "DELETE" });
}

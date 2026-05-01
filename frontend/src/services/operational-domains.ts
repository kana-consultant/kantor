import { authRequestJSON } from "@/lib/api-client";
import type {
  Domain,
  DomainDetail,
  DomainFormValues,
  DomainListFilters,
} from "@/types/domain";

export const domainKeys = {
  all: ["operational", "domains"] as const,
  list: (filters: DomainListFilters) => [...domainKeys.all, "list", { ...filters }] as const,
  detail: (domainID: string) => [...domainKeys.all, "detail", domainID] as const,
};

export async function listDomains(filters: DomainListFilters): Promise<Domain[]> {
  const params = new URLSearchParams();
  if (filters.search) params.set("search", filters.search);
  if (filters.status) params.set("status", filters.status);
  if (filters.registrar) params.set("registrar", filters.registrar);
  if (filters.tag) params.set("tag", filters.tag);
  const qs = params.toString();
  return authRequestJSON<Domain[]>(`/operational/domains${qs ? `?${qs}` : ""}`, { method: "GET" });
}

export async function getDomain(domainID: string): Promise<DomainDetail> {
  return authRequestJSON<DomainDetail>(`/operational/domains/${domainID}`, { method: "GET" });
}

export async function createDomain(input: DomainFormValues): Promise<Domain> {
  return authRequestJSON<Domain>(`/operational/domains`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateDomain(domainID: string, input: DomainFormValues): Promise<Domain> {
  return authRequestJSON<Domain>(`/operational/domains/${domainID}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteDomain(domainID: string): Promise<void> {
  await authRequestJSON<{ message: string }>(`/operational/domains/${domainID}`, { method: "DELETE" });
}

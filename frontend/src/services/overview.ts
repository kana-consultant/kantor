import { authGetJSON } from "@/lib/api-client";
import type {
  HrisOverview,
  MarketingOverview,
  OperationalOverview,
} from "@/types/overview";

export const overviewKeys = {
  operational: () => ["operational", "overview"] as const,
  hris: () => ["hris", "overview"] as const,
  marketing: () => ["marketing", "overview"] as const,
};

export async function getOperationalOverview() {
  return authGetJSON<OperationalOverview>("/operational/overview");
}

export async function getHrisOverview() {
  return authGetJSON<HrisOverview>("/hris/overview");
}

export async function getMarketingOverview() {
  return authGetJSON<MarketingOverview>("/marketing/overview");
}

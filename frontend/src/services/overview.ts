import { ApiError, getJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
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
  const token = await requireAccessToken();
  return getJSON<OperationalOverview>("/operational/overview", token);
}

export async function getHrisOverview() {
  const token = await requireAccessToken();
  return getJSON<HrisOverview>("/hris/overview", token);
}

export async function getMarketingOverview() {
  const token = await requireAccessToken();
  return getJSON<MarketingOverview>("/marketing/overview", token);
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

import { ApiError, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type {
  Subscription,
  SubscriptionAlert,
  SubscriptionFormValues,
  SubscriptionSummary,
} from "@/types/hris";

export const subscriptionsKeys = {
  all: ["hris", "subscriptions"] as const,
  list: () => [...subscriptionsKeys.all, "list"] as const,
  detail: (subscriptionId: string) => [...subscriptionsKeys.all, subscriptionId] as const,
  summary: () => [...subscriptionsKeys.all, "summary"] as const,
  alerts: () => [...subscriptionsKeys.all, "alerts"] as const,
};

export async function listSubscriptions() {
  const token = await requireAccessToken();
  return requestJSON<Subscription[]>("/hris/subscriptions", { method: "GET" }, token);
}

export async function getSubscription(subscriptionId: string) {
  const token = await requireAccessToken();
  return requestJSON<Subscription>(`/hris/subscriptions/${subscriptionId}`, { method: "GET" }, token);
}

export async function createSubscription(input: SubscriptionFormValues) {
  const token = await requireAccessToken();
  return requestJSON<Subscription>(
    "/hris/subscriptions",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeSubscriptionForm(input)),
    },
    token,
  );
}

export async function updateSubscription(subscriptionId: string, input: SubscriptionFormValues) {
  const token = await requireAccessToken();
  return requestJSON<Subscription>(
    `/hris/subscriptions/${subscriptionId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeSubscriptionForm(input)),
    },
    token,
  );
}

export async function deleteSubscription(subscriptionId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(`/hris/subscriptions/${subscriptionId}`, { method: "DELETE" }, token);
}

export async function getSubscriptionSummary() {
  const token = await requireAccessToken();
  return requestJSON<SubscriptionSummary>("/hris/subscriptions/summary", { method: "GET" }, token);
}

export async function listSubscriptionAlerts() {
  const token = await requireAccessToken();
  return requestJSON<SubscriptionAlert[]>("/hris/subscriptions/alerts", { method: "GET" }, token);
}

export async function markSubscriptionAlertRead(alertId: string) {
  const token = await requireAccessToken();
  return requestJSON<{ message: string }>(`/hris/subscriptions/alerts/${alertId}/read`, { method: "PATCH" }, token);
}

function serializeSubscriptionForm(input: SubscriptionFormValues) {
  return {
    name: input.name.trim(),
    vendor: input.vendor.trim(),
    description: input.description.trim() || null,
    cost_amount: input.cost_amount,
    cost_currency: input.cost_currency.trim().toUpperCase(),
    billing_cycle: input.billing_cycle,
    start_date: new Date(input.start_date).toISOString(),
    renewal_date: new Date(input.renewal_date).toISOString(),
    status: input.status,
    pic_employee_id: input.pic_employee_id.trim() || null,
    category: input.category.trim(),
    login_credentials: input.login_credentials.trim() || null,
    notes: input.notes.trim() || null,
  };
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

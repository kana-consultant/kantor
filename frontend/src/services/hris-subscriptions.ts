import { authRequestJSON } from "@/lib/api-client";
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
  return authRequestJSON<Subscription[]>("/hris/subscriptions", { method: "GET" });
}

export async function getSubscription(subscriptionId: string) {
  return authRequestJSON<Subscription>(`/hris/subscriptions/${subscriptionId}`, { method: "GET" });
}

export async function createSubscription(input: SubscriptionFormValues) {
  return authRequestJSON<Subscription>(
    "/hris/subscriptions",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeSubscriptionForm(input)),
    },
  );
}

export async function updateSubscription(subscriptionId: string, input: SubscriptionFormValues) {
  return authRequestJSON<Subscription>(
    `/hris/subscriptions/${subscriptionId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeSubscriptionForm(input)),
    },
  );
}

export async function deleteSubscription(subscriptionId: string) {
  return authRequestJSON<{ message: string }>(`/hris/subscriptions/${subscriptionId}`, { method: "DELETE" });
}

export async function getSubscriptionSummary() {
  return authRequestJSON<SubscriptionSummary>("/hris/subscriptions/summary", { method: "GET" });
}

export async function listSubscriptionAlerts() {
  return authRequestJSON<SubscriptionAlert[]>("/hris/subscriptions/alerts", { method: "GET" });
}

export async function markSubscriptionAlertRead(alertId: string) {
  return authRequestJSON<{ message: string }>(`/hris/subscriptions/alerts/${alertId}/read`, { method: "PATCH" });
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

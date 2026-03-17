import { useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { PermissionGate } from "@/components/shared/permission-gate";
import { SubscriptionForm } from "@/components/shared/subscription-form";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { employeesKeys, listEmployees } from "@/services/hris-employees";
import {
  createSubscription,
  deleteSubscription,
  getSubscriptionSummary,
  listSubscriptionAlerts,
  listSubscriptions,
  markSubscriptionAlertRead,
  subscriptionsKeys,
  updateSubscription,
} from "@/services/hris-subscriptions";
import type { Subscription, SubscriptionFormValues } from "@/types/hris";

export const Route = createFileRoute("/_authenticated/hris/subscriptions")({
  beforeLoad: async () => {
    await ensurePermission(permissions.hrisSubscriptionView);
  },
  component: SubscriptionsPage,
});

function SubscriptionsPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [editingSubscription, setEditingSubscription] = useState<Subscription | null>(null);

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list({ page: 1, perPage: 100, search: "", department: "", status: "" }),
    queryFn: () => listEmployees({ page: 1, perPage: 100, search: "", department: "", status: "" }),
  });

  const subscriptionsQuery = useQuery({
    queryKey: subscriptionsKeys.list(),
    queryFn: listSubscriptions,
  });

  const summaryQuery = useQuery({
    queryKey: subscriptionsKeys.summary(),
    queryFn: getSubscriptionSummary,
  });

  const alertsQuery = useQuery({
    queryKey: subscriptionsKeys.alerts(),
    queryFn: listSubscriptionAlerts,
  });

  const createMutation = useMutation({
    mutationFn: createSubscription,
    onSuccess: async () => {
      setIsCreateOpen(false);
      await invalidateSubscriptionQueries(queryClient);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (payload: { subscriptionId: string; values: SubscriptionFormValues }) =>
      updateSubscription(payload.subscriptionId, payload.values),
    onSuccess: async () => {
      setEditingSubscription(null);
      await invalidateSubscriptionQueries(queryClient);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteSubscription,
    onSuccess: async () => {
      await invalidateSubscriptionQueries(queryClient);
    },
  });

  const markReadMutation = useMutation({
    mutationFn: markSubscriptionAlertRead,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: subscriptionsKeys.alerts() });
    },
  });

  const unreadAlerts = (alertsQuery.data ?? []).filter((alert) => !alert.is_read);

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">HRIS subscriptions</p>
            <h3 className="mt-2 text-3xl font-bold">Subscription tracking</h3>
            <p className="mt-2 max-w-2xl text-muted-foreground">
              Kelola tool berlangganan, biaya bulanan/tahunan, PIC, serta alert renewal mendekati jatuh tempo.
            </p>
          </div>

          <PermissionGate permission={permissions.hrisSubscriptionCreate}>
            <Button onClick={() => setIsCreateOpen((value) => !value)}>
              {isCreateOpen ? "Close form" : "Add subscription"}
            </Button>
          </PermissionGate>
        </div>
      </Card>

      <div className="grid gap-4 lg:grid-cols-3">
        <SummaryCard label="Total monthly" value={formatIDR(summaryQuery.data?.total_monthly_cost ?? 0)} />
        <SummaryCard label="Total yearly" value={formatIDR(summaryQuery.data?.total_yearly_cost ?? 0)} />
        <SummaryCard label="Active subscriptions" value={String(summaryQuery.data?.active_count ?? 0)} />
      </div>

      <Card className="p-6">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Renewal alerts</p>
            <h4 className="mt-2 text-2xl font-bold">Unread alerts</h4>
          </div>
          <div className="rounded-full bg-primary/10 px-3 py-1 text-sm font-semibold text-primary">
            {unreadAlerts.length} unread
          </div>
        </div>
        <div className="mt-4 space-y-3">
          {unreadAlerts.length === 0 ? (
            <div className="rounded-[22px] border border-dashed border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
              Tidak ada alert renewal baru.
            </div>
          ) : (
            unreadAlerts.map((alert) => (
              <div className="flex flex-col gap-3 rounded-[22px] border border-border/70 bg-background/70 p-4 md:flex-row md:items-center md:justify-between" key={alert.id}>
                <div>
                  <p className="text-sm font-semibold">{alert.subscription_name}</p>
                  <p className="text-xs text-muted-foreground">
                    {alertLabel(alert.alert_type)} · {new Date(alert.created_at).toLocaleString()}
                  </p>
                </div>
                <Button
                  disabled={markReadMutation.isPending}
                  onClick={() => markReadMutation.mutate(alert.id)}
                  size="sm"
                  variant="outline"
                >
                  Mark read
                </Button>
              </div>
            ))
          )}
        </div>
      </Card>

      {isCreateOpen ? (
        <SubscriptionForm
          description="Buat subscription baru beserta PIC, credential terenkripsi, dan renewal cadence."
          employees={employeesQuery.data?.items ?? []}
          isSubmitting={createMutation.isPending}
          onCancel={() => setIsCreateOpen(false)}
          onSubmit={(values) => createMutation.mutate(values)}
          submitLabel="Create subscription"
          title="New subscription"
        />
      ) : null}

      {editingSubscription ? (
        <SubscriptionForm
          defaultValues={toSubscriptionFormValues(editingSubscription)}
          description="Edit detail subscription, renewal, PIC, dan credential terenkripsi."
          employees={employeesQuery.data?.items ?? []}
          isSubmitting={updateMutation.isPending}
          onCancel={() => setEditingSubscription(null)}
          onSubmit={(values) => updateMutation.mutate({ subscriptionId: editingSubscription.id, values })}
          submitLabel="Save changes"
          title={`Edit ${editingSubscription.name}`}
        />
      ) : null}

      <div className="grid gap-4 xl:grid-cols-2">
        {(subscriptionsQuery.data ?? []).map((subscription) => (
          <Card className="p-6" key={subscription.id}>
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">{subscription.category}</p>
                <h4 className="mt-2 text-2xl font-bold">{subscription.name}</h4>
                <p className="mt-2 text-sm text-muted-foreground">{subscription.vendor}</p>
              </div>
              <span className={`rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] ${renewalTone(subscription.renewal_date)}`}>
                {subscription.status}
              </span>
            </div>

            <div className="mt-5 grid gap-3 md:grid-cols-2">
              <InfoCard label="Cost" value={`${formatIDR(subscription.cost_amount)} / ${subscription.billing_cycle}`} />
              <InfoCard label="Renewal" value={new Date(subscription.renewal_date).toLocaleDateString()} />
              <InfoCard label="PIC" value={subscription.pic_employee_name || "-"} />
              <InfoCard label="Category" value={subscription.category} />
            </div>

            {subscription.description ? <p className="mt-5 text-sm text-muted-foreground">{subscription.description}</p> : null}

            <div className="mt-5 flex flex-wrap gap-3">
              <PermissionGate permission={permissions.hrisSubscriptionEdit}>
                <Button onClick={() => setEditingSubscription(subscription)} variant="outline">Edit</Button>
              </PermissionGate>
              {hasPermission(permissions.hrisSubscriptionDelete) ? (
                <Button
                  disabled={deleteMutation.isPending && deleteMutation.variables === subscription.id}
                  onClick={() => deleteMutation.mutate(subscription.id)}
                  variant="ghost"
                >
                  {deleteMutation.isPending && deleteMutation.variables === subscription.id ? "Deleting..." : "Delete"}
                </Button>
              ) : null}
            </div>
          </Card>
        ))}
      </div>

      {(subscriptionsQuery.data ?? []).length === 0 ? (
        <Card className="p-8 text-center text-sm text-muted-foreground">Belum ada subscription yang tercatat.</Card>
      ) : null}
    </div>
  );
}

async function invalidateSubscriptionQueries(queryClient: ReturnType<typeof useQueryClient>) {
  await queryClient.invalidateQueries({ queryKey: subscriptionsKeys.all });
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-6">
      <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <h4 className="mt-3 text-2xl font-bold">{value}</h4>
    </Card>
  );
}

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
      <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{label}</p>
      <p className="mt-2 text-sm font-semibold">{value}</p>
    </div>
  );
}

function toSubscriptionFormValues(subscription: Subscription): SubscriptionFormValues {
  return {
    name: subscription.name,
    vendor: subscription.vendor,
    description: subscription.description ?? "",
    cost_amount: subscription.cost_amount,
    cost_currency: subscription.cost_currency,
    billing_cycle: subscription.billing_cycle,
    start_date: subscription.start_date.slice(0, 10),
    renewal_date: subscription.renewal_date.slice(0, 10),
    status: subscription.status,
    pic_employee_id: subscription.pic_employee_id ?? "",
    category: subscription.category,
    login_credentials: subscription.login_credentials ?? "",
    notes: subscription.notes ?? "",
  };
}

function alertLabel(alertType: string) {
  switch (alertType) {
    case "30_days":
      return "Renewal in 30 days";
    case "7_days":
      return "Renewal in 7 days";
    case "1_day":
      return "Renewal tomorrow";
    default:
      return alertType;
  }
}

function renewalTone(renewalDate: string) {
  const diffDays = Math.ceil((new Date(renewalDate).getTime() - Date.now()) / (1000 * 60 * 60 * 24));
  if (diffDays <= 1) {
    return "bg-red-100 text-red-700";
  }
  if (diffDays <= 7) {
    return "bg-orange-100 text-orange-700";
  }
  if (diffDays <= 30) {
    return "bg-amber-100 text-amber-700";
  }
  return "bg-secondary text-secondary-foreground";
}

function formatIDR(value: number) {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 0,
  }).format(value);
}

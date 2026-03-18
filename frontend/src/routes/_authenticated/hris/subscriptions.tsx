import { useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { PermissionGate } from "@/components/shared/permission-gate";
import { SubscriptionForm } from "@/components/shared/subscription-form";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
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
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between border-b border-border pb-4">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-hr mb-1">HRIS subscriptions</p>
            <h3 className="text-[28px] font-[700] text-text-primary">Subscription tracking</h3>
            <p className="mt-2 max-w-2xl text-[14px] text-text-secondary leading-relaxed">
              Kelola tool berlangganan, biaya bulanan/tahunan, PIC, serta alert renewal mendekati jatuh tempo.
            </p>
          </div>

          <PermissionGate permission={permissions.hrisSubscriptionCreate}>
            <Button onClick={() => setIsCreateOpen((value) => !value)} variant="hr">
              {isCreateOpen ? "Close form" : "Add subscription"}
            </Button>
          </PermissionGate>
        </div>
      </Card>

      <div className="grid gap-4 lg:grid-cols-3">
        {summaryQuery.isLoading ? (
          <>
            <Card className="p-5 border border-border shadow-sm bg-surface">
              <Skeleton className="h-3 w-[100px] bg-muted/60 mb-2" />
              <Skeleton className="h-8 w-[140px] bg-muted/60" />
            </Card>
            <Card className="p-5 border border-border shadow-sm bg-surface">
              <Skeleton className="h-3 w-[100px] bg-muted/60 mb-2" />
              <Skeleton className="h-8 w-[140px] bg-muted/60" />
            </Card>
            <Card className="p-5 border border-border shadow-sm bg-surface">
              <Skeleton className="h-3 w-[100px] bg-muted/60 mb-2" />
              <Skeleton className="h-8 w-[140px] bg-muted/60" />
            </Card>
          </>
        ) : (
          <>
            <SummaryCard label="Total monthly" value={formatIDR(summaryQuery.data?.total_monthly_cost ?? 0)} />
            <SummaryCard label="Total yearly" value={formatIDR(summaryQuery.data?.total_yearly_cost ?? 0)} />
            <SummaryCard label="Active subscriptions" value={String(summaryQuery.data?.active_count ?? 0)} />
          </>
        )}
      </div>

      <Card className="p-6 border-hr/30">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-border pb-4">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary mb-1">Renewal alerts</p>
            <h4 className="text-[20px] font-[700] text-text-primary">Unread alerts</h4>
          </div>
          <div className="rounded-[6px] border border-hr/20 bg-hr/10 px-2 py-0.5 text-[12px] font-[700] uppercase tracking-wider text-hr self-start sm:self-auto">
            {unreadAlerts.length} unread
          </div>
        </div>
        <div className="mt-5 space-y-3">
          {unreadAlerts.length === 0 ? (
            <div className="rounded-[12px] border border-dashed border-border bg-surface-muted p-4 text-[13px] text-text-secondary">
              Tidak ada alert renewal baru.
            </div>
          ) : (
            unreadAlerts.map((alert) => (
              <div className="flex flex-col gap-3 rounded-[12px] border border-border bg-surface-muted p-4 md:flex-row md:items-center md:justify-between" key={alert.id}>
                <div>
                  <p className="text-[14px] font-[600] text-text-primary">{alert.subscription_name}</p>
                  <p className="text-[13px] text-text-secondary mt-1">
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
        {subscriptionsQuery.isLoading ? (
          [1, 2, 3, 4].map((i) => (
            <Card className="p-6 border border-border bg-surface shadow-sm" key={i}>
              <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
                <div className="space-y-2">
                  <Skeleton className="h-3 w-[100px] bg-muted/60" />
                  <Skeleton className="h-6 w-[200px] bg-muted/60" />
                  <Skeleton className="h-4 w-[150px] bg-muted/60" />
                </div>
                <Skeleton className="h-5 w-[80px] rounded-[6px] bg-muted/60" />
              </div>
              <div className="mt-5 grid gap-3 md:grid-cols-2">
                 <Skeleton className="h-16 w-full rounded-[12px] bg-muted/60" />
                 <Skeleton className="h-16 w-full rounded-[12px] bg-muted/60" />
                 <Skeleton className="h-16 w-full rounded-[12px] bg-muted/60" />
                 <Skeleton className="h-16 w-full rounded-[12px] bg-muted/60" />
              </div>
              <div className="mt-5 flex gap-3">
                <Skeleton className="h-9 w-[80px] rounded-[6px] bg-muted/60" />
                <Skeleton className="h-9 w-[80px] rounded-[6px] bg-muted/60" />
              </div>
            </Card>
          ))
        ) : (subscriptionsQuery.data ?? []).map((subscription) => (
          <Card className="p-6 border border-border bg-surface shadow-sm" key={subscription.id}>
            <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
              <div>
                <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary mb-1">{subscription.category}</p>
                <h4 className="text-[20px] font-[700] text-text-primary">{subscription.name}</h4>
                <p className="mt-1 text-[13px] text-text-secondary">{subscription.vendor}</p>
              </div>
              <span className={`rounded-[6px] border px-2 py-0.5 text-[10px] font-[700] uppercase tracking-wider self-start ${renewalTone(subscription.renewal_date)}`}>
                {subscription.status}
              </span>
            </div>

            <div className="mt-5 grid gap-3 md:grid-cols-2">
              <InfoCard label="Cost" value={`${formatIDR(subscription.cost_amount)} / ${subscription.billing_cycle}`} />
              <InfoCard label="Renewal" value={new Date(subscription.renewal_date).toLocaleDateString()} />
              <InfoCard label="PIC" value={subscription.pic_employee_name || "-"} />
              <InfoCard label="Category" value={subscription.category} />
            </div>

            {subscription.description ? <p className="mt-5 text-[13px] text-text-secondary">{subscription.description}</p> : null}

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
        <Card className="p-8 text-center text-[14px] text-text-secondary border-dashed">Belum ada subscription yang tercatat.</Card>
      ) : null}
    </div>
  );
}

async function invalidateSubscriptionQueries(queryClient: ReturnType<typeof useQueryClient>) {
  await queryClient.invalidateQueries({ queryKey: subscriptionsKeys.all });
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-5 border border-border shadow-sm bg-surface">
      <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">{label}</p>
      <p className="mt-2 text-[24px] font-[700] text-text-primary leading-none">{value}</p>
    </Card>
  );
}

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[12px] border border-border bg-surface-muted p-4">
      <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">{label}</p>
      <p className="mt-2 text-[14px] font-[600] text-text-primary">{value}</p>
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
    return "border-priority-high/20 bg-priority-high/10 text-priority-high";
  }
  if (diffDays <= 7) {
    return "border-amber-500/20 bg-amber-500/10 text-amber-700";
  }
  if (diffDays <= 30) {
    return "border-yellow-500/20 bg-yellow-500/10 text-yellow-700";
  }
  return "border-border bg-surface-muted text-text-secondary";
}

function formatIDR(value: number) {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 0,
  }).format(value);
}

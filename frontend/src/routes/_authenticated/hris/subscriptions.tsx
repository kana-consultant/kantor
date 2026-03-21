import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Banknote, CalendarClock, CreditCard, Plus } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { PermissionGate } from "@/components/shared/permission-gate";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
import { SubscriptionForm } from "@/components/shared/subscription-form";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useRBAC } from "@/hooks/use-rbac";
import { formatIDR } from "@/lib/currency";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
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
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisSubscriptionView);
  },
  component: SubscriptionsPage,
});

function SubscriptionsPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [editingSubscription, setEditingSubscription] = useState<Subscription | null>(null);
  const [subscriptionToDelete, setSubscriptionToDelete] = useState<Subscription | null>(null);

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
      setSubscriptionToDelete(null);
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
  const columns: Array<DataTableColumn<Subscription>> = [
    {
      id: "name",
      header: "Subscription",
      accessor: "name",
      sortable: true,
      cell: (subscription) => (
        <div className="space-y-1">
          <p className="font-semibold text-text-primary">{subscription.name}</p>
          <p className="text-[13px] text-text-secondary">
            {subscription.vendor} | {subscription.category}
          </p>
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      accessor: "status",
      sortable: true,
      cell: (subscription) => (
        <StatusBadge status={subscription.status} variant="subscription-status" />
      ),
    },
    {
      id: "cost",
      header: "Cost",
      accessor: "cost_amount",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (subscription) => (
        <div className="space-y-1 text-right font-mono tabular-nums">
          <p>{formatIDR(subscription.cost_amount)}</p>
          <p className="text-[12px] text-text-secondary">{subscription.billing_cycle}</p>
        </div>
      ),
    },
    {
      id: "renewal",
      header: "Renewal",
      accessor: "renewal_date",
      sortable: true,
      cell: (subscription) => (
        <div className="space-y-1">
          <p className="text-sm text-text-primary">
            {new Date(subscription.renewal_date).toLocaleDateString("id-ID")}
          </p>
          <StatusBadge status={renewalAlertLabel(subscription.renewal_date)} variant="renewal-alert" />
        </div>
      ),
    },
    {
      id: "pic",
      header: "PIC",
      accessor: "pic_employee_name",
      sortable: true,
      cell: (subscription) => (
        <span className="text-sm text-text-secondary">{subscription.pic_employee_name || "-"}</span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (subscription) => (
        <div className="flex justify-end gap-2">
          <PermissionGate permission={permissions.hrisSubscriptionEdit}>
            <Button onClick={() => setEditingSubscription(subscription)} size="sm" type="button" variant="outline">
              Edit
            </Button>
          </PermissionGate>
          {hasPermission(permissions.hrisSubscriptionDelete) ? (
            <Button
              disabled={deleteMutation.isPending && deleteMutation.variables === subscriptionToDelete?.id}
              onClick={() => setSubscriptionToDelete(subscription)}
              size="sm"
              type="button"
              variant="ghost"
            >
              Delete
            </Button>
          ) : null}
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 border-b border-border pb-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">
              HRIS subscriptions
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">Subscription tracking</h3>
            <p className="mt-2 max-w-2xl text-[14px] leading-relaxed text-text-secondary">
              Track software spend, renewal risk, and the employee responsible for each tool.
            </p>
          </div>

          <PermissionGate permission={permissions.hrisSubscriptionCreate}>
            <Button onClick={() => setIsCreateOpen(true)}>
              <Plus className="h-4 w-4" />
              Add subscription
            </Button>
          </PermissionGate>
        </div>
      </Card>

      <div className="grid gap-4 lg:grid-cols-3">
        <StatCard
          helper="Average monthly subscription spend"
          icon={Banknote}
          label="Total monthly"
          mono
          tone="hr"
          value={formatIDR(summaryQuery.data?.total_monthly_cost ?? 0)}
        />
        <StatCard
          helper="Projected yearly subscription spend"
          icon={CalendarClock}
          label="Total yearly"
          mono
          tone="hr"
          value={formatIDR(summaryQuery.data?.total_yearly_cost ?? 0)}
        />
        <StatCard
          helper={`${unreadAlerts.length} unread renewal alert${unreadAlerts.length === 1 ? "" : "s"}`}
          icon={CreditCard}
          label="Active subscriptions"
          tone="hr"
          value={String(summaryQuery.data?.active_count ?? 0)}
        />
      </div>

      <Card className="p-6">
        <div className="flex flex-col gap-4 border-b border-border pb-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
              Renewal alerts
            </p>
            <h4 className="text-[20px] font-[700] text-text-primary">Unread alerts</h4>
          </div>
          <StatusBadge
            status={unreadAlerts.length > 0 ? `${unreadAlerts.length} unread` : "No unread"}
          />
        </div>
        <div className="mt-5 space-y-3">
          {unreadAlerts.length === 0 ? (
            <p className="text-sm text-text-secondary">No unread renewal alerts right now.</p>
          ) : (
            unreadAlerts.map((alert) => (
              <div
                className="flex flex-col gap-3 rounded-md border border-border bg-surface-muted px-4 py-4 md:flex-row md:items-center md:justify-between"
                key={alert.id}
              >
                <div className="space-y-2">
                  <p className="font-semibold text-text-primary">{alert.subscription_name}</p>
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={alert.alert_type} variant="renewal-alert" />
                    <span className="text-[13px] text-text-secondary">
                      {new Date(alert.created_at).toLocaleString("id-ID")}
                    </span>
                  </div>
                </div>
                <Button
                  disabled={markReadMutation.isPending}
                  onClick={() => markReadMutation.mutate(alert.id)}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  Mark read
                </Button>
              </div>
            ))
          )}
        </div>
      </Card>

      <SubscriptionForm
        description="Capture renewal cadence, software owner, and encrypted credentials without pushing the table down."
        employees={employeesQuery.data?.items ?? []}
        isOpen={isCreateOpen}
        isSubmitting={createMutation.isPending}
        onCancel={() => setIsCreateOpen(false)}
        onSubmit={(values) => createMutation.mutate(values)}
        submitLabel="Create subscription"
        title="New subscription"
      />

      {editingSubscription ? (
        <SubscriptionForm
          defaultValues={toSubscriptionFormValues(editingSubscription)}
          description="Update ownership, renewal schedule, and secure notes inside the same workflow."
          employees={employeesQuery.data?.items ?? []}
          isOpen={Boolean(editingSubscription)}
          isSubmitting={updateMutation.isPending}
          onCancel={() => setEditingSubscription(null)}
          onSubmit={(values) => updateMutation.mutate({ subscriptionId: editingSubscription.id, values })}
          submitLabel="Save changes"
          title={`Edit ${editingSubscription.name}`}
        />
      ) : null}

      {subscriptionsQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-error">{subscriptionsQuery.error.message}</Card>
      ) : null}

      <DataTable
        columns={columns}
        data={subscriptionsQuery.data ?? []}
        emptyActionLabel={hasPermission(permissions.hrisSubscriptionCreate) ? "Add subscription" : undefined}
        emptyDescription="No subscriptions have been recorded yet. Start by adding the first tool."
        emptyTitle="No subscriptions found"
        getRowId={(subscription) => subscription.id}
        loading={subscriptionsQuery.isLoading}
        loadingRows={6}
        onEmptyAction={hasPermission(permissions.hrisSubscriptionCreate) ? () => setIsCreateOpen(true) : undefined}
      />

      <ConfirmDialog
        confirmLabel="Hapus subscription"
        description={
          subscriptionToDelete
            ? `Subscription "${subscriptionToDelete.name}" akan dihapus dari tracker dan alert renewal terkait akan hilang.`
            : ""
        }
        isLoading={deleteMutation.isPending}
        isOpen={Boolean(subscriptionToDelete)}
        onClose={() => setSubscriptionToDelete(null)}
        onConfirm={() => {
          if (subscriptionToDelete) {
            deleteMutation.mutate(subscriptionToDelete.id);
          }
        }}
        title={subscriptionToDelete ? `Hapus ${subscriptionToDelete.name}?` : "Hapus subscription?"}
      />
    </div>
  );
}

async function invalidateSubscriptionQueries(queryClient: ReturnType<typeof useQueryClient>) {
  await queryClient.invalidateQueries({ queryKey: subscriptionsKeys.all });
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

function renewalAlertLabel(renewalDate: string) {
  const diffDays = Math.ceil((new Date(renewalDate).getTime() - Date.now()) / (1000 * 60 * 60 * 24));
  if (diffDays <= 1) {
    return "1_day";
  }
  if (diffDays <= 7) {
    return "7_days";
  }
  if (diffDays <= 30) {
    return "30_days";
  }
  return "";
}

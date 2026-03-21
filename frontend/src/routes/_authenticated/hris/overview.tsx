import { useQuery } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  AlertTriangle,
  CreditCard,
  Landmark,
  Receipt,
  Users,
} from "lucide-react";

import { EmptyState } from "@/components/shared/empty-state";
import { OverviewSkeleton } from "@/components/shared/skeletons";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { formatIDR } from "@/lib/currency";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { getHrisOverview, overviewKeys } from "@/services/overview";

export const Route = createFileRoute("/_authenticated/hris/overview")({
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisOverview);
  },
  component: HrisOverviewPage,
});

function HrisOverviewPage() {
  const navigate = useNavigate();
  const { hasPermission } = useRBAC();
  const canViewFinance = hasPermission(permissions.hrisFinanceView);
  const canViewSubscription = hasPermission(permissions.hrisSubscriptionView);
  const canViewReimbursement = hasPermission(permissions.hrisReimbursementView);

  const overviewQuery = useQuery({
    queryKey: overviewKeys.hris(),
    queryFn: getHrisOverview,
  });

  if (overviewQuery.isLoading) {
    return <OverviewSkeleton />;
  }

  if (overviewQuery.isError || !overviewQuery.data) {
    return (
      <EmptyState
        actionLabel="Open employees"
        description="Data overview HRIS belum bisa dimuat. Anda masih bisa lanjut dari halaman employees atau reimbursements."
        icon={AlertTriangle}
        onAction={() => void navigate({ to: "/hris/employees" })}
        title="Overview tidak tersedia"
      />
    );
  }

  const overview = overviewQuery.data;
  const monthlyNetTone = overview.monthly_net >= 0 ? "success" : "error";

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-hr">
            HRIS
          </p>
          <h1 className="mt-2 text-[28px] font-bold tracking-tight text-text-primary">
            People and finance overview
          </h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
            Ringkasan karyawan aktif, subscription berjalan, kesehatan arus kas, dan reimbursement terbaru dalam satu layar.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => void navigate({ to: "/hris/employees" })}>Open Employees</Button>
          {canViewReimbursement && (
            <Button
              onClick={() => void navigate({ to: "/hris/reimbursements" })}
              variant="secondary"
            >
              Open Reimbursements
            </Button>
          )}
        </div>
      </div>

      <div className={`grid gap-4 ${canViewFinance ? "lg:grid-cols-4" : canViewReimbursement ? "lg:grid-cols-2" : ""}`}>
        <StatCard
          helper="Karyawan dengan status employment active."
          icon={Users}
          label="Total Employees"
          tone="hr"
          value={overview.total_employees.toLocaleString("id-ID")}
        />
        {canViewSubscription && (
          <StatCard
            helper={`${formatIDR(overview.active_subscription_monthly_cost)} per bulan`}
            icon={CreditCard}
            label="Active Subscriptions"
            tone="hr"
            value={overview.active_subscriptions.toLocaleString("id-ID")}
          />
        )}
        {canViewFinance && (
          <StatCard
            helper="Income dikurangi outcome bulan berjalan."
            icon={Landmark}
            label="Monthly Net"
            mono
            tone={monthlyNetTone}
            value={formatIDR(overview.monthly_net)}
          />
        )}
        {canViewReimbursement && (
          <StatCard
            helper="Reimbursement yang masih menunggu review."
            icon={Receipt}
            label="Pending Reimbursements"
            tone="warning"
            value={overview.pending_reimbursements.toLocaleString("id-ID")}
          />
        )}
      </div>

      {(canViewFinance || canViewSubscription) && (
        <div className="grid gap-6 xl:grid-cols-2">
          {canViewFinance && (
            <Card className="p-6">
              <div className="border-b border-border pb-4">
                <p className="text-xs font-semibold uppercase tracking-[0.08em] text-hr">
                  Income vs Outcome
                </p>
                <h2 className="mt-2 text-[22px] font-bold text-text-primary">
                  Last 6 months
                </h2>
              </div>
              <div className="mt-6 h-[320px]">
                <ResponsiveContainer height="100%" minHeight={240} minWidth={1} width="100%">
                  <BarChart data={overview.income_vs_outcome}>
                    <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="4 4" vertical={false} />
                    <XAxis dataKey="label" stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} />
                    <YAxis
                      stroke="hsl(var(--text-tertiary))"
                      tickFormatter={(value) => compactCurrency(value)}
                      tickLine={false}
                      axisLine={false}
                    />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "hsl(var(--surface))",
                        border: "1px solid hsl(var(--border))",
                        borderRadius: 8,
                        boxShadow: "0 4px 8px -2px rgba(23,43,77,0.08), 0 2px 4px -2px rgba(23,43,77,0.06)",
                      }}
                      formatter={(value) => formatIDR(Number(value))}
                    />
                    <Bar dataKey="income" fill="#36B37E" radius={[4, 4, 0, 0]} />
                    <Bar dataKey="outcome" fill="#FF5630" radius={[4, 4, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </Card>
          )}

          {canViewSubscription && (
            <Card className="p-6">
              <div className="border-b border-border pb-4">
                <p className="text-xs font-semibold uppercase tracking-[0.08em] text-hr">
                  Upcoming Renewals
                </p>
                <h2 className="mt-2 text-[22px] font-bold text-text-primary">
                  Next 30 days
                </h2>
              </div>
              <div className="mt-6 space-y-3">
                {overview.upcoming_renewals.length > 0 ? (
                  overview.upcoming_renewals.map((item) => (
                    <div className="rounded-md border border-border bg-surface p-4" key={item.id}>
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-semibold text-text-primary">{item.name}</p>
                          <p className="mt-1 text-xs text-text-secondary">
                            {item.vendor} {item.pic_employee_name ? `| PIC ${item.pic_employee_name}` : ""}
                          </p>
                        </div>
                        <StatusBadge
                          status={alertStatus(item.days_remaining)}
                          variant="renewal-alert"
                        />
                      </div>
                      <div className="mt-3 flex items-center justify-between text-xs text-text-secondary">
                        <span>{new Date(item.renewal_date).toLocaleDateString("id-ID")}</span>
                        <span className="font-mono tabular-nums">{formatIDR(item.cost_amount)}</span>
                      </div>
                    </div>
                  ))
                ) : (
                  <EmptyState
                    description="Belum ada subscription aktif yang jatuh tempo dalam 30 hari ke depan."
                    icon={CreditCard}
                    title="Tidak ada renewal dekat"
                  />
                )}
              </div>
            </Card>
          )}
        </div>
      )}

      {canViewReimbursement && (
        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-hr">
              Recent Reimbursements
            </p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">
              Latest submissions
            </h2>
          </div>
          <div className="mt-6 space-y-3">
            {overview.recent_reimbursements.length > 0 ? (
              overview.recent_reimbursements.map((item) => (
                <div
                  className="flex flex-col gap-3 rounded-md border border-border bg-surface p-4 md:flex-row md:items-center md:justify-between"
                  key={item.id}
                >
                  <div>
                    <p className="text-sm font-semibold text-text-primary">{item.title}</p>
                    <p className="mt-1 text-xs text-text-secondary">
                      {item.employee_name} | {item.category}
                    </p>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 md:justify-end">
                    <StatusBadge status={item.status} variant="reimbursement-status" />
                    <span className="text-sm font-mono tabular-nums text-text-primary">
                      {formatIDR(item.amount)}
                    </span>
                    <span className="text-xs text-text-tertiary">
                      {new Date(item.created_at).toLocaleDateString("id-ID")}
                    </span>
                  </div>
                </div>
              ))
            ) : (
              <EmptyState
                description="Rangkuman reimbursement akan muncul di sini saat pengajuan mulai masuk."
                icon={Receipt}
                title="Belum ada reimbursement"
              />
            )}
          </div>
        </Card>
      )}
    </div>
  );
}

function compactCurrency(value: number) {
  if (Math.abs(value) >= 1_000_000) {
    return `Rp${(value / 1_000_000).toFixed(0)}jt`;
  }
  if (Math.abs(value) >= 1_000) {
    return `Rp${(value / 1_000).toFixed(0)}rb`;
  }
  return `Rp${value}`;
}

function alertStatus(daysRemaining: number) {
  if (daysRemaining <= 1) {
    return "1_day";
  }
  if (daysRemaining <= 7) {
    return "7_days";
  }
  return "30_days";
}

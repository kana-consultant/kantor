import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
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
  Activity,
  AlertTriangle,
  FolderKanban,
  ListChecks,
  Users,
} from "lucide-react";

import { EmptyState } from "@/components/shared/empty-state";
import { OverviewSkeleton } from "@/components/shared/skeletons";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { overviewKeys, getOperationalOverview } from "@/services/overview";

export const Route = createFileRoute("/_authenticated/operational/overview")({
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalOverview);
  },
  component: OperationalOverviewPage,
});

function OperationalOverviewPage() {
  const navigate = useNavigate();
  const { hasPermission } = useRBAC();
  const canCreateProject = hasPermission(permissions.operationalProjectCreate);
  const overviewQuery = useQuery({
    queryKey: overviewKeys.operational(),
    queryFn: getOperationalOverview,
  });

  if (overviewQuery.isLoading) {
    return <OverviewSkeleton />;
  }

  if (overviewQuery.isError || !overviewQuery.data) {
    return (
      <EmptyState
        actionLabel="Open projects"
        description="Data overview operasional belum bisa dimuat. Buka daftar project untuk lanjut bekerja."
        icon={AlertTriangle}
        onAction={() => void navigate({ to: "/operational/projects" })}
        title="Overview tidak tersedia"
      />
    );
  }

  const overview = overviewQuery.data;

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">
            Operasional
          </p>
          <h1 className="mt-2 text-[28px] font-bold tracking-tight text-text-primary">
            Project execution overview
          </h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
            Pantau jumlah project aktif, beban task berjalan, task overdue, dan update task terbaru dari seluruh board.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => void navigate({ to: "/operational/projects" })}>
            {canCreateProject ? "Manage Projects" : "View Projects"}
          </Button>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-4">
        <StatCard
          helper="Seluruh project yang tercatat di workspace."
          icon={FolderKanban}
          label="Total Projects"
          tone="ops"
          value={overview.total_projects.toLocaleString("id-ID")}
        />
        <StatCard
          helper="Task yang saat ini berada di kolom In Progress."
          icon={Activity}
          label="Active Tasks"
          tone="ops"
          value={overview.active_tasks.toLocaleString("id-ID")}
        />
        <StatCard
          helper="Task melewati due date dan belum berada di kolom Done."
          icon={AlertTriangle}
          label="Overdue Tasks"
          tone={overview.overdue_tasks > 0 ? "error" : "success"}
          value={overview.overdue_tasks.toLocaleString("id-ID")}
        />
        <StatCard
          helper="Member unik yang sudah di-assign ke project."
          icon={Users}
          label="Team Members"
          tone="info"
          value={overview.team_members.toLocaleString("id-ID")}
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1.35fr,1fr]">
        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">
              Project Activity
            </p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">
              Tasks completed per week
            </h2>
          </div>
          <div className="mt-6 h-[320px]">
            <ResponsiveContainer height="100%" minHeight={240} minWidth={1} width="100%">
              <BarChart data={overview.completed_by_week}>
                <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="4 4" vertical={false} />
                <XAxis dataKey="label" stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} />
                <YAxis allowDecimals={false} stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} />
                <Tooltip
                  contentStyle={{
                    backgroundColor: "hsl(var(--surface))",
                    border: "1px solid hsl(var(--border))",
                    borderRadius: 8,
                    boxShadow: "0 4px 8px -2px rgba(23,43,77,0.08), 0 2px 4px -2px rgba(23,43,77,0.06)",
                  }}
                  cursor={{ fill: "hsl(var(--surface-muted))" }}
                />
                <Bar dataKey="value" fill="var(--module-primary)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card>

        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">
              Recent Tasks
            </p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">
              Updated most recently
            </h2>
          </div>
          <div className="mt-6 space-y-3">
            {overview.recent_tasks.length > 0 ? (
              overview.recent_tasks.map((task) => (
                <Link
                  className="block rounded-md border border-border bg-surface p-4 transition hover:border-ops/30 hover:shadow-card-hover"
                  key={task.id}
                  params={{ projectId: task.project_id }}
                  search={{ view: "board" }}
                  to="/operational/projects/$projectId"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-text-primary">{task.title}</p>
                      <p className="mt-1 text-xs text-text-secondary">{task.project_name}</p>
                    </div>
                    <StatusBadge status={task.status} />
                  </div>
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <StatusBadge status={task.priority} variant="priority" />
                    {task.assignee_name ? (
                      <span className="inline-flex items-center gap-2 rounded-full bg-surface-muted px-2 py-1 text-xs font-medium text-text-secondary">
                        <span className="flex h-5 w-5 items-center justify-center rounded-full bg-module text-[10px] font-semibold text-white">
                          {initials(task.assignee_name)}
                        </span>
                        {task.assignee_name}
                      </span>
                    ) : (
                      <span className="text-xs text-text-tertiary">Unassigned</span>
                    )}
                    <span className="text-xs text-text-tertiary">
                      {formatDateTime(task.updated_at)}
                    </span>
                  </div>
                </Link>
              ))
            ) : (
              <EmptyState
                description="Task terbaru akan muncul di sini setelah board mulai digunakan."
                icon={ListChecks}
                title="Belum ada aktivitas task"
              />
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}

function initials(value: string) {
  return value
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString("id-ID", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

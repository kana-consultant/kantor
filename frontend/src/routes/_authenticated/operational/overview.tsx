import { useEffect, useRef, useState } from "react";
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
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
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
  const [canRenderCharts, setCanRenderCharts] = useState(true);
  const chartContainerRef = useRef<HTMLDivElement | null>(null);
  const canCreateProject = hasPermission(permissions.operationalProjectCreate);
  const overviewQuery = useQuery({
    queryKey: overviewKeys.operational(),
    queryFn: getOperationalOverview,
  });

  useEffect(() => {
    const node = chartContainerRef.current;
    if (!node) {
      return undefined;
    }

    const updateChartReadiness = () => {
      const { width, height } = node.getBoundingClientRect();
      setCanRenderCharts(width > 0 && height > 0);
    };

    updateChartReadiness();
    const frameId = window.requestAnimationFrame(updateChartReadiness);

    const observer = new ResizeObserver(([entry]) => {
      if (!entry) {
        return;
      }
      const { width, height } = entry.contentRect;
      setCanRenderCharts(width > 0 && height > 0);
    });

    observer.observe(node);
    return () => {
      window.cancelAnimationFrame(frameId);
      observer.disconnect();
    };
  }, []);

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
      <Card className="p-5 sm:p-6 lg:p-7">
        <div className="flex flex-col gap-4 border-b border-border/80 pb-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">
              Operasional
            </p>
            <h1 className="mt-2 text-[24px] font-bold tracking-tight text-text-primary sm:text-[28px]">
              Ringkasan eksekusi project
            </h1>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
              Pantau jumlah project aktif, beban task berjalan, task overdue, dan update task terbaru dari seluruh board.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button className="w-full sm:w-auto" onClick={() => void navigate({ to: "/operational/projects" })}>
              {canCreateProject ? "Kelola project" : "Lihat project"}
            </Button>
          </div>
        </div>
      </Card>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          helper="Seluruh project yang tercatat di workspace."
          icon={FolderKanban}
          label="Total project"
          tone="ops"
          value={overview.total_projects.toLocaleString("id-ID")}
        />
        <StatCard
          helper="Task yang saat ini berada di kolom In Progress."
          icon={Activity}
          label="Task aktif"
          tone="ops"
          value={overview.active_tasks.toLocaleString("id-ID")}
        />
        <StatCard
          helper="Task melewati due date dan belum berada di kolom Done."
          icon={AlertTriangle}
          label="Task overdue"
          tone={overview.overdue_tasks > 0 ? "error" : "success"}
          value={overview.overdue_tasks.toLocaleString("id-ID")}
        />
        <StatCard
          helper="Member unik yang sudah di-assign ke project."
          icon={Users}
          label="Anggota tim"
          tone="info"
          value={overview.team_members.toLocaleString("id-ID")}
        />
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-[1fr,340px]">
        <Card className="min-w-0 p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">
              Aktivitas project
            </p>
            <h2 className="mt-1.5 text-xl font-bold text-text-primary">
              Task selesai per minggu
            </h2>
          </div>
          <div className="mt-6 h-[280px] min-w-0" ref={chartContainerRef}>
            {canRenderCharts ? (
              <ResponsiveContainer height="100%" minHeight={200} minWidth={1} width="100%">
                <BarChart data={overview.completed_by_week} margin={{ top: 4, right: 4, left: -16, bottom: 0 }}>
                  <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="4 4" vertical={false} />
                  <XAxis dataKey="label" stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} tick={{ fontSize: 12 }} />
                  <YAxis allowDecimals={false} stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} tick={{ fontSize: 12 }} />
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
            ) : (
              <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-border/80 bg-surface-muted/30 px-6 text-center text-sm text-text-secondary">
                Grafik mingguan sedang menyesuaikan ukuran tampilan.
              </div>
            )}
          </div>
        </Card>

        <Card className="p-5">
          <div className="border-b border-border pb-3">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">
              Task terbaru
            </p>
            <h2 className="mt-1.5 text-xl font-bold text-text-primary">
              Update terkini
            </h2>
          </div>
          <div className="mt-4 space-y-1">
            {overview.recent_tasks.length > 0 ? (
              overview.recent_tasks.map((task) => (
                <Link
                  className="flex items-start gap-3 rounded-lg px-3 py-2.5 transition hover:bg-surface-muted"
                  key={task.id}
                  params={{ projectId: task.project_id }}
                  search={{ view: "board" }}
                  to="/operational/projects/$projectId"
                >
                  <ProtectedAvatar
                    alt={task.assignee_name ?? "?"}
                    avatarUrl={task.assignee_avatar}
                    className="mt-0.5 h-7 w-7 shrink-0"
                    fallbackClassName="bg-module text-white text-[10px]"
                    iconClassName="h-3 w-3"
                  />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-text-primary leading-snug">{task.title}</p>
                    <div className="mt-1 flex items-center gap-1.5 flex-wrap">
                      <StatusBadge status={task.priority} variant="priority" />
                      <span className="text-xs text-text-tertiary">{task.project_name}</span>
                    </div>
                    <p className="mt-0.5 text-xs text-text-tertiary">{formatDateTime(task.updated_at)}</p>
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

function formatDateTime(value: string) {
  return new Date(value).toLocaleString("id-ID", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

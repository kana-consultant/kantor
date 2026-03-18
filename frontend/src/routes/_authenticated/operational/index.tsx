import { useQuery } from "@tanstack/react-query";
import { Bot, FolderKanban, Layers3, TimerReset } from "lucide-react";
import { Link, createFileRoute } from "@tanstack/react-router";

import { Card } from "@/components/ui/card";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { listProjects } from "@/services/operational-projects";

export const Route = createFileRoute("/_authenticated/operational/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.operationalOverview);
  },
  component: OperationalPage,
});

function OperationalPage() {
  const projectsQuery = useQuery({
    queryKey: ["operational", "projects", "hub"],
    queryFn: () =>
      listProjects({
        page: 1,
        perPage: 6,
        search: "",
        status: "",
        priority: "",
      }),
  });

  return (
    <div className="space-y-6">
      <Card className="p-8 border-none bg-gradient-to-br from-ops/5 to-surface shadow-md">
        <div className="grid gap-8 lg:grid-cols-[1.15fr_0.85fr]">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-ops mb-2">
              Operational workspace
            </p>
            <h3 className="mt-2 max-w-2xl text-[32px] font-[700] leading-tight text-text-primary">
              Board-centric workflow untuk project, member assignment, dan automation.
            </h3>
            <p className="mt-4 max-w-2xl text-[14px] text-text-secondary leading-relaxed">
              Masuk ke `Projects` untuk pengalaman utama ala Trello. Dari sana user bisa
              mengelola board, membuka task detail, invite member, dan mengaktifkan auto assign.
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <Link
                className="inline-flex h-[44px] items-center justify-center rounded-[6px] bg-ops px-5 text-[14px] font-[600] text-white transition hover:opacity-90 shadow-sm"
                to="/operational/projects"
              >
                Open project boards
              </Link>
              <Link
                className="inline-flex h-[44px] items-center justify-center rounded-[6px] border border-border bg-surface px-5 text-[14px] font-[600] text-text-primary transition hover:bg-surface-muted shadow-sm"
                to="/operational/automation"
              >
                Review automation
              </Link>
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <HubMetric
              description="Project boards siap dipakai"
              icon={FolderKanban}
              title="Boards"
              value={String(projectsQuery.data?.items.length ?? 0)}
            />
            <HubMetric
              description="Default kanban columns per project"
              icon={Layers3}
              title="Columns"
              value="5"
            />
            <HubMetric
              description="Auto assign rules per project"
              icon={Bot}
              title="Automation"
              value="Active"
            />
            <HubMetric
              description="Access token + refresh flow"
              icon={TimerReset}
              title="Auth"
              value="Ready"
            />
          </div>
        </div>
      </Card>

      <div className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <Card className="p-8">
          <div className="flex items-center justify-between gap-4 border-b border-border pb-4">
            <div>
              <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-ops mb-1">
                Recent boards
              </p>
              <h4 className="text-[20px] font-[700] text-text-primary">Jump back into work</h4>
            </div>
            <Link
              className="text-[13px] font-[600] text-ops underline-offset-4 hover:underline"
              to="/operational/projects"
            >
              View all projects
            </Link>
          </div>

          <div className="mt-6 grid gap-4 md:grid-cols-2">
            {projectsQuery.data?.items.map((project) => (
              <Link
                className="group rounded-[12px] border border-border bg-surface p-5 transition-all hover:-translate-y-0.5 hover:border-ops/30 hover:shadow-card"
                key={project.id}
                params={{ projectId: project.id }}
                search={{ view: "board" }}
                to="/operational/projects/$projectId"
              >
                <div className="flex items-center justify-between gap-3">
                  <span className="rounded-[6px] border border-border bg-surface-muted px-2 py-0.5 text-[10px] font-[700] uppercase tracking-wider text-text-secondary">
                    {project.status.replace("_", " ")}
                  </span>
                  <span className="text-[12px] font-[500] text-text-tertiary">
                    {project.member_count} members
                  </span>
                </div>
                <h5 className="mt-4 text-[16px] font-[600] text-text-primary group-hover:text-ops transition-colors">{project.name}</h5>
                <p className="mt-2 line-clamp-2 text-[13px] text-text-secondary leading-relaxed">
                  {project.description || "Open this board to manage tasks and collaborators."}
                </p>
              </Link>
            ))}

            {projectsQuery.data?.items.length === 0 ? (
              <div className="rounded-[12px] border border-dashed border-border bg-surface-muted p-6 text-center text-[13px] font-[500] text-text-tertiary">
                Belum ada project. Buat board pertama dari halaman `Projects`.
              </div>
            ) : null}
          </div>
        </Card>

        <Card className="p-8">
          <div className="border-b border-border pb-4">
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary mb-1">
              Recommended flow
            </p>
            <h4 className="text-[20px] font-[700] text-text-primary">Cara pakai paling natural</h4>
          </div>
          <div className="mt-6 space-y-3">
            {[
              "Buka `Projects` dari sidebar, lalu pilih board yang ingin dikerjakan.",
              "Kelola task langsung dari board, bukan dari form detail project.",
              "Undang member dari panel settings project, lalu atur role di project.",
              "Buka `Automation` saat ingin mengaktifkan auto assign rules per project.",
            ].map((step) => (
              <div
                className="relative pl-6 text-[13px] text-text-secondary leading-relaxed before:absolute before:left-0 before:top-2 before:h-1.5 before:w-1.5 before:rounded-full before:bg-ops/40"
                key={step}
              >
                {step}
              </div>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}

function HubMetric({
  icon: Icon,
  title,
  value,
  description,
}: {
  icon: typeof FolderKanban;
  title: string;
  value: string;
  description: string;
}) {
  return (
    <div className="rounded-[12px] border border-border bg-surface p-5 shadow-sm transition-all hover:border-ops/30">
      <div className="flex items-center justify-between gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-[8px] bg-ops/10 text-ops">
          <Icon className="h-5 w-5" />
        </div>
        <p className="text-[24px] font-[700] text-text-primary leading-none">{value}</p>
      </div>
      <p className="mt-4 text-[14px] font-[600] text-text-primary">{title}</p>
      <p className="mt-1 text-[12px] font-[500] text-text-secondary">{description}</p>
    </div>
  );
}

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
      <Card className="overflow-hidden p-8">
        <div className="grid gap-8 lg:grid-cols-[1.15fr_0.85fr]">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
              Operational workspace
            </p>
            <h3 className="mt-3 max-w-2xl text-4xl font-bold leading-tight">
              Board-centric workflow untuk project, member assignment, dan automation.
            </h3>
            <p className="mt-4 max-w-2xl text-muted-foreground">
              Masuk ke `Projects` untuk pengalaman utama ala Trello. Dari sana user bisa
              mengelola board, membuka task detail, invite member, dan mengaktifkan auto assign.
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <Link
                className="inline-flex h-11 items-center justify-center rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition hover:opacity-95"
                to="/operational/projects"
              >
                Open project boards
              </Link>
              <Link
                className="inline-flex h-11 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted"
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
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
                Recent boards
              </p>
              <h4 className="mt-2 text-2xl font-bold">Jump back into work</h4>
            </div>
            <Link
              className="text-sm font-medium text-primary underline-offset-4 hover:underline"
              to="/operational/projects"
            >
              View all projects
            </Link>
          </div>

          <div className="mt-6 grid gap-4 md:grid-cols-2">
            {projectsQuery.data?.items.map((project) => (
              <Link
                className="rounded-[28px] border border-border/70 bg-background/80 p-5 transition hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-panel"
                key={project.id}
                params={{ projectId: project.id }}
                search={{ view: "board" }}
                to="/operational/projects/$projectId"
              >
                <div className="flex items-center justify-between gap-3">
                  <span className="rounded-full bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-secondary-foreground">
                    {project.status.replace("_", " ")}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {project.member_count} members
                  </span>
                </div>
                <h5 className="mt-4 text-lg font-semibold">{project.name}</h5>
                <p className="mt-2 line-clamp-2 text-sm text-muted-foreground">
                  {project.description || "Open this board to manage tasks and collaborators."}
                </p>
              </Link>
            ))}

            {projectsQuery.data?.items.length === 0 ? (
              <div className="rounded-[28px] border border-dashed border-border/80 bg-background/80 p-6 text-sm text-muted-foreground">
                Belum ada project. Buat board pertama dari halaman `Projects`.
              </div>
            ) : null}
          </div>
        </Card>

        <Card className="p-8">
          <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
            Recommended flow
          </p>
          <h4 className="mt-2 text-2xl font-bold">Cara pakai paling natural</h4>
          <div className="mt-6 space-y-4">
            {[
              "Buka `Projects` dari sidebar, lalu pilih board yang ingin dikerjakan.",
              "Kelola task langsung dari board, bukan dari form detail project.",
              "Undang member dari panel settings project, lalu atur role di project.",
              "Buka `Automation` saat ingin mengaktifkan auto assign rules per project.",
            ].map((step) => (
              <div
                className="rounded-[24px] border border-border/70 bg-background/80 px-4 py-4 text-sm text-muted-foreground"
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
    <div className="rounded-[28px] border border-border/70 bg-background/80 p-5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-primary/15 text-primary">
          <Icon className="h-5 w-5" />
        </div>
        <p className="text-xl font-bold">{value}</p>
      </div>
      <p className="mt-4 text-sm font-semibold">{title}</p>
      <p className="mt-1 text-sm text-muted-foreground">{description}</p>
    </div>
  );
}

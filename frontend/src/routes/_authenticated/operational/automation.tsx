import { useQuery } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";

import { Card } from "@/components/ui/card";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { listProjects } from "@/services/operational-projects";

export const Route = createFileRoute("/_authenticated/operational/automation")({
  beforeLoad: async () => {
    await ensurePermission(permissions.operationalAssignmentView);
  },
  component: OperationalAutomationPage,
});

function OperationalAutomationPage() {
  const projectsQuery = useQuery({
    queryKey: ["operational", "automation", "projects"],
    queryFn: () =>
      listProjects({
        page: 1,
        perPage: 20,
        search: "",
        status: "",
        priority: "",
      }),
  });

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
          Automation
        </p>
        <h3 className="mt-3 text-3xl font-bold">Assignment rules per project</h3>
        <p className="mt-3 max-w-3xl text-muted-foreground">
          Auto assign tetap bersifat per-project. Pilih board di bawah ini untuk membuka
          panel automation langsung pada project yang relevan.
        </p>
      </Card>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {projectsQuery.data?.items.map((project) => (
          <Link
            className="rounded-[28px] border border-border/70 bg-card/80 p-6 transition hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-panel"
            key={project.id}
            params={{ projectId: project.id }}
            search={{ view: "automation" }}
            to="/operational/projects/$projectId"
          >
            <div className="flex items-center justify-between gap-3">
              <span className="rounded-full bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-secondary-foreground">
                {project.priority}
              </span>
              <span className="text-xs text-muted-foreground">
                {project.member_count} members
              </span>
            </div>
            <h4 className="mt-4 text-xl font-semibold">{project.name}</h4>
            <p className="mt-2 line-clamp-2 text-sm text-muted-foreground">
              {project.description || "Open this board to configure rule priority and auto assign flow."}
            </p>
            <p className="mt-5 text-sm font-medium text-primary">Open automation panel</p>
          </Link>
        ))}

        {projectsQuery.data?.items.length === 0 ? (
          <Card className="p-6 text-sm text-muted-foreground">
            Belum ada project. Buat project dulu dari halaman `Projects`, lalu automation bisa
            diaktifkan per board.
          </Card>
        ) : null}
      </div>
    </div>
  );
}

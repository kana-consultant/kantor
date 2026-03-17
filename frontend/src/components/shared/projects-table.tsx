import { CalendarDays, FolderKanban, Users } from "lucide-react";
import { Link } from "@tanstack/react-router";

import { Card } from "@/components/ui/card";
import type { Project } from "@/types/project";

interface ProjectsTableProps {
  projects: Project[];
  canDelete: boolean;
  onDelete: (projectId: string) => void;
  deletingId?: string | null;
}

export function ProjectsTable({
  projects,
  canDelete,
  onDelete,
  deletingId,
}: ProjectsTableProps) {
  return (
    <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
      {projects.map((project) => (
        <Card
          className="overflow-hidden border-border/70 bg-card/85 p-6 transition hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-panel"
          key={project.id}
        >
          <div className="flex items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2">
                <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-primary/15 text-primary">
                  <FolderKanban className="h-5 w-5" />
                </div>
                <div>
                  <p className="text-sm font-semibold">{project.name}</p>
                  <p className="text-xs text-muted-foreground">Board workspace</p>
                </div>
              </div>
            </div>
            <StatusBadge value={project.status} />
          </div>

          <p className="mt-4 line-clamp-2 min-h-[2.75rem] text-sm text-muted-foreground">
            {project.description || "Open this board to manage cards, members, and assignment rules."}
          </p>

          <div className="mt-5 grid gap-3 sm:grid-cols-3">
            <InfoPill icon={Users} label="Members" value={String(project.member_count)} />
            <InfoPill icon={CalendarDays} label="Deadline" value={project.deadline ? new Date(project.deadline).toLocaleDateString() : "-"} />
            <PriorityBadge value={project.priority} />
          </div>

          <div className="mt-6 flex flex-wrap gap-3">
            <Link
              className="inline-flex h-10 items-center justify-center rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition hover:opacity-95"
              params={{ projectId: project.id }}
              search={{ view: "board" }}
              to="/operational/projects/$projectId"
            >
              Open board
            </Link>
            <Link
              className="inline-flex h-10 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted"
              params={{ projectId: project.id }}
              search={{ view: "automation" }}
              to="/operational/projects/$projectId"
            >
              Automation
            </Link>
            {canDelete ? (
              <button
                className="inline-flex h-10 items-center justify-center rounded-full px-4 text-sm font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
                disabled={deletingId === project.id}
                onClick={() => onDelete(project.id)}
                type="button"
              >
                {deletingId === project.id ? "Deleting..." : "Delete"}
              </button>
            ) : null}
          </div>
        </Card>
      ))}

      {projects.length === 0 ? (
        <Card className="p-8 text-center text-sm text-muted-foreground lg:col-span-2 2xl:col-span-3">
          No projects found for the current filter.
        </Card>
      ) : null}
    </div>
  );
}

function InfoPill({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof FolderKanban;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-[22px] border border-border/70 bg-background/80 px-4 py-3">
      <div className="flex items-center gap-2 text-muted-foreground">
        <Icon className="h-4 w-4" />
        <span className="text-xs uppercase tracking-[0.18em]">{label}</span>
      </div>
      <p className="mt-2 text-sm font-semibold">{value}</p>
    </div>
  );
}

function StatusBadge({ value }: { value: Project["status"] }) {
  return (
    <span className="rounded-full bg-secondary px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.18em] text-secondary-foreground">
      {value.replace("_", " ")}
    </span>
  );
}

function PriorityBadge({ value }: { value: Project["priority"] }) {
  const tones: Record<Project["priority"], string> = {
    low: "bg-sky-100 text-sky-700",
    medium: "bg-amber-100 text-amber-700",
    high: "bg-orange-100 text-orange-700",
    critical: "bg-red-100 text-red-700",
  };

  return (
    <div className="rounded-[22px] border border-border/70 bg-background/80 px-4 py-3">
      <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Priority</p>
      <span className={`mt-2 inline-flex rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] ${tones[value]}`}>
        {value}
      </span>
    </div>
  );
}

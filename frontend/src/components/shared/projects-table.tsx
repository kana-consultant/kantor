import { CalendarDays, FolderKanban, Users } from "lucide-react";
import { Link } from "@tanstack/react-router";

import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import type { Project } from "@/types/project";
import { cn } from "@/lib/utils";

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
  if (projects.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center p-12 text-center rounded-[12px] border-2 border-dashed border-border bg-surface-muted/50">
        <FolderKanban className="w-10 h-10 text-text-tertiary mb-3" strokeWidth={1.5} />
        <h3 className="text-[14px] font-[600] text-text-primary">No projects found</h3>
        <p className="mt-1 text-[13px] text-text-secondary">
          No projects match the current filter criteria.
        </p>
      </div>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {projects.map((project) => (
        <Card
          className="overflow-hidden p-5 flex flex-col transition-shadow hover:shadow-lg"
          key={project.id}
        >
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-start gap-3">
              <div className="flex shrink-0 h-10 w-10 items-center justify-center rounded-[8px] bg-ops-light text-ops">
                <FolderKanban className="h-5 w-5" strokeWidth={1.5} />
              </div>
              <div>
                <p className="text-[15px] font-[600] text-text-primary leading-tight">{project.name}</p>
                <p className="text-[12px] text-text-tertiary mt-1">Board workspace</p>
              </div>
            </div>
          </div>

          <p className="mt-3 text-[13px] text-text-secondary line-clamp-2 min-h-[36px]">
            {project.description || "Open this board to manage cards, members, and assignment rules."}
          </p>

          <div className="mt-4 grid grid-cols-2 gap-2">
            <InfoPill icon={Users} label="Members" value={String(project.member_count)} />
            <InfoPill icon={CalendarDays} label="Deadline" value={project.deadline ? new Date(project.deadline).toLocaleDateString() : "-"} />
            
            <div className="col-span-2 flex items-center justify-between gap-2 mt-1">
               <StatusBadge value={project.status} />
               <PriorityBadge value={project.priority} />
            </div>
          </div>

          <div className="mt-5 flex items-center gap-2 pt-4 border-t border-border mt-auto">
            <Link
              className={cn("inline-flex items-center justify-center gap-2 font-[600] transition-colors focus-visible:outline-none focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50", 
                 "h-[36px] px-3 rounded-[6px] text-[13px] bg-ops text-white hover:bg-ops-dark focus-visible:ring-ops/50 flex-1"
              )}
              params={{ projectId: project.id }}
              search={{ view: "board" }}
              to="/operational/projects/$projectId"
            >
              Open board
            </Link>
            <Link
              className={cn("inline-flex items-center justify-center gap-2 font-[500] transition-colors focus-visible:outline-none focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50", 
                 "h-[36px] px-3 rounded-[6px] text-[13px] bg-surface-muted text-text-primary border border-border hover:bg-border/50 focus-visible:ring-ops/50 flex-1"
              )}
              params={{ projectId: project.id }}
              search={{ view: "automation" }}
              to="/operational/projects/$projectId"
            >
              Automation
            </Link>
          </div>
          {canDelete && (
             <div className="mt-2 text-center">
                <button
                  className="text-[12px] font-[500] text-text-tertiary hover:text-priority-high transition-colors disabled:opacity-50"
                  disabled={deletingId === project.id}
                  onClick={() => onDelete(project.id)}
                  type="button"
                >
                  {deletingId === project.id ? "Deleting..." : "Delete Project"}
                </button>
             </div>
          )}
        </Card>
      ))}
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
    <div className="rounded-[8px] bg-surface-muted px-3 py-2 flex flex-col justify-center">
      <div className="flex items-center gap-1.5 text-text-secondary mb-1">
        <Icon className="h-[14px] w-[14px]" strokeWidth={2} />
        <span className="text-[10px] uppercase tracking-wider font-[600]">{label}</span>
      </div>
      <p className="text-[13px] font-[600] text-text-primary truncate">{value}</p>
    </div>
  );
}

function StatusBadge({ value }: { value: Project["status"] }) {
  const getBadgeStyle = (status: Project["status"]) => {
    switch (status) {
      case "active":
        return "bg-sky-100 text-sky-700 border-sky-200";
      case "completed":
        return "bg-green-100 text-green-700 border-green-200";
      case "archived":
        return "bg-surface-muted text-text-secondary border-border";
      default:
        return "bg-surface-muted text-text-secondary border-border";
    }
  };

  return (
    <span className={cn(
      "inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-[600] uppercase tracking-wider border",
      getBadgeStyle(value)
    )}>
      {value.replace("_", " ")}
    </span>
  );
}

function PriorityBadge({ value }: { value: Project["priority"] }) {
  const tones: Record<Project["priority"], string> = {
    low: "bg-sky-100 text-sky-700 border-sky-200",
    medium: "bg-amber-100 text-amber-700 border-amber-200",
    high: "bg-orange-100 text-orange-700 border-orange-200",
    critical: "bg-red-100 text-red-700 border-red-200",
  };

  return (
    <span className={cn(
      "inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-[600] uppercase tracking-wider border",
      tones[value]
    )}>
      {value} Priority
    </span>
  );
}

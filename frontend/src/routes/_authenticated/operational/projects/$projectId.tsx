import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { z } from "zod";

import { AssignmentRulesPanel } from "@/components/shared/assignment-rules-panel";
import { KanbanBoard } from "@/components/shared/kanban-board";
import { PermissionGate } from "@/components/shared/permission-gate";
import { ProjectForm } from "@/components/shared/project-form";
import { StatusBadge as SharedStatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import {
  deleteProject,
  getProject,
  mutateProjectMember,
  projectsKeys,
  updateProject,
} from "@/services/operational-projects";
import type { ProjectFormValues } from "@/types/project";

const searchSchema = z.object({
  view: z.enum(["board", "settings", "automation"]).optional().catch("board"),
});

export const Route = createFileRoute("/_authenticated/operational/projects/$projectId")({
  validateSearch: searchSchema,
  beforeLoad: async () => {
    await ensurePermission(permissions.operationalProjectView);
  },
  component: ProjectWorkspacePage,
});

function ProjectWorkspacePage() {
  const { projectId } = Route.useParams();
  const search = Route.useSearch();
  const activeView = search.view ?? "board";
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [memberUserRef, setMemberUserRef] = useState("");
  const [memberRole, setMemberRole] = useState("");

  const projectQuery = useQuery({
    queryKey: projectsKeys.detail(projectId),
    queryFn: () => getProject(projectId),
  });

  const project = projectQuery.data?.project;
  const members = projectQuery.data?.members ?? [];

  const editDefaults = useMemo<ProjectFormValues | undefined>(() => {
    if (!project) {
      return undefined;
    }

    return {
      name: project.name,
      description: project.description ?? "",
      deadline: project.deadline ? project.deadline.slice(0, 10) : "",
      status: project.status,
      priority: project.priority,
    };
  }, [project]);

  const updateMutation = useMutation({
    mutationFn: (values: ProjectFormValues) => updateProject(projectId, values),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.detail(projectId) });
      await queryClient.invalidateQueries({ queryKey: projectsKeys.all });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteProject(projectId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.all });
      void navigate({ to: "/operational/projects" });
    },
  });

  const memberMutation = useMutation({
    mutationFn: (payload: {
      operation: "assign" | "remove";
      user_id?: string;
      user_email?: string;
      role_in_project?: string;
    }) => mutateProjectMember(projectId, payload),
    onSuccess: async () => {
      setMemberUserRef("");
      setMemberRole("");
      await queryClient.invalidateQueries({ queryKey: projectsKeys.detail(projectId) });
      await queryClient.invalidateQueries({ queryKey: projectsKeys.all });
    },
  });

  if (projectQuery.isLoading) {
    return (
      <div className="space-y-6">
        <Card className="overflow-hidden border-ops/20 bg-gradient-to-br from-ops/10 via-background to-background p-8">
          <div className="flex flex-col gap-6 xl:flex-row xl:items-start xl:justify-between">
            <div className="flex-1 w-full">
              <Skeleton className="h-4 w-[250px] bg-muted/60" />
              <div className="mt-5 flex flex-wrap items-center gap-3">
                <Skeleton className="h-7 w-[90px] rounded-full bg-muted/60" />
                <Skeleton className="h-7 w-[80px] rounded-full bg-muted/60" />
                <Skeleton className="h-7 w-[100px] rounded-full bg-muted/60" />
              </div>
              <Skeleton className="mt-5 h-10 w-[400px] max-w-full bg-muted/60" />
              <Skeleton className="mt-3 h-5 w-[500px] max-w-full bg-muted/60" />
              <div className="mt-6 flex flex-wrap gap-3">
                <Skeleton className="h-10 w-[80px] rounded-[6px] bg-muted/60" />
                <Skeleton className="h-10 w-[90px] rounded-[6px] bg-muted/60" />
                <Skeleton className="h-10 w-[120px] rounded-[6px] bg-muted/60" />
              </div>
            </div>
            <Skeleton className="h-[180px] w-full min-w-[20rem] xl:w-[320px] rounded-[28px] bg-muted/60" />
          </div>
        </Card>
        
        <div className="grid gap-6 2xl:grid-cols-[minmax(0,1fr)_360px]">
          <Skeleton className="h-[500px] w-full rounded-[24px] bg-muted/60" />
          <div className="space-y-6">
            <Skeleton className="h-[250px] w-full rounded-[24px] bg-muted/60" />
            <Skeleton className="h-[300px] w-full rounded-[24px] bg-muted/60" />
          </div>
        </div>
      </div>
    );
  }

  if (projectQuery.error instanceof Error || !project) {
    return (
      <Card className="p-8 text-error">
        {projectQuery.error instanceof Error ? projectQuery.error.message : "Project not found"}
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <Card className="overflow-hidden border-ops/20 bg-gradient-to-br from-ops/10 via-background to-background p-8">
        <div className="flex flex-col gap-6 xl:flex-row xl:items-start xl:justify-between">
          <div>
            <div className="flex flex-wrap items-center gap-2 text-sm font-medium text-muted-foreground">
              <Link
                className="underline-offset-4 hover:underline"
                to="/operational"
              >
                Operational
              </Link>
              <span>/</span>
              <Link
                className="underline-offset-4 hover:underline"
                to="/operational/projects"
              >
                Projects
              </Link>
              <span>/</span>
              <span>{project.name}</span>
            </div>

            <div className="mt-5 flex flex-wrap items-center gap-3">
              <SharedStatusBadge status={project.status} variant="project-status" />
              <SharedStatusBadge status={project.priority} variant="priority" />
              <Badge value={`${project.member_count} members`} />
              <Badge
                value={project.deadline ? `Due ${new Date(project.deadline).toLocaleDateString()}` : "No deadline"}
              />
            </div>

            <h3 className="mt-5 max-w-4xl text-4xl font-bold leading-tight tracking-tight text-foreground">{project.name}</h3>
            <p className="mt-3 max-w-3xl text-muted-foreground">
              {project.description || "This board is ready for daily task execution, collaboration, and automation."}
            </p>

            <div className="mt-6 flex flex-wrap gap-3">
              {(["board", "settings", "automation"] as const).map((view) => (
                <Button
                  key={view}
                  onClick={() =>
                    void navigate({
                      to: "/operational/projects/$projectId",
                      params: { projectId },
                      search: { view },
                    })
                  }
                  variant={activeView === view ? "ops" : "outline"}
                >
                  {view}
                </Button>
              ))}
            </div>
          </div>

          <div className="grid min-w-[20rem] gap-4 rounded-[28px] border border-ops/20 bg-background/80 p-5 shadow-sm backdrop-blur-md">
            <div>
              <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                Team pulse
              </p>
              <div className="mt-4 flex flex-wrap -space-x-3">
                {members.slice(0, 5).map((member) => (
                  <div
                    className="flex h-12 w-12 items-center justify-center rounded-full border-2 border-background bg-primary/15 text-sm font-semibold uppercase text-primary"
                    key={member.user_id}
                    title={member.full_name || member.user_email || member.user_id}
                  >
                    {initials(member.full_name || member.user_email || member.user_id)}
                  </div>
                ))}
                {members.length === 0 ? (
                  <div className="text-sm text-muted-foreground">No members yet</div>
                ) : null}
              </div>
            </div>

            <div className="grid gap-3">
              <Link
                className="inline-flex h-11 items-center justify-center rounded-full bg-ops px-5 text-sm font-medium text-white transition hover:opacity-95 shadow-sm"
                params={{ projectId }}
                search={{ view: "board" }}
                to="/operational/projects/$projectId"
              >
                Open board
              </Link>
              <Link
                className="inline-flex h-11 items-center justify-center rounded-full border border-border bg-card px-5 text-sm font-medium transition hover:bg-muted"
                params={{ projectId }}
                search={{ view: "automation" }}
                to="/operational/projects/$projectId"
              >
                Manage automation
              </Link>
            </div>
          </div>
        </div>
      </Card>

      {activeView === "board" ? (
        <div className="grid gap-6 2xl:grid-cols-[minmax(0,1fr)_360px]">
          <PermissionGate
            fallback={
              <Card className="p-8 text-sm text-muted-foreground">
                You do not have permission to view this board.
              </Card>
            }
            permission={permissions.operationalKanbanView}
          >
            <KanbanBoard members={members} projectId={projectId} />
          </PermissionGate>

          <div className="space-y-6">
            <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
              <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                Board summary
              </p>
              <div className="mt-4 grid gap-3">
                <SummaryRow label="Status" value={project.status.replace("_", " ")} />
                <SummaryRow label="Priority" value={project.priority} />
                <SummaryRow
                  label="Deadline"
                  value={project.deadline ? new Date(project.deadline).toLocaleDateString() : "-"}
                />
                <SummaryRow label="Members" value={String(project.member_count)} />
              </div>
            </Card>

            <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                    People on this board
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Invite by email or remove member without leaving the board.
                  </p>
                </div>
                <Button
                  onClick={() =>
                    void navigate({
                      to: "/operational/projects/$projectId",
                      params: { projectId },
                      search: { view: "settings" },
                    })
                  }
                  size="sm"
                  variant="outline"
                >
                  Manage
                </Button>
              </div>

              <div className="mt-5 space-y-3">
                {members.map((member) => (
                  <div
                    className="flex items-center justify-between gap-3 rounded-[22px] border border-border/70 bg-background/80 px-4 py-3"
                    key={member.user_id}
                  >
                    <div className="flex items-center gap-3">
                      <div className="flex h-11 w-11 items-center justify-center rounded-full bg-primary/15 text-xs font-semibold uppercase text-primary">
                        {initials(member.full_name || member.user_email || member.user_id)}
                      </div>
                      <div className="min-w-0">
                        <p className="truncate font-semibold">
                          {member.full_name || member.user_id}
                        </p>
                        <p className="truncate text-sm text-muted-foreground">
                          {member.user_email || member.user_id}
                        </p>
                      </div>
                    </div>
                    <span className="rounded-full bg-secondary px-3 py-1 text-xs font-medium text-secondary-foreground">
                      {member.role_in_project}
                    </span>
                  </div>
                ))}

                {members.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No members assigned yet.</p>
                ) : null}
              </div>
            </Card>
          </div>
        </div>
      ) : null}

      {activeView === "settings" ? (
        <div className="grid gap-6 xl:grid-cols-[0.95fr_1.05fr]">
          {editDefaults ? (
            <ProjectForm
              defaultValues={editDefaults}
              description="Edit core project information without interrupting the main board workflow."
              isSubmitting={updateMutation.isPending}
              onCancel={() =>
                void navigate({
                  to: "/operational/projects/$projectId",
                  params: { projectId },
                  search: { view: "board" },
                })
              }
              onSubmit={(values) => updateMutation.mutate(values)}
              submitLabel="Save project"
              title="Project settings"
            />
          ) : null}

          <div className="space-y-6">
            <PermissionGate permission={permissions.operationalProjectEdit}>
              <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
                <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                  Invite or remove member
                </p>
                <div className="mt-4 grid gap-4 md:grid-cols-[1.2fr_1fr_auto_auto]">
                  <Input
                    onChange={(event) => setMemberUserRef(event.target.value)}
                    placeholder="Email or user ID"
                    value={memberUserRef}
                  />
                  <Input
                    onChange={(event) => setMemberRole(event.target.value)}
                    placeholder="Role in project"
                    value={memberRole}
                  />
                  <Button
                    disabled={memberMutation.isPending}
                    onClick={() =>
                      memberMutation.mutate({
                        operation: "assign",
                        ...(memberUserRef.includes("@")
                          ? { user_email: memberUserRef }
                          : { user_id: memberUserRef }),
                        role_in_project: memberRole,
                      })
                    }
                    variant="ops"
                  >
                    Assign
                  </Button>
                  <Button
                    disabled={memberMutation.isPending}
                    onClick={() =>
                      memberMutation.mutate({
                        operation: "remove",
                        ...(memberUserRef.includes("@")
                          ? { user_email: memberUserRef }
                          : { user_id: memberUserRef }),
                      })
                    }
                    variant="ghost"
                  >
                    Remove
                  </Button>
                </div>
                <p className="mt-3 text-sm text-muted-foreground">
                  Gunakan email seed seperti `staff.ops@kantor.local` atau `viewer.ops@kantor.local`
                  agar tidak perlu lookup UUID.
                </p>
              </Card>
            </PermissionGate>

            <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                    Current members
                  </p>
                  <h4 className="mt-2 text-xl font-bold tracking-tight text-foreground">Project collaborators</h4>
                </div>
                <PermissionGate permission={permissions.operationalProjectDelete}>
                  <Button
                    disabled={deleteMutation.isPending}
                    onClick={() => {
                      if (window.confirm(`Delete project "${project.name}"?`)) {
                        deleteMutation.mutate();
                      }
                    }}
                    variant="ghost"
                  >
                    {deleteMutation.isPending ? "Deleting..." : "Delete project"}
                  </Button>
                </PermissionGate>
              </div>

              <div className="mt-5 space-y-3">
                {members.map((member) => (
                  <div
                    className="flex items-center justify-between gap-3 rounded-[22px] border border-border/70 bg-background/80 px-4 py-3"
                    key={member.user_id}
                  >
                    <div>
                      <p className="font-semibold">{member.full_name || member.user_id}</p>
                      <p className="text-sm text-muted-foreground">
                        {member.user_email || member.user_id}
                      </p>
                    </div>
                    <Badge value={member.role_in_project} />
                  </div>
                ))}
              </div>
            </Card>
          </div>
        </div>
      ) : null}

      {activeView === "automation" ? (
        <PermissionGate
          fallback={
            <Card className="p-8 text-sm text-muted-foreground">
              You do not have permission to access automation for this project.
            </Card>
          }
          permission={permissions.operationalAssignmentView}
        >
          <AssignmentRulesPanel projectId={projectId} />
        </PermissionGate>
      ) : null}
    </div>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-[22px] border border-border/70 bg-background/80 px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-semibold capitalize">{value}</span>
    </div>
  );
}

function Badge({ value }: { value: string }) {
  return (
    <span className="rounded-full border border-border bg-background/80 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
      {value}
    </span>
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

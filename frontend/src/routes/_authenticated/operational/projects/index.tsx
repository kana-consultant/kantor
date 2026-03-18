import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Users } from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { PermissionGate } from "@/components/shared/permission-gate";
import { ProjectForm } from "@/components/shared/project-form";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import {
  createProject,
  deleteProject,
  listProjects,
  projectsKeys,
} from "@/services/operational-projects";
import type { Project, ProjectFilters } from "@/types/project";

const defaultFilters: ProjectFilters = {
  page: 1,
  perPage: 10,
  search: "",
  status: "",
  priority: "",
};

export const Route = createFileRoute("/_authenticated/operational/projects/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.operationalProjectView);
  },
  component: ProjectsListPage,
});

function ProjectsListPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [filters, setFilters] = useState<ProjectFilters>(defaultFilters);
  const [isCreateOpen, setIsCreateOpen] = useState(false);

  const projectsQuery = useQuery({
    queryKey: projectsKeys.list(filters),
    queryFn: () => listProjects(filters),
  });

  const createMutation = useMutation({
    mutationFn: createProject,
    onSuccess: async () => {
      setIsCreateOpen(false);
      await queryClient.invalidateQueries({ queryKey: projectsKeys.all });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteProject,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.all });
    },
  });

  const projects = projectsQuery.data?.items ?? [];
  const meta = projectsQuery.data?.meta;
  const columns: Array<DataTableColumn<Project>> = [
    {
      id: "name",
      header: "Project",
      accessor: "name",
      sortable: true,
      cell: (project) => (
        <div className="space-y-1">
          <p className="font-semibold text-text-primary">{project.name}</p>
          <p className="line-clamp-1 text-[13px] text-text-secondary">
            {project.description || "Project board ready for execution."}
          </p>
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      accessor: "status",
      sortable: true,
      cell: (project) => <StatusBadge status={project.status} variant="project-status" />,
    },
    {
      id: "priority",
      header: "Priority",
      accessor: "priority",
      sortable: true,
      cell: (project) => <StatusBadge status={project.priority} variant="priority" />,
    },
    {
      id: "deadline",
      header: "Deadline",
      accessor: "deadline",
      sortable: true,
      cell: (project) => (
        <span className="text-sm text-text-secondary">
          {project.deadline ? new Date(project.deadline).toLocaleDateString("id-ID") : "-"}
        </span>
      ),
    },
    {
      id: "members",
      header: "Members",
      accessor: "member_count",
      align: "right",
      numeric: true,
      sortable: true,
      cell: (project) => (
        <div className="inline-flex items-center justify-end gap-2 font-mono tabular-nums">
          <Users className="h-4 w-4 text-text-tertiary" />
          <span>{project.member_count}</span>
        </div>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (project) => (
        <div className="flex justify-end gap-2">
          <Link
            className="inline-flex h-9 items-center justify-center rounded-md bg-module px-4 text-sm font-semibold text-white transition hover:brightness-95"
            params={{ projectId: project.id }}
            search={{ view: "board" }}
            to="/operational/projects/$projectId"
          >
            Open
          </Link>
          <Link
            className="inline-flex h-9 items-center justify-center rounded-md border border-border bg-surface px-4 text-sm font-semibold text-text-primary transition hover:bg-surface-muted"
            params={{ projectId: project.id }}
            search={{ view: "automation" }}
            to="/operational/projects/$projectId"
          >
            Automation
          </Link>
          {hasPermission(permissions.operationalProjectDelete) ? (
            <Button
              disabled={deleteMutation.isPending && deleteMutation.variables === project.id}
              onClick={() => deleteMutation.mutate(project.id)}
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
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-ops">
              Operational boards
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">Projects</h3>
            <p className="mt-2 max-w-2xl text-[14px] leading-relaxed text-text-secondary">
              Scan active projects, open the board directly, or jump into project automation settings.
            </p>
          </div>

          <PermissionGate permission={permissions.operationalProjectCreate}>
            <Button onClick={() => setIsCreateOpen((value) => !value)}>
              {isCreateOpen ? "Close form" : "Create project"}
            </Button>
          </PermissionGate>
        </div>
      </Card>

      {isCreateOpen ? (
        <ProjectForm
          description="Form create project menggunakan React Hook Form + Zod."
          isSubmitting={createMutation.isPending}
          onCancel={() => setIsCreateOpen(false)}
          onSubmit={(values) => createMutation.mutate(values)}
          submitLabel="Create project"
          title="New Project"
        />
      ) : null}

      <Card className="p-6">
        <div className="grid gap-4 md:grid-cols-4">
          <Input
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                page: 1,
                search: event.target.value,
              }))
            }
            placeholder="Search by project name"
            value={filters.search}
          />
          <select
            className="field-select"
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                page: 1,
                status: event.target.value,
              }))
            }
            value={filters.status}
          >
            <option value="">All statuses</option>
            <option value="draft">Draft</option>
            <option value="active">Active</option>
            <option value="on_hold">On Hold</option>
            <option value="completed">Completed</option>
            <option value="archived">Archived</option>
          </select>
          <select
            className="field-select"
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                page: 1,
                priority: event.target.value,
              }))
            }
            value={filters.priority}
          >
            <option value="">All priorities</option>
            <option value="low">Low</option>
            <option value="medium">Medium</option>
            <option value="high">High</option>
            <option value="critical">Critical</option>
          </select>
          <select
            className="field-select"
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                page: 1,
                perPage: Number(event.target.value),
              }))
            }
            value={filters.perPage}
          >
            <option value={10}>10 per page</option>
            <option value={20}>20 per page</option>
            <option value={50}>50 per page</option>
          </select>
        </div>
      </Card>

      {projectsQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-error">{projectsQuery.error.message}</Card>
      ) : null}

      <DataTable
        columns={columns}
        data={projects}
        emptyActionLabel={hasPermission(permissions.operationalProjectCreate) ? "Create project" : undefined}
        emptyDescription="No projects match the current filter. Create a new board or widen the filter."
        emptyTitle="No projects found"
        getRowId={(project) => project.id}
        loading={projectsQuery.isLoading}
        loadingRows={6}
        onEmptyAction={hasPermission(permissions.operationalProjectCreate) ? () => setIsCreateOpen(true) : undefined}
        pagination={
          meta
            ? {
                page: meta.page,
                perPage: meta.per_page,
                total: meta.total,
                onPageChange: (page) => setFilters((current) => ({ ...current, page })),
              }
            : undefined
        }
      />
    </div>
  );
}

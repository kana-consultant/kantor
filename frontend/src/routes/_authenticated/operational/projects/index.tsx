import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { PermissionGate } from "@/components/shared/permission-gate";
import { ProjectForm } from "@/components/shared/project-form";
import { ProjectsTable } from "@/components/shared/projects-table";
import { useRBAC } from "@/hooks/use-rbac";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import {
  createProject,
  deleteProject,
  listProjects,
  projectsKeys,
} from "@/services/operational-projects";
import type { ProjectFilters } from "@/types/project";

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

  const meta = projectsQuery.data?.meta;

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
              Operational boards
            </p>
            <h3 className="mt-2 text-3xl font-bold">Projects</h3>
            <p className="mt-2 max-w-2xl text-muted-foreground">
              Halaman ini sekarang berfungsi sebagai board directory. User bisa scan project,
              buka board utama, atau langsung masuk ke automation rules dari kartu project.
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
          <input
            className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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
            className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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
            className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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
            className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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

      <Card className="p-5 text-sm text-muted-foreground">
        Pilih `Open board` untuk masuk ke workspace utama ala Trello. Gunakan `Automation`
        jika ingin langsung mengatur auto assign rules per project.
      </Card>

      {projectsQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-red-700">{projectsQuery.error.message}</Card>
      ) : null}

      <ProjectsTable
        canDelete={hasPermission(permissions.operationalProjectDelete)}
        deletingId={deleteMutation.isPending ? deleteMutation.variables ?? null : null}
        onDelete={(projectId) => deleteMutation.mutate(projectId)}
        projects={projectsQuery.data?.items ?? []}
      />

      <Card className="flex flex-col gap-4 p-6 md:flex-row md:items-center md:justify-between">
        <p className="text-sm text-muted-foreground">
          Page {meta?.page ?? filters.page} of{" "}
          {meta ? Math.max(1, Math.ceil(meta.total / meta.per_page)) : 1} · Total{" "}
          {meta?.total ?? 0} projects
        </p>
        <div className="flex gap-3">
          <Button
            disabled={(meta?.page ?? filters.page) <= 1}
            onClick={() =>
              setFilters((current) => ({ ...current, page: Math.max(1, current.page - 1) }))
            }
            variant="outline"
          >
            Previous
          </Button>
          <Button
            disabled={meta ? meta.page * meta.per_page >= meta.total : true}
            onClick={() =>
              setFilters((current) => ({ ...current, page: current.page + 1 }))
            }
            variant="outline"
          >
            Next
          </Button>
        </div>
      </Card>
    </div>
  );
}

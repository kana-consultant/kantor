import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Plus, Users } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { ExportButton } from "@/components/shared/export-button";
import { PermissionGate } from "@/components/shared/permission-gate";
import { ProjectForm } from "@/components/shared/project-form";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
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
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalProjectView);
  },
  component: ProjectsListPage,
});

function ProjectsListPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [filters, setFilters] = useState<ProjectFilters>(defaultFilters);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [projectToDelete, setProjectToDelete] = useState<Project | null>(null);

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
      setProjectToDelete(null);
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
      mobilePrimary: true,
      sortable: true,
      cell: (project) => (
        <div className="space-y-1">
          <p className="font-semibold text-text-primary">{project.name}</p>
          <p className="line-clamp-1 text-[13px] text-text-secondary">
            {project.description || "Board project siap dipakai untuk eksekusi."}
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
      header: "Anggota",
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
      header: "Aksi",
      align: "right",
      cell: (project) => (
        <div className="flex flex-wrap justify-end gap-2">
          <Link
            className="inline-flex h-9 items-center justify-center rounded-md bg-module px-4 text-sm font-semibold text-white transition hover:brightness-95"
            params={{ projectId: project.id }}
            search={{ view: "board" }}
            to="/operational/projects/$projectId"
          >
            Buka
          </Link>
          <Link
            className="inline-flex h-9 items-center justify-center rounded-md border border-border bg-surface px-4 text-sm font-semibold text-text-primary transition hover:bg-surface-muted"
            params={{ projectId: project.id }}
            search={{ view: "settings" }}
            to="/operational/projects/$projectId"
          >
            Pengaturan
          </Link>
          {hasPermission(permissions.operationalProjectDelete) ? (
            <Button
              disabled={deleteMutation.isPending && deleteMutation.variables === projectToDelete?.id}
              onClick={() => setProjectToDelete(project)}
              size="sm"
              type="button"
              variant="ghost"
            >
              Hapus
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
              Board operasional
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">Daftar project</h3>
            <p className="mt-2 max-w-2xl text-[14px] leading-relaxed text-text-secondary">
              Pantau project aktif, buka board langsung, atau atur konfigurasi project dari satu tempat.
            </p>
          </div>

          <div className="flex flex-wrap gap-3">
            <PermissionGate permission={permissions.operationalProjectView}>
              <ExportButton
                endpoint="/operational/projects/export"
                filename="projects-report"
                filters={{
                  priority: filters.priority,
                  search: filters.search,
                  status: filters.status,
                }}
                formats={["pdf", "xlsx"]}
              />
            </PermissionGate>
            <PermissionGate permission={permissions.operationalProjectCreate}>
              <Button onClick={() => setIsCreateOpen(true)}>
                <Plus className="h-4 w-4" />
                Buat project
              </Button>
            </PermissionGate>
          </div>
        </div>
      </Card>

      <ProjectForm
        description="Isi ringkasan project, timeline, prioritas, lalu pilih anggota tim beserta perannya sebelum board aktif."
        isOpen={isCreateOpen}
        isSubmitting={createMutation.isPending}
        onCancel={() => setIsCreateOpen(false)}
        onSubmit={(values) => createMutation.mutate(values)}
        showMemberPicker
        submitLabel="Buat project"
        title="Project baru"
      />

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
            placeholder="Cari nama project"
            value={filters.search}
          />
          <Select
            onValueChange={(value) =>
              setFilters((current) => ({
                ...current,
                page: 1,
                status: value,
              }))
            }
            options={[
              { value: "", label: "Semua status" },
              { value: "draft", label: "Draft" },
              { value: "active", label: "Aktif" },
              { value: "on_hold", label: "Ditunda" },
              { value: "completed", label: "Selesai" },
              { value: "archived", label: "Arsip" },
            ]}
            value={filters.status}
          />
          <Select
            onValueChange={(value) =>
              setFilters((current) => ({
                ...current,
                page: 1,
                priority: value,
              }))
            }
            options={[
              { value: "", label: "Semua prioritas" },
              { value: "low", label: "Rendah" },
              { value: "medium", label: "Sedang" },
              { value: "high", label: "Tinggi" },
              { value: "critical", label: "Kritis" },
            ]}
            value={filters.priority}
          />
          <Select
            onValueChange={(value) =>
              setFilters((current) => ({
                ...current,
                page: 1,
                perPage: Number(value),
              }))
            }
            options={[
              { value: "10", label: "10 per halaman" },
              { value: "20", label: "20 per halaman" },
              { value: "50", label: "50 per halaman" },
            ]}
            value={String(filters.perPage)}
          />
        </div>
      </Card>

      {projectsQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-error">{projectsQuery.error.message}</Card>
      ) : null}

      <DataTable
        columns={columns}
        data={projects}
        emptyActionLabel={hasPermission(permissions.operationalProjectCreate) ? "Buat project" : undefined}
        emptyDescription="Belum ada project yang cocok dengan filter saat ini. Coba longgarkan filter atau buat project baru."
        emptyTitle="Project belum ditemukan"
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

      <ConfirmDialog
        confirmLabel="Hapus project"
        description={
          projectToDelete
            ? `Semua task dan data board untuk "${projectToDelete.name}" akan ikut terhapus.`
            : ""
        }
        isLoading={deleteMutation.isPending}
        isOpen={Boolean(projectToDelete)}
        onClose={() => setProjectToDelete(null)}
        onConfirm={() => {
          if (projectToDelete) {
            deleteMutation.mutate(projectToDelete.id);
          }
        }}
        title={projectToDelete ? `Hapus ${projectToDelete.name}?` : "Hapus project?"}
      />
    </div>
  );
}

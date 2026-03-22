import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { z } from "zod";
import { Plus, UserMinus, X } from "lucide-react";

import { KanbanBoard } from "@/components/shared/kanban-board";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { ExportButton } from "@/components/shared/export-button";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { ProjectForm } from "@/components/shared/project-form";
import { StatusBadge as SharedStatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Select } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  deleteProject,
  getProject,
  listAvailableUsers,
  mutateProjectMember,
  projectsKeys,
  updateProject,
} from "@/services/operational-projects";
import type { AutoAssignMode, ProjectFormValues } from "@/types/project";

const searchSchema = z.object({
  view: z.enum(["board", "settings"]).optional().catch("board"),
});

export const Route = createFileRoute("/_authenticated/operational/projects/$projectId")({
  validateSearch: searchSchema,
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
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
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [isMemberModalOpen, setIsMemberModalOpen] = useState(false);
  const [memberToRemove, setMemberToRemove] = useState<{ id: string; name: string } | null>(null);
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);

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
      setIsEditOpen(false);
      await queryClient.invalidateQueries({ queryKey: projectsKeys.detail(projectId) });
      await queryClient.invalidateQueries({ queryKey: projectsKeys.all });
    },
  });

  const autoAssignMutation = useMutation({
    mutationFn: (mode: AutoAssignMode) =>
      updateProject(projectId, {
        name: project!.name,
        description: project!.description ?? "",
        deadline: project!.deadline ? project!.deadline.slice(0, 10) : "",
        status: project!.status,
        priority: project!.priority,
        auto_assign_mode: mode,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.detail(projectId) });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteProject(projectId),
    onSuccess: async () => {
      setIsDeleteDialogOpen(false);
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
      setIsMemberModalOpen(false);
      setMemberToRemove(null);
      await queryClient.invalidateQueries({ queryKey: projectsKeys.detail(projectId) });
      await queryClient.invalidateQueries({ queryKey: projectsKeys.all });
    },
  });

  if (projectQuery.isLoading) {
    return (
      <div className="space-y-6">
        <Card className="overflow-hidden border-ops/20 bg-gradient-to-br from-ops/10 via-background to-background p-8">
          <div className="flex flex-col gap-6">
            <Skeleton className="h-4 w-[250px] bg-muted/60" />
            <div className="flex flex-wrap items-center gap-3">
              <Skeleton className="h-7 w-[90px] rounded-full bg-muted/60" />
              <Skeleton className="h-7 w-[80px] rounded-full bg-muted/60" />
              <Skeleton className="h-7 w-[100px] rounded-full bg-muted/60" />
            </div>
            <Skeleton className="h-10 w-[400px] max-w-full bg-muted/60" />
            <Skeleton className="h-5 w-[500px] max-w-full bg-muted/60" />
            <div className="flex flex-wrap gap-3">
              <Skeleton className="h-10 w-[90px] rounded-[6px] bg-muted/60" />
              <Skeleton className="h-10 w-[110px] rounded-[6px] bg-muted/60" />
              <Skeleton className="h-10 w-[110px] rounded-[6px] bg-muted/60" />
            </div>
          </div>
        </Card>
      </div>
    );
  }

  if (projectQuery.error instanceof Error || !project) {
    return (
      <Card className="p-8 text-error">
        {projectQuery.error instanceof Error ? projectQuery.error.message : "Project tidak ditemukan"}
      </Card>
    );
  }

  const autoAssignLabels: Record<AutoAssignMode, string> = {
    off: "Nonaktif",
    round_robin: "Round Robin",
    least_busy: "Paling ringan",
  };

  return (
    <div className="space-y-6">
      <Card className="overflow-hidden border-ops/20 bg-gradient-to-br from-ops/10 via-background to-background p-8">
        <div className="flex flex-col gap-6">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-sm font-medium text-muted-foreground">
              <Link
                className="underline-offset-4 hover:underline"
                to="/operational"
              >
                Operasional
              </Link>
              <span>/</span>
              <Link
                className="underline-offset-4 hover:underline"
                to="/operational/projects"
              >
                Project
              </Link>
              <span>/</span>
              <span>{project.name}</span>
            </div>

            <div className="mt-5 flex flex-wrap items-center gap-3">
              <SharedStatusBadge status={project.status} variant="project-status" />
              <SharedStatusBadge status={project.priority} variant="priority" />
              <Badge value={`${project.member_count} anggota`} />
              <Badge
                value={project.deadline ? `Deadline ${new Date(project.deadline).toLocaleDateString()}` : "Tanpa deadline"}
              />
            </div>

            <h3 className="mt-5 max-w-4xl text-3xl font-bold leading-tight tracking-tight text-foreground sm:text-4xl">{project.name}</h3>
            <p className="mt-3 max-w-3xl text-muted-foreground">
              {project.description || "Board ini siap dipakai untuk koordinasi task harian dan kerja bareng tim."}
            </p>

            <div className="mt-6 flex flex-wrap gap-3">
              {([
                { key: "board", label: "Board" },
                { key: "settings", label: "Pengaturan" },
              ] as const).map((view) => (
                <Button
                  key={view.key}
                  onClick={() =>
                    void navigate({
                      to: "/operational/projects/$projectId",
                      params: { projectId },
                      search: { view: view.key },
                    })
                  }
                  variant={activeView === view.key ? "ops" : "outline"}
                >
                  {view.label}
                </Button>
              ))}
              <PermissionGate permission={permissions.operationalProjectView}>
                <ExportButton
                  endpoint={`/operational/projects/${projectId}/export`}
                  filename={`project-${projectId}`}
                  formats={["pdf"]}
                />
              </PermissionGate>
            </div>
          </div>
        </div>
      </Card>

      {activeView === "board" ? (
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_340px]">
          <PermissionGate
            fallback={
              <Card className="p-8 text-sm text-muted-foreground">
                Anda tidak punya akses untuk melihat board ini.
              </Card>
            }
            permission={permissions.operationalTaskView}
          >
            <KanbanBoard members={members} projectId={projectId} />
          </PermissionGate>

          <div className="space-y-6">
            <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
              <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                Ringkasan board
              </p>
              <div className="mt-4 grid gap-3">
                <SummaryRow label="Status" value={project.status.replace("_", " ")} />
                <SummaryRow label="Priority" value={project.priority} />
                <SummaryRow
                  label="Deadline"
                  value={project.deadline ? new Date(project.deadline).toLocaleDateString() : "-"}
                />
                <SummaryRow label="Anggota" value={String(project.member_count)} />
                <SummaryRow label="Auto-assign" value={autoAssignLabels[project.auto_assign_mode] ?? "Nonaktif"} />
              </div>
            </Card>

            <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                    Anggota project
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Lihat siapa saja yang ikut bekerja di board ini tanpa pindah halaman.
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
                  Kelola
                </Button>
              </div>

              <div className="mt-5 space-y-3">
                {members.map((member) => (
                  <div
                    className="flex flex-col gap-3 rounded-[22px] border border-border/70 bg-background/80 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
                    key={member.user_id}
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <ProtectedAvatar
                        alt={member.full_name || member.user_email || member.user_id}
                        avatarUrl={member.avatar_url}
                        className="h-11 w-11 border border-border/70"
                      />
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-semibold">
                          {member.full_name || member.user_id}
                        </p>
                        <p className="truncate text-sm text-muted-foreground">
                          {member.user_email || member.user_id}
                        </p>
                      </div>
                    </div>
                    <span className="inline-flex max-w-full self-start rounded-full bg-secondary px-3 py-1 text-xs font-medium text-secondary-foreground sm:self-auto">
                      <span className="truncate">{member.role_in_project}</span>
                    </span>
                  </div>
                ))}

                {members.length === 0 ? (
                  <p className="text-sm text-muted-foreground">Belum ada anggota yang ditambahkan.</p>
                ) : null}
              </div>
            </Card>
          </div>
        </div>
      ) : null}

      {activeView === "settings" ? (
        <div className="grid gap-6 xl:grid-cols-[0.95fr_1.05fr]">
          <div className="space-y-6">
            <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                    Pengaturan project
                  </p>
                  <h4 className="mt-2 text-xl font-bold tracking-tight text-foreground">Detail project utama</h4>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Perbarui brief, deadline, dan prioritas project.
                  </p>
                </div>
                <PermissionGate permission={permissions.operationalProjectEdit}>
                  <Button onClick={() => setIsEditOpen(true)} variant="ops">
                    Edit project
                  </Button>
                </PermissionGate>
              </div>

              <div className="mt-5 grid gap-3">
                <SummaryRow label="Status" value={project.status.replace("_", " ")} />
                <SummaryRow label="Priority" value={project.priority} />
                <SummaryRow label="Deadline" value={project.deadline ? new Date(project.deadline).toLocaleDateString() : "-"} />
                <SummaryRow label="Deskripsi" value={project.description || "-"} />
              </div>
            </Card>

            <PermissionGate permission={permissions.operationalProjectEdit}>
              <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
                <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                  Auto-assign
                </p>
                <p className="mt-2 text-sm text-muted-foreground">
                  Task baru tanpa assignee akan otomatis di-assign ke member project.
                </p>
                <div className="mt-4 flex flex-wrap gap-2">
                  {(["off", "round_robin", "least_busy"] as const).map((mode) => (
                    <Button
                      key={mode}
                      disabled={autoAssignMutation.isPending}
                      onClick={() => autoAssignMutation.mutate(mode)}
                      variant={project.auto_assign_mode === mode ? "ops" : "outline"}
                      size="sm"
                    >
                      {autoAssignLabels[mode]}
                    </Button>
                  ))}
                </div>
                <p className="mt-3 text-xs text-muted-foreground">
                  {project.auto_assign_mode === "off"
                    ? "Auto-assign tidak aktif. Task harus di-assign manual."
                    : project.auto_assign_mode === "round_robin"
                      ? "Task di-assign bergantian merata antar member project."
                      : "Task di-assign ke member dengan task aktif paling sedikit."}
                </p>
              </Card>
            </PermissionGate>
          </div>

          <div className="space-y-6">
            <PermissionGate permission={permissions.operationalProjectEdit}>
              <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
                <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                  Anggota
                </p>
                <div className="mt-4 flex flex-wrap items-center justify-between gap-4">
                  <p className="text-sm text-muted-foreground">
                    Tambahkan user dari daftar yang tersedia, lalu kelola member dari list di bawah.
                  </p>
                  <Button onClick={() => setIsMemberModalOpen(true)} variant="ops">
                    <Plus className="h-4 w-4" />
                    Tambah anggota
                  </Button>
                </div>
              </Card>
            </PermissionGate>

            <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">
                    Anggota saat ini
                  </p>
                  <h4 className="mt-2 text-xl font-bold tracking-tight text-foreground">Kolaborator project</h4>
                </div>
              </div>

              <div className="mt-5 space-y-3">
                {members.map((member) => (
                  <div
                    className="flex flex-col gap-3 rounded-[22px] border border-border/70 bg-background/80 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
                    key={member.user_id}
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <ProtectedAvatar
                        alt={member.full_name || member.user_email || member.user_id}
                        avatarUrl={member.avatar_url}
                        className="h-11 w-11 border border-border/70"
                      />
                      <div className="min-w-0">
                        <p className="truncate font-semibold">{member.full_name || member.user_id}</p>
                        <p className="truncate text-sm text-muted-foreground">
                          {member.user_email || member.user_id}
                        </p>
                      </div>
                    </div>
                    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                      <PermissionGate fallback={<Badge value={member.role_in_project} />} permission={permissions.operationalProjectManageMembers}>
                        <Select
                          className="min-w-[150px]"
                          disabled={memberMutation.isPending}
                          onValueChange={(nextRole) =>
                            memberMutation.mutate({
                              operation: "assign",
                              user_id: member.user_id,
                              role_in_project: nextRole,
                            })
                          }
                          options={[
                            { value: "lead", label: "Lead" },
                            { value: "developer", label: "Developer" },
                            { value: "designer", label: "Designer" },
                            { value: "qa", label: "QA" },
                            { value: "member", label: "Member" },
                          ]}
                          value={member.role_in_project}
                        />
                      </PermissionGate>
                      <PermissionGate permission={permissions.operationalProjectManageMembers}>
                        <Button
                          onClick={() =>
                            setMemberToRemove({
                              id: member.user_id,
                              name: member.full_name || member.user_email || member.user_id,
                            })
                          }
                          size="sm"
                          type="button"
                          variant="ghost"
                        >
                          <UserMinus className="h-4 w-4" />
                          Keluarkan
                        </Button>
                      </PermissionGate>
                    </div>
                  </div>
                ))}

                {members.length === 0 ? (
                  <p className="text-sm text-muted-foreground">Belum ada anggota yang ditambahkan.</p>
                ) : null}
              </div>
            </Card>

            <PermissionGate permission={permissions.operationalProjectDelete}>
              <Card className="border-border/60 bg-background/50 p-6 backdrop-blur-sm">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <p className="text-sm font-semibold uppercase tracking-[0.24em] text-ops">Zona berbahaya</p>
                    <p className="mt-2 text-sm text-muted-foreground">
                      Menghapus project akan menghapus board dan semua data task terkait.
                    </p>
                  </div>
                  <Button onClick={() => setIsDeleteDialogOpen(true)} variant="ghost">
                    Hapus project
                  </Button>
                </div>
              </Card>
            </PermissionGate>
          </div>
        </div>
      ) : null}

      {editDefaults ? (
        <ProjectForm
          defaultValues={editDefaults}
          description="Edit informasi utama project tanpa kehilangan konteks board."
          isOpen={isEditOpen}
          isSubmitting={updateMutation.isPending}
          onCancel={() => setIsEditOpen(false)}
          onSubmit={(values) => updateMutation.mutate(values)}
          submitLabel="Simpan project"
          title="Pengaturan project"
        />
      ) : null}

      <AddMemberModal
        existingMemberIds={members.map((m) => m.user_id)}
        isOpen={isMemberModalOpen}
        isPending={memberMutation.isPending}
        onAdd={(email, role) => {
          memberMutation.mutate({
            operation: "assign",
            user_email: email,
            role_in_project: role,
          });
        }}
        onClose={() => {
          setIsMemberModalOpen(false);
        }}
      />

      <ConfirmDialog
        confirmLabel="Hapus project"
        description={`Semua task dan data terkait untuk "${project.name}" akan ikut terhapus.`}
        isLoading={deleteMutation.isPending}
        isOpen={isDeleteDialogOpen}
        onClose={() => setIsDeleteDialogOpen(false)}
        onConfirm={() => deleteMutation.mutate()}
        title={`Hapus ${project.name}?`}
      />

      <ConfirmDialog
        confirmLabel="Keluarkan member"
        description={
          memberToRemove
            ? `${memberToRemove.name} akan dihapus dari project ini.`
            : ""
        }
        isLoading={memberMutation.isPending}
        isOpen={Boolean(memberToRemove)}
        onClose={() => setMemberToRemove(null)}
        onConfirm={() => {
          if (memberToRemove) {
            memberMutation.mutate({ operation: "remove", user_id: memberToRemove.id });
          }
        }}
        title={memberToRemove ? `Keluarkan ${memberToRemove.name}?` : "Keluarkan member?"}
      />
    </div>
  );
}

function AddMemberModal({
  existingMemberIds,
  isOpen,
  isPending,
  onAdd,
  onClose,
}: {
  existingMemberIds: string[];
  isOpen: boolean;
  isPending: boolean;
  onAdd: (email: string, role: string) => void;
  onClose: () => void;
}) {
  const [search, setSearch] = useState("");
  const [selectedEmail, setSelectedEmail] = useState("");
  const [role, setRole] = useState("member");
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const usersQuery = useQuery({
    queryKey: [...projectsKeys.all, "available-users"],
    queryFn: listAvailableUsers,
    enabled: isOpen,
  });

  const availableUsers = (usersQuery.data ?? []).filter(
    (u) => !existingMemberIds.includes(u.id),
  );

  const filtered = availableUsers.filter(
    (u) =>
      u.email !== selectedEmail &&
      (search === "" ||
        u.full_name.toLowerCase().includes(search.toLowerCase()) ||
        u.email.toLowerCase().includes(search.toLowerCase())),
  );

  const selectedUser = availableUsers.find((u) => u.email === selectedEmail);

  useEffect(() => {
    if (!isOpen) {
      setSearch("");
      setSelectedEmail("");
      setRole("member");
      setDropdownOpen(false);
    }
  }, [isOpen]);

  useEffect(() => {
    function handleClick(event: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setDropdownOpen(false);
      }
    }
    if (dropdownOpen) {
      document.addEventListener("mousedown", handleClick);
      return () => document.removeEventListener("mousedown", handleClick);
    }
  }, [dropdownOpen]);

  return (
    <FormModal
      isLoading={isPending}
      isOpen={isOpen}
      onClose={onClose}
      onSubmit={(event) => {
        event.preventDefault();
        if (selectedEmail) {
          onAdd(selectedEmail, role);
        }
      }}
      size="md"
      submitLabel="Tambah anggota"
      title="Tambah anggota project"
      subtitle="Pilih user yang akan ditambahkan ke project ini."
    >
      <div className="grid gap-4">
        <div className="space-y-2">
          <label className="text-[13px] font-[600] text-text-primary">
            Pilih user <span className="text-priority-high">*</span>
          </label>

          {selectedUser ? (
            <div className="flex items-center justify-between rounded-[6px] border border-ops/30 bg-ops/5 px-3 py-2.5">
              <div className="flex items-center gap-3">
                <ProtectedAvatar
                  alt={selectedUser.full_name}
                  avatarUrl={selectedUser.avatar_url}
                  className="h-8 w-8 shrink-0 border border-border/70"
                />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-text-primary">{selectedUser.full_name}</p>
                  <p className="truncate text-xs text-text-tertiary">{selectedUser.email}</p>
                </div>
              </div>
              <button
                className="rounded-full p-1 hover:bg-ops/10"
                onClick={() => {
                  setSelectedEmail("");
                  setSearch("");
                }}
                type="button"
              >
                <X className="h-4 w-4 text-text-secondary" />
              </button>
            </div>
          ) : (
            <div className="relative" ref={dropdownRef}>
              <input
                className="flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10"
                onChange={(e) => {
                  setSearch(e.target.value);
                  setDropdownOpen(true);
                }}
                onFocus={() => setDropdownOpen(true)}
                placeholder="Cari nama atau email..."
                value={search}
              />
              {dropdownOpen && filtered.length > 0 && (
                <div className="absolute left-0 right-0 top-full z-[130] mt-1 max-h-52 overflow-y-auto rounded-lg border border-border bg-surface shadow-lg">
                  {filtered.slice(0, 10).map((user) => (
                    <button
                      className="flex w-full items-center gap-3 px-3 py-2.5 text-left text-sm transition-colors hover:bg-surface-muted"
                      key={user.id}
                      onMouseDown={(e) => {
                        e.preventDefault();
                        setSelectedEmail(user.email);
                        setSearch("");
                        setDropdownOpen(false);
                      }}
                      type="button"
                    >
                      <ProtectedAvatar
                        alt={user.full_name}
                        avatarUrl={user.avatar_url}
                        className="h-7 w-7 shrink-0 border border-border/70"
                      />
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-medium text-text-primary">{user.full_name}</p>
                        <p className="truncate text-xs text-text-tertiary">{user.email}</p>
                      </div>
                    </button>
                  ))}
                </div>
              )}
              {dropdownOpen && filtered.length === 0 && (
                <div className="absolute left-0 right-0 top-full z-[130] mt-1 rounded-lg border border-border bg-surface px-3 py-3 text-center text-sm text-text-tertiary shadow-lg">
                  {availableUsers.length === 0 ? "Semua user sudah menjadi member" : "Tidak ada hasil"}
                </div>
              )}
            </div>
          )}
        </div>
        <div className="space-y-2">
          <label className="text-[13px] font-[600] text-text-primary">Peran di project</label>
          <select
            className="flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10"
            onChange={(e) => setRole(e.target.value)}
            value={role}
          >
            <option value="lead">Lead</option>
            <option value="developer">Developer</option>
            <option value="designer">Designer</option>
            <option value="qa">QA</option>
            <option value="member">Member</option>
          </select>
        </div>
      </div>
    </FormModal>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-4 rounded-[22px] border border-border/70 bg-background/80 px-4 py-3">
      <span className="min-w-0 text-sm text-muted-foreground">{label}</span>
      <span className="max-w-[52vw] text-right text-sm font-semibold sm:max-w-[220px]">
        {value}
      </span>
    </div>
  );
}

function Badge({ value }: { value: string }) {
  return (
    <span className="inline-flex max-w-full items-center rounded-full border border-border bg-background/80 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
      <span className="truncate">{value}</span>
    </span>
  );
}


import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Building2, Plus } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DepartmentForm } from "@/components/shared/department-form";
import { EmptyState } from "@/components/shared/empty-state";
import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  createDepartment,
  deleteDepartment,
  departmentsKeys,
  listDepartments,
  updateDepartment,
} from "@/services/hris-departments";
import { employeesKeys, listEmployees } from "@/services/hris-employees";
import type { Department, DepartmentFormValues } from "@/types/hris";

export const Route = createFileRoute("/_authenticated/hris/departments")({
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisDepartmentView);
  },
  component: DepartmentsPage,
});

function DepartmentsPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [editingDepartment, setEditingDepartment] = useState<Department | null>(null);
  const [departmentToDelete, setDepartmentToDelete] = useState<Department | null>(null);

  const departmentsQuery = useQuery({
    queryKey: departmentsKeys.list(),
    queryFn: listDepartments,
  });

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list({ page: 1, perPage: 100, search: "", department: "", status: "" }),
    queryFn: () => listEmployees({ page: 1, perPage: 100, search: "", department: "", status: "" }),
  });

  const createMutation = useMutation({
    mutationFn: createDepartment,
    onSuccess: async () => {
      setIsCreateOpen(false);
      await queryClient.invalidateQueries({ queryKey: departmentsKeys.all });
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
    },
  });

  const updateMutation = useMutation({
    mutationFn: (payload: { departmentId: string; values: DepartmentFormValues }) =>
      updateDepartment(payload.departmentId, payload.values),
    onSuccess: async () => {
      setEditingDepartment(null);
      await queryClient.invalidateQueries({ queryKey: departmentsKeys.all });
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteDepartment,
    onSuccess: async () => {
      setDepartmentToDelete(null);
      await queryClient.invalidateQueries({ queryKey: departmentsKeys.all });
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
    },
  });

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between border-b border-border pb-4">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-hr mb-1">Organization structure</p>
            <h3 className="text-[28px] font-[700] text-text-primary">Departments</h3>
            <p className="mt-2 max-w-2xl text-[14px] text-text-secondary leading-relaxed">
              Atur department, deskripsi fungsi, dan penanggung jawab utama tiap team di HRIS.
            </p>
          </div>

          <PermissionGate permission={permissions.hrisDepartmentCreate}>
            <Button onClick={() => setIsCreateOpen(true)} variant="hr">
              <Plus className="h-4 w-4" />
              Add department
            </Button>
          </PermissionGate>
        </div>
      </Card>

      <DepartmentForm
        description="Define the team function, description, and department head in a single compact dialog."
        employees={employeesQuery.data?.items ?? []}
        isOpen={isCreateOpen}
        isSubmitting={createMutation.isPending}
        onCancel={() => setIsCreateOpen(false)}
        onSubmit={(values) => createMutation.mutate(values)}
        submitLabel="Create department"
        title="New department"
      />

      {editingDepartment ? (
        <DepartmentForm
          defaultValues={{
            name: editingDepartment.name,
            description: editingDepartment.description ?? "",
            head_id: editingDepartment.head_id ?? "",
          }}
          description="Update the department profile and head assignment without leaving this directory."
          employees={employeesQuery.data?.items ?? []}
          isOpen={Boolean(editingDepartment)}
          isSubmitting={updateMutation.isPending}
          onCancel={() => setEditingDepartment(null)}
          onSubmit={(values) => updateMutation.mutate({ departmentId: editingDepartment.id, values })}
          submitLabel="Save department"
          title={`Edit ${editingDepartment.name}`}
        />
      ) : null}

      {departmentsQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-error">{departmentsQuery.error.message}</Card>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-2">
        {(departmentsQuery.data ?? []).map((department) => (
          <Card className="p-6 border border-border bg-surface shadow-sm" key={department.id}>
            <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
              <div>
                <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary mb-1">Department</p>
                <h4 className="text-[20px] font-[700] text-text-primary">{department.name}</h4>
                <p className="mt-2 text-[13px] text-text-secondary leading-relaxed">
                  {department.description || "Belum ada deskripsi fungsi untuk department ini."}
                </p>
              </div>
              <div className="rounded-[6px] border border-border bg-surface-muted px-2 py-0.5 text-[10px] font-[700] uppercase tracking-wider text-text-secondary self-start">
                {department.head_name ? "Head assigned" : "No head"}
              </div>
            </div>

            <div className="mt-5 grid gap-3 md:grid-cols-2">
              <div className="rounded-[12px] border border-border bg-surface-muted p-4">
                <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">Department head</p>
                <p className="mt-2 text-[14px] font-[600] text-text-primary">{department.head_name || "-"}</p>
              </div>
              <div className="rounded-[12px] border border-border bg-surface-muted p-4">
                <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">Created at</p>
                <p className="mt-2 text-[14px] font-[600] text-text-primary">{new Date(department.created_at).toLocaleDateString()}</p>
              </div>
            </div>

            <div className="mt-5 flex flex-wrap gap-3">
              <PermissionGate permission={permissions.hrisDepartmentEdit}>
                <Button onClick={() => setEditingDepartment(department)} variant="outline">Edit</Button>
              </PermissionGate>
              {hasPermission(permissions.hrisDepartmentDelete) ? (
                <Button
                  disabled={deleteMutation.isPending && deleteMutation.variables === department.id}
                  onClick={() => setDepartmentToDelete(department)}
                  variant="ghost"
                >
                  {deleteMutation.isPending && deleteMutation.variables === department.id ? "Deleting..." : "Delete"}
                </Button>
              ) : null}
            </div>
          </Card>
        ))}
      </div>

      {(departmentsQuery.data ?? []).length === 0 ? (
        <EmptyState
          actionLabel={hasPermission(permissions.hrisDepartmentCreate) ? "Add department" : undefined}
          description="Create the first department to map teams and assign department heads in HRIS."
          icon={Building2}
          onAction={hasPermission(permissions.hrisDepartmentCreate) ? () => setIsCreateOpen(true) : undefined}
          title="No departments yet"
        />
      ) : null}

      <ConfirmDialog
        confirmLabel="Hapus department"
        description={
          departmentToDelete
            ? `Department "${departmentToDelete.name}" akan dihapus dari struktur organisasi.`
            : ""
        }
        isLoading={deleteMutation.isPending}
        isOpen={Boolean(departmentToDelete)}
        onClose={() => setDepartmentToDelete(null)}
        onConfirm={() => {
          if (departmentToDelete) {
            deleteMutation.mutate(departmentToDelete.id);
          }
        }}
        title={departmentToDelete ? `Hapus ${departmentToDelete.name}?` : "Hapus department?"}
      />
    </div>
  );
}

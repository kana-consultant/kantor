import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { DepartmentForm } from "@/components/shared/department-form";
import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
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
    await ensurePermission(permissions.hrisDepartmentView);
  },
  component: DepartmentsPage,
});

function DepartmentsPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [editingDepartment, setEditingDepartment] = useState<Department | null>(null);

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
      await queryClient.invalidateQueries({ queryKey: departmentsKeys.all });
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
    },
  });

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">Organization structure</p>
            <h3 className="mt-2 text-3xl font-bold">Departments</h3>
            <p className="mt-2 max-w-2xl text-muted-foreground">
              Atur department, deskripsi fungsi, dan penanggung jawab utama tiap team di HRIS.
            </p>
          </div>

          <PermissionGate permission={permissions.hrisDepartmentCreate}>
            <Button onClick={() => setIsCreateOpen((value) => !value)}>
              {isCreateOpen ? "Close form" : "Add department"}
            </Button>
          </PermissionGate>
        </div>
      </Card>

      {isCreateOpen ? (
        <DepartmentForm
          description="Form create department untuk struktur organisasi dan penunjukan head."
          employees={employeesQuery.data?.items ?? []}
          isSubmitting={createMutation.isPending}
          onCancel={() => setIsCreateOpen(false)}
          onSubmit={(values) => createMutation.mutate(values)}
          submitLabel="Create department"
          title="New department"
        />
      ) : null}

      {editingDepartment ? (
        <DepartmentForm
          defaultValues={{
            name: editingDepartment.name,
            description: editingDepartment.description ?? "",
            head_id: editingDepartment.head_id ?? "",
          }}
          description="Edit department dan sinkronkan perubahan struktur ke data employee."
          employees={employeesQuery.data?.items ?? []}
          isSubmitting={updateMutation.isPending}
          onCancel={() => setEditingDepartment(null)}
          onSubmit={(values) => updateMutation.mutate({ departmentId: editingDepartment.id, values })}
          submitLabel="Save department"
          title={`Edit ${editingDepartment.name}`}
        />
      ) : null}

      {departmentsQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-red-700">{departmentsQuery.error.message}</Card>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-2">
        {(departmentsQuery.data ?? []).map((department) => (
          <Card className="p-6" key={department.id}>
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Department</p>
                <h4 className="mt-2 text-2xl font-bold">{department.name}</h4>
                <p className="mt-2 text-sm text-muted-foreground">
                  {department.description || "Belum ada deskripsi fungsi untuk department ini."}
                </p>
              </div>
              <div className="rounded-full bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-secondary-foreground">
                {department.head_name ? "Head assigned" : "No head"}
              </div>
            </div>

            <div className="mt-5 grid gap-3 md:grid-cols-2">
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
                <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Department head</p>
                <p className="mt-2 text-sm font-semibold">{department.head_name || "-"}</p>
              </div>
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
                <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Created at</p>
                <p className="mt-2 text-sm font-semibold">{new Date(department.created_at).toLocaleDateString()}</p>
              </div>
            </div>

            <div className="mt-5 flex flex-wrap gap-3">
              <PermissionGate permission={permissions.hrisDepartmentEdit}>
                <Button onClick={() => setEditingDepartment(department)} variant="outline">Edit</Button>
              </PermissionGate>
              {hasPermission(permissions.hrisDepartmentDelete) ? (
                <Button
                  disabled={deleteMutation.isPending && deleteMutation.variables === department.id}
                  onClick={() => deleteMutation.mutate(department.id)}
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
        <Card className="p-8 text-center text-sm text-muted-foreground">Belum ada department yang tercatat.</Card>
      ) : null}
    </div>
  );
}

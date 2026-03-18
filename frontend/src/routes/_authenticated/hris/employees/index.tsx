import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";

import { EmployeeForm } from "@/components/shared/employee-form";
import { EmployeesTable } from "@/components/shared/employees-table";
import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { departmentsKeys, listDepartments } from "@/services/hris-departments";
import {
  createEmployee,
  deleteEmployee,
  employeesKeys,
  listEmployees,
} from "@/services/hris-employees";
import type { EmployeeFilters } from "@/types/hris";

const defaultFilters: EmployeeFilters = {
  page: 1,
  perPage: 10,
  search: "",
  department: "",
  status: "",
};

export const Route = createFileRoute("/_authenticated/hris/employees/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.hrisEmployeeView);
  },
  component: EmployeesPage,
});

function EmployeesPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [filters, setFilters] = useState<EmployeeFilters>(defaultFilters);
  const [isCreateOpen, setIsCreateOpen] = useState(false);

  const departmentsQuery = useQuery({
    queryKey: departmentsKeys.list(),
    queryFn: listDepartments,
  });

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list(filters),
    queryFn: () => listEmployees(filters),
  });

  const createMutation = useMutation({
    mutationFn: createEmployee,
    onSuccess: async () => {
      setIsCreateOpen(false);
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteEmployee,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
    },
  });

  const meta = employeesQuery.data?.meta;

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between border-b border-border pb-4">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-hris mb-1">HRIS directory</p>
            <h3 className="text-[28px] font-[700] text-text-primary">Employees</h3>
            <p className="mt-2 max-w-2xl text-[14px] text-text-secondary leading-relaxed">
              Kelola profil karyawan yang punya akses login maupun yang hanya dicatat sebagai data HR internal.
            </p>
          </div>

          <PermissionGate permission={permissions.hrisEmployeeCreate}>
            <Button onClick={() => setIsCreateOpen((value) => !value)} variant="hr">
              {isCreateOpen ? "Close form" : "Add employee"}
            </Button>
          </PermissionGate>
        </div>
      </Card>

      {isCreateOpen ? (
        <EmployeeForm
          departments={departmentsQuery.data ?? []}
          description="Form create employee menggunakan React Hook Form + Zod."
          isSubmitting={createMutation.isPending}
          onCancel={() => setIsCreateOpen(false)}
          onSubmit={(values) => createMutation.mutate(values)}
          submitLabel="Create employee"
          title="New employee"
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
            placeholder="Search by employee name"
            value={filters.search}
          />
          <select
            className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                page: 1,
                department: event.target.value,
              }))
            }
            value={filters.department}
          >
            <option value="">All departments</option>
            {(departmentsQuery.data ?? []).map((department) => (
              <option key={department.id} value={department.name}>
                {department.name}
              </option>
            ))}
          </select>
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
            <option value="active">Active</option>
            <option value="probation">Probation</option>
            <option value="resigned">Resigned</option>
            <option value="terminated">Terminated</option>
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

      {employeesQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-red-700">{employeesQuery.error.message}</Card>
      ) : null}

      {employeesQuery.isLoading ? (
        <Card className="p-0 overflow-hidden border-border bg-surface">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-[14px]">
              <thead className="bg-surface-muted border-b border-border">
                <tr>
                  <th className="px-6 py-4 font-[600] text-text-secondary h-[52px]">Employee</th>
                  <th className="px-6 py-4 font-[600] text-text-secondary h-[52px]">Role & Dept</th>
                  <th className="px-6 py-4 font-[600] text-text-secondary h-[52px]">Contact</th>
                  <th className="px-6 py-4 font-[600] text-text-secondary h-[52px]">Status</th>
                  <th className="px-6 py-4 font-[600] text-text-secondary h-[52px] w-[100px]"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {[1, 2, 3, 4, 5].map((i) => (
                  <tr key={i} className="hover:bg-surface-muted/50 transition-colors">
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-3">
                        <Skeleton className="h-10 w-10 shrink-0 rounded-full bg-muted/60" />
                        <div className="space-y-2">
                          <Skeleton className="h-4 w-[120px] bg-muted/60" />
                          <Skeleton className="h-3 w-[80px] bg-muted/60" />
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="space-y-2">
                        <Skeleton className="h-4 w-[100px] bg-muted/60" />
                        <Skeleton className="h-3 w-[70px] bg-muted/60" />
                      </div>
                    </td>
                    <td className="px-6 py-4 space-y-2">
                      <Skeleton className="h-4 w-[140px] bg-muted/60" />
                      <Skeleton className="h-3 w-[100px] bg-muted/60" />
                    </td>
                    <td className="px-6 py-4">
                      <Skeleton className="h-6 w-[80px] rounded-full bg-muted/60" />
                    </td>
                    <td className="px-6 py-4">
                      <Skeleton className="h-8 w-[60px] rounded-md bg-muted/60" />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      ) : (
        <EmployeesTable
          canDelete={hasPermission(permissions.hrisEmployeeDelete)}
          deletingId={deleteMutation.isPending ? deleteMutation.variables ?? null : null}
          employees={employeesQuery.data?.items ?? []}
          onDelete={(employeeId) => deleteMutation.mutate(employeeId)}
        />
      )}

      <Card className="flex flex-col gap-4 p-6 md:flex-row md:items-center md:justify-between mt-6">
        <p className="text-[14px] font-[500] text-text-secondary">
          Page {meta?.page ?? filters.page} of {meta ? Math.max(1, Math.ceil(meta.total / meta.per_page)) : 1}
          {" "}· Total {meta?.total ?? 0} employees
        </p>
        <div className="flex gap-3">
          <Button
            disabled={(meta?.page ?? filters.page) <= 1}
            onClick={() => setFilters((current) => ({ ...current, page: Math.max(1, current.page - 1) }))}
            variant="outline"
          >
            Previous
          </Button>
          <Button
            disabled={meta ? meta.page * meta.per_page >= meta.total : true}
            onClick={() => setFilters((current) => ({ ...current, page: current.page + 1 }))}
            variant="outline"
          >
            Next
          </Button>
        </div>
      </Card>
    </div>
  );
}

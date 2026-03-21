import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { departmentsKeys, listDepartments } from "@/services/hris-departments";
import {
  deleteEmployee,
  employeesKeys,
  listEmployees,
} from "@/services/hris-employees";
import type { Employee, EmployeeFilters } from "@/types/hris";

const defaultFilters: EmployeeFilters = {
  page: 1,
  perPage: 10,
  search: "",
  department: "",
  status: "",
};

export const Route = createFileRoute("/_authenticated/hris/employees/")({
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisEmployeeView);
  },
  component: EmployeesPage,
});

function EmployeesPage() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const [filters, setFilters] = useState<EmployeeFilters>(defaultFilters);
  const [employeeToDelete, setEmployeeToDelete] = useState<Employee | null>(null);

  const departmentsQuery = useQuery({
    queryKey: departmentsKeys.list(),
    queryFn: listDepartments,
  });

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list(filters),
    queryFn: () => listEmployees(filters),
  });

  const deleteMutation = useMutation({
    mutationFn: deleteEmployee,
    onSuccess: async () => {
      setEmployeeToDelete(null);
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
    },
  });

  const employees = employeesQuery.data?.items ?? [];
  const meta = employeesQuery.data?.meta;
  const columns: Array<DataTableColumn<Employee>> = [
    {
      id: "employee",
      header: "Employee",
      accessor: "full_name",
      sortable: true,
      cell: (employee) => (
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-hr-light text-sm font-semibold text-hr">
            {initials(employee.full_name)}
          </div>
          <div className="space-y-1">
            <p className="font-semibold text-text-primary">{employee.full_name}</p>
            <p className="text-[13px] text-text-secondary">{employee.email}</p>
          </div>
        </div>
      ),
    },
    {
      id: "position",
      header: "Position",
      accessor: "position",
      sortable: true,
      cell: (employee) => (
        <div className="space-y-1">
          <p className="font-medium text-text-primary">{employee.position}</p>
          <p className="text-[13px] text-text-secondary">{employee.department || "-"}</p>
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      accessor: "employment_status",
      sortable: true,
      cell: (employee) => (
        <StatusBadge status={employee.employment_status} variant="employee-status" />
      ),
    },
    {
      id: "date_joined",
      header: "Joined",
      accessor: "date_joined",
      sortable: true,
      cell: (employee) => (
        <span className="text-sm text-text-secondary">
          {new Date(employee.date_joined).toLocaleDateString("id-ID")}
        </span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (employee) => (
        <div className="flex justify-end gap-2">
          <Link
            className="inline-flex h-9 items-center justify-center rounded-md bg-module px-4 text-sm font-semibold text-white transition hover:brightness-95"
            params={{ employeeId: employee.id }}
            to="/hris/employees/$employeeId"
          >
            Open
          </Link>
          {hasPermission(permissions.hrisEmployeeDelete) ? (
            <Button
              disabled={deleteMutation.isPending && deleteMutation.variables === employee.id}
              onClick={() => setEmployeeToDelete(employee)}
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
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">
              HRIS directory
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">Employees</h3>
            <p className="mt-2 max-w-2xl text-[14px] leading-relaxed text-text-secondary">
              Employee otomatis terdaftar saat mendaftar akun. Kelola profil, departemen, dan status dari sini.
            </p>
          </div>
        </div>
      </Card>

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
            placeholder="Search by employee name"
            value={filters.search}
          />
          <select
            className="field-select"
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
            <option value="active">Active</option>
            <option value="probation">Probation</option>
            <option value="resigned">Resigned</option>
            <option value="terminated">Terminated</option>
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

      {employeesQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-error">{employeesQuery.error.message}</Card>
      ) : null}

      <DataTable
        columns={columns}
        data={employees}
        emptyDescription="Tidak ada employee yang cocok dengan filter. Coba perluas filter."
        emptyTitle="Tidak ada employee"
        getRowId={(employee) => employee.id}
        loading={employeesQuery.isLoading}
        loadingRows={6}
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
        confirmLabel="Hapus employee"
        description={
          employeeToDelete
            ? `Data employee "${employeeToDelete.full_name}" akan dihapus dari HRIS.`
            : ""
        }
        isLoading={deleteMutation.isPending}
        isOpen={Boolean(employeeToDelete)}
        onClose={() => setEmployeeToDelete(null)}
        onConfirm={() => {
          if (employeeToDelete) {
            deleteMutation.mutate(employeeToDelete.id);
          }
        }}
        title={employeeToDelete ? `Hapus ${employeeToDelete.full_name}?` : "Hapus employee?"}
      />
    </div>
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

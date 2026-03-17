import { Link } from "@tanstack/react-router";

import { Card } from "@/components/ui/card";
import type { Employee } from "@/types/hris";

interface EmployeesTableProps {
  employees: Employee[];
  canDelete: boolean;
  deletingId?: string | null;
  onDelete: (employeeId: string) => void;
}

export function EmployeesTable({ employees, canDelete, deletingId, onDelete }: EmployeesTableProps) {
  return (
    <Card className="overflow-hidden border-border/70 bg-card/85">
      <div className="overflow-x-auto">
        <table className="min-w-full text-sm">
          <thead className="bg-muted/60 text-left text-muted-foreground">
            <tr>
              <th className="px-5 py-4 font-medium">Nama</th>
              <th className="px-5 py-4 font-medium">Posisi</th>
              <th className="px-5 py-4 font-medium">Department</th>
              <th className="px-5 py-4 font-medium">Status</th>
              <th className="px-5 py-4 font-medium">Tanggal join</th>
              <th className="px-5 py-4 font-medium">Action</th>
            </tr>
          </thead>
          <tbody>
            {employees.map((employee) => (
              <tr className="border-t border-border/70" key={employee.id}>
                <td className="px-5 py-4">
                  <div>
                    <p className="font-semibold">{employee.full_name}</p>
                    <p className="text-xs text-muted-foreground">{employee.email}</p>
                  </div>
                </td>
                <td className="px-5 py-4">{employee.position}</td>
                <td className="px-5 py-4">{employee.department || "-"}</td>
                <td className="px-5 py-4">
                  <span className="rounded-full bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-secondary-foreground">
                    {employee.employment_status}
                  </span>
                </td>
                <td className="px-5 py-4">{new Date(employee.date_joined).toLocaleDateString()}</td>
                <td className="px-5 py-4">
                  <div className="flex flex-wrap gap-3">
                    <Link
                      className="text-sm font-medium text-primary hover:underline"
                      params={{ employeeId: employee.id }}
                      to="/hris/employees/$employeeId"
                    >
                      Open profile
                    </Link>
                    {canDelete ? (
                      <button
                        className="text-sm font-medium text-muted-foreground transition hover:text-foreground"
                        disabled={deletingId === employee.id}
                        onClick={() => onDelete(employee.id)}
                        type="button"
                      >
                        {deletingId === employee.id ? "Deleting..." : "Delete"}
                      </button>
                    ) : null}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {employees.length === 0 ? (
        <div className="p-8 text-center text-sm text-muted-foreground">Belum ada karyawan untuk filter saat ini.</div>
      ) : null}
    </Card>
  );
}

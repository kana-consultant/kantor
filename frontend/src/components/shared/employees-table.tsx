import { Link } from "@tanstack/react-router";
import { Users } from "lucide-react";

import { Card } from "@/components/ui/card";
import type { Employee } from "@/types/hris";
import { cn } from "@/lib/utils";

interface EmployeesTableProps {
  employees: Employee[];
  canDelete: boolean;
  deletingId?: string | null;
  onDelete: (employeeId: string) => void;
}

export function EmployeesTable({ employees, canDelete, deletingId, onDelete }: EmployeesTableProps) {
  if (employees.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center p-12 text-center rounded-[12px] border-2 border-dashed border-border bg-surface-muted/50">
        <Users className="w-10 h-10 text-text-tertiary mb-3" strokeWidth={1.5} />
        <h3 className="text-[14px] font-[600] text-text-primary">No employees found</h3>
        <p className="mt-1 text-[13px] text-text-secondary">
          No employees match the current filter criteria.
        </p>
      </div>
    );
  }

  return (
    <Card className="overflow-hidden">
      <div className="overflow-x-auto">
        <table className="min-w-full text-left border-collapse">
          <thead>
            <tr className="bg-surface-muted border-y border-border">
              <th className="px-5 py-3 text-[13px] font-[600] text-text-secondary uppercase tracking-wider">Nama</th>
              <th className="px-5 py-3 text-[13px] font-[600] text-text-secondary uppercase tracking-wider">Posisi</th>
              <th className="px-5 py-3 text-[13px] font-[600] text-text-secondary uppercase tracking-wider">Department</th>
              <th className="px-5 py-3 text-[13px] font-[600] text-text-secondary uppercase tracking-wider">Status</th>
              <th className="px-5 py-3 text-[13px] font-[600] text-text-secondary uppercase tracking-wider">Tanggal join</th>
              <th className="px-5 py-3 text-[13px] font-[600] text-text-secondary uppercase tracking-wider">Action</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {employees.map((employee) => (
              <tr className="hover:bg-surface-muted transition-colors" key={employee.id}>
                <td className="px-5 py-4">
                  <div>
                    <p className="text-[14px] font-[600] text-text-primary">{employee.full_name}</p>
                    <p className="text-[12px] text-text-secondary mt-0.5">{employee.email}</p>
                  </div>
                </td>
                <td className="px-5 py-4 text-[14px] font-[500] text-text-primary">{employee.position}</td>
                <td className="px-5 py-4 text-[14px] font-[500] text-text-primary">{employee.department || "-"}</td>
                <td className="px-5 py-4">
                  <span className={cn(
                    "inline-flex items-center px-2.5 py-1 rounded-full text-[11px] font-[600] uppercase tracking-wider border",
                    employee.employment_status.toLowerCase() === 'active' || employee.employment_status.toLowerCase() === 'aktif'
                      ? "bg-hr-light/50 text-hr border-hr/20" 
                      : "bg-surface-muted text-text-secondary border-border"
                  )}>
                    {employee.employment_status}
                  </span>
                </td>
                <td className="px-5 py-4 text-[14px] font-[500] text-text-primary">{new Date(employee.date_joined).toLocaleDateString("id-ID")}</td>
                <td className="px-5 py-4">
                  <div className="flex items-center gap-4">
                    <Link
                      className="text-[13px] font-[600] text-hr hover:text-hr-hover transition-colors"
                      params={{ employeeId: employee.id }}
                      to="/hris/employees/$employeeId"
                    >
                      Open profile
                    </Link>
                    {canDelete ? (
                      <button
                        className="text-[13px] font-[500] text-text-tertiary hover:text-priority-high transition-colors disabled:opacity-50"
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
    </Card>
  );
}

import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRight, FileText } from "lucide-react";

import {
  DeliveryStatusPill,
  formatAmount,
  formatDateTime,
  PayslipStatusPill,
} from "@/components/payslips/payslip-display";
import { buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { listEmployeePayslips, payslipsKeys } from "@/services/hris-payslips";

/**
 * 'Slip Gaji' status card on the employee's Salary tab: the latest slips and
 * a link to the Slip Gaji page. Actions live on that page only.
 */
export function PayslipStatusCard({ employeeId }: { employeeId: string }) {
  const query = useQuery({
    queryKey: payslipsKeys.employee(employeeId),
    queryFn: () => listEmployeePayslips(employeeId, 6),
    refetchOnWindowFocus: false,
  });

  const slips = query.data ?? [];
  const latest = slips[0];

  return (
    <Card className="p-6" data-testid="payslip-status-card">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <div className="rounded-md bg-hr-light p-3 text-hr">
            <FileText className="h-5 w-5" />
          </div>
          <div>
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Slip Gaji</p>
            <p className="mt-1 text-sm text-muted-foreground">Slip terakhir dan status pengirimannya.</p>
          </div>
        </div>
        <Link
          className={buttonVariants({ size: "sm", variant: "outline" })}
          search={{
            employee: employeeId,
            ...(latest ? { year: latest.period_year, month: latest.period_month } : {}),
          }}
          to="/hris/payslips"
        >
          Buka Slip Gaji
          <ArrowRight className="h-4 w-4" />
        </Link>
      </div>

      <div className="mt-4">
        {query.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-12 rounded-lg" />
            <Skeleton className="h-12 rounded-lg" />
          </div>
        ) : query.error instanceof Error ? (
          <p className="text-sm text-error">{query.error.message}</p>
        ) : slips.length === 0 ? (
          <p className="rounded-[18px] border border-dashed border-border px-4 py-3 text-sm text-muted-foreground">
            Belum ada slip gaji untuk karyawan ini.
          </p>
        ) : (
          <ul className="divide-y divide-border/70 rounded-[18px] border border-border/70">
            {slips.map((slip) => (
              <li className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between" key={slip.id}>
                <div className="min-w-0">
                  <p className="text-sm font-semibold">{slip.period_label}</p>
                  <p className="font-mono text-[11px] text-muted-foreground">{slip.doc_number}</p>
                </div>
                <div className="flex flex-wrap items-center gap-2 sm:justify-end">
                  <span className={slip.status === "void" ? "font-mono text-sm tabular-nums text-muted-foreground line-through" : "font-mono text-sm tabular-nums"}>
                    {formatAmount(slip.total_diterima)}
                  </span>
                  <PayslipStatusPill status={slip.status} />
                  {slip.last_delivery && slip.last_delivery.status !== "sent" ? (
                    <DeliveryStatusPill delivery={slip.last_delivery} />
                  ) : null}
                  {slip.last_sent_at ? (
                    <span className="text-[11px] text-muted-foreground">{formatDateTime(slip.last_sent_at)}</span>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Card>
  );
}

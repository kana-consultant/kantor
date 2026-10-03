import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRight, FileSignature, Plus } from "lucide-react";

import {
  ContractDeadlineBadges,
  ContractStatusPill,
  ContractTypeBadge,
  formatContractPeriod,
} from "@/components/contracts/contract-display";
import { formatDateTime } from "@/components/payslips/payslip-display";
import { buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { contractsKeys, listContracts } from "@/services/hris-contracts";

/**
 * The employee's 'Kontrak' tab: every contract of the employee (newest
 * first) with 'Buat Kontrak'. Actions live on the contract pages.
 */
export function EmployeeContractsCard({ employeeId }: { employeeId: string }) {
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.hrisContractManage);
  const filters = { employee_id: employeeId, limit: 100 };
  const query = useQuery({
    queryKey: contractsKeys.list(filters),
    queryFn: () => listContracts(filters),
    refetchOnWindowFocus: false,
  });
  const contracts = query.data?.items ?? [];

  return (
    <Card className="p-6" data-testid="employee-contracts-card">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <div className="rounded-md bg-hr-light p-3 text-hr">
            <FileSignature className="h-5 w-5" />
          </div>
          <div>
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Kontrak Kerja</p>
            <p className="mt-1 text-sm text-muted-foreground">
              PKWT beserta NDA &amp; HKI, serta catatan PKWTT dan magang karyawan ini.
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {canManage ? (
            <Link
              className={buttonVariants({ size: "sm", variant: "hr" })}
              data-testid="employee-create-contract"
              search={{ employee: employeeId }}
              to="/hris/contracts/new"
            >
              <Plus className="h-4 w-4" />
              Buat Kontrak
            </Link>
          ) : null}
          <Link className={buttonVariants({ size: "sm", variant: "outline" })} search={{ employee: employeeId }} to="/hris/contracts">
            Buka Kontrak Kerja
            <ArrowRight className="h-4 w-4" />
          </Link>
        </div>
      </div>

      <div className="mt-4">
        {query.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-14 rounded-lg" />
            <Skeleton className="h-14 rounded-lg" />
          </div>
        ) : query.error instanceof Error ? (
          <p className="text-sm text-error">{query.error.message}</p>
        ) : contracts.length === 0 ? (
          <p className="rounded-[18px] border border-dashed border-border px-4 py-3 text-sm text-muted-foreground">
            Belum ada kontrak untuk karyawan ini.
          </p>
        ) : (
          <ul className="divide-y divide-border/70 rounded-[18px] border border-border/70">
            {contracts.map((contract) => (
              <li key={contract.id}>
                <Link
                  className="flex flex-col gap-2 px-4 py-3 transition-colors hover:bg-surface-muted/60 sm:flex-row sm:items-center sm:justify-between"
                  params={{ contractId: contract.id }}
                  to="/hris/contracts/$contractId"
                >
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <ContractTypeBadge recordOnly={contract.is_record_only} type={contract.contract_type} />
                      <span className="text-sm font-semibold">{formatContractPeriod(contract.start_date, contract.end_date)}</span>
                    </div>
                    <p className="text-[12px] text-muted-foreground">
                      {contract.job_title}
                      {contract.doc_number ? <span className="font-mono"> · {contract.doc_number}</span> : null}
                      {contract.revision > 0 ? ` · Revisi ${contract.revision}` : ""}
                    </p>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 sm:justify-end">
                    <ContractDeadlineBadges item={contract} />
                    <ContractStatusPill status={contract.status} />
                    {contract.last_sent_at ? (
                      <span className="text-[11px] text-muted-foreground">Dikirim {formatDateTime(contract.last_sent_at)}</span>
                    ) : null}
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Card>
  );
}

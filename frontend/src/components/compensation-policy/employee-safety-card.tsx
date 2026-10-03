import { cn } from "@/lib/utils";
import { formatIDR } from "@/lib/currency";
import { initials } from "@/lib/marketing";
import { type SalarySafetyEvaluation } from "@/services/compensation-policy";
import {
  SafetyHints,
  SafetyProgress,
  SafetyWarnings,
  statusBadge,
  statusLabels,
} from "@/components/compensation-policy/safety-display";

type EmployeeSafetyCardProps = {
  evaluation: SalarySafetyEvaluation;
};

export function EmployeeSafetyCard({ evaluation }: EmployeeSafetyCardProps) {
  return (
    <div className="space-y-4 rounded-lg border border-border bg-surface p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-surface-muted text-sm font-semibold text-text-primary">
            {initials(evaluation.full_name)}
          </div>
          <div>
            <p className="font-semibold text-text-primary">
              {evaluation.full_name}
            </p>
            <p className="text-sm text-text-secondary">
              {formatIDR(evaluation.base_salary)}
            </p>
          </div>
        </div>
        <span
          className={cn(
            "inline-flex shrink-0 rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.06em]",
            statusBadge[evaluation.status],
          )}
        >
          {statusLabels[evaluation.status]}
        </span>
      </div>

      <SafetyProgress evaluation={evaluation} />
      <SafetyWarnings evaluation={evaluation} />
      <SafetyHints evaluation={evaluation} />
    </div>
  );
}

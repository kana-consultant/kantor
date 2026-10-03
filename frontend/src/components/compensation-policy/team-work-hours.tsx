import { Clock } from "lucide-react";

import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { initials } from "@/lib/marketing";
import { MonthYearPicker } from "@/components/compensation-policy/month-year-picker";
import {
  EvaluationPeriodCaption,
  SafetyHints,
  SafetyProgress,
  SafetyWarnings,
  formatHours,
  statusBadge,
  statusLabels,
} from "@/components/compensation-policy/safety-display";
import { useSalarySafety } from "@/components/compensation-policy/use-salary-safety";
import { type SalarySafetyEvaluation } from "@/services/compensation-policy";

function WorkHourCard({ evaluation }: { evaluation: SalarySafetyEvaluation }) {
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
              {formatHours(evaluation.monthly_active_hours)} jam pada periode ini
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

export function TeamWorkHours() {
  const { current, period, setPeriod, evaluations, isLoading, error } =
    useSalarySafety();

  return (
    <Card className="space-y-6 p-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="rounded-md bg-ops-light p-3 text-ops">
            <Clock className="h-5 w-5" />
          </div>
          <div>
            <h2 className="font-display text-[18px] font-[700] text-text-primary">
              Jam Kerja Karyawan
            </h2>
            <p className="mt-1 text-sm text-text-secondary">
              Jam aktif tiap karyawan dibandingkan target: target bulanan dibagi
              rata per hari kerja (Senin–Jumat), sampai kemarin untuk bulan
              berjalan atau sampai akhir bulan untuk bulan sebelumnya.
            </p>
            {!isLoading && !error ? (
              <EvaluationPeriodCaption evaluations={evaluations} />
            ) : null}
          </div>
        </div>
        {current && period ? (
          <MonthYearPicker
            current={current}
            onChange={setPeriod}
            value={period}
          />
        ) : null}
      </div>

      {isLoading ? (
        <p className="text-sm text-text-secondary">Memuat data...</p>
      ) : error ? (
        <p className="text-sm text-error">{error.message}</p>
      ) : evaluations.length === 0 ? (
        <p className="text-sm text-text-secondary">
          Belum ada karyawan untuk dievaluasi.
        </p>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {evaluations.map((evaluation) => (
            <WorkHourCard evaluation={evaluation} key={evaluation.employee_id} />
          ))}
        </div>
      )}
    </Card>
  );
}

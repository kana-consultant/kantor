import { useMemo } from "react";
import { ShieldCheck } from "lucide-react";

import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { EmployeeSafetyCard } from "@/components/compensation-policy/employee-safety-card";
import { MonthYearPicker } from "@/components/compensation-policy/month-year-picker";
import { EvaluationPeriodCaption } from "@/components/compensation-policy/safety-display";
import { useSalarySafety } from "@/components/compensation-policy/use-salary-safety";

type SummaryStat = {
  label: string;
  value: number;
  accent: string;
  hint?: string;
};

export function SalarySafetyOverview() {
  const { current, period, setPeriod, evaluations, isLoading, error } =
    useSalarySafety();

  const stats = useMemo<SummaryStat[]>(() => {
    const data = evaluations;
    const countBy = (status: string) =>
      data.filter((evaluation) => evaluation.status === status).length;
    return [
      { label: "Total Karyawan", value: data.length, accent: "text-text-primary" },
      { label: "Aman", value: countBy("safe"), accent: "text-success" },
      { label: "Berisiko", value: countBy("at_risk"), accent: "text-error" },
      {
        label: "Tidak dievaluasi",
        value: countBy("no_data"),
        accent: "text-text-secondary",
        hint: "Bukan Full Time, belum punya akun, atau periode belum berjalan",
      },
    ];
  }, [evaluations]);
  // Zero tiles while loading or after a failed load would read as "nobody at
  // risk"; only show them for a loaded result.
  const showStats = !isLoading && !error;

  return (
    <Card className="space-y-6 p-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="rounded-md bg-error-light p-3 text-error">
            <ShieldCheck className="h-5 w-5" />
          </div>
          <div>
            <h2 className="font-display text-[18px] font-[700] text-text-primary">
              Status Keamanan Gaji
            </h2>
            <p className="mt-1 text-sm text-text-secondary">
              Aman jika jam aktif sudah mencapai target: target bulanan dibagi
              rata per hari kerja Senin–Jumat, sejak awal bulan atau tanggal
              bergabung sampai kemarin untuk bulan berjalan, atau sampai akhir
              bulan untuk bulan sebelumnya. Di bawah itu berisiko. Hari kerja
              pendek atau tanpa aktivitas hanya jadi peringatan.
            </p>
            {showStats ? (
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

      {showStats ? (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {stats.map((stat) => (
            <div
              className="rounded-lg border border-border bg-surface-muted/40 p-4"
              key={stat.label}
            >
              <p className="text-xs font-semibold uppercase tracking-[0.06em] text-text-secondary">
                {stat.label}
              </p>
              <p className={cn("mt-1 font-display text-[24px] font-[700]", stat.accent)}>
                {stat.value}
              </p>
              {stat.hint ? (
                <p className="mt-0.5 text-[11px] text-text-secondary">
                  {stat.hint}
                </p>
              ) : null}
            </div>
          ))}
        </div>
      ) : null}

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
            <EmployeeSafetyCard
              evaluation={evaluation}
              key={evaluation.employee_id}
            />
          ))}
        </div>
      )}
    </Card>
  );
}

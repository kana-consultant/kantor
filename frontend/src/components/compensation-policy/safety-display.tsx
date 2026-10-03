import { AlertTriangle, Info } from "lucide-react";

import { cn } from "@/lib/utils";
import { parseCalendarDate } from "@/lib/date";
import {
  type SalarySafetyEvaluation,
  type SalarySafetyStatus,
} from "@/services/compensation-policy";

export const statusBadge: Record<SalarySafetyStatus, string> = {
  safe: "bg-success-light text-success",
  at_risk: "bg-error-light text-error",
  no_data: "bg-surface-muted text-text-secondary",
};

export const statusBar: Record<SalarySafetyStatus, string> = {
  safe: "bg-success",
  at_risk: "bg-error",
  no_data: "bg-border",
};

export const statusLabels: Record<SalarySafetyStatus, string> = {
  safe: "Aman",
  at_risk: "Berisiko",
  no_data: "Tidak dievaluasi",
};

export const exemptTypesLabel =
  "Part Time/Internship/Project Based/Outsourcing";

/** "29 Sep" (or "29 Sep 2026" with the year) from a YYYY-MM-DD string. */
export function formatSafetyDate(value: string, withYear = false) {
  const parsed = parseCalendarDate(value);
  if (!parsed) {
    return "-";
  }
  return new Intl.DateTimeFormat("id-ID", {
    day: "numeric",
    month: "short",
    ...(withYear ? { year: "numeric" } : {}),
  }).format(parsed);
}

/** Hours with one decimal, e.g. "152.7". */
export function formatHours(value: number) {
  return value.toFixed(1);
}

/** Whole hours without a decimal, e.g. "160" or "4.5". */
export function formatTargetHours(value: number) {
  return Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1);
}

/** True when the hour target applies and the period has started. */
export function hasTarget(evaluation: SalarySafetyEvaluation) {
  return (
    evaluation.reason !== "future_month" &&
    evaluation.reason !== "target_not_applicable"
  );
}

/**
 * True when the whole period has been evaluated (a past month): the target is
 * the monthly target, not a running one.
 */
export function isPeriodClosed(evaluation: SalarySafetyEvaluation) {
  if (!evaluation.evaluated_through) {
    return false;
  }
  const lastDay = new Date(
    evaluation.period_year,
    evaluation.period_month,
    0,
  ).getDate();
  return Number(evaluation.evaluated_through.slice(8, 10)) === lastDay;
}

/**
 * Progress against the target to date, rounded down so 100% only shows once
 * the target is reached; an at-risk employee never shows more than 99%. Null
 * when nothing is expected yet (the 1st of the month, joined today) or the
 * target does not apply.
 */
export function safetyPercent(evaluation: SalarySafetyEvaluation) {
  if (!hasTarget(evaluation) || evaluation.expected_hours_to_date <= 0) {
    return null;
  }
  const percent = Math.floor(
    (evaluation.monthly_active_hours / evaluation.expected_hours_to_date) * 100,
  );
  return evaluation.status === "at_risk" ? Math.min(99, percent) : percent;
}

/**
 * Actual and target hours as text. One decimal normally; two decimals when one
 * decimal would hide the comparison the status is based on (e.g. an at-risk
 * 152.72 / 152.73 h would read "152.7 / 152.7").
 */
export function formatHoursVsTarget(evaluation: SalarySafetyEvaluation) {
  const actual = evaluation.monthly_active_hours;
  const expected = evaluation.expected_hours_to_date;
  const reached =
    evaluation.status === "at_risk"
      ? false
      : evaluation.status === "safe"
        ? true
        : actual >= expected;
  const reachedAtOneDecimal =
    Number(actual.toFixed(1)) >= Number(expected.toFixed(1));
  const digits = reachedAtOneDecimal === reached ? 1 : 2;
  return {
    actual: actual.toFixed(digits),
    expected: expected.toFixed(digits),
  };
}

/** Hours still missing to reach the target to date, e.g. "0.03" or "9.7". */
function formatShortfall(evaluation: SalarySafetyEvaluation) {
  const shortfall = Math.max(
    0,
    evaluation.expected_hours_to_date - evaluation.monthly_active_hours,
  );
  return shortfall < 0.1 ? shortfall.toFixed(2) : shortfall.toFixed(1);
}

type SafetyProgressProps = {
  evaluation: SalarySafetyEvaluation;
  /** Bar colour; defaults to the status colour. */
  barClassName?: string;
};

/** Hours vs. target to date, the bar, and the full-month target line. */
export function SafetyProgress({ evaluation, barClassName }: SafetyProgressProps) {
  if (evaluation.reason === "future_month") {
    return (
      <p className="rounded-md bg-surface-muted/60 px-3 py-2 text-sm text-text-secondary">
        Periode ini belum berjalan.
      </p>
    );
  }

  if (evaluation.reason === "target_not_applicable") {
    return (
      <div className="flex items-center justify-between text-xs text-text-secondary">
        <span>Jam tercatat periode ini</span>
        <span className="font-semibold text-text-primary">
          {formatHours(evaluation.monthly_active_hours)} jam
        </span>
      </div>
    );
  }

  const percent = safetyPercent(evaluation);
  const barWidth = percent === null ? 0 : Math.min(100, Math.max(0, percent));
  const hours = formatHoursVsTarget(evaluation);
  const targetLabel = isPeriodClosed(evaluation)
    ? "target bulanan"
    : "target berjalan";

  return (
    <div className="space-y-1.5">
      <div className="flex items-start justify-between gap-3 text-xs text-text-secondary">
        <span>
          Jam aktif vs target
          {evaluation.evaluated_through ? (
            <span className="block text-[11px]">
              target s.d. {formatSafetyDate(evaluation.evaluated_through)}
            </span>
          ) : null}
        </span>
        <span className="text-right font-semibold text-text-primary">
          {hours.actual} / {hours.expected} jam
        </span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-surface-muted">
        <div
          className={cn(
            "h-full rounded-full",
            barClassName ?? statusBar[evaluation.status],
          )}
          style={{ width: `${barWidth}%` }}
        />
      </div>
      <div className="flex items-center justify-between text-[11px] text-text-secondary">
        <span>
          Target sebulan {formatTargetHours(evaluation.target_hours_month)} jam
        </span>
        <span>
          {percent === null
            ? "Belum ada hari kerja yang selesai"
            : `${percent}% dari ${targetLabel}`}
        </span>
      </div>
      {evaluation.status === "at_risk" && percent !== null ? (
        <p className="text-right text-[11px] font-medium text-error">
          Kurang {formatShortfall(evaluation)} jam dari {targetLabel}
        </p>
      ) : null}
    </div>
  );
}

/** Amber counters for short and absent weekdays. Warnings only. */
export function SafetyWarnings({
  evaluation,
}: {
  evaluation: SalarySafetyEvaluation;
}) {
  if (!hasTarget(evaluation) || !evaluation.user_id) {
    return null;
  }

  const rows = [
    {
      label: `Hari kerja < ${formatTargetHours(evaluation.min_hours_per_day)} jam`,
      value: evaluation.short_days,
    },
    { label: "Hari kerja tanpa aktivitas", value: evaluation.absent_days },
  ];

  return (
    <div className="space-y-1 rounded-md bg-surface-muted/60 px-3 py-2 text-sm">
      {rows.map((row) => (
        <div className="flex items-center justify-between" key={row.label}>
          <span
            className={cn(
              row.value > 0 ? "text-warning" : "text-text-secondary",
            )}
          >
            {row.label}
          </span>
          <span
            className={cn(
              "font-semibold",
              row.value > 0 ? "text-warning" : "text-text-primary",
            )}
          >
            {row.value}
          </span>
        </div>
      ))}
    </div>
  );
}

/** Hints explaining a missing/partial evaluation. */
export function SafetyHints({
  evaluation,
}: {
  evaluation: SalarySafetyEvaluation;
}) {
  const hints: { text: string; tone: "info" | "warning" }[] = [];

  if (evaluation.reason === "target_not_applicable") {
    hints.push({
      text: `Target tidak berlaku (${exemptTypesLabel})`,
      tone: "info",
    });
  } else if (evaluation.reason === "no_user") {
    hints.push({
      text: "Belum terhubung ke akun pengguna, jam kerja tidak bisa dilacak",
      tone: "info",
    });
  } else if (evaluation.reason === null && !evaluation.has_tracker_data) {
    hints.push({ text: "Belum ada data tracker", tone: "warning" });
  }

  if (evaluation.position_unset && hasTarget(evaluation)) {
    hints.push({
      text: "Tipe kepegawaian belum diatur, dievaluasi sebagai Full Time",
      tone: "warning",
    });
  }

  if (hints.length === 0) {
    return null;
  }

  return (
    <ul className="space-y-1">
      {hints.map((hint) => {
        const Icon = hint.tone === "warning" ? AlertTriangle : Info;
        return (
          <li
            className={cn(
              "flex items-start gap-1.5 text-xs",
              hint.tone === "warning" ? "text-warning" : "text-text-secondary",
            )}
            key={hint.text}
          >
            <Icon className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>{hint.text}</span>
          </li>
        );
      })}
    </ul>
  );
}

/**
 * "Dievaluasi s.d. 29 Sep 2026 · 21 dari 22 hari kerja (target belum termasuk
 * hari ini)" for the period of a salary-safety result set. Only the target
 * stops at yesterday; today's tracked hours already count.
 */
export function EvaluationPeriodCaption({
  evaluations,
}: {
  evaluations: SalarySafetyEvaluation[];
}) {
  if (evaluations.length === 0) {
    return null;
  }

  const first = evaluations[0];
  if (!first) {
    return null;
  }

  let text: string;
  if (evaluations.every((evaluation) => evaluation.reason === "future_month")) {
    text = "Periode ini belum berjalan, belum ada yang dievaluasi.";
  } else if (!first.evaluated_through) {
    text = "Belum ada hari kerja yang selesai pada periode ini.";
  } else {
    // Joiners have a shorter window; the longest window is the whole period.
    const elapsed = Math.max(
      ...evaluations.map((evaluation) => evaluation.working_days_elapsed),
    );
    text = `Dievaluasi s.d. ${formatSafetyDate(first.evaluated_through, true)} · ${elapsed} dari ${first.working_days_total} hari kerja${isPeriodClosed(first) ? "" : " (target belum termasuk hari ini)"}`;
  }

  return <p className="mt-1 text-xs font-medium text-text-tertiary">{text}</p>;
}

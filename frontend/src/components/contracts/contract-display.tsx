import { CalendarClock, CalendarX } from "lucide-react";

import { Pill } from "@/components/payslips/payslip-display";
import { cn } from "@/lib/utils";
import type { ContractListItem, ContractStatus, ContractType, ContractWorkMode } from "@/types/contracts";

export const contractTypeLabels: Record<ContractType, string> = {
  PKWT: "PKWT",
  PKWTT: "PKWTT",
  MAGANG: "Magang",
};

export const contractTypeDescriptions: Record<ContractType, string> = {
  PKWT: "Perjanjian Kerja Waktu Tertentu (kontrak)",
  PKWTT: "Perjanjian Kerja Waktu Tidak Tertentu (tetap)",
  MAGANG: "Magang",
};

export const contractWorkModeLabels: Record<ContractWorkMode, string> = {
  wfo: "WFO (di kantor)",
  hybrid: "Hybrid",
  remote: "Remote",
};

type Tone = "neutral" | "info" | "success" | "warning" | "error" | "muted";

const contractStatusMeta: Record<ContractStatus, { label: string; tone: Tone }> = {
  draft: { label: "Draf", tone: "info" },
  generated: { label: "Dokumen dibuat", tone: "info" },
  sent: { label: "Terkirim", tone: "success" },
  signed: { label: "Ditandatangani", tone: "success" },
  ended: { label: "Berakhir", tone: "muted" },
  cancelled: { label: "Dibatalkan", tone: "muted" },
};

export const contractStatusLabels: Record<ContractStatus, string> = Object.fromEntries(
  Object.entries(contractStatusMeta).map(([status, meta]) => [status, meta.label]),
) as Record<ContractStatus, string>;

export function ContractStatusPill({ status }: { status: ContractStatus }) {
  const meta = contractStatusMeta[status];
  return <Pill tone={meta.tone}>{meta.label}</Pill>;
}

export function ContractTypeBadge({ type, recordOnly }: { type: ContractType; recordOnly: boolean }) {
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      <span
        className={cn(
          "inline-flex items-center rounded-md px-1.5 py-0.5 text-[11px] font-[700] tracking-[0.04em]",
          type === "PKWT" ? "bg-hr-light text-hr" : "bg-surface-muted text-text-secondary",
        )}
        title={contractTypeDescriptions[type]}
      >
        {contractTypeLabels[type]}
      </span>
      {recordOnly ? (
        <span className="text-[11px] text-text-tertiary" title="Catatan saja, tanpa dokumen">
          catatan
        </span>
      ) : null}
    </span>
  );
}

function calendarDate(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(value);
  if (!match) {
    return null;
  }
  return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
}

/** "2027-08-31" -> "31 Agu 2027" (calendar date, no timezone shift). */
export function formatShortDate(value: string | null | undefined) {
  if (!value) {
    return "-";
  }
  const date = calendarDate(value);
  if (!date) {
    return value;
  }
  return new Intl.DateTimeFormat("id-ID", { day: "numeric", month: "short", year: "numeric" }).format(date);
}

/** "1 Okt 2026 – 30 Sep 2027", or "sejak 1 Okt 2026" without an end date. */
export function formatContractPeriod(startDate: string, endDate: string | null) {
  if (!endDate) {
    return `Sejak ${formatShortDate(startDate)}`;
  }
  return `${formatShortDate(startDate)} – ${formatShortDate(endDate)}`;
}

/**
 * Whole months from start to end (inclusive), as the documents count them:
 * 1 Oct 2026 – 30 Sep 2027 is 12 months. Returns null when the span is not
 * a whole number of months (the remainder in days is then > 0).
 */
export function contractDurationMonths(startDate: string, endDate: string) {
  const start = calendarDate(startDate);
  const end = calendarDate(endDate);
  if (!start || !end || end < start) {
    return null;
  }
  const dayAfterEnd = new Date(end.getFullYear(), end.getMonth(), end.getDate() + 1);
  let months = (dayAfterEnd.getFullYear() - start.getFullYear()) * 12 + (dayAfterEnd.getMonth() - start.getMonth());
  let anchor = addMonths(start, months);
  if (anchor > dayAfterEnd) {
    months -= 1;
    anchor = addMonths(start, months);
  }
  const days = Math.round((dayAfterEnd.getTime() - anchor.getTime()) / 86_400_000);
  return { months, days };
}

function addMonths(date: Date, months: number) {
  const target = new Date(date.getFullYear(), date.getMonth() + months, 1);
  const lastDay = new Date(target.getFullYear(), target.getMonth() + 1, 0).getDate();
  target.setDate(Math.min(date.getDate(), lastDay));
  return target;
}

/** The end date of a contract of `months` months starting at startDate ("YYYY-MM-DD"). */
export function endDateForMonths(startDate: string, months: number) {
  const start = calendarDate(startDate);
  if (!start) {
    return "";
  }
  const next = addMonths(start, months);
  const end = new Date(next.getFullYear(), next.getMonth(), next.getDate() - 1);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${end.getFullYear()}-${pad(end.getMonth() + 1)}-${pad(end.getDate())}`;
}

/**
 * The 'Batas pemberitahuan' badge (from notice_days + 14 days before the
 * end) and the 'Kedaluwarsa' badge (end date passed, not ended).
 */
export function ContractDeadlineBadges({
  item,
  className,
}: {
  item: Pick<ContractListItem, "notice_alert" | "notice_deadline" | "expired" | "notice_days">;
  className?: string;
}) {
  if (!item.expired && !item.notice_alert) {
    return null;
  }
  return (
    <span className={cn("inline-flex flex-wrap gap-1", className)}>
      {item.expired ? (
        <span
          className="inline-flex items-center gap-1 whitespace-nowrap rounded-full bg-error-light px-2 py-0.5 text-[11px] font-semibold text-error"
          data-testid="contract-expired-badge"
          title="Tanggal berakhir sudah lewat; perpanjang atau akhiri kontrak ini"
        >
          <CalendarX className="h-3 w-3" />
          Kedaluwarsa
        </span>
      ) : null}
      {item.notice_alert && !item.expired && item.notice_deadline ? (
        <span
          className="inline-flex items-center gap-1 whitespace-nowrap rounded-full bg-warning-light px-2 py-0.5 text-[11px] font-semibold text-warning"
          data-testid="contract-notice-badge"
          title={`Pemberitahuan perpanjangan atau pengakhiran paling lambat ${item.notice_days} hari sebelum kontrak berakhir`}
        >
          <CalendarClock className="h-3 w-3" />
          Batas pemberitahuan {formatShortDate(item.notice_deadline)}
        </span>
      ) : null}
    </span>
  );
}

/** The current date in Asia/Jakarta as "YYYY-MM-DD" (status dates may not be in the future). */
export function jakartaToday() {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Jakarta",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date());
}

/** A short sha256 for display: first 12 hex digits. */
export function shortDigest(value: string) {
  return value.length > 12 ? `${value.slice(0, 12)}…` : value;
}

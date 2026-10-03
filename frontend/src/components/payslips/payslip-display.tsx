import type { ReactNode } from "react";
import { OctagonAlert, TriangleAlert, LoaderCircle } from "lucide-react";

import { formatIDR } from "@/lib/currency";
import { cn } from "@/lib/utils";
import type {
  DocumentRenderStatus,
  EmailDeliveryStatus,
  EmailRecipientSource,
  PayslipDeliverySummary,
  PayslipListItem,
  PayslipStatus,
  PayslipWarning,
} from "@/types/documents";

export const payslipMonthLabels = [
  "Januari",
  "Februari",
  "Maret",
  "April",
  "Mei",
  "Juni",
  "Juli",
  "Agustus",
  "September",
  "Oktober",
  "November",
  "Desember",
];

export function formatPayslipPeriod(year: number, month: number) {
  return `${payslipMonthLabels[month - 1] ?? month} ${year}`;
}

/** "2026-09-25" -> "Jumat, 25 September 2026" (calendar date, no timezone shift). */
export function formatLongDate(value: string | null | undefined) {
  if (!value) {
    return "-";
  }
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(value);
  if (!match) {
    return value;
  }
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return new Intl.DateTimeFormat("id-ID", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(date);
}

/** Timestamp in Asia/Jakarta, e.g. "25 Sep 2026 14.05". */
export function formatDateTime(value: string | null | undefined) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return new Intl.DateTimeFormat("id-ID", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "Asia/Jakarta",
  }).format(date);
}

export function formatAmount(value: number) {
  return formatIDR(value);
}

export const recipientSourceLabels: Record<EmailRecipientSource, string> = {
  login: "Email login",
  employee: "Email data karyawan",
  personal: "Email pribadi",
  self: "Email sendiri",
};

type Tone = "neutral" | "info" | "success" | "warning" | "error" | "muted";

const toneClassNames: Record<Tone, { box: string; dot: string }> = {
  neutral: { box: "bg-surface-muted text-text-secondary", dot: "bg-text-tertiary" },
  info: { box: "bg-info-light text-info", dot: "bg-info" },
  success: { box: "bg-success-light text-success", dot: "bg-success" },
  warning: { box: "bg-warning-light text-warning", dot: "bg-warning" },
  error: { box: "bg-error-light text-error", dot: "bg-error" },
  muted: { box: "bg-surface-muted text-text-tertiary", dot: "bg-text-tertiary" },
};

export function Pill({
  tone,
  children,
  spinning = false,
  className,
  title,
}: {
  tone: Tone;
  children: ReactNode;
  spinning?: boolean;
  className?: string;
  title?: string;
}) {
  const classes = toneClassNames[tone];
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-[11px] font-semibold",
        classes.box,
        className,
      )}
      title={title}
    >
      {spinning ? (
        <LoaderCircle className="h-3 w-3 animate-spin" />
      ) : (
        <span className={cn("h-1.5 w-1.5 rounded-full", classes.dot)} />
      )}
      {children}
    </span>
  );
}

const payslipStatusMeta: Record<PayslipStatus, { label: string; tone: Tone }> = {
  none: { label: "Belum dibuat", tone: "neutral" },
  draft: { label: "Draf", tone: "info" },
  sent: { label: "Terkirim", tone: "success" },
  void: { label: "Dibatalkan", tone: "muted" },
};

export function PayslipStatusPill({ status, blocked = false }: { status: PayslipStatus; blocked?: boolean }) {
  if (status === "none" && blocked) {
    return <Pill tone="error">Terblokir</Pill>;
  }
  const meta = payslipStatusMeta[status];
  return <Pill tone={meta.tone}>{meta.label}</Pill>;
}

const renderStatusMeta: Record<DocumentRenderStatus, { label: string; tone: Tone; spinning?: boolean }> = {
  none: { label: "PDF belum dibuat", tone: "neutral" },
  pending: { label: "Menunggu PDF", tone: "warning", spinning: true },
  rendering: { label: "Membuat PDF", tone: "warning", spinning: true },
  ready: { label: "PDF siap", tone: "success" },
  failed: { label: "PDF gagal", tone: "error" },
};

export function RenderStatusPill({ status, error }: { status: DocumentRenderStatus; error?: string | null }) {
  const meta = renderStatusMeta[status];
  return (
    <Pill spinning={meta.spinning} title={error ?? undefined} tone={meta.tone}>
      {meta.label}
    </Pill>
  );
}

const deliveryStatusMeta: Record<EmailDeliveryStatus, { label: string; tone: Tone; spinning?: boolean }> = {
  queued: { label: "Antre kirim", tone: "warning", spinning: true },
  sending: { label: "Mengirim", tone: "warning", spinning: true },
  sent: { label: "Email terkirim", tone: "success" },
  failed: { label: "Gagal kirim", tone: "error" },
};

export function DeliveryStatusPill({ delivery }: { delivery: Pick<PayslipDeliverySummary, "status" | "error"> }) {
  const meta = deliveryStatusMeta[delivery.status];
  return (
    <Pill spinning={meta.spinning} title={delivery.error ?? undefined} tone={meta.tone}>
      {meta.label}
    </Pill>
  );
}

export function isRenderInProgress(status: DocumentRenderStatus) {
  return status === "pending" || status === "rendering";
}

export function isDeliveryInFlight(delivery: PayslipDeliverySummary | null | undefined) {
  return delivery?.status === "queued" || delivery?.status === "sending";
}

/**
 * The progress line under the status: rendering, sending or a failure. A
 * failed latest delivery is the "Gagal" state of the flow, also when it was
 * a 'Kirim ulang' of a slip that had been sent before. Without a PDF
 * converter a draft has no PDF by design (checked as DOCX), so no render
 * pill is shown.
 */
export function PayslipProgress({
  item,
  pdfAvailable = true,
}: {
  item: Pick<PayslipListItem, "status" | "render_status" | "render_error" | "last_delivery">;
  pdfAvailable?: boolean;
}) {
  if (item.status === "none") {
    return null;
  }
  if (isDeliveryInFlight(item.last_delivery)) {
    return <DeliveryStatusPill delivery={item.last_delivery!} />;
  }
  if (item.last_delivery?.status === "failed") {
    if (item.status === "sent") {
      return (
        <Pill title={item.last_delivery.error ?? undefined} tone="error">
          Kirim ulang gagal
        </Pill>
      );
    }
    return <DeliveryStatusPill delivery={item.last_delivery} />;
  }
  if (item.render_status !== "ready" && (pdfAvailable || item.render_status !== "none") && item.status === "draft") {
    return <RenderStatusPill error={item.render_error} status={item.render_status} />;
  }
  return null;
}

export function WarningChips({
  warnings,
  limit = 3,
}: {
  warnings: PayslipWarning[];
  limit?: number;
}) {
  if (warnings.length === 0) {
    return <span className="text-xs text-text-tertiary">-</span>;
  }
  const sorted = [...warnings].sort((left, right) => Number(right.blocking) - Number(left.blocking));
  const visible = sorted.slice(0, limit);
  const hidden = sorted.slice(limit);
  return (
    <div className="flex min-w-[120px] max-w-[190px] flex-wrap gap-1">
      {visible.map((warning) => (
        <WarningChip key={warning.code} warning={warning} />
      ))}
      {hidden.length > 0 ? (
        <span
          className="inline-flex items-center rounded-full bg-surface-muted px-2 py-0.5 text-[11px] font-semibold text-text-secondary"
          title={hidden.map((warning) => warning.message).join("\n")}
        >
          +{hidden.length} lainnya
        </span>
      ) : null}
    </div>
  );
}

/**
 * Short chip text: the part of the message before the explanation and
 * without a trailing "(...)" detail; the full message is the chip's title.
 */
export function shortWarning(message: string) {
  const [head] = message.split(":");
  return (head ?? message).replace(/\s*\([^)]*\)\s*$/, "").trim();
}

export function WarningChip({ warning }: { warning: PayslipWarning }) {
  const Icon = warning.blocking ? OctagonAlert : TriangleAlert;
  return (
    <span
      className={cn(
        "inline-flex max-w-full items-start gap-1 rounded-[10px] px-2 py-0.5 text-[11px] font-semibold leading-4",
        warning.blocking ? "bg-error-light text-error" : "bg-warning-light text-warning",
      )}
      title={warning.message}
    >
      <Icon className="mt-0.5 h-3 w-3 shrink-0" />
      <span>{shortWarning(warning.message)}</span>
    </span>
  );
}

export function WarningList({ warnings }: { warnings: PayslipWarning[] }) {
  if (warnings.length === 0) {
    return null;
  }
  const sorted = [...warnings].sort((left, right) => Number(right.blocking) - Number(left.blocking));
  return (
    <ul className="space-y-2">
      {sorted.map((warning) => {
        const Icon = warning.blocking ? OctagonAlert : TriangleAlert;
        return (
          <li
            className={cn(
              "flex items-start gap-2 rounded-xl px-3 py-2 text-[13px] leading-5",
              warning.blocking ? "bg-error-light text-error" : "bg-warning-light text-warning",
            )}
            key={warning.code}
          >
            <Icon className="mt-0.5 h-4 w-4 shrink-0" />
            <span>
              {warning.message}
              {warning.blocking ? <strong className="ml-1">(memblokir)</strong> : null}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

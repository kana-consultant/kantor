import { Pill } from "@/components/payslips/payslip-display";
import type { LegalDocStatus, LegalPartyType } from "@/types/legal";

type Tone = "neutral" | "info" | "success" | "warning" | "error" | "muted";

const statusMeta: Record<LegalDocStatus, { label: string; tone: Tone }> = {
  draft: { label: "Draf", tone: "info" },
  generated: { label: "Dokumen dibuat", tone: "info" },
  sent: { label: "Terkirim", tone: "success" },
  signed: { label: "Ditandatangani", tone: "success" },
  cancelled: { label: "Dibatalkan", tone: "muted" },
};

export const legalStatusValues = ["draft", "generated", "sent", "signed", "cancelled"] as const;

export const legalStatusLabels: Record<LegalDocStatus, string> = Object.fromEntries(
  Object.entries(statusMeta).map(([status, meta]) => [status, meta.label]),
) as Record<LegalDocStatus, string>;

export const partyTypeLabels: Record<LegalPartyType, string> = {
  person: "Perorangan",
  company: "Badan usaha",
};

export function LegalStatusPill({ status }: { status: LegalDocStatus }) {
  const meta = statusMeta[status];
  return <Pill tone={meta.tone}>{meta.label}</Pill>;
}

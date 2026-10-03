import { useQuery } from "@tanstack/react-query";
import { LoaderCircle, Mail, Paperclip } from "lucide-react";

import { DeliveryStatusPill, formatDateTime, recipientSourceLabels } from "@/components/payslips/payslip-display";
import { emailDeliveriesKeys, listEmailDeliveries } from "@/services/email-deliveries";
import type { DocumentReferenceType } from "@/types/documents";

/**
 * 'Riwayat Pengiriman' of one document, with each attachment's sha256 (the
 * digest recorded when it was sent). `version` (e.g. the latest delivery id
 * and status) re-fetches the list when a send finishes.
 */
export function DeliveryHistory({
  referenceType,
  referenceId,
  version,
}: {
  referenceType: DocumentReferenceType;
  referenceId: string;
  version: string;
}) {
  const query = useQuery({
    queryKey: [...emailDeliveriesKeys.reference(referenceType, referenceId), version],
    queryFn: () => listEmailDeliveries(referenceType, referenceId),
    refetchOnWindowFocus: false,
  });

  if (query.isLoading) {
    return (
      <p className="flex items-center gap-2 text-sm text-text-secondary">
        <LoaderCircle className="h-4 w-4 animate-spin" />
        Memuat riwayat...
      </p>
    );
  }
  if (query.error instanceof Error) {
    return <p className="text-sm text-error">{query.error.message}</p>;
  }
  const deliveries = query.data ?? [];
  if (deliveries.length === 0) {
    return <p className="text-sm text-text-secondary">Belum pernah dikirim.</p>;
  }

  return (
    <ul className="space-y-2" data-testid="delivery-history">
      {deliveries.map((delivery) => (
        <li className="rounded-xl border border-border/70 bg-surface-muted/40 px-3 py-2.5" key={delivery.id}>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="flex min-w-0 items-center gap-1.5 text-[13px] font-semibold text-text-primary">
              <Mail className="h-3.5 w-3.5 shrink-0 text-text-tertiary" />
              <span className="truncate">{delivery.recipient}</span>
            </p>
            <DeliveryStatusPill delivery={delivery} />
          </div>
          <p className="mt-1 text-[12px] text-text-secondary">
            {recipientSourceLabels[delivery.recipient_source] ?? delivery.recipient_source} ·{" "}
            {formatDateTime(delivery.sent_at ?? delivery.created_at)}
            {delivery.attempts > 1 ? ` · ${delivery.attempts} percobaan` : ""}
          </p>
          <p className="mt-0.5 truncate text-[12px] text-text-tertiary" title={delivery.subject}>
            {delivery.subject}
          </p>
          {(delivery.attachment_names ?? []).length > 0 ? (
            <ul className="mt-0.5 space-y-0.5" data-testid="delivery-attachments">
              {(delivery.attachment_names ?? []).map((name, index) => {
                const digest = delivery.attachment_sha256?.[index];
                return (
                  <li className="flex min-w-0 items-center gap-1 text-[12px] text-text-tertiary" key={`${name}-${index}`}>
                    <Paperclip className="h-3 w-3 shrink-0" />
                    <span className="truncate">{name}</span>
                    {digest ? (
                      <span className="shrink-0 font-mono text-[11px]" title={`SHA-256 ${digest}`}>
                        sha256 {digest.slice(0, 12)}…
                      </span>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          ) : null}
          {(delivery.cc ?? []).length > 0 ? (
            <p className="mt-0.5 text-[12px] text-text-tertiary">CC: {(delivery.cc ?? []).join(", ")}</p>
          ) : null}
          {delivery.error ? <p className="mt-1 text-[12px] text-error">{delivery.error}</p> : null}
        </li>
      ))}
    </ul>
  );
}

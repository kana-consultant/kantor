import { authGetJSON } from "@/lib/api-client";
import type { DocumentReferenceType, EmailDelivery } from "@/types/documents";

export const emailDeliveriesKeys = {
  all: ["hris", "email-deliveries"] as const,
  reference: (referenceType: DocumentReferenceType, referenceId: string) =>
    [...emailDeliveriesKeys.all, referenceType, referenceId] as const,
};

/** Delivery history ('Riwayat Pengiriman') of one payslip or contract, newest first. */
export function listEmailDeliveries(referenceType: DocumentReferenceType, referenceId: string) {
  const search = new URLSearchParams({ reference_type: referenceType, reference_id: referenceId });
  return authGetJSON<EmailDelivery[]>(`/hris/email-deliveries?${search.toString()}`);
}

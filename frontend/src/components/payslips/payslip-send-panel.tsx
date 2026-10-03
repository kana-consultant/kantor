import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LoaderCircle, Mail, Send } from "lucide-react";

import { Pill, recipientSourceLabels } from "@/components/payslips/payslip-display";
import { Button } from "@/components/ui/button";
import { emailDeliveriesKeys } from "@/services/email-deliveries";
import { getPayslipRecipient, payslipsKeys, sendPayslip } from "@/services/hris-payslips";
import { toast } from "@/stores/toast-store";
import type { PayslipDetail, PayslipRecipientChoice } from "@/types/documents";

const choiceLabels: Record<PayslipRecipientChoice, string> = {
  default: "Otomatis (email login / email karyawan)",
  login: "Email login",
  employee: "Email data karyawan",
  personal: "Email pribadi (profil HR)",
};

/**
 * Single send of one slip (Kirim / Kirim ulang). The resolved recipient is
 * shown before sending; 'alamat baru' flags an address that differs from the
 * last successful delivery to the employee.
 */
export function PayslipSendPanel({
  detail,
  documentMailReady,
  disabledReason,
}: {
  detail: PayslipDetail;
  documentMailReady: boolean;
  disabledReason: string | null;
}) {
  const queryClient = useQueryClient();
  const [choice, setChoice] = useState<PayslipRecipientChoice>("default");

  // The default resolution also says which other sources exist (linked
  // login, personal e-mail); the chosen one is shown before sending.
  const baseQuery = useQuery({
    queryKey: payslipsKeys.recipient(detail.id, "default"),
    queryFn: () => getPayslipRecipient(detail.id, "default"),
    refetchOnWindowFocus: false,
    retry: false,
  });
  const recipientQuery = useQuery({
    queryKey: payslipsKeys.recipient(detail.id, choice),
    queryFn: () => getPayslipRecipient(detail.id, choice),
    refetchOnWindowFocus: false,
    retry: false,
  });

  const sendMutation = useMutation({
    mutationFn: () => sendPayslip(detail.id, choice),
    onSuccess: async (result) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: payslipsKeys.all }),
        queryClient.invalidateQueries({ queryKey: emailDeliveriesKeys.all }),
      ]);
      if (result.sent) {
        toast.success(`Slip ${result.payslip.doc_number} terkirim`, `Ke ${result.delivery.recipient}`);
      } else {
        toast.error("Email gagal dikirim", result.error_message ?? result.delivery.error ?? undefined);
      }
    },
    onError: (error) => {
      toast.error("Gagal mengirim slip", error instanceof Error ? error.message : undefined);
    },
  });

  const recipient = recipientQuery.data;
  const base = baseQuery.data;
  // Only offer the choices that can resolve for this employee.
  const choices: PayslipRecipientChoice[] = ["default"];
  if (base?.linked) {
    choices.push("login", "employee");
  }
  if (base?.personal_available) {
    choices.push("personal");
  }
  if (!choices.includes(choice)) {
    choices.push(choice);
  }

  const blocked = disabledReason ?? (!documentMailReady ? "Email Dokumen belum siap" : null);
  const label = detail.status === "sent" ? "Kirim ulang" : "Kirim";

  return (
    <div className="space-y-3" data-testid="payslip-send-panel">
      <div className="space-y-1.5">
        <label className="block text-sm font-medium text-text-primary" htmlFor="payslip-recipient-source">
          Penerima
        </label>
        <select
          className="field-select"
          id="payslip-recipient-source"
          onChange={(event) => setChoice(event.target.value as PayslipRecipientChoice)}
          value={choice}
        >
          {choices.map((option) => (
            <option key={option} value={option}>
              {choiceLabels[option]}
            </option>
          ))}
        </select>
      </div>

      <div className="rounded-xl border border-border/70 bg-surface-muted/40 px-3 py-2.5">
        {recipientQuery.isLoading ? (
          <p className="flex items-center gap-2 text-sm text-text-secondary">
            <LoaderCircle className="h-4 w-4 animate-spin" />
            Memeriksa alamat penerima...
          </p>
        ) : recipientQuery.error instanceof Error ? (
          <p className="text-sm text-error">{recipientQuery.error.message}</p>
        ) : recipient ? (
          <div className="space-y-1">
            <p className="flex flex-wrap items-center gap-2 text-sm">
              <Mail className="h-4 w-4 text-text-tertiary" />
              <span className="font-semibold text-text-primary" data-testid="payslip-recipient">
                {recipient.recipient}
              </span>
              {recipient.is_new ? <Pill tone="warning">alamat baru</Pill> : null}
              {recipient.recipient_source === "personal" ? <Pill tone="warning">email pribadi</Pill> : null}
            </p>
            <p className="text-[12px] text-text-secondary">
              {recipientSourceLabels[recipient.recipient_source]}
              {recipient.is_new && recipient.previous_recipient
                ? ` · sebelumnya dikirim ke ${recipient.previous_recipient}`
                : ""}
            </p>
          </div>
        ) : null}
      </div>

      {blocked ? <p className="text-[13px] text-warning">{blocked}</p> : null}

      <div className="flex justify-end">
        <Button
          disabled={Boolean(blocked) || !recipient || sendMutation.isPending}
          onClick={() => sendMutation.mutate()}
          size="sm"
          type="button"
        >
          <Send className="h-4 w-4" />
          {sendMutation.isPending ? "Mengirim..." : label}
        </Button>
      </div>
    </div>
  );
}

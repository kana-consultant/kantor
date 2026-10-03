import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { TriangleAlert, LoaderCircle, Mail, Send } from "lucide-react";

import { Pill, recipientSourceLabels } from "@/components/payslips/payslip-display";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { payslipsKeys, previewPayslipSend, sendPayslipBatch } from "@/services/hris-payslips";
import { toast } from "@/stores/toast-store";
import type { PayslipRecipient } from "@/types/documents";

/**
 * Confirm dialog of 'Kirim Terpilih': lists every recipient the batch would
 * use, flags new ('alamat baru') and non-login addresses, and skips slips
 * that were already sent unless HR opts in.
 */
export function SendPayslipsDialog({
  open,
  onClose,
  payslipIds,
  onQueued,
}: {
  open: boolean;
  onClose: () => void;
  payslipIds: string[];
  onQueued?: () => void;
}) {
  const queryClient = useQueryClient();
  const [includeAlreadySent, setIncludeAlreadySent] = useState(false);

  useEffect(() => {
    if (!open) {
      setIncludeAlreadySent(false);
    }
  }, [open]);

  const previewQuery = useQuery({
    queryKey: [...payslipsKeys.all, "send-preview", payslipIds, includeAlreadySent],
    queryFn: () => previewPayslipSend(payslipIds, includeAlreadySent),
    enabled: open && payslipIds.length > 0,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });

  const sendMutation = useMutation({
    mutationFn: () => sendPayslipBatch(payslipIds, includeAlreadySent),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: payslipsKeys.all });
      const queued = result.queued.length;
      const skippedOther = result.skipped.length - result.already_sent_skipped;
      toast.success(
        `${queued} slip masuk antrean kirim`,
        [
          "Email dikirim satu per satu di latar belakang; status di tabel diperbarui otomatis.",
          result.already_sent_skipped > 0 ? `${result.already_sent_skipped} sudah terkirim, dilewati.` : "",
          skippedOther > 0 ? `${skippedOther} lainnya dilewati.` : "",
        ]
          .filter(Boolean)
          .join(" "),
      );
      onQueued?.();
      onClose();
    },
    onError: (error) => {
      toast.error("Gagal mengirim slip gaji", error instanceof Error ? error.message : undefined);
    },
  });

  const preview = previewQuery.data;
  const recipients = preview?.recipients ?? [];
  const otherSkipped = (preview?.skipped ?? []).filter((item) => item.code !== "already_sent");
  const newCount = recipients.filter((item) => item.is_new).length;
  const busy = sendMutation.isPending;

  return (
    <Dialog dismissible={!busy} onOpenChange={(next) => (!next ? onClose() : undefined)} open={open}>
      <DialogContent size="lg">
        <DialogHeader className="flex items-start justify-between gap-4">
          <div>
            <DialogTitle>Kirim slip gaji</DialogTitle>
            <DialogDescription>
              Periksa penerima sebelum mengirim. Setiap email hanya berisi PDF slip, tanpa nominal di badan email.
            </DialogDescription>
          </div>
          <DialogClose disabled={busy} />
        </DialogHeader>
        <DialogBody>
          {previewQuery.isLoading ? (
            <div className="flex items-center gap-2 py-8 text-sm text-text-secondary">
              <LoaderCircle className="h-4 w-4 animate-spin" />
              Menyiapkan daftar penerima...
            </div>
          ) : previewQuery.error instanceof Error ? (
            <p className="rounded-xl bg-error-light px-4 py-3 text-sm text-error">{previewQuery.error.message}</p>
          ) : preview ? (
            <div className="space-y-4">
              {!preview.document_mail_ready ? (
                <p className="flex items-start gap-2 rounded-xl bg-error-light px-4 py-3 text-sm text-error">
                  <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
                  Email Dokumen belum siap. Admin perlu mengaktifkan dan mengisi akun Gmail di Admin &gt; Pengaturan.
                </p>
              ) : null}

              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span className="font-semibold text-text-primary" data-testid="send-recipient-count">
                  {recipients.length} penerima
                </span>
                {newCount > 0 ? <Pill tone="warning">{newCount} alamat baru</Pill> : null}
                {preview.already_sent_skipped > 0 && !includeAlreadySent ? (
                  <Pill tone="neutral">{preview.already_sent_skipped} sudah terkirim, dilewati</Pill>
                ) : null}
              </div>

              {recipients.length > 0 ? (
                <ul className="max-h-[340px] divide-y divide-border overflow-y-auto rounded-xl border border-border/70" data-testid="send-recipient-list">
                  {recipients.map((recipient) => (
                    <RecipientRow key={recipient.payslip_id} recipient={recipient} />
                  ))}
                </ul>
              ) : (
                <p className="rounded-xl bg-surface-muted px-4 py-3 text-sm text-text-secondary">
                  Tidak ada slip yang akan dikirim.
                </p>
              )}

              {otherSkipped.length > 0 ? (
                <div className="rounded-xl border border-warning/30 bg-warning-light/60 px-4 py-3 text-[13px] text-text-secondary">
                  <p className="font-semibold text-warning">{otherSkipped.length} slip dilewati</p>
                  <ul className="mt-1 space-y-0.5">
                    {otherSkipped.map((item) => (
                      <li key={`${item.payslip_id}-${item.code}`}>
                        {item.employee_name || item.doc_number || "Slip"}: {item.reason}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}

              {preview.already_sent_skipped > 0 || includeAlreadySent ? (
                <label className="flex cursor-pointer items-start gap-2 text-sm text-text-secondary">
                  <input
                    checked={includeAlreadySent}
                    className="mt-0.5 h-4 w-4 accent-primary"
                    onChange={(event) => setIncludeAlreadySent(event.target.checked)}
                    type="checkbox"
                  />
                  <span>Kirim ulang juga slip yang sudah terkirim</span>
                </label>
              ) : null}
            </div>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button disabled={busy} onClick={onClose} type="button" variant="ghost">
            Batal
          </Button>
          <Button
            disabled={busy || !preview || !preview.document_mail_ready || recipients.length === 0 || previewQuery.isFetching}
            onClick={() => sendMutation.mutate()}
            type="button"
          >
            <Send className="h-4 w-4" />
            {busy ? "Mengirim..." : `Kirim ${recipients.length} slip`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RecipientRow({ recipient }: { recipient: PayslipRecipient }) {
  const highlight = recipient.is_new || recipient.recipient_source === "personal";
  return (
    <li
      className={highlight ? "bg-warning-light/50 px-4 py-3" : "px-4 py-3"}
      data-new={recipient.is_new ? "true" : undefined}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-semibold text-text-primary">{recipient.employee_name}</p>
          <p className="font-mono text-[11px] text-text-tertiary">
            {recipient.doc_number}
            {recipient.status === "sent" ? " · kirim ulang" : ""}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {recipient.is_new ? (
            <Pill
              title={recipient.previous_recipient ? `Sebelumnya: ${recipient.previous_recipient}` : undefined}
              tone="warning"
            >
              alamat baru
            </Pill>
          ) : null}
          {recipient.recipient_source === "personal" ? <Pill tone="warning">email pribadi</Pill> : null}
          {recipient.recipient_source === "employee" ? (
            <Pill tone="neutral" title="Karyawan belum tertaut ke akun login">
              email data karyawan
            </Pill>
          ) : null}
        </div>
      </div>
      <p className="mt-1 flex items-center gap-1.5 text-[13px] text-text-secondary">
        <Mail className="h-3.5 w-3.5" />
        <span className="font-medium text-text-primary">{recipient.recipient}</span>
        <span className="text-text-tertiary">({recipientSourceLabels[recipient.recipient_source]})</span>
      </p>
      {recipient.is_new && recipient.previous_recipient ? (
        <p className="mt-0.5 text-[12px] text-warning">Sebelumnya dikirim ke {recipient.previous_recipient}</p>
      ) : null}
    </li>
  );
}

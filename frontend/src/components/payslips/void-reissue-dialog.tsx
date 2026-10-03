import { useEffect } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { emailDeliveriesKeys } from "@/services/email-deliveries";
import { payslipsKeys, voidReissuePayslip } from "@/services/hris-payslips";
import { toast } from "@/stores/toast-store";
import type { PayslipDetail, VoidReissuePayslipResponse } from "@/types/documents";

const voidSchema = z.object({
  reason: z.string().trim().min(3, "Alasan minimal 3 karakter").max(200, "Maksimal 200 karakter"),
});

type VoidFormValues = z.infer<typeof voidSchema>;

/** Mirrors the backend: '<base number>-R<revision+1>'. */
function reissueNumber(detail: PayslipDetail) {
  return `${detail.doc_number.replace(/-R\d+$/, "")}-R${detail.revision + 1}`;
}

/**
 * 'Batalkan & Terbitkan Ulang': voids a sent slip (it is kept, marked
 * Dibatalkan) and creates a draft replacement numbered '<no>-R<n>' with the
 * same bonuses and reimbursements. The replacement still has to be sent.
 */
export function VoidReissueDialog({
  open,
  onClose,
  detail,
  onDone,
}: {
  open: boolean;
  onClose: () => void;
  detail: PayslipDetail;
  onDone?: (result: VoidReissuePayslipResponse) => void;
}) {
  const queryClient = useQueryClient();
  const form = useForm<VoidFormValues>({
    resolver: zodResolver(voidSchema),
    defaultValues: { reason: "" },
  });
  const { reset } = form;

  useEffect(() => {
    if (open) {
      reset({ reason: "" });
    }
  }, [open, reset]);

  const mutation = useMutation({
    mutationFn: (values: VoidFormValues) => voidReissuePayslip(detail.id, values.reason.trim()),
    onSuccess: async (result) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: payslipsKeys.all }),
        queryClient.invalidateQueries({ queryKey: emailDeliveriesKeys.all }),
      ]);
      toast.success(
        `Slip ${result.voided.doc_number} dibatalkan`,
        `Draf pengganti ${result.reissue.doc_number} dibuat. Periksa lalu kirim ulang ke karyawan.`,
      );
      onDone?.(result);
      onClose();
    },
    onError: (error) => {
      toast.error("Gagal membatalkan slip", error instanceof Error ? error.message : undefined);
    },
  });

  return (
    <FormModal
      isLoading={mutation.isPending}
      isOpen={open}
      onClose={onClose}
      onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
      size="md"
      submitLabel="Batalkan & Terbitkan Ulang"
      subtitle={`Slip ${detail.doc_number} akan ditandai Dibatalkan (tetap disimpan) dan diganti draf baru bernomor ${reissueNumber(detail)}. Email slip pengganti menyebutkan nomor slip yang digantikan.`}
      title="Batalkan & terbitkan ulang slip"
    >
      <div className="space-y-1.5">
        <label className="block text-sm font-medium text-text-primary" htmlFor="payslip-void-reason">
          Alasan pembatalan<span className="ml-0.5 text-priority-high">*</span>
        </label>
        <textarea
          className="min-h-24 w-full rounded-xl border border-border/70 bg-surface-muted/90 px-3.5 py-2 text-[14px] text-text-primary outline-none transition-all duration-150 placeholder:text-text-tertiary focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
          id="payslip-void-reason"
          maxLength={200}
          placeholder="mis. Tunjangan transport salah input"
          {...form.register("reason")}
        />
        {form.formState.errors.reason?.message ? (
          <p className="text-[12px] font-[500] text-priority-high">{form.formState.errors.reason.message}</p>
        ) : (
          <p className="text-xs text-text-secondary">Dicatat di audit log; tidak dicetak di slip.</p>
        )}
      </div>
    </FormModal>
  );
}

import { useEffect } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarCheck2, CircleStop } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { formatShortDate, jakartaToday } from "@/components/contracts/contract-display";
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
import { Input } from "@/components/ui/input";
import { contractsKeys, updateContractStatus } from "@/services/hris-contracts";
import { toast } from "@/stores/toast-store";
import type { ContractDetail, UpdateContractStatusPayload } from "@/types/contracts";

const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

function statusDate(label: string) {
  return z
    .string()
    .trim()
    .refine((value) => DATE_PATTERN.test(value), `${label} wajib diisi`)
    .refine((value) => value <= jakartaToday(), `${label} tidak boleh di masa depan`)
    .refine((value) => value >= "2000-01-01", `${label} tidak valid`);
}

/** Shared mutation of the status dialogs: updates the detail cache and the lists. */
function useStatusMutation(contract: ContractDetail, onDone: () => void, successTitle: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: UpdateContractStatusPayload) => updateContractStatus(contract.id, payload),
    onSuccess: async (saved) => {
      queryClient.setQueryData(contractsKeys.detail(saved.id), saved);
      await queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "list"] });
      toast.success(successTitle);
      onDone();
    },
    onError: (error) => {
      toast.error("Gagal mengubah status kontrak", error instanceof Error ? error.message : undefined);
    },
  });
}

const signSchema = z.object({ signed_at: statusDate("Tanggal tanda tangan") });
type SignValues = z.infer<typeof signSchema>;

/** Tandai Ditandatangani: the signing date (today by default, never in the future). */
export function SignContractDialog({
  contract,
  open,
  onClose,
}: {
  contract: ContractDetail;
  open: boolean;
  onClose: () => void;
}) {
  const mutation = useStatusMutation(contract, onClose, "Kontrak ditandai ditandatangani");
  const {
    formState: { errors },
    handleSubmit,
    register,
    reset,
  } = useForm<SignValues>({ resolver: zodResolver(signSchema), defaultValues: { signed_at: jakartaToday() } });

  useEffect(() => {
    if (open) {
      reset({ signed_at: jakartaToday() });
    }
  }, [open, reset]);

  return (
    <Dialog dismissible={!mutation.isPending} onOpenChange={(next) => (!next ? onClose() : undefined)} open={open}>
      <DialogContent size="sm">
        <form
          className="flex min-h-0 flex-1 flex-col overflow-hidden"
          noValidate
          onSubmit={handleSubmit((values) => mutation.mutate({ status: "signed", signed_at: values.signed_at }))}
        >
          <DialogHeader className="flex items-start justify-between gap-4">
            <div>
              <DialogTitle>Tandai ditandatangani</DialogTitle>
              <DialogDescription>
                {contract.is_record_only
                  ? "Catatan kontrak ini menjadi kontrak yang berlaku."
                  : "Setelah ditandatangani, kontrak tidak dapat diedit lagi. Perubahan berikutnya lewat Perpanjang."}
              </DialogDescription>
            </div>
            <DialogClose disabled={mutation.isPending} />
          </DialogHeader>
          <DialogBody className="space-y-1.5">
            <label className="text-sm font-medium text-text-primary" htmlFor="contract-signed-at">
              Tanggal tanda tangan
            </label>
            <Input
              disabled={mutation.isPending}
              id="contract-signed-at"
              max={jakartaToday()}
              type="date"
              {...register("signed_at")}
            />
            {errors.signed_at?.message ? <p className="text-[12px] text-error">{errors.signed_at.message}</p> : null}
          </DialogBody>
          <DialogFooter>
            <Button disabled={mutation.isPending} onClick={onClose} type="button" variant="ghost">
              Batal
            </Button>
            <Button data-testid="contract-sign-confirm" disabled={mutation.isPending} type="submit">
              <CalendarCheck2 className="h-4 w-4" />
              {mutation.isPending ? "Menyimpan..." : "Tandai ditandatangani"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

const endSchema = z.object({
  ended_at: statusDate("Tanggal berakhir"),
  end_notes: z.string().trim().max(500, "Maksimal 500 karakter"),
  compensation_checked: z.boolean().refine((value) => value, "Centang setelah uang kompensasi dijadwalkan"),
  handover_checked: z.boolean().refine((value) => value, "Centang setelah serah terima diatur"),
});
type EndValues = z.infer<typeof endSchema>;

function defaultEndDate(contract: ContractDetail) {
  const today = jakartaToday();
  return contract.end_date && contract.end_date < today ? contract.end_date : today;
}

/**
 * Akhiri: the end date and notes, behind a checklist that reminds HR of
 * the uang kompensasi (a manual line on the final payslip; PP 35/2021) and
 * the serah terima (assets, access, work handover).
 */
export function EndContractDialog({
  contract,
  open,
  onClose,
}: {
  contract: ContractDetail;
  open: boolean;
  onClose: () => void;
}) {
  const mutation = useStatusMutation(contract, onClose, "Kontrak diakhiri");
  const pkwt = contract.contract_type === "PKWT";
  const {
    formState: { errors },
    handleSubmit,
    register,
    reset,
  } = useForm<EndValues>({
    resolver: zodResolver(endSchema),
    defaultValues: {
      ended_at: defaultEndDate(contract),
      end_notes: "",
      compensation_checked: !pkwt,
      handover_checked: false,
    },
  });

  useEffect(() => {
    if (open) {
      reset({ ended_at: defaultEndDate(contract), end_notes: "", compensation_checked: !pkwt, handover_checked: false });
    }
  }, [open, reset, contract, pkwt]);

  return (
    <Dialog dismissible={!mutation.isPending} onOpenChange={(next) => (!next ? onClose() : undefined)} open={open}>
      <DialogContent size="md">
        <form
          className="flex min-h-0 flex-1 flex-col overflow-hidden"
          noValidate
          onSubmit={handleSubmit((values) =>
            mutation.mutate({
              status: "ended",
              ended_at: values.ended_at,
              ...(values.end_notes ? { end_notes: values.end_notes } : {}),
            }),
          )}
        >
          <DialogHeader className="flex items-start justify-between gap-4">
            <div>
              <DialogTitle>Akhiri kontrak</DialogTitle>
              <DialogDescription>
                {contract.end_date
                  ? `Periode kontrak sampai ${formatShortDate(contract.end_date)}.`
                  : "Kontrak tanpa tanggal berakhir."}{" "}
                Pastikan kewajiban akhir kontrak sudah diatur.
              </DialogDescription>
            </div>
            <DialogClose disabled={mutation.isPending} />
          </DialogHeader>
          <DialogBody className="space-y-4">
            <div className="space-y-2 rounded-xl border border-border/70 p-3" data-testid="contract-end-checklist">
              <p className="text-[12px] font-[700] uppercase tracking-[0.08em] text-text-secondary">Checklist</p>
              {pkwt ? (
                <label className="flex cursor-pointer items-start gap-2 text-[13px] text-text-primary">
                  <input className="mt-0.5 h-4 w-4 accent-primary" type="checkbox" {...register("compensation_checked")} />
                  <span>
                    Uang kompensasi PKWT sudah dihitung dan dijadwalkan sebagai baris penyesuaian manual di slip gaji
                    terakhir.
                  </span>
                </label>
              ) : null}
              {errors.compensation_checked?.message ? (
                <p className="text-[12px] text-error">{errors.compensation_checked.message}</p>
              ) : null}
              <label className="flex cursor-pointer items-start gap-2 text-[13px] text-text-primary">
                <input className="mt-0.5 h-4 w-4 accent-primary" type="checkbox" {...register("handover_checked")} />
                <span>Serah terima pekerjaan, aset perusahaan, dan akses akun sudah diatur.</span>
              </label>
              {errors.handover_checked?.message ? (
                <p className="text-[12px] text-error">{errors.handover_checked.message}</p>
              ) : null}
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-text-primary" htmlFor="contract-ended-at">
                Tanggal berakhir
              </label>
              <Input disabled={mutation.isPending} id="contract-ended-at" max={jakartaToday()} type="date" {...register("ended_at")} />
              {errors.ended_at?.message ? <p className="text-[12px] text-error">{errors.ended_at.message}</p> : null}
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-text-primary" htmlFor="contract-end-notes">
                Catatan (opsional)
              </label>
              <textarea
                className="min-h-20 w-full rounded-xl border border-border/70 bg-surface-muted/90 px-3.5 py-2 text-[14px] text-text-primary outline-none transition-all duration-150 placeholder:text-text-tertiary focus-visible:border-[#4C9AFF] focus-visible:bg-surface focus-visible:shadow-focus"
                disabled={mutation.isPending}
                id="contract-end-notes"
                maxLength={500}
                placeholder="mis. Tidak diperpanjang sesuai kesepakatan"
                {...register("end_notes")}
              />
              {errors.end_notes?.message ? <p className="text-[12px] text-error">{errors.end_notes.message}</p> : null}
            </div>
          </DialogBody>
          <DialogFooter>
            <Button disabled={mutation.isPending} onClick={onClose} type="button" variant="ghost">
              Batal
            </Button>
            <Button data-testid="contract-end-confirm" disabled={mutation.isPending} type="submit" variant="danger">
              <CircleStop className="h-4 w-4" />
              {mutation.isPending ? "Menyimpan..." : "Akhiri kontrak"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

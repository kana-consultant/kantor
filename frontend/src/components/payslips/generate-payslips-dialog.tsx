import { useEffect } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm, useWatch } from "react-hook-form";
import { z } from "zod";

import { formatLongDate, formatPayslipPeriod } from "@/components/payslips/payslip-display";
import { FormModal } from "@/components/shared/form-modal";
import { Input } from "@/components/ui/input";
import { generatePayslips, payslipsKeys } from "@/services/hris-payslips";
import { toast } from "@/stores/toast-store";
import type { GeneratePayslipsResponse } from "@/types/documents";

export interface GenerateTarget {
  employee_id: string;
  employee_name: string;
  status: "none" | "draft" | "sent" | "void";
}

function pad(value: number) {
  return String(value).padStart(2, "0");
}

function isoDate(date: Date) {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/**
 * The backend accepts a pay date from the weekday on or before the 1st of
 * the period (a payday on a weekend 1st moves back into the month before)
 * to the end of the month after it.
 */
export function payDateBounds(year: number, month: number) {
  const first = new Date(year, month - 1, 1);
  const weekday = first.getDay();
  if (weekday === 0) {
    first.setDate(first.getDate() - 2);
  } else if (weekday === 6) {
    first.setDate(first.getDate() - 1);
  }
  return { min: isoDate(first), max: isoDate(new Date(year, month + 1, 0)) };
}

function buildSchema(year: number, month: number) {
  const { min, max } = payDateBounds(year, month);
  return z.object({
    pay_date: z
      .string()
      .regex(/^\d{4}-\d{2}-\d{2}$/, "Tanggal bayar wajib diisi")
      .refine((value) => value >= min && value <= max, "Tanggal bayar harus di bulan periode atau bulan berikutnya (atau hari kerja terakhir sebelum tanggal 1)"),
  });
}

type GenerateFormValues = { pay_date: string };

export function GeneratePayslipsDialog({
  open,
  onClose,
  year,
  month,
  targets,
  defaultPayDate,
  title = "Generate Draft",
  onGenerated,
}: {
  open: boolean;
  onClose: () => void;
  year: number;
  month: number;
  targets: GenerateTarget[];
  /** The server default (payday moved to the previous weekday) or the slip's own date on regenerate. */
  defaultPayDate: string;
  title?: string;
  onGenerated?: (result: GeneratePayslipsResponse) => void;
}) {
  const queryClient = useQueryClient();
  const { min, max } = payDateBounds(year, month);
  const form = useForm<GenerateFormValues>({
    resolver: zodResolver(buildSchema(year, month)),
    defaultValues: { pay_date: defaultPayDate },
  });
  const { reset } = form;

  useEffect(() => {
    if (open) {
      reset({ pay_date: defaultPayDate });
    }
  }, [defaultPayDate, open, reset]);

  const payDate = useWatch({ control: form.control, name: "pay_date" });
  const regenerateCount = targets.filter((target) => target.status === "draft").length;

  const mutation = useMutation({
    mutationFn: (values: GenerateFormValues) =>
      generatePayslips({
        year,
        month,
        employee_ids: targets.map((target) => target.employee_id),
        pay_date: values.pay_date,
      }),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: payslipsKeys.all });
      const generated = result.generated.length;
      if (result.skipped.length > 0) {
        toast.warning(
          `${generated} draf dibuat, ${result.skipped.length} dilewati`,
          result.skipped
            .slice(0, 4)
            .map((item) => `${item.employee_name || item.doc_number || "Slip"}: ${item.reason}`)
            .join(" · "),
        );
      } else {
        toast.success(`${generated} draf slip gaji dibuat`, "PDF dibuat di latar belakang; status akan diperbarui otomatis.");
      }
      onGenerated?.(result);
      onClose();
    },
    onError: (error) => {
      toast.error("Gagal membuat draf slip gaji", error instanceof Error ? error.message : undefined);
    },
  });

  return (
    <FormModal
      isLoading={mutation.isPending}
      isOpen={open}
      onClose={onClose}
      onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
      size="md"
      submitDisabled={targets.length === 0}
      submitLabel={`Generate (${targets.length})`}
      subtitle={`Periode ${formatPayslipPeriod(year, month)}`}
      title={title}
    >
      <div className="space-y-2">
        <label className="block text-sm font-medium text-text-primary" htmlFor="payslip-pay-date">
          Tanggal bayar<span className="ml-0.5 text-priority-high">*</span>
        </label>
        <Input id="payslip-pay-date" max={max} min={min} type="date" {...form.register("pay_date")} />
        {form.formState.errors.pay_date?.message ? (
          <p className="text-[12px] font-[500] text-priority-high">{form.formState.errors.pay_date.message}</p>
        ) : (
          <p className="text-xs text-text-secondary">
            {formatLongDate(payDate)}. Bawaan: tanggal gajian perusahaan, dimundurkan ke hari kerja sebelumnya bila jatuh di akhir pekan.
          </p>
        )}
      </div>

      <div className="rounded-xl border border-border/70 bg-surface-muted/60 p-4 text-sm text-text-secondary">
        <p className="font-semibold text-text-primary">
          {targets.length} karyawan
          {regenerateCount > 0 ? ` (${regenerateCount} draf dibuat ulang)` : ""}
        </p>
        <p className="mt-1 max-h-24 overflow-y-auto text-[13px] leading-5">
          {targets.map((target) => target.employee_name).join(", ")}
        </p>
        <p className="mt-3 text-[13px] leading-5">
          Data gaji, bonus yang disetujui, dan reimbursement yang sudah dibayar diambil saat ini. Draf yang sudah ada
          dibuat ulang dengan catatan dan baris penyesuaiannya tetap. Slip yang sudah terkirim tidak diubah.
        </p>
      </div>
    </FormModal>
  );
}

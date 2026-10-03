import { useEffect } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Save, Trash2 } from "lucide-react";
import { Controller, useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { payslipsKeys, updatePayslip } from "@/services/hris-payslips";
import { toast } from "@/stores/toast-store";
import type { PayslipDetail, PayslipManualLine } from "@/types/documents";

// Limits mirrored from the backend: the 08 template only fits one A4 page
// with a single-line catatan and a handful of extra rows.
export const PAYSLIP_NOTE_MAX = 120;
export const PAYSLIP_MANUAL_LINES_MAX = 5;
const LABEL_MAX = 60;
const KETERANGAN_MAX = 60;

const manualLineSchema = z.object({
  label: z.string().trim().min(1, "Label wajib diisi").max(LABEL_MAX, `Maksimal ${LABEL_MAX} karakter`),
  direction: z.enum(["earning", "deduction"]),
  amount: z.number().int().min(1, "Nominal wajib diisi"),
  keterangan: z.string().trim().max(KETERANGAN_MAX, `Maksimal ${KETERANGAN_MAX} karakter`),
});

const editSchema = z.object({
  note: z
    .string()
    .max(PAYSLIP_NOTE_MAX, `Maksimal ${PAYSLIP_NOTE_MAX} karakter`)
    .refine((value) => !/[\r\n]/.test(value), "Catatan hanya satu baris"),
  manual_lines: z
    .array(manualLineSchema)
    .max(PAYSLIP_MANUAL_LINES_MAX, `Maksimal ${PAYSLIP_MANUAL_LINES_MAX} baris penyesuaian`),
});

type EditFormValues = z.infer<typeof editSchema>;

function toFormValues(detail: PayslipDetail): EditFormValues {
  return {
    note: detail.note ?? "",
    manual_lines: (detail.manual_lines ?? []).map((line: PayslipManualLine) => ({
      label: line.label,
      direction: line.amount < 0 ? "deduction" : "earning",
      amount: Math.abs(line.amount),
      keterangan: line.keterangan ?? "",
    })),
  };
}

/**
 * Catatan (one short line) and manual adjustment lines of a draft slip. A
 * positive line is printed in the gaji table, a negative one in the potongan
 * table. Saving re-renders the PDF; regenerating the draft keeps both.
 */
export function PayslipEditForm({ detail }: { detail: PayslipDetail }) {
  const queryClient = useQueryClient();
  const form = useForm<EditFormValues>({
    resolver: zodResolver(editSchema),
    defaultValues: toFormValues(detail),
  });
  const {
    control,
    formState: { errors, isDirty },
    handleSubmit,
    register,
    reset,
  } = form;
  const { fields, append, remove } = useFieldArray({ control, name: "manual_lines" });

  // Pick up server-side changes (e.g. a regenerate) unless HR is mid-edit.
  useEffect(() => {
    if (!isDirty) {
      reset(toFormValues(detail));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detail.id, detail.updated_at]);

  const mutation = useMutation({
    mutationFn: (values: EditFormValues) =>
      updatePayslip(detail.id, {
        note: values.note.trim(),
        manual_lines: values.manual_lines.map((line) => ({
          label: line.label.trim(),
          amount: line.direction === "deduction" ? -line.amount : line.amount,
          keterangan: line.keterangan.trim(),
        })),
      }),
    onSuccess: async (saved) => {
      reset(toFormValues(saved));
      await queryClient.invalidateQueries({ queryKey: payslipsKeys.all });
      toast.success("Draf slip diperbarui", "PDF dibuat ulang di latar belakang.");
    },
    onError: (error) => {
      toast.error("Gagal menyimpan draf", error instanceof Error ? error.message : undefined);
    },
  });

  return (
    <form
      className="space-y-4"
      data-testid="payslip-edit-form"
      noValidate
      onSubmit={handleSubmit((values) => mutation.mutate(values))}
    >
      <div className="space-y-1.5">
        <label className="text-sm font-medium text-text-primary" htmlFor="payslip-note">
          Catatan slip
        </label>
        <Input
          id="payslip-note"
          maxLength={PAYSLIP_NOTE_MAX}
          placeholder="mis. Termasuk penyesuaian pro-rata bulan pertama"
          {...register("note")}
        />
        {errors.note?.message ? (
          <p className="text-[12px] font-[500] text-priority-high">{errors.note.message}</p>
        ) : (
          <p className="text-xs text-text-secondary">Satu baris, maksimal {PAYSLIP_NOTE_MAX} karakter (slip harus muat satu halaman).</p>
        )}
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <p className="text-sm font-medium text-text-primary">Baris penyesuaian</p>
          <Button
            disabled={fields.length >= PAYSLIP_MANUAL_LINES_MAX}
            onClick={() => append({ label: "", direction: "deduction", amount: 0, keterangan: "" })}
            size="xs"
            type="button"
            variant="outline"
          >
            <Plus className="h-4 w-4" />
            Tambah baris
          </Button>
        </div>
        {fields.length === 0 ? (
          <p className="rounded-xl bg-surface-muted px-3 py-2 text-[13px] text-text-secondary">
            Belum ada penyesuaian. Gunakan untuk pro-rata, koreksi, atau potongan lain.
          </p>
        ) : (
          <div className="space-y-3">
            {fields.map((field, index) => {
              const lineErrors = errors.manual_lines?.[index];
              return (
                <div className="space-y-2 rounded-xl border border-border/70 bg-surface-muted/40 p-3" data-testid="manual-line" key={field.id}>
                  <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_150px]">
                    <div>
                      <Input
                        aria-label={`Label baris ${index + 1}`}
                        maxLength={LABEL_MAX}
                        placeholder="Label, mis. Penyesuaian pro-rata"
                        {...register(`manual_lines.${index}.label`)}
                      />
                      {lineErrors?.label?.message ? (
                        <p className="mt-1 text-[12px] text-priority-high">{lineErrors.label.message}</p>
                      ) : null}
                    </div>
                    <select
                      aria-label={`Jenis baris ${index + 1}`}
                      className="field-select"
                      {...register(`manual_lines.${index}.direction`)}
                    >
                      <option value="deduction">Potongan (−)</option>
                      <option value="earning">Pendapatan (+)</option>
                    </select>
                  </div>
                  <div className="grid gap-2 sm:grid-cols-[180px_minmax(0,1fr)_auto]">
                    <div>
                      <Controller
                        control={control}
                        name={`manual_lines.${index}.amount`}
                        render={({ field: amountField }) => (
                          <CurrencyInput
                            aria-label={`Nominal baris ${index + 1}`}
                            onBlur={amountField.onBlur}
                            onValueChange={amountField.onChange}
                            ref={amountField.ref}
                            value={amountField.value}
                          />
                        )}
                      />
                      {lineErrors?.amount?.message ? (
                        <p className="mt-1 text-[12px] text-priority-high">{lineErrors.amount.message}</p>
                      ) : null}
                    </div>
                    <div>
                      <Input
                        aria-label={`Keterangan baris ${index + 1}`}
                        className="h-10"
                        maxLength={KETERANGAN_MAX}
                        placeholder="Keterangan (opsional)"
                        {...register(`manual_lines.${index}.keterangan`)}
                      />
                      {lineErrors?.keterangan?.message ? (
                        <p className="mt-1 text-[12px] text-priority-high">{lineErrors.keterangan.message}</p>
                      ) : null}
                    </div>
                    <Button
                      aria-label={`Hapus baris ${index + 1}`}
                      className="text-error hover:text-error"
                      onClick={() => remove(index)}
                      size="icon"
                      type="button"
                      variant="ghost"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
        {errors.manual_lines?.root?.message || errors.manual_lines?.message ? (
          <p className="text-[12px] text-priority-high">{errors.manual_lines?.root?.message ?? errors.manual_lines?.message}</p>
        ) : null}
      </div>

      <div className="flex justify-end gap-2">
        <Button
          disabled={!isDirty || mutation.isPending}
          onClick={() => reset(toFormValues(detail))}
          size="sm"
          type="button"
          variant="ghost"
        >
          Batalkan perubahan
        </Button>
        <Button disabled={!isDirty || mutation.isPending} size="sm" type="submit">
          <Save className="h-4 w-4" />
          {mutation.isPending ? "Menyimpan..." : "Simpan & render ulang"}
        </Button>
      </div>
    </form>
  );
}

import { useEffect, type ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { Employee, SubscriptionFormValues } from "@/types/hris";
import { cn } from "@/lib/utils";

const subscriptionSchema = z.object({
  name: z.string().min(2, "Nama langganan minimal 2 karakter"),
  vendor: z.string().min(2, "Vendor minimal 2 karakter"),
  description: z.string(),
  cost_amount: z.coerce.number().min(1, "Biaya wajib diisi"),
  cost_currency: z.string().length(3, "Currency harus 3 karakter"),
  billing_cycle: z.enum(["monthly", "quarterly", "yearly"]),
  start_date: z.string().min(1, "Tanggal mulai wajib diisi"),
  renewal_date: z.string().min(1, "Tanggal perpanjangan wajib diisi"),
  status: z.enum(["active", "cancelled", "expired"]),
  pic_employee_id: z.string(),
  category: z.string().min(2, "Kategori wajib diisi"),
  login_credentials: z.string(),
  notes: z.string(),
});

const baseValues: SubscriptionFormValues = {
  name: "",
  vendor: "",
  description: "",
  cost_amount: 0,
  cost_currency: "IDR",
  billing_cycle: "monthly",
  start_date: "",
  renewal_date: "",
  status: "active",
  pic_employee_id: "",
  category: "",
  login_credentials: "",
  notes: "",
};

interface SubscriptionFormProps {
  isOpen: boolean;
  employees: Employee[];
  defaultValues?: SubscriptionFormValues;
  title: string;
  description: string;
  submitLabel: string;
  isSubmitting: boolean;
  onCancel?: () => void;
  onSubmit: (values: SubscriptionFormValues) => void;
}

export function SubscriptionForm({
  isOpen,
  employees,
  defaultValues,
  title,
  description,
  submitLabel,
  isSubmitting,
  onCancel,
  onSubmit,
}: SubscriptionFormProps) {
  const {
    control,
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<SubscriptionFormValues>({
    resolver: zodResolver(subscriptionSchema) as never,
    defaultValues: defaultValues ?? baseValues,
  });

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-hr focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-hr/10 disabled:cursor-not-allowed disabled:opacity-50";
  const textareaClass = cn(formControlClass, "h-auto min-h-[96px] py-3");
  const billingCycleOptions = [
    { value: "monthly", label: "Monthly" },
    { value: "quarterly", label: "Quarterly" },
    { value: "yearly", label: "Yearly" },
  ];
  const statusOptions = [
    { value: "active", label: "Active" },
    { value: "cancelled", label: "Cancelled" },
    { value: "expired", label: "Expired" },
  ];
  const employeeOptions = [
    { value: "", label: "Belum ditentukan" },
    ...employees.map((employee) => ({
      value: employee.id,
      label: employee.full_name,
      description: employee.position,
    })),
  ];

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    reset(defaultValues ?? baseValues);
  }, [defaultValues, isOpen, reset]);

  return (
    <FormModal
      isLoading={isSubmitting}
      isOpen={isOpen}
      onClose={onCancel ?? (() => undefined)}
      onSubmit={handleSubmit(onSubmit)}
      size="lg"
      submitLabel={submitLabel}
      title={title}
      subtitle={description}
    >
        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.name?.message} label="Nama" required>
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("name")} placeholder="Notion" />
          </Field>
          <Field error={errors.vendor?.message} label="Vendor" required>
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("vendor")} placeholder="Notion Labs" />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-3">
          <Field error={errors.cost_amount?.message} label="Biaya" required>
            <Controller
              control={control}
              name="cost_amount"
              render={({ field }) => (
                <CurrencyInput
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  ref={field.ref}
                  value={field.value}
                />
              )}
            />
          </Field>
          <Field error={errors.cost_currency?.message} label="Mata uang">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("cost_currency")} />
          </Field>
          <Field error={errors.category?.message} label="Kategori" required>
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("category")} placeholder="Project management" />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-3">
          <Field error={errors.billing_cycle?.message} label="Siklus tagihan">
            <Controller
              control={control}
              name="billing_cycle"
              render={({ field }) => (
                <Select
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  options={billingCycleOptions}
                  triggerClassName="focus-visible:border-hr focus-visible:ring-hr/10"
                  value={field.value}
                />
              )}
            />
          </Field>
          <Field error={errors.start_date?.message} label="Tanggal mulai" required>
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("start_date")} type="date" />
          </Field>
          <Field error={errors.renewal_date?.message} label="Tanggal perpanjangan" required>
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("renewal_date")} type="date" />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.status?.message} label="Status">
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  options={statusOptions}
                  triggerClassName="focus-visible:border-hr focus-visible:ring-hr/10"
                  value={field.value}
                />
              )}
            />
          </Field>
          <Field error={errors.pic_employee_id?.message} label="PIC karyawan">
            <Controller
              control={control}
              name="pic_employee_id"
              render={({ field }) => (
                <Select
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  options={employeeOptions}
                  triggerClassName="focus-visible:border-hr focus-visible:ring-hr/10"
                  value={field.value}
                />
              )}
            />
          </Field>
        </div>

        <Field error={errors.description?.message} label="Deskripsi">
          <textarea
            className={textareaClass}
            {...register("description")}
          />
        </Field>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.login_credentials?.message} label="Kredensial login (terenkripsi)">
            <textarea
               className={cn(textareaClass, "font-mono text-[13px]")}
              {...register("login_credentials")}
              placeholder="email: ops@company.com | password: ********"
            />
          </Field>
          <Field error={errors.notes?.message} label="Catatan">
            <textarea
              className={textareaClass}
              {...register("notes")}
            />
          </Field>
        </div>
    </FormModal>
  );
}

function Field({
  label,
  error,
  required,
  children,
}: {
  label: string;
  error?: string;
  required?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1.5 flex flex-col">
      <label className="text-[13px] font-[500] text-text-secondary">
        {label}
        {required ? <span className="ml-0.5 text-priority-high">*</span> : null}
      </label>
      {children}
      {error ? <p className="text-[12px] text-priority-high mt-1 font-[500]">{error}</p> : null}
    </div>
  );
}

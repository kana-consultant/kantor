import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import type { Employee, SubscriptionFormValues } from "@/types/hris";
import { cn } from "@/lib/utils";

const subscriptionSchema = z.object({
  name: z.string().min(2, "Nama subscription minimal 2 karakter"),
  vendor: z.string().min(2, "Vendor wajib diisi"),
  description: z.string(),
  cost_amount: z.coerce.number().min(0, "Biaya minimal 0"),
  cost_currency: z.string().length(3, "Currency harus 3 karakter"),
  billing_cycle: z.enum(["monthly", "quarterly", "yearly"]),
  start_date: z.string().min(1, "Start date wajib diisi"),
  renewal_date: z.string().min(1, "Renewal date wajib diisi"),
  status: z.enum(["active", "cancelled", "expired"]),
  pic_employee_id: z.string(),
  category: z.string().min(2, "Category wajib diisi"),
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
    formState: { errors },
  } = useForm<SubscriptionFormValues>({
    resolver: zodResolver(subscriptionSchema) as any,
    defaultValues: defaultValues ?? baseValues,
  });

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-hr focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-hr/10 disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <Card className="p-6">
      <div className="mb-6">
        <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-hr mb-1">Subscription Form</p>
        <h3 className="text-[20px] font-[700] text-text-primary leading-tight">{title}</h3>
        <p className="mt-1 text-[13px] text-text-secondary">{description}</p>
      </div>

      <form className="space-y-5" onSubmit={handleSubmit(onSubmit)}>
        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.name?.message} label="Name">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("name")} placeholder="Notion" />
          </Field>
          <Field error={errors.vendor?.message} label="Vendor">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("vendor")} placeholder="Notion Labs" />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-3">
          <Field error={errors.cost_amount?.message} label="Cost amount">
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
          <Field error={errors.cost_currency?.message} label="Currency">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("cost_currency")} />
          </Field>
          <Field error={errors.category?.message} label="Category">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("category")} placeholder="Project management" />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-3">
          <Field error={errors.billing_cycle?.message} label="Billing cycle">
            <select
              className={formControlClass}
              {...register("billing_cycle")}
            >
              <option value="monthly">Monthly</option>
              <option value="quarterly">Quarterly</option>
              <option value="yearly">Yearly</option>
            </select>
          </Field>
          <Field error={errors.start_date?.message} label="Start date">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("start_date")} type="date" />
          </Field>
          <Field error={errors.renewal_date?.message} label="Renewal date">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("renewal_date")} type="date" />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.status?.message} label="Status">
            <select
              className={formControlClass}
              {...register("status")}
            >
              <option value="active">Active</option>
              <option value="cancelled">Cancelled</option>
              <option value="expired">Expired</option>
            </select>
          </Field>
          <Field error={errors.pic_employee_id?.message} label="PIC employee">
            <select
              className={formControlClass}
              {...register("pic_employee_id")}
            >
              <option value="">Belum ditentukan</option>
              {employees.map((employee) => (
                <option key={employee.id} value={employee.id}>
                  {employee.full_name} - {employee.position}
                </option>
              ))}
            </select>
          </Field>
        </div>

        <Field error={errors.description?.message} label="Description">
          <textarea
            className={cn(formControlClass, "min-h-[96px] py-3")}
            {...register("description")}
          />
        </Field>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.login_credentials?.message} label="Encrypted login credentials">
            <textarea
               className={cn(formControlClass, "min-h-[96px] py-3 font-mono text-[13px]")}
              {...register("login_credentials")}
              placeholder="email: ops@company.com | password: ********"
            />
          </Field>
          <Field error={errors.notes?.message} label="Notes">
            <textarea
              className={cn(formControlClass, "min-h-[96px] py-3")}
              {...register("notes")}
            />
          </Field>
        </div>

        <div className="flex flex-wrap gap-3 pt-2">
          <Button variant="hr" disabled={isSubmitting} type="submit">
            {isSubmitting ? "Saving..." : submitLabel}
          </Button>
          {onCancel ? (
            <Button onClick={onCancel} type="button" variant="ghost">
              Cancel
            </Button>
          ) : null}
        </div>
      </form>
    </Card>
  );
}

function Field({
  label,
  error,
  children,
}: {
  label: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1.5 flex flex-col">
      <label className="text-[13px] font-[500] text-text-secondary">{label}</label>
      {children}
      {error ? <p className="text-[12px] text-priority-high mt-1 font-[500]">{error}</p> : null}
    </div>
  );
}

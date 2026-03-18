import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { DepartmentFormValues, Employee } from "@/types/hris";
import { cn } from "@/lib/utils";

const departmentFormSchema = z.object({
  name: z.string().min(2, "Nama department minimal 2 karakter"),
  description: z.string(),
  head_id: z.string(),
});

const baseValues: DepartmentFormValues = {
  name: "",
  description: "",
  head_id: "",
};

interface DepartmentFormProps {
  employees: Employee[];
  defaultValues?: DepartmentFormValues;
  title: string;
  description: string;
  submitLabel: string;
  isSubmitting: boolean;
  onCancel?: () => void;
  onSubmit: (values: DepartmentFormValues) => void;
}

export function DepartmentForm({
  employees,
  defaultValues,
  title,
  description,
  submitLabel,
  isSubmitting,
  onCancel,
  onSubmit,
}: DepartmentFormProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<DepartmentFormValues>({
    resolver: zodResolver(departmentFormSchema),
    defaultValues: defaultValues ?? baseValues,
  });

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-hr focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-hr/10 disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <Card className="p-6">
      <div className="mb-6">
        <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-hr mb-1">Department Form</p>
        <h3 className="text-[20px] font-[700] text-text-primary leading-tight">{title}</h3>
        <p className="mt-1 text-[13px] text-text-secondary">{description}</p>
      </div>

      <form className="space-y-5" onSubmit={handleSubmit(onSubmit)}>
        <Field error={errors.name?.message} label="Nama department">
          <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("name")} placeholder="Engineering" />
        </Field>

        <Field error={errors.description?.message} label="Deskripsi">
          <textarea
            className={cn(formControlClass, "min-h-[96px] py-3")}
            {...register("description")}
            placeholder="Ringkasan fungsi team atau ruang lingkup department."
          />
        </Field>

        <Field error={errors.head_id?.message} label="Department head">
          <select
            className={formControlClass}
            {...register("head_id")}
          >
            <option value="">Belum ditentukan</option>
            {employees.map((employee) => (
              <option key={employee.id} value={employee.id}>
                {employee.full_name} - {employee.position}
              </option>
            ))}
          </select>
        </Field>

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

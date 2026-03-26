import { useEffect, useMemo } from "react";
import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
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
  isOpen: boolean;
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
  isOpen,
  employees,
  defaultValues,
  title,
  description,
  submitLabel,
  isSubmitting,
  onCancel,
  onSubmit,
}: DepartmentFormProps) {
  const normalizedDefaultValues = useMemo<DepartmentFormValues>(
    () => ({
      name: defaultValues?.name ?? baseValues.name,
      description: defaultValues?.description ?? baseValues.description,
      head_id: defaultValues?.head_id ?? baseValues.head_id,
    }),
    [defaultValues?.name, defaultValues?.description, defaultValues?.head_id],
  );

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<DepartmentFormValues>({
    resolver: zodResolver(departmentFormSchema),
    defaultValues: normalizedDefaultValues,
  });

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    reset(normalizedDefaultValues);
  }, [isOpen, normalizedDefaultValues, reset]);

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-hr focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-hr/10 disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <FormModal
      isLoading={isSubmitting}
      isOpen={isOpen}
      onClose={onCancel ?? (() => undefined)}
      onSubmit={handleSubmit(onSubmit)}
      size="md"
      submitLabel={submitLabel}
      title={title}
      subtitle={description}
    >
        <Field error={errors.name?.message} label="Nama department" required>
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

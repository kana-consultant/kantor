import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { DepartmentFormValues, Employee } from "@/types/hris";

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

  return (
    <Card className="p-6">
      <div className="mb-6">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">Department form</p>
        <h3 className="mt-2 text-2xl font-bold">{title}</h3>
        <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      </div>

      <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
        <Field error={errors.name?.message} label="Nama department">
          <Input {...register("name")} placeholder="Engineering" />
        </Field>

        <Field error={errors.description?.message} label="Deskripsi">
          <textarea
            className="min-h-24 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
            {...register("description")}
            placeholder="Ringkasan fungsi team atau ruang lingkup department."
          />
        </Field>

        <Field error={errors.head_id?.message} label="Department head">
          <select
            className="flex h-12 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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

        <div className="flex flex-wrap gap-3">
          <Button disabled={isSubmitting} type="submit">
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
    <div className="space-y-2">
      <label className="text-sm font-medium">{label}</label>
      {children}
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
    </div>
  );
}

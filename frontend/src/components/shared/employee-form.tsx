import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { Input } from "@/components/ui/input";
import type { Department, EmployeeFormValues } from "@/types/hris";
import { cn } from "@/lib/utils";

const employeeFormSchema = z.object({
  full_name: z.string().min(3, "Nama minimal 3 karakter"),
  email: z.string().email("Email tidak valid"),
  phone: z.string(),
  position: z.string().min(2, "Posisi wajib diisi"),
  department: z.string(),
  date_joined: z.string().min(1, "Tanggal join wajib diisi"),
  employment_status: z.enum(["active", "probation", "resigned", "terminated"]),
  address: z.string(),
  emergency_contact: z.string(),
  avatar_url: z.string(),
});

const baseValues: EmployeeFormValues = {
  full_name: "",
  email: "",
  phone: "",
  position: "",
  department: "",
  date_joined: "",
  employment_status: "active",
  address: "",
  emergency_contact: "",
  avatar_url: "",
};

interface EmployeeFormProps {
  isOpen: boolean;
  defaultValues?: EmployeeFormValues;
  departments: Department[];
  title: string;
  description: string;
  submitLabel: string;
  isSubmitting: boolean;
  onCancel?: () => void;
  onSubmit: (values: EmployeeFormValues) => void;
}

export function EmployeeForm({
  isOpen,
  defaultValues,
  departments,
  title,
  description,
  submitLabel,
  isSubmitting,
  onCancel,
  onSubmit,
}: EmployeeFormProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<EmployeeFormValues>({
    resolver: zodResolver(employeeFormSchema),
    defaultValues: defaultValues ?? baseValues,
  });

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-hr focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-hr/10 disabled:cursor-not-allowed disabled:opacity-50";

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
          <Field error={errors.full_name?.message} label="Nama lengkap">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("full_name")} placeholder="Safri Ahmad" />
          </Field>
          <Field error={errors.email?.message} label="Email">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("email")} placeholder="staff@kantor.local" type="email" />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-3">
          <Field error={errors.position?.message} label="Posisi">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("position")} placeholder="Backend Engineer" />
          </Field>
          <Field error={errors.department?.message} label="Department">
            <select
              className={formControlClass}
              {...register("department")}
            >
              <option value="">Pilih department</option>
              {departments.map((department) => (
                <option key={department.id} value={department.name}>
                  {department.name}
                </option>
              ))}
            </select>
          </Field>
          <Field error={errors.employment_status?.message} label="Status">
            <select
              className={formControlClass}
              {...register("employment_status")}
            >
              <option value="active">Active</option>
              <option value="probation">Probation</option>
              <option value="resigned">Resigned</option>
              <option value="terminated">Terminated</option>
            </select>
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.phone?.message} label="Phone">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("phone")} placeholder="+62..." />
          </Field>
          <Field error={errors.date_joined?.message} label="Tanggal join">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("date_joined")} type="date" />
          </Field>
        </div>

        <Field error={errors.address?.message} label="Alamat">
          <textarea
            className={cn(formControlClass, "min-h-[96px] py-3")}
            {...register("address")}
            placeholder="Alamat tempat tinggal"
          />
        </Field>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.emergency_contact?.message} label="Emergency contact">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("emergency_contact")} placeholder="Nama - nomor telepon" />
          </Field>
          <Field error={errors.avatar_url?.message} label="Avatar URL">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("avatar_url")} placeholder="https://..." />
          </Field>
        </div>
    </FormModal>
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

import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { Department, EmployeeFormValues } from "@/types/hris";

const employeeFormSchema = z.object({
  user_id: z.string(),
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
  user_id: "",
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

  return (
    <Card className="p-6">
      <div className="mb-6">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">Employee form</p>
        <h3 className="mt-2 text-2xl font-bold">{title}</h3>
        <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      </div>

      <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
        <div className="grid gap-4 md:grid-cols-2">
          <Field error={errors.full_name?.message} label="Nama lengkap">
            <Input {...register("full_name")} placeholder="Safri Ahmad" />
          </Field>
          <Field error={errors.email?.message} label="Email">
            <Input {...register("email")} placeholder="staff@kantor.local" type="email" />
          </Field>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Field error={errors.position?.message} label="Posisi">
            <Input {...register("position")} placeholder="Backend Engineer" />
          </Field>
          <Field error={errors.department?.message} label="Department">
            <select
              className="flex h-12 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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
              className="flex h-12 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
              {...register("employment_status")}
            >
              <option value="active">Active</option>
              <option value="probation">Probation</option>
              <option value="resigned">Resigned</option>
              <option value="terminated">Terminated</option>
            </select>
          </Field>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Field error={errors.phone?.message} label="Phone">
            <Input {...register("phone")} placeholder="+62..." />
          </Field>
          <Field error={errors.date_joined?.message} label="Tanggal join">
            <Input {...register("date_joined")} type="date" />
          </Field>
          <Field error={errors.user_id?.message} label="User ID">
            <Input {...register("user_id")} placeholder="Kosongkan jika belum punya akses login" />
          </Field>
        </div>

        <Field error={errors.address?.message} label="Alamat">
          <textarea
            className="min-h-24 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
            {...register("address")}
            placeholder="Alamat tempat tinggal"
          />
        </Field>

        <div className="grid gap-4 md:grid-cols-2">
          <Field error={errors.emergency_contact?.message} label="Emergency contact">
            <Input {...register("emergency_contact")} placeholder="Nama - nomor telepon" />
          </Field>
          <Field error={errors.avatar_url?.message} label="Avatar URL">
            <Input {...register("avatar_url")} placeholder="https://..." />
          </Field>
        </div>

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

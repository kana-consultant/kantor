import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Department, EmployeeFormValues } from "@/types/hris";
import { cn } from "@/lib/utils";

const employeeFormSchema = z.object({
  full_name: z.string().min(3, "Nama minimal 3 karakter"),
  email: z.string().email("Email tidak valid"),
  phone: z.string(),
  position: z.string().min(1, "Role wajib dipilih"),
  department: z.string(),
  date_joined: z.string().min(1, "Tanggal join wajib diisi"),
  employment_status: z.enum(["active", "probation", "resigned", "terminated"]),
  address: z.string(),
  emergency_contact: z.string(),
  avatar_url: z.string(),
  bank_account_number: z.string(),
  bank_name: z.string(),
  linkedin_profile: z.string(),
  ssh_keys: z.string(),
});

const EMPLOYEE_ROLE_OPTIONS = [
  "Full Time",
  "Part Time",
  "Internship",
  "Project Based",
] as const;

function normalizeEmployeeRole(value?: string) {
  const trimmed = value?.trim() ?? "";
  if (EMPLOYEE_ROLE_OPTIONS.includes(trimmed as (typeof EMPLOYEE_ROLE_OPTIONS)[number])) {
    return trimmed;
  }
  return "";
}

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
  bank_account_number: "",
  bank_name: "",
  linkedin_profile: "",
  ssh_keys: "",
};

interface EmployeeFormProps {
  isOpen: boolean;
  defaultValues?: EmployeeFormValues;
  departments: Department[];
  existingAvatarPath?: string | null;
  avatarFile: File | null;
  title: string;
  description: string;
  submitLabel: string;
  isSubmitting: boolean;
  onCancel?: () => void;
  onAvatarFileChange: (file: File | null) => void;
  onSubmit: (values: EmployeeFormValues) => void;
}

export function EmployeeForm({
  isOpen,
  defaultValues,
  departments,
  existingAvatarPath,
  avatarFile,
  title,
  description,
  submitLabel,
  isSubmitting,
  onCancel,
  onAvatarFileChange,
  onSubmit,
}: EmployeeFormProps) {
  const normalizedDefaultValues = useMemo<EmployeeFormValues>(
    () => ({
      full_name: defaultValues?.full_name ?? baseValues.full_name,
      email: defaultValues?.email ?? baseValues.email,
      phone: defaultValues?.phone ?? baseValues.phone,
      position: normalizeEmployeeRole(defaultValues?.position),
      department: defaultValues?.department ?? baseValues.department,
      date_joined: defaultValues?.date_joined ?? baseValues.date_joined,
      employment_status: defaultValues?.employment_status ?? baseValues.employment_status,
      address: defaultValues?.address ?? baseValues.address,
      emergency_contact: defaultValues?.emergency_contact ?? baseValues.emergency_contact,
      avatar_url: defaultValues?.avatar_url ?? baseValues.avatar_url,
      bank_account_number: defaultValues?.bank_account_number ?? baseValues.bank_account_number,
      bank_name: defaultValues?.bank_name ?? baseValues.bank_name,
      linkedin_profile: defaultValues?.linkedin_profile ?? baseValues.linkedin_profile,
      ssh_keys: defaultValues?.ssh_keys ?? baseValues.ssh_keys,
    }),
    [
      defaultValues?.full_name,
      defaultValues?.email,
      defaultValues?.phone,
      defaultValues?.position,
      defaultValues?.department,
      defaultValues?.date_joined,
      defaultValues?.employment_status,
      defaultValues?.address,
      defaultValues?.emergency_contact,
      defaultValues?.avatar_url,
      defaultValues?.bank_account_number,
      defaultValues?.bank_name,
      defaultValues?.linkedin_profile,
      defaultValues?.ssh_keys,
    ],
  );

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<EmployeeFormValues>({
    resolver: zodResolver(employeeFormSchema),
    defaultValues: normalizedDefaultValues,
  });

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    reset(normalizedDefaultValues);
  }, [isOpen, normalizedDefaultValues, reset]);

  const [selectedAvatarPreview, setSelectedAvatarPreview] = useState<string | null>(null);

  useEffect(() => {
    if (!avatarFile) {
      setSelectedAvatarPreview(null);
      return;
    }

    const objectUrl = URL.createObjectURL(avatarFile);
    setSelectedAvatarPreview(objectUrl);

    return () => {
      URL.revokeObjectURL(objectUrl);
    };
  }, [avatarFile]);

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-hr focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-hr/10 disabled:cursor-not-allowed disabled:opacity-50";
  const avatarInputKey = avatarFile?.name ?? existingAvatarPath ?? "empty-avatar";

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
          <Field error={errors.position?.message} label="Role">
            <select className={formControlClass} {...register("position")}>
              <option value="">Pilih role</option>
              {EMPLOYEE_ROLE_OPTIONS.map((roleOption) => (
                <option key={roleOption} value={roleOption}>
                  {roleOption}
                </option>
              ))}
            </select>
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
          <Field label="Avatar">
            <div className="space-y-2">
              <div className="flex items-center gap-3 rounded-[12px] border border-border/70 bg-background/70 p-3">
                <ProtectedAvatar
                  alt={defaultValues?.full_name || "Employee avatar"}
                  avatarUrl={existingAvatarPath}
                  className="h-16 w-16 border border-border/70"
                  srcOverride={selectedAvatarPreview}
                />
                <div className="space-y-1 text-[12px] text-text-secondary">
                  <p className="font-medium text-text-primary">
                    {selectedAvatarPreview ? "Preview avatar baru" : existingAvatarPath ? "Avatar saat ini" : "Belum ada avatar"}
                  </p>
                  <p>Gunakan gambar JPG, PNG, atau WebP dengan ukuran maksimal 5 MB.</p>
                </div>
              </div>
              <input
                accept="image/*"
                className={cn(
                  formControlClass,
                  "px-3 py-2 file:mr-3 file:rounded-md file:border-0 file:bg-hr/10 file:px-3 file:py-2 file:text-[13px] file:font-medium file:text-hr",
                )}
                key={avatarInputKey}
                onChange={(event) => onAvatarFileChange(event.target.files?.[0] ?? null)}
                type="file"
              />
              <div className="flex flex-wrap items-center gap-2 text-[12px] text-text-secondary">
                <span>{avatarFile ? "Avatar baru siap di-upload saat disimpan." : "Jika tidak memilih file baru, avatar yang ada akan tetap dipakai."}</span>
                {avatarFile ? (
                  <Button onClick={() => onAvatarFileChange(null)} size="sm" type="button" variant="ghost">
                    Hapus pilihan
                  </Button>
                ) : null}
              </div>
            </div>
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.bank_account_number?.message} label="Nomor Rekening">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("bank_account_number")} placeholder="Nomor rekening atau akun e-wallet" />
          </Field>
          <Field error={errors.bank_name?.message} label="Bank / E-Wallet">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("bank_name")} placeholder="BCA, BRI, OVO, GoPay, DANA" />
          </Field>
        </div>

        <Field error={errors.linkedin_profile?.message} label="LinkedIn Profile">
          <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("linkedin_profile")} placeholder="https://linkedin.com/in/username" />
        </Field>

        <Field error={errors.ssh_keys?.message} label="SSH Keys">
          <textarea
            className={cn(formControlClass, "min-h-[120px] py-3 font-mono text-[13px]")}
            {...register("ssh_keys")}
            placeholder="Tempel public key SSH. Jika lebih dari satu, pisahkan per baris."
          />
        </Field>
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

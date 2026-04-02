import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { Department, EmployeeFormValues } from "@/types/hris";
import { cn } from "@/lib/utils";

const MAX_AVATAR_SIZE_BYTES = 5 * 1024 * 1024;
const ACCEPTED_AVATAR_TYPES = new Set(["image/jpeg", "image/png", "image/webp"]);

const employeeFormSchema = z.object({
  full_name: z.string().min(3, "Nama minimal 3 karakter"),
  email: z.string().email("Email tidak valid"),
  phone: z.string().refine((value) => value.trim() === "" || isValidEmployeePhone(value), {
    message: "Format telepon harus 08xx, 8xx, 628xx, atau +628xx",
  }),
  position: z.string().min(1, "Tipe kepegawaian wajib dipilih"),
  department: z.string(),
  date_joined: z.string().min(1, "Tanggal join wajib diisi"),
  employment_status: z.enum(["active", "probation", "resigned", "terminated"]),
  address: z.string(),
  emergency_contact: z.string(),
  avatar_url: z.string(),
  bank_account_number: z.string(),
  bank_name: z.string(),
  linkedin_profile: z.string().refine((value) => value.trim() === "" || isValidLinkedInProfile(value), {
    message: "URL LinkedIn tidak valid",
  }),
  ssh_keys: z.string(),
});

const EMPLOYEE_ROLE_OPTIONS = [
  "Full Time",
  "Part Time",
  "Internship",
  "Project Based",
  "Outsourcing",
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
  const [selectedAvatarPreview, setSelectedAvatarPreview] = useState<string | null>(null);
  const [avatarError, setAvatarError] = useState<string | null>(null);
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
    control,
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
      setAvatarError(null);
      return;
    }

    reset(normalizedDefaultValues);
  }, [isOpen, normalizedDefaultValues, reset]);

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
  const richTextareaClass = cn(formControlClass, "h-auto min-h-[96px] py-3");
  const employeeRoleOptions = [
    { value: "", label: "Pilih role" },
    ...EMPLOYEE_ROLE_OPTIONS.map((roleOption) => ({
      value: roleOption,
      label: roleOption,
    })),
  ];
  const departmentOptions = [
    { value: "", label: "Pilih departemen" },
    ...departments.map((department) => ({
      value: department.name,
      label: department.name,
    })),
  ];
  const employmentStatusOptions = [
    { value: "active", label: "Aktif" },
    { value: "probation", label: "Probation" },
    { value: "resigned", label: "Resign" },
    { value: "terminated", label: "Diberhentikan" },
  ];
  const avatarInputKey = avatarFile?.name ?? existingAvatarPath ?? "empty-avatar";
  const handleFormSubmit = handleSubmit((values) => {
    if (avatarError) {
      return;
    }
    onSubmit(values);
  });
  const handleAvatarChange = (file: File | null) => {
    const nextError = validateAvatarFile(file);
    setAvatarError(nextError);
    if (nextError) {
      onAvatarFileChange(null);
      return;
    }
    onAvatarFileChange(file);
  };

  return (
    <FormModal
      isLoading={isSubmitting}
      isOpen={isOpen}
      onClose={onCancel ?? (() => undefined)}
      onSubmit={handleFormSubmit}
      size="lg"
      submitLabel={submitLabel}
      title={title}
      subtitle={description}
    >
        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.full_name?.message} label="Nama lengkap" required>
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("full_name")} placeholder="Safri Ahmad" />
          </Field>
          <Field error={errors.email?.message} label="Email" required>
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("email")} placeholder="staff@kantor.local" type="email" />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-3">
          <Field error={errors.position?.message} label="Tipe kepegawaian" required>
            <Controller
              control={control}
              name="position"
              render={({ field }) => (
                <Select
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  options={employeeRoleOptions}
                  triggerClassName="focus-visible:border-hr focus-visible:ring-hr/10"
                  value={field.value}
                />
              )}
            />
          </Field>
          <Field error={errors.department?.message} label="Departemen">
            <Controller
              control={control}
              name="department"
              render={({ field }) => (
                <Select
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  options={departmentOptions}
                  triggerClassName="focus-visible:border-hr focus-visible:ring-hr/10"
                  value={field.value}
                />
              )}
            />
          </Field>
          <Field error={errors.employment_status?.message} label="Status kepegawaian">
            <Controller
              control={control}
              name="employment_status"
              render={({ field }) => (
                <Select
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  options={employmentStatusOptions}
                  triggerClassName="focus-visible:border-hr focus-visible:ring-hr/10"
                  value={field.value}
                />
              )}
            />
          </Field>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.phone?.message} label="Telepon">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("phone")} placeholder="+62..." />
          </Field>
          <Field error={errors.date_joined?.message} label="Tanggal join" required>
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("date_joined")} type="date" />
          </Field>
        </div>

        <Field error={errors.address?.message} label="Alamat">
          <textarea
            className={richTextareaClass}
            {...register("address")}
            placeholder="Alamat tempat tinggal"
          />
        </Field>

        <div className="grid gap-5 md:grid-cols-2">
          <Field error={errors.emergency_contact?.message} label="Kontak darurat">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("emergency_contact")} placeholder="Nama - nomor telepon" />
          </Field>
          <Field error={avatarError ?? undefined} label="Foto profil">
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
                accept="image/jpeg,image/png,image/webp"
                className={cn(
                  formControlClass,
                  "px-3 py-2 file:mr-3 file:rounded-md file:border-0 file:bg-hr/10 file:px-3 file:py-2 file:text-[13px] file:font-medium file:text-hr",
                )}
                key={avatarInputKey}
                onChange={(event) => handleAvatarChange(event.target.files?.[0] ?? null)}
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
          <Field error={errors.bank_name?.message} label="Nama bank / E-Wallet">
            <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("bank_name")} placeholder="BCA, BRI, OVO, GoPay, DANA" />
          </Field>
        </div>

        <Field error={errors.linkedin_profile?.message} label="Profil LinkedIn">
          <Input className="focus-visible:border-hr focus-visible:ring-hr/10" {...register("linkedin_profile")} placeholder="https://linkedin.com/in/username" />
        </Field>

        <Field error={errors.ssh_keys?.message} label="Kunci SSH">
          <textarea
            className={cn(richTextareaClass, "min-h-[120px] font-mono text-[13px]")}
            {...register("ssh_keys")}
            placeholder="Tempel public key SSH. Jika lebih dari satu, pisahkan per baris."
          />
        </Field>
    </FormModal>
  );
}

function isValidEmployeePhone(value: string) {
  const normalized = value.replace(/[\s()-]/g, "");
  return /^(?:\+62|62|0|8)\d{8,13}$/.test(normalized);
}

function isValidLinkedInProfile(value: string) {
  try {
    const parsed = new URL(value);
    return /^https?:$/.test(parsed.protocol) && (parsed.hostname === "linkedin.com" || parsed.hostname.endsWith(".linkedin.com"));
  } catch {
    return false;
  }
}

function validateAvatarFile(file: File | null) {
  if (!file) {
    return null;
  }
  if (!ACCEPTED_AVATAR_TYPES.has(file.type)) {
    return "Avatar harus berupa JPG, PNG, atau WebP";
  }
  if (file.size > MAX_AVATAR_SIZE_BYTES) {
    return "Ukuran avatar maksimal 5 MB";
  }
  return null;
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

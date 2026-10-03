import { useState, type ReactNode } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleAlert, CircleCheck, IdCard, Lock, Pencil, Save, Scale } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { IdentityForm } from "@/components/hr-profile/identity-card";
import { Pill } from "@/components/payslips/payslip-display";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useRBAC } from "@/hooks/use-rbac";
import { formatCalendarDate } from "@/lib/date";
import { permissions } from "@/lib/permissions";
import { contractsKeys } from "@/services/hris-contracts";
import { employeesKeys, getEmployee, patchEmployeeFields } from "@/services/hris-employees";
import { getHRProfile, hrProfileKeys } from "@/services/hris-hr-profile";
import { toast } from "@/stores/toast-store";
import type { Employee } from "@/types/hris";

interface CheckItem {
  label: string;
  value: ReactNode;
  ok: boolean;
}

const genderLabels: Record<string, string> = { male: "Laki-laki", female: "Perempuan" };

/**
 * 'Data Legal' of the contract form: what the PKWT and NDA print about the
 * employee, and what is still missing. Identity fields (NIK, birth data,
 * gender, account holder, KTP address) are fixed inline through the HR
 * profile (hris:employee_identity:edit; the KTP address is prefilled from
 * the employee address); phone and bank through the employee record
 * (hris:employee:edit). The server preflight stays the authority before
 * Generate.
 */
export function DataLegalPanel({ employeeId }: { employeeId: string }) {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canViewIdentity = hasPermission(permissions.hrisEmployeeIdentityView);
  const canEditIdentity = hasPermission(permissions.hrisEmployeeIdentityEdit);
  const canEditEmployee = hasPermission(permissions.hrisEmployeeEdit);
  const [editing, setEditing] = useState<"identity" | "employee" | null>(null);

  const employeeQuery = useQuery({
    queryKey: employeesKeys.detail(employeeId),
    queryFn: () => getEmployee(employeeId),
  });
  // Shared with the employee page; identity reads are access-logged, so no
  // focus refetch.
  const profileQuery = useQuery({
    queryKey: hrProfileKeys.detail(employeeId),
    queryFn: () => getHRProfile(employeeId),
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  });

  const employee = employeeQuery.data;
  const profile = profileQuery.data;
  const loading = employeeQuery.isLoading || profileQuery.isLoading;
  const error =
    employeeQuery.error instanceof Error
      ? employeeQuery.error.message
      : profileQuery.error instanceof Error
        ? profileQuery.error.message
        : null;

  const refreshPreflight = () =>
    queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "preflight"] });

  const identityVisible = profile?.identity_visible ?? false;
  const identity = profile?.identity;
  const identityItems: CheckItem[] = identity
    ? [
        {
          label: "NIK",
          value: identity.has_nik ? <span className="font-mono tracking-[0.06em]">{identity.nik_masked}</span> : null,
          ok: identity.has_nik,
        },
        { label: "Tempat lahir", value: identity.birth_place, ok: identity.birth_place !== "" },
        {
          label: "Tanggal lahir",
          value: identity.birth_date ? formatCalendarDate(identity.birth_date) : null,
          ok: identity.birth_date !== "",
        },
        { label: "Jenis kelamin", value: genderLabels[identity.gender] ?? null, ok: identity.gender !== "" },
        { label: "Nama pemilik rekening", value: identity.bank_account_name, ok: identity.bank_account_name !== "" },
        { label: "Alamat sesuai KTP", value: identity.ktp_address, ok: identity.ktp_address !== "" },
      ]
    : [];
  const employeeItems: CheckItem[] = employee
    ? [
        { label: "Email", value: employee.email, ok: Boolean(employee.email) },
        { label: "Telepon", value: employee.phone, ok: Boolean(employee.phone?.trim()) },
        { label: "Bank", value: employee.bank_name, ok: Boolean(employee.bank_name?.trim()) },
        {
          label: "Nomor rekening",
          value: employee.bank_account_number ? <span className="font-mono">{employee.bank_account_number}</span> : null,
          ok: Boolean(employee.bank_account_number?.trim()),
        },
      ]
    : [];
  const missingCount = [...identityItems, ...employeeItems].filter((item) => !item.ok).length;

  return (
    <section className="space-y-4" data-testid="data-legal-panel">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="flex items-start gap-2 text-[13px] text-text-secondary">
          <Scale className="mt-0.5 h-4 w-4 shrink-0 text-hr" />
          Data karyawan yang dicetak di PKWT dan NDA. NIK dan nomor rekening lengkap hanya muncul di PKWT.
        </p>
        {!loading && !error ? (
          missingCount > 0 ? (
            <Pill tone="warning">{missingCount} data belum lengkap</Pill>
          ) : identityVisible ? (
            <Pill tone="success">Lengkap</Pill>
          ) : null
        ) : null}
      </div>

      {error ? <p className="text-sm text-error">{error}</p> : null}

      {loading ? (
        <div className="grid gap-3 md:grid-cols-2">
          <Skeleton className="h-40 rounded-xl" />
          <Skeleton className="h-40 rounded-xl" />
        </div>
      ) : employee && profile ? (
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="space-y-3 rounded-xl border border-border/70 p-4">
            <div className="flex items-center justify-between gap-2">
              <p className="flex items-center gap-2 text-[12px] font-[700] uppercase tracking-[0.08em] text-text-secondary">
                <IdCard className="h-3.5 w-3.5" />
                Identitas (KTP)
              </p>
              {canEditIdentity && editing !== "identity" ? (
                <Button onClick={() => setEditing("identity")} size="xs" type="button" variant="outline">
                  <Pencil className="h-3.5 w-3.5" />
                  {identityVisible && identityItems.some((item) => !item.ok) ? "Lengkapi" : "Ubah"}
                </Button>
              ) : null}
            </div>
            {editing === "identity" ? (
              <IdentityForm
                employeeId={employeeId}
                onDone={() => setEditing(null)}
                onSaved={() => void refreshPreflight()}
                prefill={{ ktp_address: employee.address ?? "" }}
                profile={profile}
              />
            ) : identityVisible ? (
              <CheckList items={identityItems} />
            ) : (
              <p className="flex items-start gap-2 text-[13px] text-text-secondary">
                <Lock className="mt-0.5 h-4 w-4 shrink-0" />
                {canViewIdentity
                  ? "Data identitas belum dapat dimuat."
                  : `Anda tidak memiliki izin melihat data identitas; kelengkapannya diperiksa saat Generate.${
                      canEditIdentity ? " Anda tetap bisa mengganti seluruh data identitas." : ""
                    }`}
              </p>
            )}
            {!canEditIdentity && identityItems.some((item) => !item.ok) ? (
              <p className="text-[12px] text-text-tertiary">
                Minta HR dengan izin ubah data identitas untuk melengkapinya.
              </p>
            ) : null}
          </div>

          <div className="space-y-3 rounded-xl border border-border/70 p-4">
            <div className="flex items-center justify-between gap-2">
              <p className="text-[12px] font-[700] uppercase tracking-[0.08em] text-text-secondary">Data karyawan</p>
              {canEditEmployee && editing !== "employee" ? (
                <Button onClick={() => setEditing("employee")} size="xs" type="button" variant="outline">
                  <Pencil className="h-3.5 w-3.5" />
                  Ubah telepon & rekening
                </Button>
              ) : null}
            </div>
            {editing === "employee" ? (
              <EmployeeContactForm
                employee={employee}
                onDone={() => setEditing(null)}
                onSaved={() => void refreshPreflight()}
              />
            ) : (
              <CheckList items={employeeItems} />
            )}
            <p className="text-[12px] text-text-tertiary">
              Kontrak dikirim ke email login karyawan (atau email pribadi bila HR memilihnya saat mengirim).
            </p>
          </div>
        </div>
      ) : null}
    </section>
  );
}

function CheckList({ items }: { items: CheckItem[] }) {
  return (
    <dl className="space-y-2">
      {items.map((item) => (
        <div className="flex items-start gap-2 text-[13px]" data-ok={item.ok ? "true" : "false"} key={item.label}>
          {item.ok ? (
            <CircleCheck className="mt-0.5 h-4 w-4 shrink-0 text-success" />
          ) : (
            <CircleAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
          )}
          <dt className="w-40 shrink-0 text-text-secondary">{item.label}</dt>
          <dd className="min-w-0 break-words font-medium text-text-primary">
            {item.ok ? item.value : <span className="font-normal text-warning">Belum diisi</span>}
          </dd>
        </div>
      ))}
    </dl>
  );
}

const contactSchema = z.object({
  phone: z.string().trim().max(50, "Maksimal 50 karakter"),
  bank_name: z.string().trim().max(100, "Maksimal 100 karakter"),
  bank_account_number: z
    .string()
    .trim()
    .max(100, "Maksimal 100 karakter")
    .refine((value) => value === "" || value.includes("*") || /^[0-9 .-]+$/.test(value), "Nomor rekening hanya angka"),
});

type ContactValues = z.infer<typeof contactSchema>;

/**
 * Phone and bank fixed inline on the employee record (full-replacement PUT
 * with every other field as last read). A masked account number left as is
 * keeps the stored one.
 */
function EmployeeContactForm({
  employee,
  onDone,
  onSaved,
}: {
  employee: Employee;
  onDone: () => void;
  onSaved: (saved: Employee) => void;
}) {
  const queryClient = useQueryClient();
  const masked = Boolean(employee.bank_account_number?.includes("*"));
  const {
    formState: { errors, isDirty },
    handleSubmit,
    register,
  } = useForm<ContactValues>({
    resolver: zodResolver(contactSchema),
    defaultValues: {
      phone: employee.phone ?? "",
      bank_name: employee.bank_name ?? "",
      bank_account_number: employee.bank_account_number ?? "",
    },
  });

  const mutation = useMutation({
    mutationFn: (values: ContactValues) => patchEmployeeFields(employee, values),
    onSuccess: async (saved) => {
      queryClient.setQueryData(employeesKeys.detail(employee.id), saved);
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
      toast.success("Data karyawan diperbarui");
      onSaved(saved);
      onDone();
    },
    onError: (error) => {
      toast.error("Gagal menyimpan data karyawan", error instanceof Error ? error.message : undefined);
    },
  });

  const pending = mutation.isPending;
  return (
    <form className="space-y-3" noValidate onSubmit={handleSubmit((values) => mutation.mutate(values))}>
      <Field error={errors.phone?.message} htmlFor="legal-phone" label="Telepon">
        <Input disabled={pending} id="legal-phone" maxLength={50} placeholder="08xxxxxxxxxx" {...register("phone")} />
      </Field>
      <Field error={errors.bank_name?.message} htmlFor="legal-bank-name" label="Nama bank">
        <Input disabled={pending} id="legal-bank-name" maxLength={100} placeholder="BCA" {...register("bank_name")} />
      </Field>
      <Field
        error={errors.bank_account_number?.message}
        hint={masked ? "Nomor tersimpan disamarkan; biarkan apa adanya untuk mempertahankannya." : undefined}
        htmlFor="legal-bank-account"
        label="Nomor rekening"
      >
        <Input
          autoComplete="off"
          className="font-mono"
          disabled={pending}
          id="legal-bank-account"
          inputMode="numeric"
          maxLength={100}
          {...register("bank_account_number")}
        />
      </Field>
      <div className="flex justify-end gap-2">
        <Button disabled={pending} onClick={onDone} size="sm" type="button" variant="ghost">
          Batal
        </Button>
        <Button disabled={pending || !isDirty} size="sm" type="submit">
          <Save className="h-4 w-4" />
          {pending ? "Menyimpan..." : "Simpan"}
        </Button>
      </div>
    </form>
  );
}

function Field({
  children,
  error,
  hint,
  htmlFor,
  label,
}: {
  children: ReactNode;
  error?: string;
  hint?: string;
  htmlFor: string;
  label: string;
}) {
  return (
    <div className="space-y-1.5">
      <label className="text-sm font-medium text-text-primary" htmlFor={htmlFor}>
        {label}
      </label>
      {children}
      {hint ? <p className="text-xs text-text-secondary">{hint}</p> : null}
      {error ? <p className="text-[12px] text-error">{error}</p> : null}
    </div>
  );
}

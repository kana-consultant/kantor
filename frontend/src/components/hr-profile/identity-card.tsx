import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { IdCard, Lock, Pencil, Save, ShieldAlert } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { ApiError } from "@/lib/api-client";
import { formatDateInputValue, parseCalendarDate } from "@/lib/date";
import { nikBirthDataError, nikFormatError, normalizeNIK } from "@/lib/nik";
import { hrProfileKeys, updateHRProfile } from "@/services/hris-hr-profile";
import { toast } from "@/stores/toast-store";
import type { HRGender, HRIdentity, HRProfile } from "@/types/hris";

const genderLabels: Record<HRGender, string> = {
  male: "Laki-laki",
  female: "Perempuan",
};

const emailSchema = z.email();

// hasStoredNIK is only known when the caller can read the identity. A kept
// NIK must still match the (possibly new) birth date and gender; the backend
// checks that, the client can only require both to be present.
// nikRequired: a caller who cannot read the identity replaces all of it and
// must enter a NIK (the backend never keeps the stored one for them, so its
// answers cannot reveal the stored birth date or gender).
function buildIdentitySchema(hasStoredNIK: boolean, nikRequired: boolean) {
  return z
    .object({
      nik: z.string().transform(normalizeNIK),
      birth_place: z.string().trim().max(100, "Maksimal 100 karakter"),
      birth_date: z.string().trim(),
      gender: z.enum(["", "male", "female"]),
      bank_account_name: z.string().trim().max(150, "Maksimal 150 karakter"),
      ktp_address: z.string().trim().max(500, "Maksimal 500 karakter"),
      personal_email: z
        .string()
        .trim()
        .max(160, "Maksimal 160 karakter")
        .refine((value) => value === "" || emailSchema.safeParse(value).success, "Format email tidak valid"),
    })
    .superRefine((values, ctx) => {
      if (values.birth_date !== "") {
        const parsed = /^\d{4}-\d{2}-\d{2}$/.test(values.birth_date)
          ? parseCalendarDate(values.birth_date)
          : null;
        if (
          !parsed ||
          formatDateInputValue(parsed) !== values.birth_date ||
          values.birth_date < "1900-01-01" ||
          values.birth_date > formatDateInputValue(new Date())
        ) {
          ctx.addIssue({ code: "custom", path: ["birth_date"], message: "Tanggal lahir tidak valid" });
          return;
        }
      }

      const needsBirthData = values.nik !== "" || hasStoredNIK;
      if (values.nik === "" && nikRequired) {
        ctx.addIssue({
          code: "custom",
          path: ["nik"],
          message: "NIK wajib diisi karena seluruh data identitas diganti",
        });
      }
      if (values.nik !== "") {
        const formatError = nikFormatError(values.nik);
        if (formatError) {
          ctx.addIssue({ code: "custom", path: ["nik"], message: formatError });
          return;
        }
      }
      if (needsBirthData) {
        const reason = values.nik !== "" ? "bersama NIK" : "karena NIK sudah tersimpan";
        if (values.birth_date === "") {
          ctx.addIssue({
            code: "custom",
            path: ["birth_date"],
            message: `Tanggal lahir wajib diisi ${reason}`,
          });
        }
        if (values.gender === "") {
          ctx.addIssue({
            code: "custom",
            path: ["gender"],
            message: `Jenis kelamin wajib diisi ${reason}`,
          });
        }
      }
      if (values.nik !== "" && values.birth_date !== "" && values.gender !== "") {
        const mismatch = nikBirthDataError(values.nik, values.birth_date, values.gender);
        if (mismatch) {
          ctx.addIssue({ code: "custom", path: ["nik"], message: mismatch });
        }
      }
    });
}

type IdentityFormInput = z.input<ReturnType<typeof buildIdentitySchema>>;
type IdentityFormValues = z.output<ReturnType<typeof buildIdentitySchema>>;

/** Values suggested for empty identity fields (e.g. the KTP address from employees.address). */
export interface IdentityPrefill {
  ktp_address?: string;
}

function toFormValues(profile: HRProfile | undefined, prefill?: IdentityPrefill): IdentityFormInput {
  const identity = profile?.identity;
  return {
    nik: "",
    birth_place: identity?.birth_place ?? "",
    birth_date: identity?.birth_date ?? "",
    gender: identity?.gender ?? "",
    bank_account_name: identity?.bank_account_name ?? "",
    ktp_address: identity?.ktp_address || prefill?.ktp_address?.trim() || "",
    personal_email: profile?.personal_email ?? "",
  };
}

// Backend detail keys -> form fields.
const serverFieldMap: Record<string, keyof IdentityFormInput> = {
  "identity.nik": "nik",
  "identity.birth_date": "birth_date",
  "identity.gender": "gender",
  personal_email: "personal_email",
};

const inputClass = "focus-visible:border-hr focus-visible:ring-hr/10";
const selectClass =
  "flex h-11 w-full rounded-xl border border-border/70 bg-surface-muted/90 px-3.5 text-[14px] text-text-primary outline-none transition-all duration-150 focus-visible:border-hr focus-visible:bg-surface focus-visible:shadow-focus disabled:cursor-not-allowed disabled:text-text-tertiary";
const textareaClass =
  "min-h-24 w-full rounded-xl border border-border/70 bg-surface-muted/90 px-3.5 py-2 text-[14px] text-text-primary outline-none transition-all duration-150 placeholder:text-text-tertiary focus-visible:border-hr focus-visible:bg-surface focus-visible:shadow-focus disabled:cursor-not-allowed disabled:text-text-tertiary";

export function IdentityCard({
  canEdit,
  employeeId,
  error,
  loading,
  profile,
}: {
  canEdit: boolean;
  employeeId: string;
  error: string | null;
  loading: boolean;
  profile: HRProfile | undefined;
}) {
  const [isEditing, setIsEditing] = useState(false);
  const visible = profile?.identity_visible ?? false;

  return (
    <Card className="space-y-5 p-6">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <div className="rounded-md bg-hr-light p-3 text-hr">
            <IdCard className="h-5 w-5" />
          </div>
          <div>
            <h3 className="font-display text-[18px] font-[700] text-text-primary">Data Identitas</h3>
            <p className="mt-1 text-sm text-text-secondary">
              Data sesuai KTP untuk kontrak kerja. Disimpan terenkripsi dan setiap akses tercatat di audit.
            </p>
          </div>
        </div>
        {canEdit && !isEditing && profile ? (
          <Button onClick={() => setIsEditing(true)} size="sm" type="button" variant="outline">
            <Pencil className="h-4 w-4" />
            {visible ? "Ubah" : "Ganti data"}
          </Button>
        ) : null}
      </div>

      {error ? <p className="text-sm text-error">{error}</p> : null}

      {loading ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-16 rounded-lg" />
          <Skeleton className="h-16 rounded-lg" />
          <Skeleton className="h-16 rounded-lg" />
          <Skeleton className="h-16 rounded-lg" />
        </div>
      ) : !profile ? null : isEditing ? (
        <IdentityForm employeeId={employeeId} onDone={() => setIsEditing(false)} profile={profile} />
      ) : visible && profile.identity ? (
        <IdentityView identity={profile.identity} personalEmail={profile.personal_email ?? ""} />
      ) : (
        <div className="flex items-start gap-3 rounded-[22px] border border-border/70 bg-background/70 p-4 text-sm text-text-secondary">
          <Lock className="mt-0.5 h-4 w-4 shrink-0" />
          <p>
            Anda tidak memiliki izin melihat data identitas karyawan.
            {canEdit
              ? " Anda tetap bisa mengganti seluruh data identitas termasuk NIK; data yang tersimpan tidak ditampilkan."
              : null}
          </p>
        </div>
      )}
    </Card>
  );
}

function IdentityView({ identity, personalEmail }: { identity: HRIdentity; personalEmail: string }) {
  const birthDate = parseCalendarDate(identity.birth_date);
  const items: Array<{ label: string; value: ReactNode; wide?: boolean }> = [
    {
      label: "NIK",
      value: identity.has_nik ? (
        <span className="font-mono tracking-[0.08em]">{identity.nik_masked}</span>
      ) : (
        "Belum diisi"
      ),
    },
    { label: "Jenis kelamin", value: identity.gender ? genderLabels[identity.gender] : "-" },
    { label: "Tempat lahir", value: identity.birth_place || "-" },
    {
      label: "Tanggal lahir",
      value: birthDate ? new Intl.DateTimeFormat("id-ID", { dateStyle: "long" }).format(birthDate) : "-",
    },
    { label: "Nama pemilik rekening", value: identity.bank_account_name || "-" },
    { label: "Email pribadi", value: personalEmail || "-" },
    { label: "Alamat sesuai KTP", value: identity.ktp_address || "-", wide: true },
  ];

  return (
    <div className="grid gap-4 md:grid-cols-2">
      {items.map((item) => (
        <div
          className={`rounded-[22px] border border-border/70 bg-background/70 p-4${item.wide ? " md:col-span-2" : ""}`}
          key={item.label}
        >
          <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{item.label}</p>
          <p className="mt-2 whitespace-pre-line break-words text-sm font-semibold">{item.value}</p>
        </div>
      ))}
      {identity.has_nik ? (
        <p className="text-xs text-muted-foreground md:col-span-2">
          NIK lengkap tidak pernah ditampilkan setelah disimpan; HR hanya bisa menggantinya.
        </p>
      ) : null}
    </div>
  );
}

/**
 * The identity form (replace-only NIK). Also used inline by the contract
 * form's 'Data Legal' panel, which passes the KTP address prefill and is
 * told about the save through onSaved.
 */
export function IdentityForm({
  employeeId,
  onDone,
  profile,
  prefill,
  onSaved,
}: {
  employeeId: string;
  onDone: () => void;
  profile: HRProfile;
  prefill?: IdentityPrefill;
  onSaved?: (saved: HRProfile) => void;
}) {
  const queryClient = useQueryClient();
  const visible = profile.identity_visible;
  const identity = profile.identity;
  const hasStoredNIK = visible && (identity?.has_nik ?? false);
  const schema = useMemo(() => buildIdentitySchema(hasStoredNIK, !visible), [hasStoredNIK, visible]);

  const {
    formState: { errors },
    handleSubmit,
    register,
    setError,
  } = useForm<IdentityFormInput, unknown, IdentityFormValues>({
    resolver: zodResolver(schema),
    defaultValues: toFormValues(visible ? profile : undefined, prefill),
  });

  const saveMutation = useMutation({
    mutationFn: (values: IdentityFormValues) =>
      updateHRProfile(employeeId, {
        personal_email: values.personal_email.toLowerCase(),
        identity: {
          nik: values.nik,
          birth_place: values.birth_place,
          birth_date: values.birth_date,
          gender: values.gender,
          bank_account_name: values.bank_account_name,
          ktp_address: values.ktp_address,
        },
      }),
    onSuccess: (saved) => {
      toast.success("Data identitas berhasil disimpan");
      queryClient.setQueryData(hrProfileKeys.detail(employeeId), saved);
      onSaved?.(saved);
      onDone();
    },
    onError: (mutationError) => {
      let mapped = false;
      if (mutationError instanceof ApiError) {
        if (mutationError.code === "NIK_INVALID" || mutationError.code === "NIK_MISMATCH") {
          setError("nik", { message: mutationError.message });
          mapped = true;
        } else if (mutationError.code === "VALIDATION_ERROR") {
          const details = (mutationError.details ?? {}) as Record<string, string>;
          for (const key of Object.keys(details)) {
            const field = serverFieldMap[key];
            if (field) {
              setError(field, { message: mutationError.message });
              mapped = true;
            }
          }
        }
      }
      toast.error(
        "Gagal menyimpan data identitas",
        mapped ? undefined : mutationError instanceof Error ? mutationError.message : undefined,
      );
    },
  });

  const onSubmit = handleSubmit((values) => saveMutation.mutate(values));
  const pending = saveMutation.isPending;

  return (
    <form className="space-y-5" noValidate onSubmit={onSubmit}>
      {!visible ? (
        <div className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning-light px-4 py-3 text-sm text-text-primary">
          <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
          <p>
            Data tersimpan tidak ditampilkan. Menyimpan formulir ini mengganti seluruh data identitas dan email
            pribadi, sehingga NIK wajib diisi.
          </p>
        </div>
      ) : null}

      <FormField
        error={errors.nik?.message}
        hint={
          hasStoredNIK
            ? "Kosongkan untuk mempertahankan NIK tersimpan. NIK baru harus cocok dengan tanggal lahir dan jenis kelamin."
            : "16 digit sesuai KTP. Digit 7-12 harus cocok dengan tanggal lahir (+40 untuk perempuan), bulan, dan tahun."
        }
        htmlFor="identity-nik"
        label={hasStoredNIK ? "Ganti NIK" : "NIK"}
      >
        {hasStoredNIK && identity ? (
          <p className="text-xs text-text-secondary">
            NIK tersimpan: <span className="font-mono tracking-[0.08em]">{identity.nik_masked}</span>
          </p>
        ) : null}
        <Input
          autoComplete="off"
          className={`${inputClass} font-mono`}
          disabled={pending}
          id="identity-nik"
          inputMode="numeric"
          maxLength={24}
          placeholder={hasStoredNIK ? "Kosongkan untuk mempertahankan" : "3273015402980001"}
          spellCheck={false}
          {...register("nik")}
        />
      </FormField>

      <div className="grid gap-4 md:grid-cols-2">
        <FormField error={errors.birth_place?.message} htmlFor="identity-birth-place" label="Tempat lahir">
          <Input
            className={inputClass}
            disabled={pending}
            id="identity-birth-place"
            maxLength={100}
            placeholder="Bandung"
            {...register("birth_place")}
          />
        </FormField>
        <FormField error={errors.birth_date?.message} htmlFor="identity-birth-date" label="Tanggal lahir">
          <Input
            className={inputClass}
            disabled={pending}
            id="identity-birth-date"
            max={formatDateInputValue(new Date())}
            min="1900-01-01"
            type="date"
            {...register("birth_date", { deps: ["nik"] })}
          />
        </FormField>
        <FormField error={errors.gender?.message} htmlFor="identity-gender" label="Jenis kelamin">
          <select className={selectClass} disabled={pending} id="identity-gender" {...register("gender", { deps: ["nik"] })}>
            <option value="">Pilih jenis kelamin</option>
            <option value="male">{genderLabels.male}</option>
            <option value="female">{genderLabels.female}</option>
          </select>
        </FormField>
        <FormField
          error={errors.bank_account_name?.message}
          hint="Nama sesuai buku tabungan, jika berbeda dari nama karyawan."
          htmlFor="identity-bank-account-name"
          label="Nama pemilik rekening"
        >
          <Input
            className={inputClass}
            disabled={pending}
            id="identity-bank-account-name"
            maxLength={150}
            {...register("bank_account_name")}
          />
        </FormField>
      </div>

      <FormField error={errors.ktp_address?.message} htmlFor="identity-ktp-address" label="Alamat sesuai KTP">
        <textarea
          className={textareaClass}
          disabled={pending}
          id="identity-ktp-address"
          maxLength={500}
          placeholder="Alamat lengkap sesuai KTP"
          {...register("ktp_address")}
        />
      </FormField>

      <FormField
        error={errors.personal_email?.message}
        hint="Opsional. Slip gaji dan kontrak tetap dikirim ke email login karyawan kecuali HR memilih alamat ini saat mengirim."
        htmlFor="identity-personal-email"
        label="Email pribadi"
      >
        <Input
          autoComplete="off"
          className={inputClass}
          disabled={pending}
          id="identity-personal-email"
          maxLength={160}
          placeholder="nama@gmail.com"
          type="email"
          {...register("personal_email")}
        />
      </FormField>

      <div className="flex justify-end gap-2">
        <Button disabled={pending} onClick={onDone} size="sm" type="button" variant="ghost">
          Batal
        </Button>
        <Button disabled={pending} size="sm" type="submit">
          <Save className="h-4 w-4" />
          {pending ? "Menyimpan..." : "Simpan data identitas"}
        </Button>
      </div>
    </form>
  );
}

function FormField({
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
    <div className="space-y-2">
      <label className="text-sm font-medium" htmlFor={htmlFor}>
        {label}
      </label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      {error ? <p className="text-sm text-error">{error}</p> : null}
    </div>
  );
}

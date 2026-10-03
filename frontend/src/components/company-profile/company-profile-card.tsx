import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, ImageUp, Save, Trash2, TriangleAlert } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { ApiError } from "@/lib/api-client";
import { permissions } from "@/lib/permissions";
import {
  COMPANY_LOGO_MAX_BYTES,
  COMPANY_LOGO_TYPES,
  companyProfileKeys,
  deleteCompanyLogo,
  fetchCompanyLogo,
  getCompanyProfile,
  updateCompanyProfile,
  uploadCompanyLogo,
} from "@/services/company-profile";
import { toast } from "@/stores/toast-store";
import type { CompanyProfile } from "@/types/admin";

// Mirrors the backend: upper-cased, letters/digits/hyphen, max 20, no leading hyphen.
const docCodePattern = /^[A-Z0-9][A-Z0-9-]{0,19}$/;
const emailSchema = z.email();

const companyProfileSchema = z.object({
  legal_name: z.string().trim().max(200, "Maksimal 200 karakter"),
  address: z.string().trim().max(500, "Maksimal 500 karakter"),
  business_type: z.string().trim().max(200, "Maksimal 200 karakter"),
  city: z.string().trim().max(100, "Maksimal 100 karakter"),
  signer_name: z.string().trim().max(120, "Maksimal 120 karakter"),
  signer_title: z.string().trim().max(120, "Maksimal 120 karakter"),
  hr_contact_email: z
    .string()
    .trim()
    .max(160, "Maksimal 160 karakter")
    .refine((value) => value === "" || emailSchema.safeParse(value).success, "Format email tidak valid"),
  doc_code: z
    .string()
    .trim()
    .transform((value) => value.toUpperCase())
    .refine(
      (value) => value === "" || docCodePattern.test(value),
      "Hanya huruf, angka, dan tanda hubung (maksimal 20 karakter, tidak diawali tanda hubung)",
    ),
  payday_day: z
    .number({ error: "Tanggal gajian wajib diisi" })
    .int("Harus bilangan bulat")
    .min(1, "Minimal tanggal 1")
    .max(31, "Maksimal tanggal 31"),
  annual_leave_days: z
    .number({ error: "Jumlah cuti tahunan wajib diisi" })
    .int("Harus bilangan bulat")
    .min(0, "Minimal 0 hari")
    .max(365, "Maksimal 365 hari"),
});

type CompanyProfileFormInput = z.input<typeof companyProfileSchema>;
type CompanyProfileFormValues = z.output<typeof companyProfileSchema>;

function toFormValues(profile: CompanyProfile | undefined): CompanyProfileFormInput {
  return {
    legal_name: profile?.legal_name ?? "",
    address: profile?.address ?? "",
    business_type: profile?.business_type ?? "",
    city: profile?.city ?? "",
    signer_name: profile?.signer_name ?? "",
    signer_title: profile?.signer_title ?? "",
    hr_contact_email: profile?.hr_contact_email ?? "",
    doc_code: profile?.doc_code ?? "",
    payday_day: profile?.payday_day ?? 25,
    annual_leave_days: profile?.annual_leave_days ?? 12,
  };
}

const inputClass =
  "h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20";
const textareaClass =
  "min-h-24 w-full rounded-[6px] border-[1.5px] border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary outline-none transition-all duration-150 placeholder:text-text-tertiary focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus disabled:cursor-not-allowed disabled:text-text-tertiary";

export function CompanyProfileCard() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.adminSettingsManage);

  const profileQuery = useQuery({
    queryKey: companyProfileKeys.profile(),
    queryFn: getCompanyProfile,
  });
  const profile = profileQuery.data;

  const {
    formState: { errors },
    handleSubmit,
    register,
    reset,
    setError,
  } = useForm<CompanyProfileFormInput, unknown, CompanyProfileFormValues>({
    resolver: zodResolver(companyProfileSchema),
    defaultValues: toFormValues(undefined),
  });

  // The cached profile also changes on a logo upload/delete or a background
  // refetch; keep whatever the admin has typed but not saved yet.
  useEffect(() => {
    if (profile) {
      reset(toFormValues(profile), { keepDirtyValues: true });
    }
  }, [profile, reset]);

  const saveMutation = useMutation({
    mutationFn: updateCompanyProfile,
    onSuccess: (saved) => {
      toast.success("Profil perusahaan berhasil diperbarui");
      queryClient.setQueryData(companyProfileKeys.profile(), saved);
      reset(toFormValues(saved));
    },
    onError: (error) => {
      if (error instanceof ApiError && error.code === "VALIDATION_ERROR") {
        const details = (error.details ?? {}) as Record<string, string>;
        if (details.doc_code) {
          setError("doc_code", { message: error.message });
        }
      }
      toast.error(
        "Gagal memperbarui profil perusahaan",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  const onSubmit = handleSubmit((values) => {
    saveMutation.mutate({
      legal_name: values.legal_name,
      address: values.address,
      business_type: values.business_type,
      city: values.city,
      signer_name: values.signer_name,
      signer_title: values.signer_title,
      hr_contact_email: values.hr_contact_email.toLowerCase(),
      doc_code: values.doc_code,
      payday_day: values.payday_day,
      annual_leave_days: values.annual_leave_days,
    });
  });

  const disabled = !canManage || profileQuery.isLoading;

  return (
    <Card className="space-y-5 p-6 xl:col-span-2">
      <div className="flex items-start gap-3">
        <div className="rounded-md bg-error-light p-3 text-error">
          <Building2 className="h-5 w-5" />
        </div>
        <div>
          <h2 className="font-display text-[18px] font-[700] text-text-primary">Profil Perusahaan</h2>
          <p className="mt-1 text-sm text-text-secondary">
            Data perusahaan yang dicetak pada kontrak kerja dan slip gaji: nama badan usaha,
            penandatangan, kode dokumen, dan logo di kop slip.
          </p>
        </div>
      </div>

      {profileQuery.error instanceof Error ? (
        <p className="flex items-start gap-2 text-sm text-error" role="alert">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
          {profileQuery.error.message}
        </p>
      ) : null}

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,0.8fr)]">
        <form className="space-y-5" noValidate onSubmit={onSubmit}>
          <div className="grid gap-4 md:grid-cols-2">
            <Field error={errors.legal_name?.message} htmlFor="company-legal-name" label="Nama PT">
              <Input
                className={inputClass}
                disabled={disabled}
                id="company-legal-name"
                placeholder="PT Contoh Teknologi Nusantara"
                {...register("legal_name")}
              />
            </Field>
            <Field error={errors.business_type?.message} htmlFor="company-business-type" label="Bidang usaha">
              <Input
                className={inputClass}
                disabled={disabled}
                id="company-business-type"
                placeholder="Jasa konsultasi teknologi informasi"
                {...register("business_type")}
              />
            </Field>
          </div>

          <Field error={errors.address?.message} htmlFor="company-address" label="Alamat">
            <textarea
              className={textareaClass}
              disabled={disabled}
              id="company-address"
              placeholder="Alamat lengkap kantor sesuai akta"
              {...register("address")}
            />
          </Field>

          <div className="grid gap-4 md:grid-cols-2">
            <Field error={errors.city?.message} htmlFor="company-city" hint="Dipakai pada tempat penandatanganan dokumen." label="Kota">
              <Input
                className={inputClass}
                disabled={disabled}
                id="company-city"
                placeholder="Bandung"
                {...register("city")}
              />
            </Field>
            <Field
              error={errors.hr_contact_email?.message}
              htmlFor="company-hr-email"
              hint="Kontak HR yang dicantumkan di dokumen."
              label="Email HR"
            >
              <Input
                className={inputClass}
                disabled={disabled}
                id="company-hr-email"
                placeholder="hr@perusahaan.co.id"
                type="email"
                {...register("hr_contact_email")}
              />
            </Field>
            <Field error={errors.signer_name?.message} htmlFor="company-signer-name" label="Nama penandatangan">
              <Input
                className={inputClass}
                disabled={disabled}
                id="company-signer-name"
                placeholder="Nama direktur / pejabat berwenang"
                {...register("signer_name")}
              />
            </Field>
            <Field error={errors.signer_title?.message} htmlFor="company-signer-title" label="Jabatan penandatangan">
              <Input
                className={inputClass}
                disabled={disabled}
                id="company-signer-title"
                placeholder="Direktur Utama"
                {...register("signer_title")}
              />
            </Field>
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            <Field
              error={errors.doc_code?.message}
              htmlFor="company-doc-code"
              hint="Awalan nomor dokumen dan kode karyawan, mis. CTN → CTN-0001. Isi sebelum membuat slip atau kontrak."
              label="Kode dokumen"
            >
              <Input
                autoComplete="off"
                className={`${inputClass} font-mono uppercase`}
                disabled={disabled}
                id="company-doc-code"
                maxLength={20}
                placeholder="CTN"
                {...register("doc_code")}
              />
            </Field>
            <Field
              error={errors.payday_day?.message}
              htmlFor="company-payday"
              hint="Tanggal 1-31. Jika jatuh di akhir pekan, slip memakai hari kerja sebelumnya."
              label="Tanggal gajian"
            >
              <Input
                className={inputClass}
                disabled={disabled}
                id="company-payday"
                inputMode="numeric"
                max={31}
                min={1}
                type="number"
                {...register("payday_day", { valueAsNumber: true })}
              />
            </Field>
            <Field
              error={errors.annual_leave_days?.message}
              htmlFor="company-annual-leave"
              hint="Hak cuti tahunan yang dicantumkan di kontrak."
              label="Cuti tahunan (hari)"
            >
              <Input
                className={inputClass}
                disabled={disabled}
                id="company-annual-leave"
                inputMode="numeric"
                max={365}
                min={0}
                type="number"
                {...register("annual_leave_days", { valueAsNumber: true })}
              />
            </Field>
          </div>

          <div className="flex justify-end">
            <Button disabled={disabled || saveMutation.isPending} type="submit">
              <Save className="h-4 w-4" />
              {saveMutation.isPending ? "Menyimpan..." : "Simpan Profil Perusahaan"}
            </Button>
          </div>
        </form>

        <CompanyLogoPanel canManage={canManage} loading={profileQuery.isLoading} profile={profile} />
      </div>
    </Card>
  );
}

function CompanyLogoPanel({
  canManage,
  loading,
  profile,
}: {
  canManage: boolean;
  loading: boolean;
  profile: CompanyProfile | undefined;
}) {
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [fileError, setFileError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [objectUrl, setObjectUrl] = useState<string | null>(null);
  const [dimensions, setDimensions] = useState<{ width: number; height: number } | null>(null);

  const hasLogo = profile?.has_logo ?? false;
  const logoQuery = useQuery({
    queryKey: companyProfileKeys.logo(profile?.logo_updated_at ?? null),
    queryFn: fetchCompanyLogo,
    enabled: hasLogo,
    gcTime: 0,
    staleTime: Infinity,
  });
  const logoBlob = hasLogo ? logoQuery.data : undefined;

  // One object URL per fetched blob, revoked when the blob changes or the
  // panel unmounts.
  useEffect(() => {
    if (!logoBlob) {
      setObjectUrl(null);
      setDimensions(null);
      return;
    }
    const url = URL.createObjectURL(logoBlob);
    setObjectUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [logoBlob]);

  const onLogoSaved = (saved: CompanyProfile) => {
    queryClient.setQueryData(companyProfileKeys.profile(), saved);
  };

  const uploadMutation = useMutation({
    mutationFn: uploadCompanyLogo,
    onSuccess: (saved) => {
      toast.success("Logo perusahaan berhasil diunggah");
      setFileError(null);
      onLogoSaved(saved);
    },
    onError: (error) => {
      const message = describeLogoError(error);
      setFileError(message);
      toast.error("Gagal mengunggah logo", message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteCompanyLogo,
    onSuccess: (saved) => {
      toast.success("Logo perusahaan dihapus");
      setConfirmDelete(false);
      setFileError(null);
      onLogoSaved(saved);
    },
    onError: (error) => {
      toast.error("Gagal menghapus logo", error instanceof Error ? error.message : undefined);
    },
  });

  const onFileSelected = (file: File | undefined) => {
    if (fileInputRef.current) {
      // Allow picking the same file again after an error.
      fileInputRef.current.value = "";
    }
    if (!file) {
      return;
    }
    if (!(COMPANY_LOGO_TYPES as readonly string[]).includes(file.type)) {
      setFileError("Logo harus berupa gambar PNG atau JPEG.");
      return;
    }
    if (file.size > COMPANY_LOGO_MAX_BYTES) {
      setFileError("Ukuran logo maksimal 2 MB.");
      return;
    }
    setFileError(null);
    uploadMutation.mutate(file);
  };

  const busy = uploadMutation.isPending || deleteMutation.isPending;
  const disabled = !canManage || loading || busy;

  return (
    <div className="space-y-4 rounded-md border border-border bg-surface-muted/40 p-4">
      <div>
        <p className="text-sm font-semibold text-text-primary">Logo perusahaan</p>
        <p className="mt-1 text-xs text-text-secondary">
          Logo tampil di kop slip gaji. Tepi kosong (transparan atau putih) dipangkas otomatis, lalu logo
          diperkecil tanpa distorsi agar muat di area logo slip (5,03 × 1,08 cm). Logo mendatar paling
          pas.
        </p>
      </div>

      <div className="space-y-2">
        <p className="text-[11px] font-semibold uppercase tracking-[0.08em] text-text-secondary">
          Hasil pemrosesan
        </p>
        <div className="flex min-h-28 items-center justify-center rounded-md border border-border bg-white p-3">
          {hasLogo && objectUrl ? (
            <img
              alt="Logo perusahaan (hasil pemrosesan)"
              className="max-h-24 max-w-full object-contain"
              onLoad={(event) =>
                setDimensions({
                  width: event.currentTarget.naturalWidth,
                  height: event.currentTarget.naturalHeight,
                })
              }
              src={objectUrl}
            />
          ) : hasLogo && logoQuery.isError ? (
            <p className="text-xs text-error">Logo tidak dapat dimuat.</p>
          ) : hasLogo ? (
            <div className="h-16 w-full animate-pulse rounded-md bg-surface-muted" />
          ) : (
            <p className="text-center text-xs text-slate-500">
              Belum ada logo. Area logo di slip gaji dibiarkan kosong.
            </p>
          )}
        </div>
        {hasLogo && dimensions ? (
          <p className="text-xs text-text-secondary">
            {dimensions.width} × {dimensions.height} px (rasio{" "}
            {(dimensions.width / Math.max(dimensions.height, 1)).toFixed(2).replace(".", ",")}:1)
          </p>
        ) : null}
      </div>

      {hasLogo && objectUrl ? (
        <div className="space-y-2">
          <p className="text-[11px] font-semibold uppercase tracking-[0.08em] text-text-secondary">
            Pratinjau area logo slip gaji
          </p>
          <div className="rounded-md border border-dashed border-border bg-white p-2">
            <div className="aspect-[503/108] w-full">
              <img
                alt="Pratinjau logo di kop slip gaji"
                className="h-full w-full object-contain object-left"
                src={objectUrl}
              />
            </div>
          </div>
        </div>
      ) : null}

      <input
        accept={COMPANY_LOGO_TYPES.join(",")}
        className="hidden"
        disabled={disabled}
        onChange={(event) => onFileSelected(event.target.files?.[0])}
        ref={fileInputRef}
        type="file"
      />

      <div className="flex flex-wrap gap-2">
        <Button
          disabled={disabled}
          onClick={() => fileInputRef.current?.click()}
          size="sm"
          type="button"
          variant="outline"
        >
          <ImageUp className="h-4 w-4" />
          {uploadMutation.isPending ? "Mengunggah..." : hasLogo ? "Ganti logo" : "Unggah logo"}
        </Button>
        {hasLogo ? (
          <Button
            disabled={disabled}
            onClick={() => setConfirmDelete(true)}
            size="sm"
            type="button"
            variant="ghost"
          >
            <Trash2 className="h-4 w-4" />
            Hapus logo
          </Button>
        ) : null}
      </div>
      <p className="text-xs text-text-secondary">PNG atau JPEG, maksimal 2 MB.</p>
      {fileError ? (
        <p className="flex items-start gap-2 text-xs text-error" role="alert">
          <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          {fileError}
        </p>
      ) : null}

      <ConfirmDialog
        confirmLabel="Hapus logo"
        description="Slip gaji yang dibuat setelah ini tidak lagi menampilkan logo sampai logo baru diunggah."
        isLoading={deleteMutation.isPending}
        isOpen={confirmDelete}
        onClose={() => setConfirmDelete(false)}
        onConfirm={() => deleteMutation.mutate()}
        title="Hapus logo perusahaan?"
      />
    </div>
  );
}

function describeLogoError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 413 || error.code === "LOGO_TOO_LARGE") {
      return "Ukuran logo maksimal 2 MB.";
    }
    if (error.status === 429) {
      return "Terlalu banyak unggahan. Coba lagi dalam satu menit.";
    }
    // Backend messages start lower-case (Go error style).
    return error.message.charAt(0).toUpperCase() + error.message.slice(1);
  }
  return error instanceof Error ? error.message : "Logo tidak dapat diunggah.";
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
      <label className="text-[13px] font-[600] text-text-primary" htmlFor={htmlFor}>
        {label}
      </label>
      {children}
      {hint ? <p className="text-xs text-text-secondary">{hint}</p> : null}
      {error ? <p className="text-xs text-error">{error}</p> : null}
    </div>
  );
}

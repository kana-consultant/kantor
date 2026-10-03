import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, FileText, FlaskConical, Save, TriangleAlert } from "lucide-react";
import { useForm, useWatch } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { ApiError } from "@/lib/api-client";
import { permissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import {
  documentMailKeys,
  getDocumentMailSetting,
  sendDocumentMailTest,
  updateDocumentMailSetting,
} from "@/services/document-mail";
import { toast } from "@/stores/toast-store";
import type {
  DocumentMailSetting,
  DocumentMailTestResult,
  DocumentPdfStatus,
} from "@/types/admin";

const portOptions = [
  { value: "587", label: "587 — STARTTLS (disarankan)" },
  { value: "465", label: "465 — SSL/TLS" },
] as const;

const emailSchema = z.email();

// Changing the Gmail address or port drops the stored app password, so the
// save needs a new one in the same request ("Hapus password tersimpan" does
// not replace it). Emptying the address removes the account instead: the
// stored password is simply dropped and no new one is needed.
function isAccountChange(
  current: DocumentMailSetting | undefined,
  username: string,
  port: string,
): boolean {
  if (!current?.has_smtp_password || isAccountRemoval(username)) {
    return false;
  }
  return (
    username.trim().toLowerCase() !== current.smtp_username || Number(port) !== current.smtp_port
  );
}

function isAccountRemoval(username: string): boolean {
  return username.trim() === "";
}

function buildDocumentMailSchema(current: DocumentMailSetting | undefined) {
  return z
    .object({
      enabled: z.boolean(),
      smtp_username: z
        .string()
        .trim()
        .max(160, "Maksimal 160 karakter")
        .refine(
          (value) => value === "" || emailSchema.safeParse(value).success,
          "Format email tidak valid",
        ),
      smtp_password: z.string().max(200, "Maksimal 200 karakter"),
      clear_smtp_password: z.boolean(),
      smtp_port: z.enum(["587", "465"]),
      sender_name: z.string().trim().max(120, "Maksimal 120 karakter"),
    })
    .superRefine((values, ctx) => {
      if (values.enabled && values.smtp_username === "") {
        ctx.addIssue({
          code: "custom",
          path: ["smtp_username"],
          message: "Alamat Gmail wajib diisi saat Email Dokumen diaktifkan",
        });
      }
      if (
        isAccountChange(current, values.smtp_username, values.smtp_port) &&
        values.smtp_password.trim() === ""
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["smtp_password"],
          message:
            "Alamat Gmail atau port berubah: isi app password baru (password lama tidak dipakai untuk akun lain).",
        });
      }
    });
}

type DocumentMailFormValues = z.infer<ReturnType<typeof buildDocumentMailSchema>>;

function toFormValues(setting: DocumentMailSetting | undefined): DocumentMailFormValues {
  return {
    enabled: setting?.enabled ?? false,
    smtp_username: setting?.smtp_username ?? "",
    smtp_password: "",
    clear_smtp_password: false,
    smtp_port: setting?.smtp_port === 465 ? "465" : "587",
    sender_name: setting?.sender_name ?? "",
  };
}

// Mailpit's web UI runs next to its SMTP port; the link is only shown for a
// capture server on this machine.
const mailpitUIURL = "http://localhost:8025";

function isLocalCaptureAddr(addr: string): boolean {
  const host = addr.replace(/:\d+$/, "").replace(/^\[|\]$/g, "");
  return host === "localhost" || host === "127.0.0.1" || host === "::1";
}

// Read-only server status: the LibreOffice binary is chosen by the server
// environment (auto-detected or SOFFICE_BIN), never from this page.
function describePdfStatus(pdf: DocumentPdfStatus | undefined): {
  text: string;
  tone: "success" | "warning" | "muted";
} | null {
  if (!pdf) {
    return null;
  }
  if (pdf.enabled) {
    return pdf.source === "auto"
      ? { text: "aktif (LibreOffice terdeteksi otomatis)", tone: "success" }
      : { text: "aktif (LibreOffice dari SOFFICE_BIN)", tone: "success" };
  }
  switch (pdf.source) {
    case "disabled":
      return { text: "nonaktif (SOFFICE_BIN=off)", tone: "muted" };
    case "env_invalid":
      return {
        text: "nonaktif — SOFFICE_BIN tidak valid; perbaiki atau hapus nilainya agar LibreOffice dideteksi otomatis, lalu restart backend",
        tone: "warning",
      };
    default:
      return {
        text: "nonaktif — LibreOffice tidak ditemukan di server; pasang LibreOffice lalu restart backend",
        tone: "warning",
      };
  }
}

const inputClass =
  "h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20";

export function DocumentMailCard() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.adminSettingsManage);
  const [testResult, setTestResult] = useState<
    { kind: "sent"; recipient: string } | { kind: "error"; message: string } | null
  >(null);

  const settingQuery = useQuery({
    queryKey: documentMailKeys.setting(),
    queryFn: getDocumentMailSetting,
  });
  const setting = settingQuery.data;

  const schema = useMemo(() => buildDocumentMailSchema(setting), [setting]);
  const {
    control,
    formState: { errors, isDirty },
    handleSubmit,
    register,
    reset,
    setError,
  } = useForm<DocumentMailFormValues>({
    resolver: zodResolver(schema),
    defaultValues: toFormValues(undefined),
  });

  useEffect(() => {
    if (setting) {
      reset(toFormValues(setting));
    }
  }, [setting, reset]);

  const enabled = useWatch({ control, name: "enabled" });
  const clearPassword = useWatch({ control, name: "clear_smtp_password" });
  const typedPassword = useWatch({ control, name: "smtp_password" });
  const watchedUsername = useWatch({ control, name: "smtp_username" });
  const watchedPort = useWatch({ control, name: "smtp_port" });
  const accountChanged = isAccountChange(setting, watchedUsername, watchedPort);
  const accountRemoved = isAccountRemoval(watchedUsername);
  // On an account change the new password replaces the stored one, so the
  // clear checkbox has no effect and must not lock the password input.
  const clearingPassword = clearPassword && !accountChanged;

  const saveMutation = useMutation({
    mutationFn: updateDocumentMailSetting,
    onSuccess: async (saved) => {
      toast.success("Pengaturan Email Dokumen berhasil diperbarui");
      setTestResult(null);
      queryClient.setQueryData(documentMailKeys.setting(), saved);
      reset(toFormValues(saved));
      await queryClient.invalidateQueries({ queryKey: documentMailKeys.setting() });
    },
    onError: (error) => {
      if (error instanceof ApiError && error.code === "SMTP_PASSWORD_REQUIRED") {
        setError("smtp_password", { message: error.message });
      } else if (error instanceof ApiError && error.code === "VALIDATION_ERROR") {
        const details = (error.details ?? {}) as Record<string, string>;
        if (details.smtp_username) {
          setError("smtp_username", { message: "Alamat Gmail tidak valid atau belum diisi" });
        }
      }
      toast.error(
        "Gagal memperbarui Email Dokumen",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  const testMutation = useMutation({
    mutationFn: sendDocumentMailTest,
    onMutate: () => setTestResult(null),
    onSuccess: async (result: DocumentMailTestResult) => {
      if (result.sent) {
        setTestResult({ kind: "sent", recipient: result.recipient });
      } else {
        setTestResult({
          kind: "error",
          message: result.error_message ?? "Pengiriman email uji gagal",
        });
      }
    },
    onError: (error) => {
      setTestResult({
        kind: "error",
        message: error instanceof Error ? error.message : "Pengiriman email uji gagal",
      });
    },
  });

  const onSubmit = handleSubmit((values) => {
    const password = values.smtp_password.trim();
    // Emptying the address removes the account: the server drops the stored
    // password and never keeps one without an address.
    const removal = isAccountRemoval(values.smtp_username);
    const clear =
      (removal && (setting?.has_smtp_password ?? false)) ||
      (values.clear_smtp_password &&
        !isAccountChange(setting, values.smtp_username, values.smtp_port));
    saveMutation.mutate({
      enabled: values.enabled,
      smtp_username: values.smtp_username.trim().toLowerCase(),
      smtp_password: clear || removal || password === "" ? null : password,
      clear_smtp_password: clear,
      smtp_port: values.smtp_port === "465" ? 465 : 587,
      sender_name: values.sender_name.trim(),
    });
  });

  const hasStoredPassword = setting?.has_smtp_password ?? false;
  const devAddr =
    setting?.delivery?.mode === "dev_capture"
      ? (setting.delivery.capture_addr ?? setting.dev_smtp_addr ?? null)
      : (setting?.dev_smtp_addr ?? null);
  const pdfStatus = describePdfStatus(setting?.pdf);
  const disabled = !canManage || settingQuery.isLoading;
  // Saved state only, like the other "Status saat ini" lines; pending edits
  // are described next to the password field.
  const passwordStatus = hasStoredPassword
    ? "Tersimpan"
    : devAddr
      ? "Tidak diperlukan (mode dev)"
      : "Belum ada";
  const passwordNote = accountRemoved
    ? hasStoredPassword || typedPassword.trim()
      ? "Alamat Gmail dikosongkan: app password tidak disimpan dan yang tersimpan akan dihapus saat disimpan."
      : "App password belum tersimpan untuk tenant ini."
    : accountChanged
      ? "Alamat Gmail atau port berubah: app password tersimpan akan dihapus, isi app password baru untuk akun ini."
      : clearingPassword
        ? "App password tersimpan akan dihapus saat disimpan."
        : typedPassword.trim()
          ? hasStoredPassword
            ? "App password baru akan menggantikan yang tersimpan saat disimpan."
            : "App password baru akan disimpan (terenkripsi) saat disimpan."
          : hasStoredPassword
            ? "App password sudah tersimpan (terenkripsi, tidak pernah ditampilkan). Isi lagi hanya jika ingin mengganti."
            : "App password belum tersimpan untuk tenant ini.";

  return (
    <Card className="space-y-5 p-6 xl:col-span-2">
      <div className="flex items-start gap-3">
        <div className="rounded-md bg-error-light p-3 text-error">
          <FileText className="h-5 w-5" />
        </div>
        <div>
          <h2 className="font-display text-[18px] font-[700] text-text-primary">
            Email Dokumen (Gmail)
          </h2>
          <p className="mt-1 text-sm text-text-secondary">
            Akun Gmail untuk mengirim slip gaji dan kontrak kerja sebagai lampiran PDF. Server SMTP
            selalu <code>smtp.gmail.com</code>. Reset kata sandi dan notifikasi tetap memakai Email
            Tenant di atas.
          </p>
        </div>
      </div>

      {devAddr ? (
        <div
          className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning-light px-4 py-3 text-sm text-text-primary"
          data-testid="document-mail-dev-capture"
        >
          <FlaskConical className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
          <p>
            <span className="font-semibold">
              Mode development: email dokumen tidak dikirim ke Gmail, tetapi ditangkap Mailpit di{" "}
              {devAddr}
            </span>
            {isLocalCaptureAddr(devAddr) ? (
              <>
                {" "}
                — buka{" "}
                <a
                  className="font-semibold text-ops underline"
                  href={mailpitUIURL}
                  rel="noreferrer"
                  target="_blank"
                >
                  {mailpitUIURL}
                </a>
              </>
            ) : null}
            <span className="text-text-secondary">
              {" "}
              (SMTP lokal tanpa TLS/login). App password tidak diperlukan.
            </span>
          </p>
        </div>
      ) : null}

      <form className="space-y-5" noValidate onSubmit={onSubmit}>
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,0.8fr)]">
          <div className="space-y-4">
            <div className="rounded-md border border-border bg-surface-muted/60 p-4">
              <label className="flex items-center justify-between gap-4">
                <div>
                  <p className="text-sm font-semibold text-text-primary">Aktifkan Email Dokumen</p>
                  <p className="text-xs text-text-secondary">
                    Saat nonaktif, slip gaji dan kontrak tidak bisa dikirim lewat email.
                  </p>
                </div>
                <input
                  className="h-5 w-5 accent-[var(--module-primary)]"
                  disabled={disabled}
                  type="checkbox"
                  {...register("enabled", { deps: ["smtp_username"] })}
                />
              </label>
            </div>

            <div className="grid gap-4 md:grid-cols-2">
              <Field
                error={errors.smtp_username?.message}
                htmlFor="document-mail-username"
                label="Alamat Gmail"
                required={enabled}
              >
                <Input
                  autoComplete="off"
                  className={inputClass}
                  disabled={disabled}
                  id="document-mail-username"
                  placeholder="slip@perusahaan.co.id"
                  type="email"
                  {...register("smtp_username", { deps: ["smtp_password"] })}
                />
              </Field>
              <Field error={errors.sender_name?.message} htmlFor="document-mail-sender" label="Nama Pengirim">
                <Input
                  className={inputClass}
                  disabled={disabled}
                  id="document-mail-sender"
                  placeholder="HR Perusahaan"
                  {...register("sender_name")}
                />
              </Field>
              <Field error={errors.smtp_port?.message} htmlFor="document-mail-port" label="Port">
                <select
                  className="h-10 w-full rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
                  disabled={disabled}
                  id="document-mail-port"
                  {...register("smtp_port", { deps: ["smtp_password"] })}
                >
                  {portOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </Field>
              <Field htmlFor="document-mail-host" label="Server SMTP">
                <Input className={inputClass} disabled id="document-mail-host" value="smtp.gmail.com" readOnly />
              </Field>
            </div>

            <Field error={errors.smtp_password?.message} htmlFor="document-mail-password" label="App Password">
              <Input
                autoComplete="new-password"
                className={inputClass}
                disabled={disabled || clearingPassword}
                id="document-mail-password"
                placeholder={
                  accountChanged
                    ? "Wajib diisi: app password untuk akun/port baru"
                    : accountRemoved
                      ? "Isi alamat Gmail terlebih dahulu"
                      : hasStoredPassword
                        ? "Kosongkan untuk mempertahankan app password saat ini"
                        : "abcd efgh ijkl mnop"
                }
                type="password"
                {...register("smtp_password")}
              />
              <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-text-secondary">
                <span>{passwordNote}</span>
                <label className="inline-flex items-center gap-2">
                  <input
                    className="h-4 w-4 accent-[var(--module-primary)]"
                    disabled={disabled || !hasStoredPassword || accountChanged}
                    type="checkbox"
                    {...register("clear_smtp_password")}
                  />
                  Hapus password tersimpan
                </label>
              </div>
            </Field>

            <div className="rounded-md border border-border bg-surface-muted/40 px-4 py-3 text-sm text-text-secondary">
              <p className="font-semibold text-text-primary">Sebelum mengisi</p>
              <ul className="mt-2 list-disc space-y-1 pl-5">
                <li>Gunakan akun Gmail/Workspace khusus pengirim dokumen, bukan inbox harian seseorang.</li>
                <li>
                  Aktifkan Verifikasi 2 Langkah di akun tersebut, lalu buat <em>App Password</em> (16
                  karakter) dan tempel di sini.
                </li>
                <li>
                  Setiap kali password akun Google diganti, App Password otomatis dicabut. Buat yang baru
                  dan kirim email uji lagi.
                </li>
                <li>
                  Gmail menyimpan salinan setiap email beserta lampirannya di Sent Mail. Pasang aturan
                  retensi agar arsip slip gaji dan kontrak tidak menumpuk di sana.
                </li>
                <li>
                  Mengganti alamat Gmail atau port akan menghapus App Password tersimpan. Mengosongkan
                  alamat Gmail menghapus akun beserta App Password-nya.
                </li>
              </ul>
            </div>
          </div>

          <div className="space-y-4 rounded-md border border-border bg-surface-muted/40 p-4">
            <div className="rounded-md border border-border bg-surface px-4 py-3 text-sm">
              <p className="font-semibold text-text-primary">Status saat ini</p>
              <div className="mt-2 space-y-1 text-text-secondary">
                <p>Email Dokumen aktif: {setting?.enabled ? "Ya" : "Belum"}</p>
                <p>Alamat Gmail: {setting?.smtp_username || "Belum diisi"}</p>
                <p>App password: {passwordStatus}</p>
                <p>
                  Tujuan pengiriman:{" "}
                  {devAddr ? `Mailpit lokal (${devAddr})` : "Gmail (smtp.gmail.com)"}
                </p>
                {pdfStatus ? (
                  <p data-testid="document-mail-pdf-status">
                    Konversi PDF:{" "}
                    <span
                      className={cn(
                        "font-semibold",
                        pdfStatus.tone === "success"
                          ? "text-success"
                          : pdfStatus.tone === "warning"
                            ? "text-warning"
                            : "text-text-primary",
                      )}
                    >
                      {pdfStatus.text}
                    </span>
                  </p>
                ) : null}
                <p>
                  Siap mengirim:{" "}
                  <span className={cn("font-semibold", setting?.ready ? "text-success" : "text-text-primary")}>
                    {setting?.ready ? "Ya" : "Belum"}
                  </span>
                </p>
              </div>
            </div>

            <div className="space-y-3 rounded-md border border-border bg-surface px-4 py-3">
              <div>
                <p className="text-sm font-semibold text-text-primary">Kirim email uji</p>
                <p className="mt-1 text-xs text-text-secondary">
                  Email uji dikirim ke alamat login Anda sendiri memakai pengaturan yang sudah disimpan.
                </p>
              </div>
              <Button
                disabled={
                  !canManage || !setting?.ready || isDirty || testMutation.isPending || settingQuery.isLoading
                }
                onClick={() => testMutation.mutate()}
                size="sm"
                type="button"
                variant="outline"
              >
                <FlaskConical className="h-4 w-4" />
                {testMutation.isPending ? "Mengirim..." : "Kirim email uji"}
              </Button>
              {isDirty && setting?.ready ? (
                <p className="text-xs text-text-secondary">Simpan perubahan dulu sebelum mengirim email uji.</p>
              ) : null}
              {!setting?.ready && !settingQuery.isLoading ? (
                <p className="text-xs text-text-secondary">
                  Aktifkan dan lengkapi alamat Gmail serta app password, lalu simpan.
                </p>
              ) : null}
              {!isDirty && testResult?.kind === "sent" ? (
                <p className="flex items-start gap-2 text-sm text-success" role="status">
                  <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
                  Email uji terkirim ke {testResult.recipient}.
                </p>
              ) : null}
              {!isDirty && testResult?.kind === "error" ? (
                <p className="flex items-start gap-2 text-sm text-error" role="alert">
                  <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
                  {testResult.message}
                </p>
              ) : null}
            </div>
          </div>
        </div>

        <div className="flex justify-end">
          <Button disabled={disabled || saveMutation.isPending} type="submit">
            <Save className="h-4 w-4" />
            Simpan Email Dokumen
          </Button>
        </div>
      </form>
    </Card>
  );
}

function Field({
  children,
  error,
  htmlFor,
  label,
  required,
}: {
  children: ReactNode;
  error?: string;
  htmlFor: string;
  label: string;
  required?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <label className="text-[13px] font-[600] text-text-primary" htmlFor={htmlFor}>
        {label}
        {required ? <span className="text-error"> *</span> : null}
      </label>
      {children}
      {error ? <p className="text-xs text-error">{error}</p> : null}
    </div>
  );
}

import { useEffect, useState } from "react";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Eye, EyeOff, RefreshCw, Save, ShieldCheck } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { ApiError } from "@/lib/api-client";
import { ensureModuleAccess } from "@/lib/rbac";
import { useAuthStore } from "@/stores/auth-store";
import {
  adminRbacKeys,
  getRegistrationSettings,
  rollRegistrationCode,
  updateRegistrationSettings,
  type RegistrationSettingsView,
} from "@/services/admin-rbac";
import { toast } from "@/stores/toast-store";

export const Route = createFileRoute("/_authenticated/admin/registration")({
  beforeLoad: async () => {
    await ensureModuleAccess("admin");
    const session = useAuthStore.getState().session;
    if (!session?.is_super_admin) {
      throw redirect({ to: "/admin/audit-logs" });
    }
  },
  component: AdminRegistrationPage,
});

const registrationKey = ["admin-rbac", "registration"] as const;

function formatDateTime(value: string | null | undefined) {
  if (!value) return "-";
  try {
    return new Date(value).toLocaleString("id-ID", {
      dateStyle: "medium",
      timeStyle: "short",
    });
  } catch {
    return value;
  }
}

function AdminRegistrationPage() {
  const queryClient = useQueryClient();

  const settingsQuery = useQuery({
    queryKey: registrationKey,
    queryFn: getRegistrationSettings,
  });

  const [enabled, setEnabled] = useState(false);
  const [rotationDays, setRotationDays] = useState(7);
  const [domainsInput, setDomainsInput] = useState("");
  const [codeVisible, setCodeVisible] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    if (!settingsQuery.data) return;
    setEnabled(settingsQuery.data.enabled);
    setRotationDays(settingsQuery.data.rotation_interval_days);
    setDomainsInput(settingsQuery.data.allowed_email_domains.join(", "));
  }, [settingsQuery.data]);

  const saveMutation = useMutation({
    mutationFn: updateRegistrationSettings,
    onSuccess: (data) => {
      queryClient.setQueryData<RegistrationSettingsView>(registrationKey, data);
      void queryClient.invalidateQueries({ queryKey: adminRbacKeys.settings() });
      toast.success("Pengaturan registrasi disimpan", "");
      setFormError(null);
    },
    onError: (error) => {
      setFormError(error instanceof ApiError ? error.message : "Gagal menyimpan pengaturan");
    },
  });

  const rollMutation = useMutation({
    mutationFn: rollRegistrationCode,
    onSuccess: (data) => {
      queryClient.setQueryData<RegistrationSettingsView>(registrationKey, data.settings);
      setCodeVisible(true);
      toast.success("Kode registrasi diperbarui", "");
    },
    onError: (error) => {
      toast.error(
        "Gagal memutar kode",
        error instanceof ApiError ? error.message : "Terjadi kesalahan",
      );
    },
  });

  const onSave = () => {
    const normalizedDomains = domainsInput
      .split(",")
      .map((d) => d.trim().replace(/^@/, "").toLowerCase())
      .filter((d) => d.length > 0);

    saveMutation.mutate({
      enabled,
      rotation_interval_days: rotationDays,
      allowed_email_domains: normalizedDomains,
    });
  };

  const onCopy = async () => {
    const code = settingsQuery.data?.code ?? null;
    if (!code) return;
    if (navigator.clipboard && window.isSecureContext) {
      try {
        await navigator.clipboard.writeText(code);
        toast.success("Kode disalin", "");
        return;
      } catch {
        // fall through to legacy path
      }
    }
    // Legacy fallback for non-secure contexts (HTTP on non-localhost).
    const textarea = document.createElement("textarea");
    textarea.value = code;
    textarea.setAttribute("readonly", "");
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    document.body.appendChild(textarea);
    textarea.select();
    let ok = false;
    try {
      ok = document.execCommand("copy");
    } catch {
      ok = false;
    }
    document.body.removeChild(textarea);
    if (ok) {
      toast.success("Kode disalin", "");
    } else {
      toast.error("Gagal menyalin kode", "Salin manual dari kotak di atas.");
    }
  };

  const data = settingsQuery.data;
  const hasCode = data?.has_code ?? false;
  const code = data?.code ?? null;
  const expiresAt = data?.code_expires_at ?? null;
  const expired = expiresAt ? new Date(expiresAt).getTime() <= Date.now() : true;
  const maskedCode = code ? "•".repeat(Math.min(code.length, 24)) : "";

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-ops-light text-ops">
          <ShieldCheck className="h-5 w-5" />
        </div>
        <div>
          <h1 className="text-[20px] font-[700] text-text-primary">Pengaturan Registrasi</h1>
          <p className="text-[13px] text-text-secondary">
            Kelola kode registrasi dan domain email yang diizinkan untuk self-registration.
          </p>
        </div>
      </div>

      <Card className="space-y-5 p-6">
        <div className="space-y-3">
          <h2 className="text-[15px] font-[700] text-text-primary">Aktivasi</h2>
          <label className="flex items-start gap-3">
            <input
              checked={enabled}
              className="mt-1 h-4 w-4 rounded border-border text-ops focus:ring-ops"
              onChange={(event) => setEnabled(event.target.checked)}
              type="checkbox"
            />
            <span className="text-[14px] text-text-primary">
              Aktifkan self-registration.
              <span className="block text-[12px] text-text-secondary">
                Form registrasi publik hanya muncul saat aktif dan kode masih valid.
              </span>
            </span>
          </label>
        </div>

        <div className="space-y-2">
          <label className="text-[13px] font-[600] text-text-primary">
            Rotasi otomatis (hari)
          </label>
          <Input
            className="h-10 w-32 rounded-[6px]"
            max={90}
            min={1}
            onChange={(event) => setRotationDays(Number(event.target.value) || 7)}
            type="number"
            value={rotationDays}
          />
          <p className="text-[12px] text-text-secondary">
            Kode di-roll otomatis setelah periode ini jika sudah kadaluwarsa. Default 7 hari.
          </p>
        </div>

        <div className="space-y-2">
          <label className="text-[13px] font-[600] text-text-primary">
            Domain email yang diizinkan
          </label>
          <Input
            className="h-10 rounded-[6px]"
            onChange={(event) => setDomainsInput(event.target.value)}
            placeholder="perusahaan.com, partner.co.id"
            value={domainsInput}
          />
          <p className="text-[12px] text-text-secondary">
            Pisahkan dengan koma. Kosongkan untuk mengizinkan semua domain.
          </p>
        </div>

        {formError ? (
          <div className="rounded-[8px] border border-error/20 bg-error-light px-4 py-3 text-[13px] text-error">
            {formError}
          </div>
        ) : null}

        <div className="flex justify-end">
          <Button
            className="gap-2 bg-ops text-white hover:bg-ops-dark"
            disabled={saveMutation.isPending}
            onClick={onSave}
            type="button"
          >
            <Save className="h-4 w-4" />
            {saveMutation.isPending ? "Menyimpan..." : "Simpan"}
          </Button>
        </div>
      </Card>

      <Card className="space-y-5 p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-[15px] font-[700] text-text-primary">Kode Registrasi</h2>
            <p className="text-[13px] text-text-secondary">
              Bagikan kode ini ke calon user. Klik mata untuk tampilkan.
            </p>
          </div>
          <Button
            className="gap-2"
            disabled={rollMutation.isPending}
            onClick={() => rollMutation.mutate()}
            type="button"
            variant="outline"
          >
            <RefreshCw className={cn("h-4 w-4", rollMutation.isPending && "animate-spin")} />
            {rollMutation.isPending ? "Memutar..." : "Roll kode baru"}
          </Button>
        </div>

        {code ? (
          <div className="rounded-[8px] border border-border bg-surface-muted p-4">
            <div className="flex items-center gap-2">
              <code className="flex-1 rounded-[6px] bg-surface px-3 py-2 font-mono text-[14px] text-text-primary">
                {codeVisible ? code : maskedCode}
              </code>
              <Button
                className="gap-2"
                onClick={() => setCodeVisible((v) => !v)}
                type="button"
                variant="outline"
              >
                {codeVisible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                {codeVisible ? "Sembunyikan" : "Tampilkan"}
              </Button>
              <Button className="gap-2" onClick={onCopy} type="button" variant="outline">
                <Copy className="h-4 w-4" />
                Salin
              </Button>
            </div>
          </div>
        ) : (
          <div className="rounded-[8px] border border-border bg-surface-muted px-4 py-3 text-[13px] text-text-secondary">
            Belum ada kode. Klik "Roll kode baru" untuk generate.
          </div>
        )}

        <dl className="grid gap-3 text-[13px] sm:grid-cols-2">
          <div>
            <dt className="text-text-secondary">Status kode</dt>
            <dd className="font-[600] text-text-primary">
              {hasCode ? (expired ? "Kadaluwarsa" : "Aktif") : "Belum ada"}
            </dd>
          </div>
          <div>
            <dt className="text-text-secondary">Berlaku sampai</dt>
            <dd className="font-[600] text-text-primary">{formatDateTime(expiresAt)}</dd>
          </div>
          <div>
            <dt className="text-text-secondary">Terakhir di-roll</dt>
            <dd className="font-[600] text-text-primary">
              {formatDateTime(data?.last_rolled_at)}
            </dd>
          </div>
          <div>
            <dt className="text-text-secondary">Di-roll oleh</dt>
            <dd className="font-[600] text-text-primary">
              {data?.last_rolled_by_name ?? data?.last_rolled_by ?? "-"}
            </dd>
          </div>
        </dl>
      </Card>
    </div>
  );
}

import { useState } from "react";
import { useMutation } from "@tanstack/react-query";

import { ApiError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import {
  createVPS,
  createVPSApp,
  createVPSCheck,
  updateVPS,
  updateVPSApp,
  updateVPSCheck,
} from "@/services/operational-vps";
import { toast } from "@/stores/toast-store";
import type {
  VPSApp,
  VPSAppFormValues,
  VPSCheckFormValues,
  VPSFormValues,
  VPSHealthCheck,
  VPSHealthStatus,
  VPSServer,
} from "@/types/vps";

export const HEALTH_TONES: Record<VPSHealthStatus, string> = {
  up: "bg-success-light text-success",
  degraded: "bg-warning-light text-warning",
  down: "bg-error-light text-error",
  unknown: "bg-surface-muted text-text-tertiary",
};

export const HEALTH_LABELS: Record<VPSHealthStatus, string> = {
  up: "Up",
  degraded: "Degraded",
  down: "Down",
  unknown: "Unknown",
};

export function HealthPill({ status }: { status: VPSHealthStatus }) {
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium", HEALTH_TONES[status])}>
      <span className="h-1.5 w-1.5 rounded-full bg-current" />
      {HEALTH_LABELS[status]}
    </span>
  );
}

export function formatDateTime(iso?: string | null) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString();
}

export function relativeTime(iso?: string | null): string {
  if (!iso) return "belum pernah";
  const d = new Date(iso).getTime();
  const diff = Date.now() - d;
  if (diff < 60_000) return "<1 menit lalu";
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} menit lalu`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} jam lalu`;
  return `${Math.floor(diff / 86_400_000)} hari lalu`;
}

export function describeError(err: unknown, fallback: string): string {
  if (err instanceof ApiError) {
    if (err.details && typeof err.details === "object") {
      const pairs = Object.entries(err.details as Record<string, unknown>)
        .map(([k, v]) => `${k}: ${String(v)}`);
      if (pairs.length > 0) return `${err.message} — ${pairs.join("; ")}`;
    }
    return err.message;
  }
  if (err instanceof Error) return err.message;
  return fallback;
}

export function emptyVPSForm(): VPSFormValues {
  return {
    label: "",
    provider: "",
    hostname: "",
    ip_address: "",
    region: "",
    cpu_cores: 0,
    ram_mb: 0,
    disk_gb: 0,
    cost_amount: 0,
    cost_currency: "IDR",
    billing_cycle: "monthly",
    renewal_date: null,
    status: "active",
    tags: [],
    notes: "",
  };
}

export function fromServer(s: VPSServer): VPSFormValues {
  return {
    label: s.label,
    provider: s.provider,
    hostname: s.hostname,
    ip_address: s.ip_address,
    region: s.region,
    cpu_cores: s.cpu_cores,
    ram_mb: s.ram_mb,
    disk_gb: s.disk_gb,
    cost_amount: s.cost_amount,
    cost_currency: s.cost_currency,
    billing_cycle: s.billing_cycle,
    renewal_date: s.renewal_date ?? null,
    status: s.status,
    tags: s.tags,
    notes: s.notes,
  };
}

// ---------- form primitives ---------------------------------------------------

export function FormSection({
  title, hint, children,
}: {
  title: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <div>
        <h4 className="text-sm font-semibold">{title}</h4>
        {hint && <p className="text-xs text-text-tertiary">{hint}</p>}
      </div>
      <div className="grid gap-3 md:grid-cols-2">{children}</div>
    </div>
  );
}

export function LabeledInput({
  label, value, onChange, type, placeholder, required, hint,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  placeholder?: string;
  required?: boolean;
  hint?: string;
}) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block font-medium">{label}{required && <span className="ml-0.5 text-error">*</span>}</span>
      <Input value={value} onChange={(e) => onChange(e.target.value)} type={type} placeholder={placeholder} />
      {hint && <span className="mt-1 block text-xs text-text-tertiary">{hint}</span>}
    </label>
  );
}

export function LabeledNumber({
  label, value, onChange, hint, suffix,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  hint?: string;
  suffix?: string;
}) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block font-medium">{label}{suffix && <span className="ml-1 text-xs text-text-tertiary">({suffix})</span>}</span>
      <Input type="number" value={value} onChange={(e) => onChange(Number(e.target.value) || 0)} />
      {hint && <span className="mt-1 block text-xs text-text-tertiary">{hint}</span>}
    </label>
  );
}

export function LabeledSelect({
  label, value, onValueChange, options, hint,
}: {
  label: string;
  value: string;
  onValueChange: (v: string) => void;
  options: { value: string; label: string }[];
  hint?: string;
}) {
  return (
    <div className="block text-sm">
      <span className="mb-1 block font-medium">{label}</span>
      <Select value={value} options={options} onValueChange={onValueChange} />
      {hint && <span className="mt-1 block text-xs text-text-tertiary">{hint}</span>}
    </div>
  );
}

const CURRENCY_OPTIONS = [
  { value: "IDR", label: "IDR" },
  { value: "USD", label: "USD" },
  { value: "EUR", label: "EUR" },
  { value: "SGD", label: "SGD" },
  { value: "MYR", label: "MYR" },
];

export function CostField({
  currency, amount, onChange,
}: {
  currency: string;
  amount: number;
  onChange: (currency: string, amount: number) => void;
}) {
  const [raw, setRaw] = useState<string>(amount ? amount.toLocaleString("id-ID") : "");
  return (
    <div className="block text-sm">
      <span className="mb-1 block font-medium">Biaya per cycle</span>
      <div className="flex gap-2">
        <div className="w-28 shrink-0">
          <Select value={currency || "IDR"} options={CURRENCY_OPTIONS} onValueChange={(v) => onChange(v, amount)} />
        </div>
        <Input
          inputMode="numeric"
          placeholder="0"
          value={raw}
          onChange={(e) => {
            const digits = e.target.value.replace(/\D/g, "");
            const num = digits ? Number(digits) : 0;
            setRaw(digits ? num.toLocaleString("id-ID") : "");
            onChange(currency || "IDR", num);
          }}
        />
      </div>
      <span className="mt-1 block text-xs text-text-tertiary">Kosongkan jika belum ada angka pasti.</span>
    </div>
  );
}

// ---------- VPS form ----------------------------------------------------------

export function VPSFormDialog({
  initial, editing, onClose, onSaved,
}: {
  initial: VPSFormValues;
  editing: VPSServer | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [values, setValues] = useState<VPSFormValues>(initial);
  const [tagsRaw, setTagsRaw] = useState(initial.tags.join(", "));
  const [submitError, setSubmitError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      setSubmitError(null);
      const payload: VPSFormValues = {
        ...values,
        tags: tagsRaw.split(",").map((t) => t.trim()).filter(Boolean),
        renewal_date: values.renewal_date ? values.renewal_date : null,
      };
      return editing ? updateVPS(editing.id, payload) : createVPS(payload);
    },
    onSuccess: () => {
      toast.success(editing ? "VPS diupdate" : "VPS dibuat");
      onSaved();
    },
    onError: (err: unknown) => setSubmitError(describeError(err, "Gagal simpan VPS")),
  });

  return (
    <Dialog open onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{editing ? `Edit ${editing.label}` : "Tambah VPS"}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-5">
          {submitError && (
            <div className="rounded border border-error/40 bg-error-light px-3 py-2 text-sm text-error">{submitError}</div>
          )}

          <FormSection title="Identitas" hint="Label & provider — buat memudahkan kamu cari VPS ini di list.">
            <LabeledInput label="Label" value={values.label} onChange={(v) => setValues({ ...values, label: v })} required placeholder="kantor-prod-1" />
            <LabeledInput label="Provider" value={values.provider} onChange={(v) => setValues({ ...values, provider: v })} placeholder="Hetzner / Contabo / DO" />
            <LabeledInput label="Hostname" value={values.hostname} onChange={(v) => setValues({ ...values, hostname: v })} placeholder="prod-1.example.com" />
            <LabeledInput label="IP Address" value={values.ip_address} onChange={(v) => setValues({ ...values, ip_address: v })} placeholder="203.0.113.10" />
            <LabeledInput label="Region" value={values.region} onChange={(v) => setValues({ ...values, region: v })} placeholder="Singapore / Frankfurt" />
            <LabeledInput label="Tags" value={tagsRaw} onChange={setTagsRaw} placeholder="prod, db, asia" hint="Pisahkan koma. Lowercase otomatis." />
          </FormSection>

          <FormSection title="Spesifikasi" hint="Hanya catatan — tidak dipakai untuk monitoring.">
            <LabeledNumber label="CPU cores" value={values.cpu_cores} onChange={(v) => setValues({ ...values, cpu_cores: v })} />
            <LabeledNumber label="RAM" suffix="MB" value={values.ram_mb} onChange={(v) => setValues({ ...values, ram_mb: v })} />
            <LabeledNumber label="Disk" suffix="GB" value={values.disk_gb} onChange={(v) => setValues({ ...values, disk_gb: v })} />
          </FormSection>

          <FormSection title="Billing" hint="Untuk reminder renewal otomatis.">
            <CostField
              currency={values.cost_currency}
              amount={values.cost_amount}
              onChange={(c, a) => setValues({ ...values, cost_currency: c, cost_amount: a })}
            />
            <LabeledSelect
              label="Billing cycle"
              value={values.billing_cycle}
              onValueChange={(v) => setValues({ ...values, billing_cycle: v as VPSFormValues["billing_cycle"] })}
              options={[
                { value: "monthly", label: "Bulanan" },
                { value: "quarterly", label: "Triwulanan" },
                { value: "yearly", label: "Tahunan" },
              ]}
            />
            <LabeledInput
              label="Renewal date"
              type="date"
              value={values.renewal_date ?? ""}
              onChange={(v) => setValues({ ...values, renewal_date: v || null })}
              hint="Alert dikirim H-7 sebelum tanggal ini."
            />
            <LabeledSelect
              label="Status"
              value={values.status}
              onValueChange={(v) => setValues({ ...values, status: v as VPSFormValues["status"] })}
              options={[
                { value: "active", label: "Active" },
                { value: "suspended", label: "Suspended" },
                { value: "decommissioned", label: "Decommissioned" },
              ]}
            />
          </FormSection>

          <div>
            <label className="mb-1 block text-sm font-medium">Notes</label>
            <textarea
              className="w-full rounded border border-border bg-surface px-3 py-2 text-sm"
              rows={3}
              placeholder="Catatan internal — kredensial root jangan ditaruh sini."
              value={values.notes}
              onChange={(e) => setValues({ ...values, notes: e.target.value })}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>Batal</Button>
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            {mutation.isPending ? "Saving…" : editing ? "Simpan perubahan" : "Tambah VPS"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Check form --------------------------------------------------------

export function CheckFormDialog({
  vpsID, editing, onClose, onSaved,
}: {
  vpsID: string;
  editing: VPSHealthCheck | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [values, setValues] = useState<VPSCheckFormValues>(
    editing
      ? {
          label: editing.label,
          type: editing.type,
          target: editing.target,
          interval_seconds: editing.interval_seconds,
          timeout_seconds: editing.timeout_seconds,
          enabled: editing.enabled,
        }
      : { label: "", type: "https", target: "", interval_seconds: 60, timeout_seconds: 5, enabled: true }
  );
  const [submitError, setSubmitError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      setSubmitError(null);
      return editing
        ? updateVPSCheck(vpsID, editing.id, values)
        : createVPSCheck(vpsID, values);
    },
    onSuccess: () => {
      toast.success(editing ? "Check diupdate" : "Check dibuat");
      onSaved();
    },
    onError: (e: unknown) => setSubmitError(describeError(e, "Gagal simpan check")),
  });

  return (
    <Dialog open onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editing ? `Edit ${editing.label}` : "Tambah Health Check"}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {submitError && (
            <div className="rounded border border-error/40 bg-error-light px-3 py-2 text-sm text-error">{submitError}</div>
          )}
          <Card className="bg-info-light/40 p-3 text-xs text-text-secondary">
            <strong className="text-text-primary">Tipe check:</strong>
            <ul className="mt-1 space-y-0.5 list-disc pl-4">
              <li><strong>HTTPS</strong>: GET URL + cek SSL expiry. Target: full URL.</li>
              <li><strong>HTTP</strong>: GET URL tanpa SSL. Target: full URL.</li>
              <li><strong>TCP</strong>: handshake socket. Target: <code>host:port</code>.</li>
              <li><strong>ICMP</strong>: ping (fallback ke TCP:80 di container). Target: hostname / IP.</li>
            </ul>
          </Card>

          <LabeledInput label="Label" value={values.label} onChange={(v) => setValues({ ...values, label: v })} required placeholder="api uptime" />
          <LabeledSelect
            label="Type"
            value={values.type}
            onValueChange={(v) => setValues({ ...values, type: v as VPSCheckFormValues["type"] })}
            options={[
              { value: "https", label: "HTTPS (URL + SSL)" },
              { value: "http", label: "HTTP (URL)" },
              { value: "tcp", label: "TCP (host:port)" },
              { value: "icmp", label: "ICMP / ping (host)" },
            ]}
          />
          <LabeledInput
            label="Target"
            value={values.target}
            onChange={(v) => setValues({ ...values, target: v })}
            placeholder={values.type === "tcp" ? "host:port" : values.type.startsWith("http") ? "https://example.com" : "example.com"}
            required
          />
          <div className="grid gap-3 md:grid-cols-2">
            <LabeledNumber label="Interval" suffix="detik" value={values.interval_seconds} onChange={(v) => setValues({ ...values, interval_seconds: v })} hint="Min 30, max 86400." />
            <LabeledNumber label="Timeout" suffix="detik" value={values.timeout_seconds} onChange={(v) => setValues({ ...values, timeout_seconds: v })} hint="Min 1, max 60." />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={values.enabled ?? true}
              onChange={(e) => setValues({ ...values, enabled: e.target.checked })}
            />
            Enabled — uncheck untuk pause check tanpa hapus
          </label>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>Batal</Button>
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            {mutation.isPending ? "Saving…" : editing ? "Simpan perubahan" : "Tambah Check"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------- App form ----------------------------------------------------------

export function AppFormDialog({
  vpsID, editing, checks, onClose, onSaved,
}: {
  vpsID: string;
  editing: VPSApp | null;
  checks: VPSHealthCheck[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [values, setValues] = useState<VPSAppFormValues>(
    editing
      ? {
          name: editing.name,
          app_type: editing.app_type,
          port: editing.port ?? null,
          url: editing.url,
          notes: editing.notes,
          check_id: editing.check_id ?? null,
        }
      : { name: "", app_type: "", port: null, url: "", notes: "", check_id: null }
  );
  const [submitError, setSubmitError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      setSubmitError(null);
      return editing
        ? updateVPSApp(vpsID, editing.id, values)
        : createVPSApp(vpsID, values);
    },
    onSuccess: () => {
      toast.success(editing ? "App diupdate" : "App dibuat");
      onSaved();
    },
    onError: (e: unknown) => setSubmitError(describeError(e, "Gagal simpan app")),
  });

  return (
    <Dialog open onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editing ? `Edit ${editing.name}` : "Tambah App"}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {submitError && (
            <div className="rounded border border-error/40 bg-error-light px-3 py-2 text-sm text-error">{submitError}</div>
          )}
          <p className="text-xs text-text-tertiary">App = service yang jalan di VPS ini (postgres, nginx, cron, dll). Link ke check kalau mau dimonitor uptime-nya.</p>
          <LabeledInput label="Nama" value={values.name} onChange={(v) => setValues({ ...values, name: v })} required placeholder="postgres-prod" />
          <LabeledInput label="Tipe" value={values.app_type} onChange={(v) => setValues({ ...values, app_type: v })} placeholder="database / web / cache / cron" />
          <div className="grid gap-3 md:grid-cols-2">
            <LabeledNumber label="Port" value={values.port ?? 0} onChange={(v) => setValues({ ...values, port: v || null })} hint="0 = tidak ada port spesifik" />
            <LabeledInput label="URL" value={values.url} onChange={(v) => setValues({ ...values, url: v })} placeholder="https://api.example.com" />
          </div>
          <LabeledSelect
            label="Linked check (opsional)"
            value={values.check_id ?? ""}
            onValueChange={(v) => setValues({ ...values, check_id: v || null })}
            options={[{ value: "", label: "— Tidak dimonitor —" }, ...checks.map((c) => ({ value: c.id, label: `${c.label} (${c.type})` }))]}
            hint="Pilih check yang sudah dibuat untuk lihat uptime per app."
          />
          <div>
            <label className="mb-1 block text-sm font-medium">Notes</label>
            <textarea
              className="w-full rounded border border-border bg-surface px-3 py-2 text-sm"
              rows={3}
              value={values.notes}
              onChange={(e) => setValues({ ...values, notes: e.target.value })}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>Batal</Button>
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            {mutation.isPending ? "Saving…" : editing ? "Simpan perubahan" : "Tambah App"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

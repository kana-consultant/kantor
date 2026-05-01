import { useState } from "react";
import { useMutation } from "@tanstack/react-query";

import { ApiError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { createDomain, updateDomain } from "@/services/operational-domains";
import { toast } from "@/stores/toast-store";
import type {
  Domain,
  DomainCheckStatus,
  DomainFormValues,
} from "@/types/domain";

export const DNS_TONES: Record<DomainCheckStatus, string> = {
  up: "bg-success-light text-success",
  down: "bg-error-light text-error",
  unknown: "bg-surface-muted text-text-tertiary",
};

export const DNS_LABELS: Record<DomainCheckStatus, string> = {
  up: "DNS Up",
  down: "DNS Down",
  unknown: "DNS Unknown",
};

export function DNSPill({ status }: { status: DomainCheckStatus }) {
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium", DNS_TONES[status])}>
      <span className="h-1.5 w-1.5 rounded-full bg-current" />
      {DNS_LABELS[status]}
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

export function daysUntil(iso?: string | null): number | null {
  if (!iso) return null;
  const d = new Date(iso).getTime();
  if (Number.isNaN(d)) return null;
  return Math.ceil((d - Date.now()) / 86_400_000);
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

export function emptyDomainForm(): DomainFormValues {
  return {
    name: "",
    registrar: "",
    nameservers: [],
    expiry_date: null,
    cost_amount: 0,
    cost_currency: "IDR",
    billing_cycle: "yearly",
    status: "active",
    tags: [],
    notes: "",
    dns_check_enabled: true,
    dns_expected_ip: "",
    dns_check_interval_seconds: 3600,
    whois_sync_enabled: true,
  };
}

export function fromDomain(d: Domain): DomainFormValues {
  return {
    name: d.name,
    registrar: d.registrar,
    nameservers: d.nameservers,
    expiry_date: d.expiry_date ?? null,
    cost_amount: d.cost_amount,
    cost_currency: d.cost_currency,
    billing_cycle: d.billing_cycle,
    status: d.status,
    tags: d.tags,
    notes: d.notes,
    dns_check_enabled: d.dns_check_enabled,
    dns_expected_ip: d.dns_expected_ip,
    dns_check_interval_seconds: d.dns_check_interval_seconds || 3600,
    whois_sync_enabled: d.whois_sync_enabled,
  };
}

const CURRENCY_OPTIONS = [
  { value: "IDR", label: "IDR" },
  { value: "USD", label: "USD" },
  { value: "EUR", label: "EUR" },
  { value: "SGD", label: "SGD" },
  { value: "MYR", label: "MYR" },
];

function FormSection({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
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

function LabeledInput({
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

function LabeledNumber({
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

function LabeledSelect({
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

function CostField({
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
    </div>
  );
}

export function DomainFormDialog({
  initial, editing, onClose, onSaved,
}: {
  initial: DomainFormValues;
  editing: Domain | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [values, setValues] = useState<DomainFormValues>(initial);
  const [tagsRaw, setTagsRaw] = useState(initial.tags.join(", "));
  const [nsRaw, setNsRaw] = useState(initial.nameservers.join(", "));
  const [submitError, setSubmitError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      setSubmitError(null);
      const payload: DomainFormValues = {
        ...values,
        tags: tagsRaw.split(",").map((t) => t.trim()).filter(Boolean),
        nameservers: nsRaw.split(",").map((t) => t.trim()).filter(Boolean),
        expiry_date: values.expiry_date ? values.expiry_date : null,
      };
      return editing ? updateDomain(editing.id, payload) : createDomain(payload);
    },
    onSuccess: () => {
      toast.success(editing ? "Domain diupdate" : "Domain dibuat");
      onSaved();
    },
    onError: (err: unknown) => setSubmitError(describeError(err, "Gagal simpan domain")),
  });

  return (
    <Dialog open onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{editing ? `Edit ${editing.name}` : "Tambah Domain"}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-5">
          {submitError && (
            <div className="rounded border border-error/40 bg-error-light px-3 py-2 text-sm text-error">{submitError}</div>
          )}

          <FormSection title="Identitas" hint="Nama domain + registrar — buat memudahkan tracking renewal.">
            <LabeledInput
              label="Nama domain"
              value={values.name}
              onChange={(v) => setValues({ ...values, name: v })}
              required
              placeholder="example.com"
              hint="Lowercase otomatis."
            />
            <LabeledInput
              label="Registrar"
              value={values.registrar}
              onChange={(v) => setValues({ ...values, registrar: v })}
              placeholder="Namecheap / Cloudflare / Niagahoster"
            />
            <LabeledInput
              label="Nameservers"
              value={nsRaw}
              onChange={setNsRaw}
              placeholder="ns1.example.com, ns2.example.com"
              hint="Pisahkan koma."
            />
            <LabeledInput label="Tags" value={tagsRaw} onChange={setTagsRaw} placeholder="prod, marketing" hint="Pisahkan koma." />
          </FormSection>

          <FormSection title="Billing" hint="Tagihan & expiry date — alert renewal H-30 sebelum expired.">
            <CostField
              currency={values.cost_currency}
              amount={values.cost_amount}
              onChange={(c, a) => setValues({ ...values, cost_currency: c, cost_amount: a })}
            />
            <LabeledSelect
              label="Billing cycle"
              value={values.billing_cycle}
              onValueChange={(v) => setValues({ ...values, billing_cycle: v as DomainFormValues["billing_cycle"] })}
              options={[
                { value: "yearly", label: "Tahunan" },
                { value: "monthly", label: "Bulanan" },
              ]}
            />
            <LabeledInput
              label="Expiry date"
              type="date"
              value={values.expiry_date ?? ""}
              onChange={(v) => setValues({ ...values, expiry_date: v || null })}
              hint="Kalau WHOIS sync aktif, akan auto-update tiap 24 jam."
            />
            <LabeledSelect
              label="Status"
              value={values.status}
              onValueChange={(v) => setValues({ ...values, status: v as DomainFormValues["status"] })}
              options={[
                { value: "active", label: "Active" },
                { value: "expired", label: "Expired" },
                { value: "transferring", label: "Transferring" },
                { value: "parked", label: "Parked" },
              ]}
            />
          </FormSection>

          <FormSection title="Monitoring otomatis" hint="DNS check + WHOIS sync. Optional.">
            <label className="flex items-center gap-2 text-sm md:col-span-2">
              <input
                type="checkbox"
                checked={values.dns_check_enabled}
                onChange={(e) => setValues({ ...values, dns_check_enabled: e.target.checked })}
              />
              <span>
                <strong>DNS check</strong> — resolve domain berkala, alert kalau gagal.
              </span>
            </label>
            <LabeledNumber
              label="DNS interval"
              suffix="detik"
              value={values.dns_check_interval_seconds}
              onChange={(v) => setValues({ ...values, dns_check_interval_seconds: v })}
              hint="Min 60, max 86400 (1 hari). Default 3600 (1 jam)."
            />
            <LabeledInput
              label="Expected IP (opsional)"
              value={values.dns_expected_ip}
              onChange={(v) => setValues({ ...values, dns_expected_ip: v })}
              placeholder="203.0.113.10"
              hint="Kalau diisi, domain dianggap up hanya kalau resolve ke IP ini."
            />
            <label className="flex items-center gap-2 text-sm md:col-span-2">
              <input
                type="checkbox"
                checked={values.whois_sync_enabled}
                onChange={(e) => setValues({ ...values, whois_sync_enabled: e.target.checked })}
              />
              <span>
                <strong>WHOIS auto-sync</strong> — sinkron expiry date dari registry tiap 24 jam.
              </span>
            </label>
          </FormSection>

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
            {mutation.isPending ? "Saving…" : editing ? "Simpan perubahan" : "Tambah Domain"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export { Card };

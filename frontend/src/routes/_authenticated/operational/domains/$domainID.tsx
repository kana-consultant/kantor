import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, CheckCircle2, Clock, Pencil, RefreshCw, ShieldAlert, Trash2, XCircle } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  DNSPill,
  DomainFormDialog,
  daysUntil,
  formatDateTime,
  fromDomain,
  relativeTime,
} from "@/components/domain/shared";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { cn } from "@/lib/utils";
import { deleteDomain, domainKeys, getDomain } from "@/services/operational-domains";
import { toast } from "@/stores/toast-store";
import type { Domain, DomainHealthEvent } from "@/types/domain";

export const Route = createFileRoute("/_authenticated/operational/domains/$domainID")({
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalDomainView);
  },
  component: DomainDetailPage,
});

function DomainDetailPage() {
  const { domainID } = Route.useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canEdit = hasPermission(permissions.operationalDomainEdit);
  const canDelete = hasPermission(permissions.operationalDomainDelete);

  const detailQuery = useQuery({
    queryKey: domainKeys.detail(domainID),
    queryFn: () => getDomain(domainID),
    refetchInterval: 60_000,
  });
  const detail = detailQuery.data;

  const [editOpen, setEditOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const deleteMutation = useMutation({
    mutationFn: () => deleteDomain(domainID),
    onSuccess: () => {
      toast.success("Domain dihapus");
      queryClient.invalidateQueries({ queryKey: domainKeys.all });
      navigate({ to: "/operational/domains" });
    },
    onError: (err: Error) => toast.error(err.message ?? "Gagal hapus domain"),
  });

  if (detailQuery.isLoading) {
    return <div className="p-6 text-text-tertiary">Loading…</div>;
  }
  if (!detail) {
    return (
      <div className="p-6">
        <Link to="/operational/domains" className="text-info hover:underline">← Kembali</Link>
        <p className="mt-4 text-text-tertiary">Domain tidak ditemukan.</p>
      </div>
    );
  }

  const { domain, events } = detail;
  const days = daysUntil(domain.expiry_date);

  return (
    <div className="space-y-5 p-6">
      <div className="flex items-center justify-between">
        <Link to="/operational/domains" className="inline-flex items-center gap-1 text-sm text-text-secondary hover:text-info">
          <ArrowLeft className="h-4 w-4" /> Semua Domain
        </Link>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={() => detailQuery.refetch()}>
            <RefreshCw className={cn("mr-1 h-4 w-4", detailQuery.isFetching && "animate-spin")} /> Refresh
          </Button>
          {canEdit && (
            <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
              <Pencil className="mr-1 h-4 w-4" /> Edit
            </Button>
          )}
          {canDelete && (
            <Button variant="danger" size="sm" onClick={() => setConfirmDelete(true)}>
              <Trash2 className="mr-1 h-4 w-4" /> Hapus
            </Button>
          )}
        </div>
      </div>

      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">{domain.name}</h1>
          <p className="text-sm text-text-secondary">{domain.registrar || "Tanpa registrar"}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {domain.dns_check_enabled && <DNSPill status={domain.dns_last_status} />}
          {domain.dns_alert_active && (
            <span className="inline-flex items-center gap-1 rounded-full bg-error-light px-2 py-0.5 text-xs font-medium text-error">
              <ShieldAlert className="h-3 w-3" /> Alert aktif
            </span>
          )}
        </div>
      </header>

      <Card className="grid gap-3 p-4 md:grid-cols-3">
        <Field label="Status" value={<span className="capitalize">{domain.status}</span>} />
        <Field
          label="Expiry date"
          value={
            domain.expiry_date
              ? <span className={cn(
                  days === null ? ""
                  : days < 0 ? "text-error font-semibold"
                  : days <= 7 ? "text-error font-semibold"
                  : days <= 30 ? "text-warning font-semibold"
                  : ""
                )}>
                  {domain.expiry_date}
                  {days !== null && (
                    <span className="ml-1 text-xs">
                      ({days < 0 ? `expired ${-days} hari lalu` : `${days} hari lagi`})
                    </span>
                  )}
                </span>
              : "—"
          }
        />
        <Field label="Billing" value={domain.cost_amount ? `${domain.cost_currency} ${domain.cost_amount.toLocaleString("id-ID")} / ${domain.billing_cycle === "yearly" ? "tahun" : "bulan"}` : "—"} />
        <Field label="Nameservers" value={domain.nameservers.length ? domain.nameservers.join(", ") : "—"} />
        <Field label="Tags" value={domain.tags.length ? domain.tags.join(", ") : "—"} />
        <Field label="Last DNS check" value={relativeTime(domain.dns_last_check_at)} />
        {domain.notes && (
          <div className="col-span-full">
            <div className="text-xs uppercase text-text-tertiary">Notes</div>
            <p className="mt-1 whitespace-pre-wrap rounded bg-surface-muted p-2 text-sm">{domain.notes}</p>
          </div>
        )}
      </Card>

      <DNSSection domain={domain} />
      <WhoisSection domain={domain} />
      <EventsSection events={events} />

      {editOpen && (
        <DomainFormDialog
          initial={fromDomain(domain)}
          editing={domain}
          onClose={() => setEditOpen(false)}
          onSaved={() => {
            queryClient.invalidateQueries({ queryKey: domainKeys.detail(domainID) });
            queryClient.invalidateQueries({ queryKey: domainKeys.all });
            setEditOpen(false);
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDialog
          isOpen
          title={`Hapus domain ${domain.name}?`}
          description="Tindakan ini akan menghapus domain + semua event historisnya."
          confirmLabel="Hapus"
          tone="danger"
          isLoading={deleteMutation.isPending}
          onConfirm={() => deleteMutation.mutate()}
          onClose={() => setConfirmDelete(false)}
        />
      )}
    </div>
  );
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <div className="text-xs uppercase text-text-tertiary">{label}</div>
      <div className="mt-0.5 text-sm font-medium">{value}</div>
    </div>
  );
}

function DNSSection({ domain }: { domain: Domain }) {
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-base font-semibold">DNS Resolution Check</h2>
        <p className="text-xs text-text-tertiary">
          {domain.dns_check_enabled
            ? `Berjalan tiap ${domain.dns_check_interval_seconds}s. Probe terbaru: ${relativeTime(domain.dns_last_check_at)}.`
            : "Disabled. Aktifkan via tombol Edit."}
        </p>
      </div>
      {domain.dns_check_enabled ? (
        <Card className="p-4">
          <div className="grid gap-3 md:grid-cols-2">
            <Field
              label="Last status"
              value={
                <span className="inline-flex items-center gap-2">
                  {domain.dns_last_status === "up"
                    ? <CheckCircle2 className="h-4 w-4 text-success" />
                    : domain.dns_last_status === "down"
                    ? <XCircle className="h-4 w-4 text-error" />
                    : <Clock className="h-4 w-4 text-text-tertiary" />}
                  <span className="capitalize">{domain.dns_last_status}</span>
                </span>
              }
            />
            <Field label="Resolved IPs" value={domain.dns_last_resolved_ips.length ? domain.dns_last_resolved_ips.join(", ") : "—"} />
            <Field label="Expected IP" value={domain.dns_expected_ip || "—"} />
            <Field label="Consecutive fails" value={domain.dns_consecutive_fails > 0 ? <span className="text-error">{domain.dns_consecutive_fails}</span> : "0"} />
          </div>
          {domain.dns_last_error && (
            <div className="mt-3 rounded bg-error-light px-2 py-1 text-xs text-error">{domain.dns_last_error}</div>
          )}
        </Card>
      ) : (
        <Card className="p-4 text-sm text-text-tertiary">DNS check di-disable.</Card>
      )}
    </section>
  );
}

function WhoisSection({ domain }: { domain: Domain }) {
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-base font-semibold">WHOIS Auto-Sync</h2>
        <p className="text-xs text-text-tertiary">
          {domain.whois_sync_enabled
            ? `Sinkron tiap 24 jam. Update field expiry_date kalau registry merespon.`
            : "Disabled. Aktifkan via tombol Edit."}
        </p>
      </div>
      <Card className="p-4">
        <div className="grid gap-3 md:grid-cols-2">
          <Field label="Last sync" value={relativeTime(domain.whois_last_sync_at)} />
          <Field
            label="Status"
            value={domain.whois_last_error
              ? <span className="text-error">Error</span>
              : domain.whois_last_sync_at
              ? <span className="text-success">OK</span>
              : <span className="text-text-tertiary">Belum pernah sync</span>}
          />
        </div>
        {domain.whois_last_error && (
          <div className="mt-3 rounded bg-error-light px-2 py-1 text-xs text-error">{domain.whois_last_error}</div>
        )}
      </Card>
    </section>
  );
}

function EventsSection({ events }: { events: DomainHealthEvent[] }) {
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-base font-semibold">Event terakhir</h2>
        <p className="text-xs text-text-tertiary">100 event terakhir. Yang &gt;7 hari otomatis dihapus.</p>
      </div>
      {events.length === 0 ? (
        <Card className="p-6 text-center text-sm text-text-tertiary">Belum ada event.</Card>
      ) : (
        <Card className="overflow-hidden">
          <table className="w-full text-xs">
            <thead className="bg-surface-muted text-left text-text-tertiary">
              <tr>
                <th className="px-3 py-2">Time</th>
                <th className="px-3 py-2">Type</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2">Detail</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id} className="border-t border-border-subtle">
                  <td className="px-3 py-2">{formatDateTime(e.created_at)}</td>
                  <td className="px-3 py-2 uppercase">{e.event_type}</td>
                  <td className={cn(
                    "px-3 py-2 capitalize",
                    e.status === "up" || e.status === "synced" ? "text-success"
                    : e.status === "down" || e.status === "error" ? "text-error"
                    : "",
                  )}>{e.status}</td>
                  <td className="px-3 py-2 text-text-secondary">{e.detail || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </section>
  );
}

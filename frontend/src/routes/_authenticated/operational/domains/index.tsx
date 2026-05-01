import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { ChevronRight, Globe, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  DNSPill,
  DomainFormDialog,
  daysUntil,
  emptyDomainForm,
  fromDomain,
  relativeTime,
} from "@/components/domain/shared";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { cn } from "@/lib/utils";
import { deleteDomain, domainKeys, listDomains } from "@/services/operational-domains";
import { toast } from "@/stores/toast-store";
import type { Domain, DomainListFilters } from "@/types/domain";

export const Route = createFileRoute("/_authenticated/operational/domains/")({
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalDomainView);
  },
  component: DomainListPage,
});

const STATUS_OPTIONS = [
  { value: "", label: "Semua status" },
  { value: "active", label: "Active" },
  { value: "expired", label: "Expired" },
  { value: "transferring", label: "Transferring" },
  { value: "parked", label: "Parked" },
];

function DomainListPage() {
  const { hasPermission } = useRBAC();
  const canCreate = hasPermission(permissions.operationalDomainCreate);
  const canEdit = hasPermission(permissions.operationalDomainEdit);
  const canDelete = hasPermission(permissions.operationalDomainDelete);

  const [filters, setFilters] = useState<DomainListFilters>({ search: "", status: "", registrar: "", tag: "" });
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Domain | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<Domain | null>(null);

  const queryClient = useQueryClient();
  const listQuery = useQuery({
    queryKey: domainKeys.list(filters),
    queryFn: () => listDomains(filters),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteDomain(id),
    onSuccess: () => {
      toast.success("Domain dihapus");
      queryClient.invalidateQueries({ queryKey: domainKeys.all });
      setConfirmDelete(null);
    },
    onError: (err: Error) => toast.error(err.message ?? "Gagal hapus domain"),
  });

  const domains = listQuery.data ?? [];
  const totals = aggregate(domains);

  return (
    <div className="space-y-5 p-6">
      <header className="flex items-start justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <Globe className="h-5 w-5" /> Domains
          </h1>
          <p className="text-sm text-text-secondary">
            Inventaris domain + renewal alert + DNS check + WHOIS auto-sync. Klik domain untuk detail.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={() => listQuery.refetch()}>
            <RefreshCw className={cn("mr-1 h-4 w-4", listQuery.isFetching && "animate-spin")} /> Refresh
          </Button>
          {canCreate && (
            <Button size="sm" onClick={() => { setEditing(null); setFormOpen(true); }}>
              <Plus className="mr-1 h-4 w-4" /> Tambah Domain
            </Button>
          )}
        </div>
      </header>

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <SummaryCard label="Total" value={totals.total} tone="neutral" />
        <SummaryCard label="Expiring ≤30 hari" value={totals.expiringSoon} tone="warning" />
        <SummaryCard label="DNS Down" value={totals.dnsDown} tone="error" />
        <SummaryCard label="Expired" value={totals.expired} tone="error" />
      </div>

      <Card className="p-3">
        <div className="grid gap-2 md:grid-cols-4">
          <Input
            placeholder="Cari nama / registrar"
            value={filters.search ?? ""}
            onChange={(e) => setFilters((f) => ({ ...f, search: e.target.value }))}
          />
          <Select
            value={filters.status ?? ""}
            options={STATUS_OPTIONS}
            onValueChange={(v) => setFilters((f) => ({ ...f, status: v as DomainListFilters["status"] }))}
          />
          <Input
            placeholder="Registrar"
            value={filters.registrar ?? ""}
            onChange={(e) => setFilters((f) => ({ ...f, registrar: e.target.value }))}
          />
          <Input
            placeholder="Tag"
            value={filters.tag ?? ""}
            onChange={(e) => setFilters((f) => ({ ...f, tag: e.target.value }))}
          />
        </div>
      </Card>

      {listQuery.isLoading && (
        <Card className="p-8 text-center text-text-tertiary">Loading…</Card>
      )}

      {!listQuery.isLoading && domains.length === 0 && (
        <EmptyState onCreate={canCreate ? () => { setEditing(null); setFormOpen(true); } : undefined} />
      )}

      {domains.length > 0 && (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {domains.map((d) => (
            <DomainCard
              key={d.id}
              domain={d}
              canEdit={canEdit}
              canDelete={canDelete}
              onEdit={() => { setEditing(d); setFormOpen(true); }}
              onDelete={() => setConfirmDelete(d)}
            />
          ))}
        </div>
      )}

      {formOpen && (
        <DomainFormDialog
          initial={editing ? fromDomain(editing) : emptyDomainForm()}
          editing={editing}
          onClose={() => { setFormOpen(false); setEditing(null); }}
          onSaved={() => {
            queryClient.invalidateQueries({ queryKey: domainKeys.all });
            setFormOpen(false); setEditing(null);
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDialog
          isOpen
          title={`Hapus domain ${confirmDelete.name}?`}
          description="Tindakan ini akan menghapus domain + semua event historisnya."
          confirmLabel="Hapus"
          tone="danger"
          isLoading={deleteMutation.isPending}
          onConfirm={() => deleteMutation.mutate(confirmDelete.id)}
          onClose={() => setConfirmDelete(null)}
        />
      )}
    </div>
  );
}

function aggregate(domains: Domain[]) {
  let expiringSoon = 0, dnsDown = 0, expired = 0;
  for (const d of domains) {
    const days = daysUntil(d.expiry_date);
    if (days !== null && days >= 0 && days <= 30) expiringSoon++;
    if (d.dns_last_status === "down") dnsDown++;
    if (d.status === "expired") expired++;
  }
  return { total: domains.length, expiringSoon, dnsDown, expired };
}

function SummaryCard({ label, value, tone }: { label: string; value: number; tone: "neutral" | "warning" | "error" }) {
  const toneCls: Record<typeof tone, string> = {
    neutral: "text-text-primary",
    warning: "text-warning",
    error: "text-error",
  };
  return (
    <Card className="p-3">
      <div className="text-xs uppercase text-text-tertiary">{label}</div>
      <div className={cn("text-2xl font-semibold", toneCls[tone])}>{value}</div>
    </Card>
  );
}

function DomainCard({
  domain, canEdit, canDelete, onEdit, onDelete,
}: {
  domain: Domain;
  canEdit: boolean;
  canDelete: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const days = daysUntil(domain.expiry_date);
  const expiryTone =
    days === null ? "text-text-tertiary"
    : days < 0 ? "text-error"
    : days <= 7 ? "text-error"
    : days <= 30 ? "text-warning"
    : "text-text-secondary";
  const expiryLabel =
    days === null ? "—"
    : days < 0 ? `Expired ${-days} hari lalu`
    : days === 0 ? "Expired hari ini"
    : `${days} hari lagi (${domain.expiry_date})`;

  return (
    <Card className="flex flex-col gap-3 p-4 transition-shadow hover:shadow-md">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <Link
            to="/operational/domains/$domainID"
            params={{ domainID: domain.id }}
            className="block truncate text-base font-semibold text-info hover:underline"
          >
            {domain.name}
          </Link>
          <div className="truncate text-xs text-text-tertiary">{domain.registrar || "—"}</div>
        </div>
        {domain.dns_check_enabled && <DNSPill status={domain.dns_last_status} />}
      </div>

      <div className="grid grid-cols-2 gap-2 text-xs">
        <InfoRow label="Status" value={<span className="capitalize">{domain.status}</span>} />
        <InfoRow label="Renewal" valueClass={expiryTone} value={expiryLabel} />
        <InfoRow label="Last DNS check" value={relativeTime(domain.dns_last_check_at)} />
        <InfoRow label="WHOIS sync" value={domain.whois_sync_enabled ? relativeTime(domain.whois_last_sync_at) : "Off"} />
      </div>

      {domain.dns_alert_active && (
        <div className="rounded bg-error-light px-2 py-1 text-xs text-error">DNS alert aktif — {domain.dns_consecutive_fails}× fail</div>
      )}

      {domain.tags.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {domain.tags.map((t) => (
            <span key={t} className="rounded-full bg-surface-muted px-2 py-0.5 text-[10px] uppercase text-text-tertiary">{t}</span>
          ))}
        </div>
      )}

      <div className="flex items-center justify-between border-t border-border-subtle pt-2">
        <Link
          to="/operational/domains/$domainID"
          params={{ domainID: domain.id }}
          className="inline-flex items-center text-xs font-medium text-info hover:underline"
        >
          Buka detail <ChevronRight className="h-3 w-3" />
        </Link>
        <div className="flex gap-1">
          {canEdit && (
            <Button variant="ghost" size="sm" onClick={onEdit} title="Edit domain">
              <Pencil className="h-4 w-4" />
            </Button>
          )}
          {canDelete && (
            <Button variant="ghost" size="sm" onClick={onDelete} title="Hapus domain">
              <Trash2 className="h-4 w-4 text-error" />
            </Button>
          )}
        </div>
      </div>
    </Card>
  );
}

function InfoRow({ label, value, valueClass }: { label: string; value: React.ReactNode; valueClass?: string }) {
  return (
    <div className="min-w-0">
      <div className="text-text-tertiary">{label}</div>
      <div className={cn("truncate font-medium", valueClass ?? "text-text-primary")}>{value}</div>
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate?: () => void }) {
  return (
    <Card className="flex flex-col items-center gap-3 p-10 text-center">
      <Globe className="h-10 w-10 text-text-tertiary" />
      <div>
        <h3 className="text-base font-semibold">Belum ada domain terdaftar</h3>
        <p className="mt-1 max-w-md text-sm text-text-secondary">
          Tambah domain pertama untuk mulai monitor renewal, DNS resolution, dan auto-sync expiry dari WHOIS.
        </p>
      </div>
      {onCreate && (
        <Button onClick={onCreate}>
          <Plus className="mr-1 h-4 w-4" /> Tambah Domain pertama
        </Button>
      )}
    </Card>
  );
}

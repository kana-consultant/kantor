import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute } from "@tanstack/react-router";
import { ChevronRight, Globe, Pencil, Plus, RefreshCw, Server, Trash2 } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  HealthPill,
  VPSFormDialog,
  emptyVPSForm,
  fromServer,
  relativeTime,
} from "@/components/vps/shared";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { cn } from "@/lib/utils";
import { deleteVPS, listVPS, vpsKeys } from "@/services/operational-vps";
import { toast } from "@/stores/toast-store";
import type { VPSListFilters, VPSServer, VPSServerSummary } from "@/types/vps";

export const Route = createFileRoute("/_authenticated/operational/vps/")({
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalVPSView);
  },
  component: VPSListPage,
});

const STATUS_OPTIONS = [
  { value: "", label: "Semua status" },
  { value: "active", label: "Active" },
  { value: "suspended", label: "Suspended" },
  { value: "decommissioned", label: "Decommissioned" },
];

function VPSListPage() {
  const { hasPermission } = useRBAC();
  const canCreate = hasPermission(permissions.operationalVPSCreate);
  const canEdit = hasPermission(permissions.operationalVPSEdit);
  const canDelete = hasPermission(permissions.operationalVPSDelete);

  const [filters, setFilters] = useState<VPSListFilters>({ search: "", status: "", provider: "", tag: "" });
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<VPSServer | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<VPSServer | null>(null);

  const queryClient = useQueryClient();
  const listQuery = useQuery({
    queryKey: vpsKeys.list(filters),
    queryFn: () => listVPS(filters),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteVPS(id),
    onSuccess: () => {
      toast.success("VPS dihapus");
      queryClient.invalidateQueries({ queryKey: vpsKeys.all });
      setConfirmDelete(null);
    },
    onError: (err: Error) => toast.error(err.message ?? "Gagal hapus VPS"),
  });

  const servers = listQuery.data ?? [];
  const totals = aggregate(servers);

  return (
    <div className="space-y-5 p-6">
      <header className="flex items-start justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <Server className="h-5 w-5" /> VPS Monitor
          </h1>
          <p className="text-sm text-text-secondary">
            Inventaris VPS lintas provider + uptime checks. Klik sebuah VPS untuk kelola apps & checks.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={() => listQuery.refetch()}>
            <RefreshCw className={cn("mr-1 h-4 w-4", listQuery.isFetching && "animate-spin")} /> Refresh
          </Button>
          {canCreate && (
            <Button size="sm" onClick={() => { setEditing(null); setFormOpen(true); }}>
              <Plus className="mr-1 h-4 w-4" /> Tambah VPS
            </Button>
          )}
        </div>
      </header>

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <SummaryCard label="Total VPS" value={totals.total} tone="neutral" />
        <SummaryCard label="Up" value={totals.up} tone="success" />
        <SummaryCard label="Degraded" value={totals.degraded} tone="warning" />
        <SummaryCard label="Down" value={totals.down} tone="error" />
      </div>

      <Card className="p-3">
        <div className="grid gap-2 md:grid-cols-4">
          <Input
            placeholder="Cari label / hostname / IP"
            value={filters.search ?? ""}
            onChange={(e) => setFilters((f) => ({ ...f, search: e.target.value }))}
          />
          <Select
            value={filters.status ?? ""}
            options={STATUS_OPTIONS}
            onValueChange={(v) => setFilters((f) => ({ ...f, status: v as VPSListFilters["status"] }))}
          />
          <Input
            placeholder="Provider"
            value={filters.provider ?? ""}
            onChange={(e) => setFilters((f) => ({ ...f, provider: e.target.value }))}
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

      {!listQuery.isLoading && servers.length === 0 && (
        <EmptyState onCreate={canCreate ? () => { setEditing(null); setFormOpen(true); } : undefined} />
      )}

      {servers.length > 0 && (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {servers.map((s) => (
            <VPSCard
              key={s.id}
              server={s}
              canEdit={canEdit}
              canDelete={canDelete}
              onEdit={() => { setEditing(s); setFormOpen(true); }}
              onDelete={() => setConfirmDelete(s)}
            />
          ))}
        </div>
      )}

      {formOpen && (
        <VPSFormDialog
          initial={editing ? fromServer(editing) : emptyVPSForm()}
          editing={editing}
          onClose={() => { setFormOpen(false); setEditing(null); }}
          onSaved={() => {
            queryClient.invalidateQueries({ queryKey: vpsKeys.all });
            setFormOpen(false); setEditing(null);
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDialog
          isOpen
          title={`Hapus VPS ${confirmDelete.label}?`}
          description="Tindakan ini akan menghapus VPS, semua check, app, dan event historisnya."
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

function aggregate(servers: VPSServerSummary[]) {
  const totals = { total: servers.length, up: 0, degraded: 0, down: 0, unknown: 0 };
  servers.forEach((s) => {
    if (s.last_status === "up") totals.up++;
    else if (s.last_status === "degraded") totals.degraded++;
    else if (s.last_status === "down") totals.down++;
    else totals.unknown++;
  });
  return totals;
}

function SummaryCard({ label, value, tone }: { label: string; value: number; tone: "neutral" | "success" | "warning" | "error" }) {
  const toneCls: Record<typeof tone, string> = {
    neutral: "text-text-primary",
    success: "text-success",
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

function VPSCard({
  server, canEdit, canDelete, onEdit, onDelete,
}: {
  server: VPSServerSummary;
  canEdit: boolean;
  canDelete: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <Card className="flex flex-col gap-3 p-4 transition-shadow hover:shadow-md">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <Link
            to="/operational/vps/$vpsID"
            params={{ vpsID: server.id }}
            className="block truncate text-base font-semibold text-info hover:underline"
          >
            {server.label}
          </Link>
          <div className="truncate text-xs text-text-tertiary">
            {[server.provider, server.region].filter(Boolean).join(" · ") || "—"}
          </div>
        </div>
        <HealthPill status={server.last_status} />
      </div>

      <div className="grid grid-cols-2 gap-2 text-xs">
        <InfoRow icon={<Globe className="h-3 w-3" />} label="Host" value={server.hostname || server.ip_address || "—"} />
        <InfoRow label="Status" value={<span className="capitalize">{server.status}</span>} />
        <InfoRow label="Last check" value={relativeTime(server.last_check_at)} />
        <InfoRow label="Renewal" value={server.renewal_date ?? "—"} />
      </div>

      <div className="flex gap-2 border-t border-border-subtle pt-3">
        <CountBadge
          label="Apps"
          count={server.apps_count}
          tone="neutral"
          to={`/operational/vps/${server.id}#apps`}
        />
        <CountBadge
          label="Checks"
          count={server.checks_count}
          tone={server.down_checks_count > 0 ? "error" : "neutral"}
          subLabel={server.down_checks_count > 0 ? `${server.down_checks_count} down` : undefined}
          to={`/operational/vps/${server.id}#checks`}
        />
      </div>

      {server.tags.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {server.tags.map((t) => (
            <span key={t} className="rounded-full bg-surface-muted px-2 py-0.5 text-[10px] uppercase text-text-tertiary">{t}</span>
          ))}
        </div>
      )}

      <div className="flex items-center justify-between border-t border-border-subtle pt-2">
        <Link
          to="/operational/vps/$vpsID"
          params={{ vpsID: server.id }}
          className="inline-flex items-center text-xs font-medium text-info hover:underline"
        >
          Buka detail <ChevronRight className="h-3 w-3" />
        </Link>
        <div className="flex gap-1">
          {canEdit && (
            <Button variant="ghost" size="sm" onClick={onEdit} title="Edit info VPS">
              <Pencil className="h-4 w-4" />
            </Button>
          )}
          {canDelete && (
            <Button variant="ghost" size="sm" onClick={onDelete} title="Hapus VPS">
              <Trash2 className="h-4 w-4 text-error" />
            </Button>
          )}
        </div>
      </div>
    </Card>
  );
}

function InfoRow({ icon, label, value }: { icon?: React.ReactNode; label: string; value: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-1 text-text-tertiary">{icon}{label}</div>
      <div className="truncate font-medium text-text-primary">{value}</div>
    </div>
  );
}

function CountBadge({
  label, count, tone, subLabel, to,
}: {
  label: string;
  count: number;
  tone: "neutral" | "error";
  subLabel?: string;
  to: string;
}) {
  const cls = tone === "error" ? "bg-error-light text-error" : "bg-surface-muted text-text-secondary";
  return (
    <Link to={to} className={cn("flex flex-1 flex-col rounded-lg px-3 py-2 transition hover:opacity-80", cls)}>
      <span className="text-[10px] uppercase">{label}</span>
      <span className="text-base font-semibold">{count}</span>
      {subLabel && <span className="text-[10px]">{subLabel}</span>}
    </Link>
  );
}

function EmptyState({ onCreate }: { onCreate?: () => void }) {
  return (
    <Card className="flex flex-col items-center gap-3 p-10 text-center">
      <Server className="h-10 w-10 text-text-tertiary" />
      <div>
        <h3 className="text-base font-semibold">Belum ada VPS terdaftar</h3>
        <p className="mt-1 max-w-md text-sm text-text-secondary">
          Tambah VPS pertama untuk mulai monitoring uptime, melacak apps yang berjalan, dan menerima alert saat
          server down atau renewal mendekati.
        </p>
      </div>
      {onCreate && (
        <Button onClick={onCreate}>
          <Plus className="mr-1 h-4 w-4" /> Tambah VPS pertama
        </Button>
      )}
    </Card>
  );
}

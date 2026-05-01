import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  Boxes,
  CheckCircle2,
  Clock,
  Pencil,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  XCircle,
} from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  AppFormDialog,
  CheckFormDialog,
  HealthPill,
  VPSFormDialog,
  formatDateTime,
  fromServer,
  relativeTime,
} from "@/components/vps/shared";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { cn } from "@/lib/utils";
import {
  deleteVPS,
  deleteVPSApp,
  deleteVPSCheck,
  getVPS,
  vpsKeys,
} from "@/services/operational-vps";
import { toast } from "@/stores/toast-store";
import type { VPSApp, VPSHealthCheck, VPSHealthEvent } from "@/types/vps";

export const Route = createFileRoute("/_authenticated/operational/vps/$vpsID")({
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalVPSView);
  },
  component: VPSDetailPage,
});

function VPSDetailPage() {
  const { vpsID } = Route.useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canEdit = hasPermission(permissions.operationalVPSEdit);
  const canDelete = hasPermission(permissions.operationalVPSDelete);

  const detailQuery = useQuery({
    queryKey: vpsKeys.detail(vpsID),
    queryFn: () => getVPS(vpsID),
    refetchInterval: 30_000,
  });
  const detail = detailQuery.data;

  const [editOpen, setEditOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const deleteMutation = useMutation({
    mutationFn: () => deleteVPS(vpsID),
    onSuccess: () => {
      toast.success("VPS dihapus");
      queryClient.invalidateQueries({ queryKey: vpsKeys.all });
      navigate({ to: "/operational/vps" });
    },
    onError: (err: Error) => toast.error(err.message ?? "Gagal hapus VPS"),
  });

  if (detailQuery.isLoading) {
    return <div className="p-6 text-text-tertiary">Loading…</div>;
  }
  if (!detail) {
    return (
      <div className="p-6">
        <Link to="/operational/vps" className="text-info hover:underline">← Kembali</Link>
        <p className="mt-4 text-text-tertiary">VPS tidak ditemukan.</p>
      </div>
    );
  }

  const { server, checks, apps, events, daily } = detail;

  return (
    <div className="space-y-5 p-6">
      <div className="flex items-center justify-between">
        <Link to="/operational/vps" className="inline-flex items-center gap-1 text-sm text-text-secondary hover:text-info">
          <ArrowLeft className="h-4 w-4" /> Semua VPS
        </Link>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={() => detailQuery.refetch()}>
            <RefreshCw className={cn("mr-1 h-4 w-4", detailQuery.isFetching && "animate-spin")} /> Refresh
          </Button>
          {canEdit && (
            <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
              <Pencil className="mr-1 h-4 w-4" /> Edit info
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
          <h1 className="text-2xl font-semibold">{server.label}</h1>
          <p className="text-sm text-text-secondary">
            {[server.provider, server.region].filter(Boolean).join(" · ") || "Tanpa provider info"}
          </p>
        </div>
        <HealthPill status={server.last_status} />
      </header>

      <Card className="grid gap-3 p-4 md:grid-cols-3">
        <Field label="Hostname" value={server.hostname || "—"} />
        <Field label="IP Address" value={server.ip_address || "—"} />
        <Field label="Region" value={server.region || "—"} />
        <Field label="Spec" value={`${server.cpu_cores} core · ${server.ram_mb} MB · ${server.disk_gb} GB`} />
        <Field label="Biaya" value={server.cost_amount ? `${server.cost_currency} ${server.cost_amount.toLocaleString("id-ID")} / ${cycleLabel(server.billing_cycle)}` : "—"} />
        <Field label="Renewal" value={server.renewal_date ?? "—"} />
        <Field label="Status" value={<span className="capitalize">{server.status}</span>} />
        <Field label="Last check" value={relativeTime(server.last_check_at)} />
        <Field label="Tags" value={server.tags.length ? server.tags.join(", ") : "—"} />
        {server.notes && (
          <div className="col-span-full">
            <div className="text-xs uppercase text-text-tertiary">Notes</div>
            <p className="mt-1 whitespace-pre-wrap rounded bg-surface-muted p-2 text-sm">{server.notes}</p>
          </div>
        )}
      </Card>

      <ChecksSection vpsID={vpsID} checks={checks} canEdit={canEdit} />
      <AppsSection vpsID={vpsID} apps={apps} checks={checks} canEdit={canEdit} />
      <UptimeSection daily={daily} checks={checks} />
      <EventsSection events={events} checks={checks} />

      {editOpen && (
        <VPSFormDialog
          initial={fromServer(server)}
          editing={server}
          onClose={() => setEditOpen(false)}
          onSaved={() => {
            queryClient.invalidateQueries({ queryKey: vpsKeys.detail(vpsID) });
            queryClient.invalidateQueries({ queryKey: vpsKeys.all });
            setEditOpen(false);
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDialog
          isOpen
          title={`Hapus VPS ${server.label}?`}
          description="Tindakan ini akan menghapus VPS, semua check, app, dan event historisnya."
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

function cycleLabel(c: string) {
  return c === "monthly" ? "bulan" : c === "quarterly" ? "triwulan" : c === "yearly" ? "tahun" : c;
}

// ---------- Checks section ---------------------------------------------------

function ChecksSection({ vpsID, checks, canEdit }: { vpsID: string; checks: VPSHealthCheck[]; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<VPSHealthCheck | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [confirmDel, setConfirmDel] = useState<VPSHealthCheck | null>(null);

  const delMut = useMutation({
    mutationFn: (id: string) => deleteVPSCheck(vpsID, id),
    onSuccess: () => {
      toast.success("Check dihapus");
      queryClient.invalidateQueries({ queryKey: vpsKeys.detail(vpsID) });
      queryClient.invalidateQueries({ queryKey: vpsKeys.all });
      setConfirmDel(null);
    },
    onError: (e: Error) => toast.error(e.message ?? "Gagal hapus check"),
  });

  return (
    <section id="checks" className="space-y-3">
      <SectionHeader
        icon={<ShieldCheck className="h-4 w-4" />}
        title="Health Checks"
        hint="Probe ICMP/TCP/HTTP/HTTPS yang jalan otomatis. Setiap check punya interval & timeout sendiri."
        action={
          canEdit ? (
            <Button size="sm" onClick={() => { setEditing(null); setShowForm(true); }}>
              <Plus className="mr-1 h-4 w-4" /> Tambah Check
            </Button>
          ) : null
        }
      />

      {checks.length === 0 ? (
        <Card className="p-6 text-center">
          <p className="text-sm text-text-secondary">Belum ada health check.</p>
          <p className="mt-1 text-xs text-text-tertiary">
            Tambah check supaya VPS ini ikut terpantau (otomatis tiap interval yang kamu set).
          </p>
        </Card>
      ) : (
        <div className="grid gap-2">
          {checks.map((c) => (
            <Card key={c.id} className="p-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <CheckStatusIcon status={c.last_status} />
                    <span className="font-medium">{c.label}</span>
                    <span className="rounded bg-surface-muted px-1.5 py-0.5 text-[10px] uppercase">{c.type}</span>
                    {!c.enabled && <span className="text-xs text-text-tertiary">(disabled)</span>}
                    {c.alert_active && <span className="rounded bg-error-light px-1.5 py-0.5 text-[10px] text-error">ALERT ACTIVE</span>}
                  </div>
                  <div className="mt-0.5 truncate font-mono text-xs text-text-secondary">{c.target}</div>
                  <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-secondary">
                    <span>Latency: <strong>{c.last_latency_ms ?? "—"} ms</strong></span>
                    <span>Interval: {c.interval_seconds}s</span>
                    <span>Timeout: {c.timeout_seconds}s</span>
                    <span>Last: {relativeTime(c.last_check_at)}</span>
                    {c.consecutive_fails > 0 && <span className="text-error">Fails: {c.consecutive_fails}</span>}
                  </div>
                  {c.last_error && <div className="mt-1 rounded bg-error-light px-2 py-1 text-xs text-error">{c.last_error}</div>}
                  {c.ssl_expires_at && (
                    <div className="mt-1 text-xs text-text-secondary">
                      SSL: expires {formatDateTime(c.ssl_expires_at)}{c.ssl_issuer && ` · ${c.ssl_issuer.split(",")[0]}`}
                    </div>
                  )}
                </div>
                {canEdit && (
                  <div className="flex gap-1">
                    <Button variant="ghost" size="sm" onClick={() => { setEditing(c); setShowForm(true); }} title="Edit check">
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => setConfirmDel(c)} title="Hapus check">
                      <Trash2 className="h-4 w-4 text-error" />
                    </Button>
                  </div>
                )}
              </div>
            </Card>
          ))}
        </div>
      )}

      {showForm && (
        <CheckFormDialog
          vpsID={vpsID}
          editing={editing}
          onClose={() => { setShowForm(false); setEditing(null); }}
          onSaved={() => {
            queryClient.invalidateQueries({ queryKey: vpsKeys.detail(vpsID) });
            queryClient.invalidateQueries({ queryKey: vpsKeys.all });
            setShowForm(false); setEditing(null);
          }}
        />
      )}

      {confirmDel && (
        <ConfirmDialog
          isOpen
          title={`Hapus check ${confirmDel.label}?`}
          description="Event historis akan ikut terhapus."
          confirmLabel="Hapus"
          tone="danger"
          isLoading={delMut.isPending}
          onConfirm={() => delMut.mutate(confirmDel.id)}
          onClose={() => setConfirmDel(null)}
        />
      )}
    </section>
  );
}

function CheckStatusIcon({ status }: { status: VPSHealthCheck["last_status"] }) {
  if (status === "up") return <CheckCircle2 className="h-4 w-4 text-success" />;
  if (status === "down") return <XCircle className="h-4 w-4 text-error" />;
  return <Clock className="h-4 w-4 text-text-tertiary" />;
}

// ---------- Apps section -----------------------------------------------------

function AppsSection({ vpsID, apps, checks, canEdit }: { vpsID: string; apps: VPSApp[]; checks: VPSHealthCheck[]; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<VPSApp | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [confirmDel, setConfirmDel] = useState<VPSApp | null>(null);

  const delMut = useMutation({
    mutationFn: (id: string) => deleteVPSApp(vpsID, id),
    onSuccess: () => {
      toast.success("App dihapus");
      queryClient.invalidateQueries({ queryKey: vpsKeys.detail(vpsID) });
      queryClient.invalidateQueries({ queryKey: vpsKeys.all });
      setConfirmDel(null);
    },
    onError: (e: Error) => toast.error(e.message ?? "Gagal hapus app"),
  });

  const checkMap = useMemo(() => {
    const m = new Map<string, VPSHealthCheck>();
    checks.forEach((c) => m.set(c.id, c));
    return m;
  }, [checks]);

  return (
    <section id="apps" className="space-y-3">
      <SectionHeader
        icon={<Boxes className="h-4 w-4" />}
        title="Apps"
        hint="Service yang berjalan di VPS ini. Tidak semua perlu dimonitor — link ke check kalau mau dipantau uptime."
        action={
          canEdit ? (
            <Button size="sm" onClick={() => { setEditing(null); setShowForm(true); }}>
              <Plus className="mr-1 h-4 w-4" /> Tambah App
            </Button>
          ) : null
        }
      />

      {apps.length === 0 ? (
        <Card className="p-6 text-center">
          <p className="text-sm text-text-secondary">Belum ada app terdaftar.</p>
          <p className="mt-1 text-xs text-text-tertiary">
            App contohnya: postgres, nginx, redis, cron job. Bisa di-link ke health check supaya muncul status uptime per app.
          </p>
        </Card>
      ) : (
        <div className="grid gap-2 md:grid-cols-2">
          {apps.map((a) => {
            const linkedCheck = a.check_id ? checkMap.get(a.check_id) : null;
            return (
              <Card key={a.id} className="p-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{a.name}</span>
                      {a.app_type && <span className="rounded bg-surface-muted px-1.5 py-0.5 text-[10px] uppercase">{a.app_type}</span>}
                    </div>
                    <div className="mt-0.5 text-xs text-text-secondary">
                      {a.port ? `Port: ${a.port}` : null}
                      {a.port && a.url ? " · " : null}
                      {a.url ? <a href={a.url} target="_blank" rel="noreferrer" className="text-info hover:underline">{a.url}</a> : null}
                      {!a.port && !a.url && "—"}
                    </div>
                    {linkedCheck ? (
                      <div className="mt-1 flex items-center gap-1 text-xs">
                        <CheckStatusIcon status={linkedCheck.last_status} />
                        <span className="text-text-secondary">Linked check: <strong>{linkedCheck.label}</strong></span>
                      </div>
                    ) : (
                      <div className="mt-1 text-xs text-text-tertiary">Tidak dimonitor</div>
                    )}
                    {a.notes && <div className="mt-1 whitespace-pre-wrap text-xs text-text-secondary">{a.notes}</div>}
                  </div>
                  {canEdit && (
                    <div className="flex gap-1">
                      <Button variant="ghost" size="sm" onClick={() => { setEditing(a); setShowForm(true); }} title="Edit app">
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => setConfirmDel(a)} title="Hapus app">
                        <Trash2 className="h-4 w-4 text-error" />
                      </Button>
                    </div>
                  )}
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {showForm && (
        <AppFormDialog
          vpsID={vpsID}
          editing={editing}
          checks={checks}
          onClose={() => { setShowForm(false); setEditing(null); }}
          onSaved={() => {
            queryClient.invalidateQueries({ queryKey: vpsKeys.detail(vpsID) });
            queryClient.invalidateQueries({ queryKey: vpsKeys.all });
            setShowForm(false); setEditing(null);
          }}
        />
      )}

      {confirmDel && (
        <ConfirmDialog
          isOpen
          title={`Hapus app ${confirmDel.name}?`}
          description="App akan dihapus dari inventaris."
          confirmLabel="Hapus"
          tone="danger"
          isLoading={delMut.isPending}
          onConfirm={() => delMut.mutate(confirmDel.id)}
          onClose={() => setConfirmDel(null)}
        />
      )}
    </section>
  );
}

// ---------- Uptime section ---------------------------------------------------

function UptimeSection({ daily, checks }: { daily: { summary_date: string; check_id: string; uptime_pct: number }[]; checks: VPSHealthCheck[] }) {
  const checkMap = useMemo(() => {
    const m = new Map<string, string>();
    checks.forEach((c) => m.set(c.id, c.label));
    return m;
  }, [checks]);

  if (daily.length === 0) {
    return (
      <section id="uptime" className="space-y-3">
        <SectionHeader title="Uptime harian" hint="Akan muncul setelah 24 jam pertama probe terkumpul." />
      </section>
    );
  }

  // group by check
  const byCheck = new Map<string, { date: string; pct: number }[]>();
  daily.forEach((d) => {
    const arr = byCheck.get(d.check_id) ?? [];
    arr.push({ date: d.summary_date.slice(0, 10), pct: d.uptime_pct });
    byCheck.set(d.check_id, arr);
  });

  return (
    <section id="uptime" className="space-y-3">
      <SectionHeader title="Uptime harian" hint="Persentase up dalam 30 hari terakhir, per check." />
      <div className="grid gap-2">
        {Array.from(byCheck.entries()).map(([checkID, rows]) => (
          <Card key={checkID} className="p-3">
            <div className="mb-2 text-sm font-medium">{checkMap.get(checkID) ?? checkID.slice(0, 8)}</div>
            <div className="flex flex-wrap gap-1">
              {rows.slice(0, 30).map((r) => (
                <div
                  key={r.date}
                  title={`${r.date}: ${r.pct.toFixed(2)}%`}
                  className={cn(
                    "h-6 w-3 rounded-sm",
                    r.pct >= 99 ? "bg-success" : r.pct >= 95 ? "bg-warning" : "bg-error",
                  )}
                />
              ))}
            </div>
          </Card>
        ))}
      </div>
    </section>
  );
}

// ---------- Events section ---------------------------------------------------

function EventsSection({ events, checks }: { events: VPSHealthEvent[]; checks: VPSHealthCheck[] }) {
  const labels = useMemo(() => {
    const m = new Map<string, string>();
    checks.forEach((c) => m.set(c.id, c.label));
    return m;
  }, [checks]);

  return (
    <section id="events" className="space-y-3">
      <SectionHeader title="Event terakhir" hint="100 probe terakhir. Event lebih lama dari 7 hari otomatis dihapus." />
      {events.length === 0 ? (
        <Card className="p-6 text-center text-sm text-text-tertiary">Belum ada event tercatat.</Card>
      ) : (
        <Card className="overflow-hidden">
          <table className="w-full text-xs">
            <thead className="bg-surface-muted text-left text-text-tertiary">
              <tr>
                <th className="px-3 py-2">Time</th>
                <th className="px-3 py-2">Check</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2">Latency</th>
                <th className="px-3 py-2">Error</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id} className="border-t border-border-subtle">
                  <td className="px-3 py-2">{formatDateTime(e.created_at)}</td>
                  <td className="px-3 py-2">{labels.get(e.check_id) ?? e.check_id.slice(0, 8)}</td>
                  <td className={cn("px-3 py-2 capitalize", e.status === "up" ? "text-success" : "text-error")}>{e.status}</td>
                  <td className="px-3 py-2">{e.latency_ms ?? "—"} ms</td>
                  <td className="px-3 py-2 text-text-secondary">{e.error_message || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </section>
  );
}

function SectionHeader({
  title, hint, icon, action,
}: {
  title: string;
  hint?: string;
  icon?: React.ReactNode;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div>
        <h2 className="flex items-center gap-2 text-base font-semibold">{icon}{title}</h2>
        {hint && <p className="text-xs text-text-tertiary">{hint}</p>}
      </div>
      {action}
    </div>
  );
}

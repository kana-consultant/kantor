import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  Activity,
  BarChart3,
  CheckCircle2,
  Clock3,
  Download,
  Eye,
  Globe2,
  PieChart as PieChartIcon,
  ShieldAlert,
  ShieldCheck,
  Users2,
  Pencil,
  Trash2,
  PlugZap,
} from "lucide-react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { Drawer, DrawerBody, DrawerClose, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from "@/components/shared/drawer";
import { EmptyState } from "@/components/shared/empty-state";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { OverviewSkeleton } from "@/components/shared/skeletons";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogBody, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/hooks/use-auth";
import { useRBAC } from "@/hooks/use-rbac";
import { env } from "@/lib/env";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { cn } from "@/lib/utils";
import {
  trackerKeys,
  createTrackerDomain,
  deleteTrackerDomain,
  downloadTrackerExtension,
  getMyTrackerActivity,
  getTeamTrackerActivity,
  getTrackerConsent,
  getTrackerSummary,
  getTrackerUserActivity,
  listTrackerConsents,
  listTrackerDomains,
  revokeTrackerConsent,
  updateTrackerDomain,
} from "@/services/operational-tracker";
import { toast } from "@/stores/toast-store";
import type { DomainCategory, TrackerActivityOverview, TrackerConsentAudit, TrackerDailySummary, TrackerTeamOverview, TrackerUserSummary } from "@/types/tracker";

const CATEGORY_COLORS = ["#0065FF", "#4C9AFF", "#6554C0", "#FF5630", "#FF8B00", "#36B37E", "#00B8D9", "#97A0AF"];
const TRACKER_WEB_SOURCE = "KANTOR_WEB_APP";
const TRACKER_EXTENSION_SOURCE = "KANTOR_TRACKER_EXTENSION";

export const Route = createFileRoute("/_authenticated/operational/tracker")({
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalTrackerView);
  },
  component: OperationalTrackerPage,
});

function OperationalTrackerPage() {
  const queryClient = useQueryClient();
  const { session } = useAuth();
  const { hasPermission } = useRBAC();
  const canViewTeam = hasPermission(permissions.operationalTrackerViewTeam);
  const canAuditConsent = hasPermission(permissions.operationalTrackerViewTeam);
  const canManageDomains = hasPermission(permissions.operationalTrackerDomainManage);

  const today = formatDateInput(new Date());
  const [activeTab, setActiveTab] = useState<"setup" | "my" | "team">("setup");
  const [dateFrom, setDateFrom] = useState(today);
  const [dateTo, setDateTo] = useState(today);
  const [teamUserFilter, setTeamUserFilter] = useState("");
  const [consentDialogOpen, setConsentDialogOpen] = useState(false);
  const [domainModalOpen, setDomainModalOpen] = useState(false);
  const [editingDomain, setEditingDomain] = useState<DomainCategory | null>(null);
  const [deletingDomain, setDeletingDomain] = useState<DomainCategory | null>(null);
  const [selectedUser, setSelectedUser] = useState<TrackerUserSummary | null>(null);
  const [extensionInstalled, setExtensionInstalled] = useState<boolean | null>(null);
  const [isConnectingExtension, setIsConnectingExtension] = useState(false);
  const [isDownloadingExtension, setIsDownloadingExtension] = useState(false);
  const [domainForm, setDomainForm] = useState({
    domainPattern: "",
    category: "development",
    isProductive: true,
  });
  const pendingExtensionRequests = useRef<
    Map<string, { resolve: (value: unknown) => void; reject: (reason?: unknown) => void; timeoutId: number }>
  >(new Map());

  const consentQuery = useQuery({
    queryKey: trackerKeys.consent(),
    queryFn: getTrackerConsent,
  });
  const myActivityQuery = useQuery({
    queryKey: trackerKeys.myActivity(dateFrom, dateTo),
    queryFn: () => getMyTrackerActivity(dateFrom, dateTo),
  });
  const summaryQuery = useQuery({
    queryKey: trackerKeys.summary(today),
    queryFn: () => getTrackerSummary(today),
    enabled: canViewTeam,
  });
  const consentAuditQuery = useQuery({
    queryKey: trackerKeys.consents(),
    queryFn: listTrackerConsents,
    enabled: canAuditConsent && activeTab === "team",
    refetchInterval: canAuditConsent && activeTab === "team" ? 5_000 : false,
    refetchIntervalInBackground: true,
  });
  const teamActivityQuery = useQuery({
    queryKey: trackerKeys.teamActivity(dateFrom, dateTo, teamUserFilter || undefined),
    queryFn: () => getTeamTrackerActivity(dateFrom, dateTo, teamUserFilter || undefined),
    enabled: canViewTeam,
    refetchInterval: canViewTeam && activeTab === "team" ? 5_000 : false,
    refetchIntervalInBackground: true,
  });
  const userDetailQuery = useQuery({
    queryKey: trackerKeys.userActivity(selectedUser?.user_id ?? "", dateFrom, dateTo),
    queryFn: () => getTrackerUserActivity(selectedUser!.user_id, dateFrom, dateTo),
    enabled: Boolean(selectedUser),
  });
  const domainsQuery = useQuery({
    queryKey: trackerKeys.domains(),
    queryFn: listTrackerDomains,
    enabled: canManageDomains && domainModalOpen,
  });

  useEffect(() => {
    if (!consentQuery.isLoading && consentQuery.data?.consented && extensionInstalled !== false) {
      setActiveTab((current) => (current === "setup" ? "my" : current));
    }
  }, [consentQuery.data?.consented, consentQuery.isLoading, extensionInstalled]);

  const revokeConsentMutation = useMutation({
    mutationFn: revokeTrackerConsent,
    onSuccess: () => {
      toast.warning("Consent dicabut", "Extension tracker harus meminta izin lagi sebelum mengirim heartbeat.");
      void queryClient.invalidateQueries({ queryKey: trackerKeys.consent() });
      void queryClient.invalidateQueries({ queryKey: trackerKeys.consents() });
    },
  });
  const saveDomainMutation = useMutation({
    mutationFn: () =>
      editingDomain
        ? updateTrackerDomain(editingDomain.id, {
            domain_pattern: domainForm.domainPattern,
            category: domainForm.category,
            is_productive: domainForm.isProductive,
          })
        : createTrackerDomain({
            domain_pattern: domainForm.domainPattern,
            category: domainForm.category,
            is_productive: domainForm.isProductive,
          }),
    onSuccess: () => {
      toast.success(editingDomain ? "Domain diperbarui" : "Domain ditambahkan");
      setDomainModalOpen(false);
      setEditingDomain(null);
      resetDomainForm();
      void queryClient.invalidateQueries({ queryKey: trackerKeys.domains() });
    },
    onError: () => {
      toast.error("Gagal menyimpan domain tracker");
    },
  });
  const deleteDomainMutation = useMutation({
    mutationFn: (domainId: string) => deleteTrackerDomain(domainId),
    onSuccess: () => {
      toast.success("Domain tracker dihapus");
      setDeletingDomain(null);
      void queryClient.invalidateQueries({ queryKey: trackerKeys.domains() });
    },
    onError: () => {
      toast.error("Gagal menghapus domain tracker");
    },
  });

  useEffect(() => {
    const handleMessage = (event: MessageEvent) => {
      if (event.source !== window || !event.data || typeof event.data !== "object") {
        return;
      }

      const data = event.data as {
        source?: string;
        type?: string;
        requestId?: string;
        success?: boolean;
        error?: string;
        payload?: unknown;
      };

      if (data.source !== TRACKER_EXTENSION_SOURCE) {
        return;
      }

      if (data.type === "KANTOR_TRACKER_READY") {
        setExtensionInstalled(true);
        return;
      }

      if (data.type === "KANTOR_TRACKER_RESULT" && data.requestId) {
        const pending = pendingExtensionRequests.current.get(data.requestId);
        if (!pending) {
          return;
        }

        window.clearTimeout(pending.timeoutId);
        pendingExtensionRequests.current.delete(data.requestId);

        if (data.success) {
          pending.resolve(data.payload);
        } else {
          pending.reject(new Error(data.error || "Extension action failed"));
        }
      }
    };

    window.addEventListener("message", handleMessage);
    const pingTimeout = window.setTimeout(() => {
      setExtensionInstalled((current) => current ?? false);
    }, 1500);
    window.postMessage({ source: TRACKER_WEB_SOURCE, type: "KANTOR_TRACKER_PING" }, window.location.origin);

    return () => {
      window.removeEventListener("message", handleMessage);
      window.clearTimeout(pingTimeout);
      for (const pending of pendingExtensionRequests.current.values()) {
        window.clearTimeout(pending.timeoutId);
      }
      pendingExtensionRequests.current.clear();
    };
  }, []);

  const trackerApiBaseUrl = useMemo(() => {
    try {
      return new URL(env.VITE_API_BASE_URL, window.location.origin).toString();
    } catch {
      return `${window.location.origin}/api/v1`;
    }
  }, []);

  const categoryChartData = myActivityQuery.data?.category_breakdown ?? [];
  const teamUsers = teamActivityQuery.data?.users ?? [];
  const stackedCategoryKeys = useMemo(() => {
    const categories = new Set<string>();
    for (const user of teamUsers) {
      for (const key of Object.keys(user.category_breakdown || {})) {
        categories.add(key);
      }
    }
    return Array.from(categories);
  }, [teamUsers]);
  const stackedTeamData = teamUsers.map((user) => ({
    user_name: user.user_name,
    ...user.category_breakdown,
  }));

  async function requestExtensionAction(type: string) {
    if (!session?.tokens.access_token) {
      throw new Error("Session web KANTOR tidak ditemukan. Login ulang lalu coba lagi.");
    }

    const requestId = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`;

    return new Promise<unknown>((resolve, reject) => {
      const timeoutId = window.setTimeout(() => {
        pendingExtensionRequests.current.delete(requestId);
        setExtensionInstalled(false);
        reject(new Error("Extension belum terdeteksi. Buka tab Setup Tracker untuk langkah pemasangan, lalu coba lagi."));
      }, 2500);

      pendingExtensionRequests.current.set(requestId, { resolve, reject, timeoutId });
      window.postMessage(
        {
          source: TRACKER_WEB_SOURCE,
          type,
          requestId,
          payload: {
            apiBaseUrl: trackerApiBaseUrl,
            dashboardUrl: window.location.href,
            token: session.tokens.access_token,
          },
        },
        window.location.origin,
      );
    });
  }

  async function handleExtensionConnect(enableTracking: boolean) {
    setIsConnectingExtension(true);
    try {
      await requestExtensionAction(enableTracking ? "KANTOR_TRACKER_ENABLE" : "KANTOR_TRACKER_CONNECT");
      setExtensionInstalled(true);
      if (enableTracking) {
        toast.success("Tracker aktif di browser ini", "Extension sudah terhubung dan consent tracker langsung diaktifkan.");
        setConsentDialogOpen(false);
        await queryClient.invalidateQueries({ queryKey: trackerKeys.consent() });
        await queryClient.invalidateQueries({ queryKey: trackerKeys.consents() });
      } else {
        toast.success("Extension tersambung", "Browser ini sudah terhubung ke KANTOR Tracker.");
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal menghubungkan extension tracker");
    } finally {
      setIsConnectingExtension(false);
    }
  }

  async function handleExtensionDownload() {
    setIsDownloadingExtension(true);
    try {
      const result = await downloadTrackerExtension();
      const url = window.URL.createObjectURL(result.blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = result.filename || "kantor-activity-tracker.zip";
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.URL.revokeObjectURL(url);
      toast.success("Extension berhasil diunduh", "Extract file ZIP lalu pasang extension lewat chrome://extensions.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal mengunduh extension tracker");
    } finally {
      setIsDownloadingExtension(false);
    }
  }

  const topDomainColumns: Array<DataTableColumn<TrackerActivityOverview["top_domains"][number]>> = [
    {
      id: "domain",
      header: "Domain",
      accessor: "domain",
      sortable: true,
      cell: (row) => <span className="font-mono text-[13px]">{row.domain}</span>,
    },
    {
      id: "category",
      header: "Kategori",
      accessor: "category",
      sortable: true,
    },
    {
      id: "productive",
      header: "Produktivitas",
      cell: (row) => <StatusBadge status={row.is_productive ? "productive" : "unproductive"} />,
    },
    {
      id: "duration",
      header: "Durasi",
      sortable: true,
      numeric: true,
      cell: (row) => <span className="font-mono tabular-nums">{formatDuration(row.duration_seconds)}</span>,
    },
    {
      id: "percentage",
      header: "Persentase",
      sortable: true,
      numeric: true,
      cell: (row) => <span className="font-mono tabular-nums">{formatPercent(row.percentage)}</span>,
    },
  ];

  const teamColumns: Array<DataTableColumn<TrackerUserSummary>> = [
    {
      id: "user_name",
      header: "Nama",
      accessor: "user_name",
      sortable: true,
      cell: (row) => (
        <div>
          <p className="font-semibold text-text-primary">{row.user_name}</p>
          <p className="text-xs text-text-secondary">{row.top_domain || "Belum ada domain dominan"}</p>
        </div>
      ),
    },
    {
      id: "active_seconds",
      header: "Active",
      sortable: true,
      numeric: true,
      cell: (row) => <span className="font-mono tabular-nums">{formatDuration(row.active_seconds)}</span>,
    },
    {
      id: "idle_seconds",
      header: "Idle",
      sortable: true,
      numeric: true,
      cell: (row) => <span className="font-mono tabular-nums">{formatDuration(row.idle_seconds)}</span>,
    },
    {
      id: "productivity_score",
      header: "Productivity",
      sortable: true,
      numeric: true,
      cell: (row) => <span className="font-mono tabular-nums">{formatPercent(row.productivity_score)}</span>,
    },
    {
      id: "top_domain",
      header: "Top Domain",
      accessor: "top_domain",
      sortable: true,
      cell: (row) => <span className="font-mono text-[13px]">{row.top_domain || "-"}</span>,
    },
  ];

  const domainColumns: Array<DataTableColumn<DomainCategory>> = [
    {
      id: "domain_pattern",
      header: "Domain",
      accessor: "domain_pattern",
      sortable: true,
      cell: (row) => <span className="font-mono text-[13px]">{row.domain_pattern}</span>,
    },
    {
      id: "category",
      header: "Kategori",
      accessor: "category",
      sortable: true,
    },
    {
      id: "is_productive",
      header: "Produktivitas",
      sortable: true,
      cell: (row) => <StatusBadge status={row.is_productive ? "productive" : "unproductive"} />,
    },
    {
      id: "actions",
      header: "Aksi",
      align: "right",
      cell: (row) => (
        <div className="flex justify-end gap-1">
          <Button
            size="icon"
            type="button"
            variant="ghost"
            onClick={(event) => {
              event.stopPropagation();
              setEditingDomain(row);
              setDomainForm({
                domainPattern: row.domain_pattern,
                category: row.category,
                isProductive: row.is_productive,
              });
              setDomainModalOpen(true);
            }}
          >
            <Pencil className="h-4 w-4" />
          </Button>
          <Button
            size="icon"
            type="button"
            variant="ghost"
            className="text-error hover:text-error"
            onClick={(event) => {
              event.stopPropagation();
              setDeletingDomain(row);
            }}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      ),
    },
  ];

  const consentAuditColumns: Array<DataTableColumn<TrackerConsentAudit>> = [
    {
      id: "user_name",
      header: "User",
      accessor: "user_name",
      sortable: true,
      cell: (row) => (
        <div>
          <p className="font-semibold text-text-primary">{row.user_name}</p>
          <p className="text-xs text-text-secondary">{row.user_email}</p>
        </div>
      ),
    },
    {
      id: "consented",
      header: "Status",
      sortable: true,
      cell: (row) => <StatusBadge status={row.consented ? "active" : "inactive"} />,
    },
    {
      id: "consented_at",
      header: "Aktif Sejak",
      sortable: true,
      cell: (row) => <span className="font-mono text-[13px]">{formatDateTime(row.consented_at)}</span>,
    },
    {
      id: "revoked_at",
      header: "Dicabut",
      sortable: true,
      cell: (row) => <span className="font-mono text-[13px]">{formatDateTime(row.revoked_at)}</span>,
    },
    {
      id: "last_activity_at",
      header: "Aktivitas Terakhir",
      sortable: true,
      cell: (row) => <span className="font-mono text-[13px]">{formatDateTime(row.last_activity_at)}</span>,
    },
    {
      id: "ip_address",
      header: "IP",
      sortable: true,
      cell: (row) => <span className="font-mono text-[13px]">{row.ip_address || "-"}</span>,
    },
  ];

  if (consentQuery.isLoading && myActivityQuery.isLoading) {
    return <OverviewSkeleton />;
  }

  const consented = Boolean(consentQuery.data?.consented);
  const isExtensionReady = extensionInstalled === true;
  const trackerReady = consented && isExtensionReady;

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Operasional</p>
          <h1 className="mt-2 text-[28px] font-bold tracking-tight text-text-primary">Activity Tracker</h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
            Pantau waktu aktif, idle, kategori aktivitas, dan domain dominan yang dikirim dari Chrome extension KANTOR.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={() => void handleExtensionDownload()} disabled={isDownloadingExtension}>
            <Download className="h-4 w-4" />
            {isDownloadingExtension ? "Mengunduh..." : "Download Extension"}
          </Button>
          <Button
            type="button"
            variant={activeTab === "setup" ? "primary" : "outline"}
            onClick={() => setActiveTab("setup")}
          >
            <PlugZap className="h-4 w-4" />
            Setup Tracker
          </Button>
          <Button
            type="button"
            variant={isExtensionReady ? "outline" : "secondary"}
            onClick={() => void handleExtensionConnect(false)}
            disabled={isConnectingExtension}
          >
            {isExtensionReady ? <CheckCircle2 className="h-4 w-4" /> : <PlugZap className="h-4 w-4" />}
            {isConnectingExtension ? "Mengecek..." : isExtensionReady ? "Sinkronkan Browser" : "Cek Extension"}
          </Button>
          {isExtensionReady || consented ? (
            <Button
              type="button"
              variant={consented ? "outline" : "primary"}
              onClick={() => (consented ? setConsentDialogOpen(true) : void handleExtensionConnect(true))}
              disabled={isConnectingExtension}
            >
              <ShieldCheck className="h-4 w-4" />
              {consented ? "Izin Tracking" : "Aktifkan Tracking"}
            </Button>
          ) : null}
          {canManageDomains ? (
            <Button
              type="button"
              onClick={() => {
                setEditingDomain(null);
                resetDomainForm();
                setDomainModalOpen(true);
              }}
            >
              <Globe2 className="h-4 w-4" />
              Kelola Domain
            </Button>
          ) : null}
        </div>
      </div>

      {!trackerReady ? (
        <Card className="border-ops/20 bg-ops-light/40 p-5">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <p className="text-sm font-semibold text-ops">Tracker browser belum siap dipakai</p>
              <p className="mt-2 text-sm text-text-secondary">
                Gunakan tab <strong>Setup Tracker</strong> untuk melihat langkah pemasangan extension, mengecek browser,
                lalu menyalakan izin tracking dengan urutan yang benar.
              </p>
            </div>
            <Button type="button" onClick={() => setActiveTab("setup")}>
              <PlugZap className="h-4 w-4" />
              Buka Setup Tracker
            </Button>
          </div>
        </Card>
      ) : null}

      <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-surface p-2">
        <button
          type="button"
          className={cn(
            "rounded-md px-4 py-2 text-sm font-semibold transition",
            activeTab === "setup" ? "bg-ops-light text-ops" : "text-text-secondary hover:bg-surface-muted hover:text-text-primary",
          )}
          onClick={() => setActiveTab("setup")}
        >
          Setup Tracker
        </button>
        <button
          type="button"
          className={cn(
            "rounded-md px-4 py-2 text-sm font-semibold transition",
            activeTab === "my" ? "bg-ops-light text-ops" : "text-text-secondary hover:bg-surface-muted hover:text-text-primary",
          )}
          onClick={() => setActiveTab("my")}
        >
          My Activity
        </button>
        <PermissionGate permission={permissions.operationalTrackerViewTeam}>
          <button
            type="button"
            className={cn(
              "rounded-md px-4 py-2 text-sm font-semibold transition",
              activeTab === "team" ? "bg-ops-light text-ops" : "text-text-secondary hover:bg-surface-muted hover:text-text-primary",
            )}
            onClick={() => setActiveTab("team")}
          >
            Team Activity
          </button>
        </PermissionGate>
      </div>

      {activeTab !== "setup" ? (
        <div className="flex flex-col gap-4 rounded-md border border-border bg-surface p-4 lg:flex-row lg:items-end">
          <label className="flex-1 text-sm font-medium text-text-primary">
            Dari tanggal
            <Input className="mt-1.5" type="date" value={dateFrom} onChange={(event) => setDateFrom(event.target.value)} />
          </label>
          <label className="flex-1 text-sm font-medium text-text-primary">
            Sampai tanggal
            <Input className="mt-1.5" type="date" value={dateTo} onChange={(event) => setDateTo(event.target.value)} />
          </label>
          {activeTab === "team" && canViewTeam ? (
            <label className="flex-1 text-sm font-medium text-text-primary">
              Filter employee
              <select
                className="mt-1.5 h-10 w-full rounded-[6px] border-[1.5px] border-transparent bg-surface-muted px-3 text-sm text-text-primary outline-none focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
                value={teamUserFilter}
                onChange={(event) => setTeamUserFilter(event.target.value)}
              >
                <option value="">Semua user</option>
                {teamActivityQuery.data?.users.map((user) => (
                  <option key={user.user_id} value={user.user_id}>
                    {user.user_name}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
        </div>
      ) : null}

      {activeTab === "setup" ? (
        <TrackerSetupTab
          consented={consented}
          extensionInstalled={extensionInstalled}
          isConnectingExtension={isConnectingExtension}
          isDownloadingExtension={isDownloadingExtension}
          onDownloadExtension={() => void handleExtensionDownload()}
          onCheckExtension={() => void handleExtensionConnect(false)}
          onEnableTracking={() => void handleExtensionConnect(true)}
          onOpenConsent={() => setConsentDialogOpen(true)}
          showAdminAuditHint={canAuditConsent}
        />
      ) : activeTab === "my" ? (
        <MyActivityTab data={myActivityQuery.data} isLoading={myActivityQuery.isLoading} topDomainColumns={topDomainColumns} />
      ) : (
        <PermissionGate
          permission={permissions.operationalTrackerViewTeam}
          fallback={
            <EmptyState
              icon={Users2}
              title="Akses team activity tidak tersedia"
              description="Halaman ini hanya bisa dibuka manager atau admin operasional."
            />
          }
        >
          <TeamActivityTab
            columns={teamColumns}
            consentAuditColumns={consentAuditColumns}
            consentAuditItems={consentAuditQuery.data}
            data={teamActivityQuery.data}
            isLoading={teamActivityQuery.isLoading || summaryQuery.isLoading || consentAuditQuery.isLoading}
            stackedCategoryKeys={stackedCategoryKeys}
            stackedTeamData={stackedTeamData}
            summary={summaryQuery.data}
            onSelectUser={setSelectedUser}
            showConsentAudit={canAuditConsent}
          />
        </PermissionGate>
      )}

      <Dialog open={consentDialogOpen} onOpenChange={setConsentDialogOpen}>
        <DialogContent size="md">
          <DialogHeader className="flex items-start justify-between gap-4">
            <div>
              <DialogTitle>Izin Tracking Browser</DialogTitle>
              <DialogDescription>
                Tombol ini dipakai untuk menyalakan atau mematikan izin extension mengirim data aktivitas browser ke dashboard KANTOR.
              </DialogDescription>
            </div>
            <DialogClose />
          </DialogHeader>
          <DialogBody className="space-y-4 text-sm leading-6 text-text-secondary">
            <p>Izin tracking bukan untuk memasang extension. Extension tetap harus terpasang dulu di browser Chrome.</p>
            <p>Setelah extension terpasang, tombol ini menentukan apakah browser boleh mengirim domain aktif, judul tab, waktu aktif, dan idle ke sistem.</p>
            <p>Jika izin dimatikan, extension tetap ada di browser tetapi heartbeat baru akan ditolak sampai Anda menyalakannya lagi.</p>
            <p>Extension tidak merekam halaman internal browser seperti <code>chrome://</code> atau domain yang Anda masukkan ke daftar excluded.</p>
          </DialogBody>
          <DialogFooter>
            {consentQuery.data?.consented ? (
              <Button
                type="button"
                variant="danger"
                onClick={() => revokeConsentMutation.mutate()}
                disabled={revokeConsentMutation.isPending}
              >
                {revokeConsentMutation.isPending ? "Mematikan..." : "Matikan Tracking"}
              </Button>
            ) : isExtensionReady ? (
              <>
                <Button
                  type="button"
                  onClick={() => void handleExtensionConnect(true)}
                  disabled={isConnectingExtension}
                >
                  {isConnectingExtension ? "Mengaktifkan..." : "Aktifkan Tracking"}
                </Button>
              </>
            ) : (
              <Button type="button" onClick={() => setActiveTab("setup")}>
                Buka Setup Tracker
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <FormModal
        isOpen={domainModalOpen}
        onClose={() => {
          setDomainModalOpen(false);
          setEditingDomain(null);
          resetDomainForm();
        }}
        onSubmit={(event) => {
          event.preventDefault();
          saveDomainMutation.mutate();
        }}
        title={editingDomain ? "Edit domain category" : "Tambah domain category"}
        subtitle="Gunakan domain pattern untuk mengelompokkan aktivitas extension ke kategori produktif atau non-produktif."
        submitLabel={editingDomain ? "Simpan Perubahan" : "Simpan Domain"}
        isLoading={saveDomainMutation.isPending}
        submitDisabled={!domainForm.domainPattern.trim()}
        size="xl"
      >
        <div className="grid gap-4 md:grid-cols-2">
          <label className="text-sm font-medium text-text-primary">
            Domain pattern
            <Input
              className="mt-1.5"
              placeholder="contoh: github.com"
              value={domainForm.domainPattern}
              onChange={(event) => setDomainForm((current) => ({ ...current, domainPattern: event.target.value }))}
            />
          </label>
          <label className="text-sm font-medium text-text-primary">
            Kategori
            <select
              className="mt-1.5 h-10 w-full rounded-[6px] border-[1.5px] border-transparent bg-surface-muted px-3 text-sm text-text-primary outline-none focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
              value={domainForm.category}
              onChange={(event) => setDomainForm((current) => ({ ...current, category: event.target.value }))}
            >
              {["development", "documentation", "communication", "design", "social_media", "entertainment", "other", "uncategorized"].map((option) => (
                <option key={option} value={option}>
                  {option.replace(/_/g, " ")}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label className="inline-flex items-center gap-3 text-sm font-medium text-text-primary">
          <input
            type="checkbox"
            checked={domainForm.isProductive}
            onChange={(event) => setDomainForm((current) => ({ ...current, isProductive: event.target.checked }))}
          />
          Tandai sebagai productive domain
        </label>
        <DataTable
          columns={domainColumns}
          data={domainsQuery.data ?? []}
          emptyDescription="Tambahkan pattern domain agar aktivitas extension punya kategori yang konsisten."
          emptyTitle="Belum ada domain categories"
          getRowId={(row) => row.id}
          loading={domainsQuery.isLoading}
        />
      </FormModal>

      <ConfirmDialog
        isOpen={Boolean(deletingDomain)}
        onClose={() => setDeletingDomain(null)}
        onConfirm={() => {
          if (deletingDomain) {
            deleteDomainMutation.mutate(deletingDomain.id);
          }
        }}
        title={`Hapus domain ${deletingDomain?.domain_pattern ?? ""}?`}
        description="Domain category yang dihapus tidak akan dipakai lagi untuk klasifikasi heartbeat baru."
        confirmLabel="Hapus Domain"
        isLoading={deleteDomainMutation.isPending}
      />

      <Drawer open={Boolean(selectedUser)} onOpenChange={(open) => (!open ? setSelectedUser(null) : undefined)}>
        <DrawerContent size="lg">
          <DrawerHeader className="flex items-start justify-between gap-4">
            <div>
              <DrawerTitle>{selectedUser?.user_name ?? "Detail user"}</DrawerTitle>
              <DrawerDescription>Drill-down aktivitas user dari data Chrome extension pada rentang tanggal terpilih.</DrawerDescription>
            </div>
            <DrawerClose />
          </DrawerHeader>
          <DrawerBody>
            {userDetailQuery.isLoading ? (
              <OverviewSkeleton />
            ) : userDetailQuery.data ? (
              <MyActivityContent data={userDetailQuery.data} topDomainColumns={topDomainColumns} />
            ) : (
              <EmptyState
                icon={Eye}
                title="Detail aktivitas belum tersedia"
                description="Pilih user yang memiliki data tracker pada rentang tanggal ini."
              />
            )}
          </DrawerBody>
        </DrawerContent>
      </Drawer>
    </div>
  );

  function resetDomainForm() {
    setDomainForm({
      domainPattern: "",
      category: "development",
      isProductive: true,
    });
  }
}

function TrackerSetupTab({
  consented,
  extensionInstalled,
  isConnectingExtension,
  isDownloadingExtension,
  onDownloadExtension,
  onCheckExtension,
  onEnableTracking,
  onOpenConsent,
  showAdminAuditHint,
}: {
  consented: boolean;
  extensionInstalled: boolean | null;
  isConnectingExtension: boolean;
  isDownloadingExtension: boolean;
  onDownloadExtension: () => void;
  onCheckExtension: () => void;
  onEnableTracking: () => void;
  onOpenConsent: () => void;
  showAdminAuditHint: boolean;
}) {
  const extensionStatus = extensionInstalled === true ? "active" : extensionInstalled === false ? "inactive" : "pending";

  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-3">
        <StatCard
          icon={PlugZap}
          label="Extension"
          tone={extensionInstalled === true ? "success" : "warning"}
          value={extensionInstalled === true ? "Siap di browser ini" : extensionInstalled === false ? "Belum terdeteksi" : "Sedang dicek"}
        />
        <StatCard
          icon={ShieldCheck}
          label="Izin Tracking"
          tone={consented ? "success" : "warning"}
          value={consented ? "Aktif" : "Belum aktif"}
          helper="Izin ini menentukan apakah extension boleh mengirim heartbeat aktivitas."
        />
        <StatCard
          icon={Users2}
          label="Laporan Admin"
          tone="info"
          value={showAdminAuditHint ? "Ada di Team Activity" : "Manager/Admin only"}
          helper="Admin dapat melihat siapa yang menyalakan atau mematikan tracker di tabel Consent Audit."
        />
      </div>

      <Card className="p-6">
        <div className="border-b border-border pb-4">
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Setup Tracker</p>
          <h2 className="mt-2 text-[22px] font-bold text-text-primary">Langkah yang paling mudah untuk user</h2>
        </div>
        <div className="mt-6 grid gap-4 lg:grid-cols-3">
          <div className="rounded-lg border border-border bg-surface-muted/50 p-4">
            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-ops-light text-sm font-bold text-ops">1</div>
              <div className="space-y-2">
                <p className="font-semibold text-text-primary">Download extension dari dashboard ini</p>
                <p className="text-sm leading-6 text-text-secondary">
                  User tidak perlu akses repo. Klik tombol download di bawah untuk mengambil paket ZIP extension resmi dari KANTOR.
                </p>
                <Button type="button" variant="outline" onClick={onDownloadExtension} disabled={isDownloadingExtension}>
                  <Download className="h-4 w-4" />
                  {isDownloadingExtension ? "Mengunduh..." : "Download Extension (.zip)"}
                </Button>
                <p className="text-sm leading-6 text-text-secondary">
                  Setelah selesai, extract ZIP tersebut ke folder biasa di komputer Anda.
                </p>
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-border bg-surface-muted/50 p-4">
            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-ops-light text-sm font-bold text-ops">2</div>
              <div className="space-y-2">
                <p className="font-semibold text-text-primary">Pasang di Chrome sekali saja</p>
                <p className="text-sm leading-6 text-text-secondary">
                  Buka <span className="font-mono">chrome://extensions</span>, aktifkan <strong>Developer mode</strong>, lalu klik <strong>Load unpacked</strong>.
                </p>
                <p className="text-sm leading-6 text-text-secondary">
                  Pilih folder hasil extract ZIP tadi, bukan file ZIP-nya langsung.
                </p>
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-border bg-surface-muted/50 p-4">
            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-ops-light text-sm font-bold text-ops">3</div>
              <div className="space-y-2">
                <p className="font-semibold text-text-primary">Cek browser ini</p>
                <p className="text-sm leading-6 text-text-secondary">
                  Setelah extension terpasang, klik tombol di bawah untuk memastikan browser yang sedang Anda pakai sudah dikenali oleh dashboard.
                </p>
                <div className="pt-2">
                  <StatusBadge status={extensionStatus} />
                </div>
                <Button type="button" variant="outline" onClick={onCheckExtension} disabled={isConnectingExtension}>
                  {isConnectingExtension ? "Mengecek..." : "Cek Extension"}
                </Button>
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-border bg-surface-muted/50 p-4">
            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-ops-light text-sm font-bold text-ops">4</div>
              <div className="space-y-2">
                <p className="font-semibold text-text-primary">Nyalakan tracking</p>
                <p className="text-sm leading-6 text-text-secondary">
                  Jika browser sudah terbaca, cukup satu klik untuk menghubungkan browser dan langsung mengaktifkan izin tracking.
                </p>
                <div className="flex flex-wrap gap-2 pt-2">
                  <Button type="button" onClick={onEnableTracking} disabled={isConnectingExtension}>
                    {isConnectingExtension ? "Menghubungkan..." : "Hubungkan & Aktifkan"}
                  </Button>
                  <Button type="button" variant="outline" onClick={onOpenConsent}>
                    <ShieldCheck className="h-4 w-4" />
                    Apa itu Izin Tracking?
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Card>

      <Card className="p-6">
        <div className="border-b border-border pb-4">
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Penjelasan Singkat</p>
          <h2 className="mt-2 text-[22px] font-bold text-text-primary">Supaya tombolnya tidak membingungkan</h2>
        </div>
        <div className="mt-6 grid gap-4 lg:grid-cols-2">
          <div className="rounded-lg border border-border bg-surface-muted/50 p-4">
            <div className="flex items-start gap-3">
              <ShieldAlert className="mt-0.5 h-5 w-5 text-warning" />
              <div className="space-y-2">
                <p className="font-semibold text-text-primary">Apa fungsi tombol Izin Tracking?</p>
                <p className="text-sm leading-6 text-text-secondary">
                  Tombol itu hanya untuk menyalakan atau mematikan izin pengiriman data dari extension. Bukan untuk memasang extension.
                </p>
                <p className="text-sm leading-6 text-text-secondary">
                  Jika dimatikan, extension tetap ada di Chrome tetapi data baru tidak akan dikirim ke sistem sampai Anda menyalakannya lagi.
                </p>
              </div>
            </div>
          </div>
          <div className="rounded-lg border border-border bg-surface-muted/50 p-4">
            <div className="flex items-start gap-3">
              <CheckCircle2 className="mt-0.5 h-5 w-5 text-success" />
              <div className="space-y-2">
                <p className="font-semibold text-text-primary">Di mana admin lihat siapa yang on/off?</p>
                <p className="text-sm leading-6 text-text-secondary">
                  Admin atau super admin bisa membuka tab <strong>Team Activity</strong>, lalu lihat tabel <strong>Consent Audit</strong>.
                </p>
                <p className="text-sm leading-6 text-text-secondary">
                  Di sana terlihat siapa yang sedang aktif, kapan consent dinyalakan, kapan dimatikan, dan aktivitas terakhirnya.
                </p>
              </div>
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
}

function MyActivityTab({ data, isLoading, topDomainColumns }: { data?: TrackerActivityOverview; isLoading: boolean; topDomainColumns: Array<DataTableColumn<TrackerActivityOverview["top_domains"][number]>> }) {
  if (isLoading) {
    return <OverviewSkeleton />;
  }
  if (!data) {
    return <EmptyState icon={Activity} title="Belum ada aktivitas tracker" description="Setelah Chrome extension mengirim heartbeat, ringkasan personal akan muncul di halaman ini." />;
  }
  return <MyActivityContent data={data} topDomainColumns={topDomainColumns} />;
}

function MyActivityContent({ data, topDomainColumns }: { data: TrackerActivityOverview; topDomainColumns: Array<DataTableColumn<TrackerActivityOverview["top_domains"][number]>> }) {
  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-4">
        <StatCard icon={Clock3} label="Total Active Time" mono tone="ops" value={formatDuration(data.total_active_seconds)} />
        <StatCard icon={Activity} label="Total Idle Time" mono tone="warning" value={formatDuration(data.total_idle_seconds)} />
        <StatCard
          icon={BarChart3}
          label="Productivity Score"
          mono
          tone={data.productivity_score >= 70 ? "success" : data.productivity_score >= 40 ? "warning" : "error"}
          value={formatPercent(data.productivity_score)}
        />
        <StatCard icon={Globe2} label="Most Used Domain" helper="Domain dominan pada rentang tanggal aktif." tone="info" value={data.most_used_domain ?? "-"} />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1.35fr,1fr]">
        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Timeline</p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">Breakdown per jam</h2>
          </div>
          <div className="mt-6 h-[320px]">
            <ResponsiveContainer width="100%" height="100%" minHeight={240} minWidth={1}>
              <BarChart data={data.hourly_breakdown}>
                <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="4 4" vertical={false} />
                <XAxis dataKey="label" stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} interval={1} angle={-45} textAnchor="end" height={70} />
                <YAxis stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} tickFormatter={(value) => formatHourTick(value)} />
                <Tooltip formatter={(value: number) => formatDuration(Number(value))} contentStyle={tooltipStyle} />
                <Bar dataKey="duration_seconds" fill="#0065FF" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card>

        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Category Mix</p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">Distribusi kategori</h2>
          </div>
          <div className="mt-6 h-[320px]">
            {data.category_breakdown.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%" minHeight={240} minWidth={1}>
                <PieChart>
                  <Pie data={data.category_breakdown} dataKey="duration_seconds" nameKey="category" innerRadius={72} outerRadius={104}>
                    {data.category_breakdown.map((entry, index) => (
                      <Cell key={entry.category} fill={CATEGORY_COLORS[index % CATEGORY_COLORS.length]} />
                    ))}
                  </Pie>
                  <Tooltip formatter={(value: number) => formatDuration(Number(value))} contentStyle={tooltipStyle} />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            ) : (
              <EmptyState
                icon={PieChartIcon}
                title="Belum ada kategori"
                description="Kategori aktivitas akan muncul setelah heartbeat extension tercatat."
              />
            )}
          </div>
        </Card>
      </div>

      <Card className="p-6">
        <div className="border-b border-border pb-4">
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Top Domains</p>
          <h2 className="mt-2 text-[22px] font-bold text-text-primary">Domain paling sering digunakan</h2>
        </div>
        <div className="mt-6">
          <DataTable
            columns={topDomainColumns}
            data={data.top_domains}
            emptyDescription="Top domains akan tampil setelah extension mencatat aktivitas di browser."
            emptyTitle="Belum ada domain activity"
            getRowId={(row) => row.domain}
          />
        </div>
      </Card>
    </div>
  );
}

function TeamActivityTab({
  data,
  summary,
  columns,
  consentAuditColumns,
  consentAuditItems,
  isLoading,
  stackedCategoryKeys,
  stackedTeamData,
  onSelectUser,
  showConsentAudit,
}: {
  data?: TrackerTeamOverview;
  summary?: TrackerDailySummary;
  columns: Array<DataTableColumn<TrackerUserSummary>>;
  consentAuditColumns: Array<DataTableColumn<TrackerConsentAudit>>;
  consentAuditItems?: TrackerConsentAudit[];
  isLoading: boolean;
  stackedCategoryKeys: string[];
  stackedTeamData: Array<Record<string, string | number>>;
  onSelectUser: (user: TrackerUserSummary) => void;
  showConsentAudit: boolean;
}) {
  if (isLoading) {
    return <OverviewSkeleton />;
  }
  if (!data || data.users.length === 0) {
    return <EmptyState icon={Users2} title="Belum ada data team tracker" description="Per-user summary akan muncul setelah extension mengirim heartbeat dari anggota tim." />;
  }
  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-4">
        <StatCard icon={Users2} label="Members Tracked" tone="ops" value={String(data.members_tracked)} />
        <StatCard icon={Clock3} label="Avg Active Time" mono tone="info" value={formatDuration(data.avg_active_seconds)} />
        <StatCard icon={BarChart3} label="Top Productive Member" tone="success" value={data.top_productive_member ?? "-"} />
        <StatCard icon={Activity} label="Least Productive Member" tone="warning" value={data.least_productive_member ?? "-"} />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1.3fr,1fr]">
        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Category Comparison</p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">Stacked category time per user</h2>
          </div>
          <div className="mt-6 h-[320px]">
            <ResponsiveContainer width="100%" height="100%" minHeight={240} minWidth={1}>
              <BarChart data={stackedTeamData}>
                <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="4 4" vertical={false} />
                <XAxis dataKey="user_name" stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} />
                <YAxis stroke="hsl(var(--text-tertiary))" tickLine={false} axisLine={false} tickFormatter={(value) => formatHourTick(value)} />
                <Tooltip formatter={(value: number) => formatDuration(Number(value))} contentStyle={tooltipStyle} />
                <Legend />
                {stackedCategoryKeys.map((key, index) => (
                  <Bar key={key} dataKey={key} stackId="categories" fill={CATEGORY_COLORS[index % CATEGORY_COLORS.length]} radius={index === stackedCategoryKeys.length - 1 ? [4, 4, 0, 0] : [0, 0, 0, 0]} />
                ))}
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card>

        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Daily Summary</p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">Domain highlights</h2>
          </div>
          <div className="mt-6 space-y-6">
            <div>
              <p className="text-sm font-semibold text-text-primary">Top productive domains</p>
              <div className="mt-3 space-y-2">
                {summary?.top_productive_domains?.length ? summary.top_productive_domains.map((item) => (
                  <div key={item.domain} className="flex items-center justify-between rounded-md border border-border bg-surface-muted/50 px-3 py-2">
                    <div>
                      <p className="font-mono text-[13px] text-text-primary">{item.domain}</p>
                      <p className="text-xs text-text-secondary">{item.category}</p>
                    </div>
                    <StatusBadge status="productive" />
                  </div>
                )) : <p className="text-sm text-text-secondary">Belum ada domain produktif untuk tanggal ini.</p>}
              </div>
            </div>
            <div>
              <p className="text-sm font-semibold text-text-primary">Top unproductive domains</p>
              <div className="mt-3 space-y-2">
                {summary?.top_unproductive_domains?.length ? summary.top_unproductive_domains.map((item) => (
                  <div key={item.domain} className="flex items-center justify-between rounded-md border border-border bg-surface-muted/50 px-3 py-2">
                    <div>
                      <p className="font-mono text-[13px] text-text-primary">{item.domain}</p>
                      <p className="text-xs text-text-secondary">{item.category}</p>
                    </div>
                    <StatusBadge status="unproductive" />
                  </div>
                )) : <p className="text-sm text-text-secondary">Belum ada domain non-produktif untuk tanggal ini.</p>}
              </div>
            </div>
          </div>
        </Card>
      </div>

      <Card className="p-6">
        <div className="border-b border-border pb-4">
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Per-user Summary</p>
          <h2 className="mt-2 text-[22px] font-bold text-text-primary">Klik row untuk drill-down</h2>
        </div>
        <div className="mt-6">
          <DataTable
            columns={columns}
            data={data.users}
            emptyDescription="Summary per user akan muncul setelah extension aktif dipakai tim."
            emptyTitle="Belum ada user yang ter-track"
            getRowId={(row) => row.user_id}
            onRowClick={onSelectUser}
          />
        </div>
      </Card>

      {showConsentAudit ? (
        <Card className="p-6">
          <div className="border-b border-border pb-4">
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ops">Consent Audit</p>
            <h2 className="mt-2 text-[22px] font-bold text-text-primary">Siapa yang menyalakan atau mematikan tracker</h2>
          </div>
          <div className="mt-6">
            <DataTable
              columns={consentAuditColumns}
              data={consentAuditItems ?? []}
              emptyDescription="Riwayat consent akan muncul setelah user pertama kali mengaktifkan tracker di browser mereka."
              emptyTitle="Belum ada consent tracker"
              getRowId={(row) => row.user_id}
            />
          </div>
        </Card>
      ) : null}
    </div>
  );
}

function formatDateInput(value: Date) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function formatDuration(totalSeconds: number) {
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}j ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}d`;
  }
  return `${seconds}d`;
}

function formatPercent(value: number) {
  return `${value.toFixed(1)}%`;
}

function formatDateTime(value?: string | null) {
  if (!value) {
    return "-";
  }

  return new Date(value).toLocaleString("id-ID");
}

function formatHourTick(value: number) {
  const hours = Math.floor(value / 3600);
  if (hours > 0) {
    return `${hours}j`;
  }
  const minutes = Math.floor(value / 60);
  return `${minutes}m`;
}

const tooltipStyle = {
  backgroundColor: "hsl(var(--surface))",
  border: "1px solid hsl(var(--border))",
  borderRadius: 8,
  boxShadow: "0 4px 8px -2px rgba(23,43,77,0.08), 0 2px 4px -2px rgba(23,43,77,0.06)",
};

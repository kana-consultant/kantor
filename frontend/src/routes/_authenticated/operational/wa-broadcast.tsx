import { useState, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  MessageCircle,
  Play,
  Square,
  Send,
  Eye,
  Pencil,
  Trash2,
  Lock,
  Plus,
  RefreshCw,
  ChevronDown,
  ChevronRight,
  Wifi,
  WifiOff,
  QrCode,
  Info,
  Copy,
} from "lucide-react";

import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { useRBAC } from "@/hooks/use-rbac";
import { cn } from "@/lib/utils";
import {
  waKeys,
  getWAStatus,
  getWAStats,
  getWAQR,
  startWASession,
  stopWASession,
  listTemplates,
  createTemplate,
  updateTemplate,
  deleteTemplate,
  previewTemplate,
  listSchedules,
  createSchedule,
  updateSchedule,
  deleteSchedule,
  triggerSchedule,
  toggleSchedule,
  listLogs,
  getLogSummary,
  quickSend,
  type WATemplate,
  type WASchedule,
  type WALogFilters,
} from "@/services/wa-broadcast";

export const Route = createFileRoute("/_authenticated/operational/wa-broadcast")({
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalWAView);
  },
  component: WABroadcastPage,
});

type TabKey = "dashboard" | "templates" | "schedules" | "logs";

function WABroadcastPage() {
  const [activeTab, setActiveTab] = useState<TabKey>("dashboard");

  const tabs: { key: TabKey; label: string }[] = [
    { key: "dashboard", label: "Dashboard" },
    { key: "templates", label: "Templates" },
    { key: "schedules", label: "Schedules" },
    { key: "logs", label: "Logs" },
  ];

  return (
    <div className="space-y-6">
      <div>
        <div className="flex items-center gap-3">
          <MessageCircle className="h-6 w-6 text-ops" />
          <h1 className="text-2xl font-bold">WA Broadcast</h1>
        </div>
        <p className="mt-1 text-sm text-text-secondary">
          Kelola WhatsApp broadcast, template pesan, dan jadwal pengiriman otomatis
        </p>
      </div>

      <div className="flex gap-1 rounded-lg bg-surface-muted p-1">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={cn(
              "rounded-md px-4 py-2 text-sm font-medium transition-colors",
              activeTab === tab.key
                ? "bg-surface text-text-primary shadow-sm"
                : "text-text-secondary hover:text-text-primary"
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === "dashboard" && <DashboardTab />}
      {activeTab === "templates" && <TemplatesTab />}
      {activeTab === "schedules" && <SchedulesTab />}
      {activeTab === "logs" && <LogsTab />}
    </div>
  );
}

// ===================== Dashboard Tab =====================

function DashboardTab() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.operationalWAManage);

  const statusQuery = useQuery({ queryKey: waKeys.status(), queryFn: getWAStatus, refetchInterval: 5000 });
  const statsQuery = useQuery({ queryKey: waKeys.stats(), queryFn: getWAStats, refetchInterval: 30000 });
  const summaryQuery = useQuery({ queryKey: waKeys.logSummary(), queryFn: () => getLogSummary() });
  const [quickSendOpen, setQuickSendOpen] = useState(false);

  const sessionStatus = statusQuery.data?.session?.status ?? "STOPPED";
  const enabled = statusQuery.data?.enabled ?? false;
  const isConnected = sessionStatus === "WORKING";
  const isFailed = sessionStatus === "FAILED";
  const isScanning = sessionStatus === "SCAN_QR_CODE";
  const isStarting = sessionStatus === "STARTING";

  const startMutation = useMutation({
    mutationFn: startWASession,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: waKeys.status() });
      queryClient.invalidateQueries({ queryKey: [...waKeys.status(), "qr"] });
    },
  });
  const stopMutation = useMutation({
    mutationFn: stopWASession,
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: waKeys.status() }); },
  });

  // Only fetch QR when WAHA is actually in SCAN_QR_CODE state
  const qrQuery = useQuery({
    queryKey: [...waKeys.status(), "qr"],
    queryFn: getWAQR,
    enabled: isScanning,
    refetchInterval: isScanning ? 5000 : false,
    retry: false,
  });

  return (
    <div className="space-y-6">
      {/* Connection Card */}
      <Card className="p-6">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Koneksi WhatsApp</h3>
          {enabled && (
            <div className="flex items-center gap-2">
              {canManage && !isConnected && !isScanning && (
                <Button size="sm" onClick={() => startMutation.mutate()} disabled={startMutation.isPending}>
                  <Play className="mr-1.5 h-3.5 w-3.5" />
                  {startMutation.isPending ? "Memulai..." : isFailed ? "Reconnect" : "Start Session"}
                </Button>
              )}
              {canManage && isConnected && (
                <Button size="sm" variant="outline" onClick={() => stopMutation.mutate()} disabled={stopMutation.isPending}>
                  <Square className="mr-1.5 h-3.5 w-3.5" />
                  {stopMutation.isPending ? "Stopping..." : "Stop Session"}
                </Button>
              )}
            </div>
          )}
        </div>

        {!enabled ? (
          <div className="mt-4 flex items-center gap-3 rounded-lg border border-border bg-surface-muted p-4">
            <WifiOff className="h-5 w-5 text-text-secondary" />
            <p className="text-sm text-text-secondary">
              WA Broadcast tidak aktif. Set <code className="rounded bg-surface px-1.5 py-0.5 text-xs font-mono">WAHA_ENABLED=true</code> di environment variables.
            </p>
          </div>
        ) : (
          <div className="mt-4 space-y-4">
            {/* Status indicator */}
            <div className="flex items-center gap-3">
              <div className={cn(
                "flex h-10 w-10 items-center justify-center rounded-full",
                isConnected ? "bg-green-100" : isScanning ? "bg-yellow-100" : isFailed ? "bg-red-100" : "bg-surface-muted"
              )}>
                {isConnected ? (
                  <Wifi className="h-5 w-5 text-green-600" />
                ) : isScanning ? (
                  <QrCode className="h-5 w-5 text-yellow-600" />
                ) : isFailed ? (
                  <WifiOff className="h-5 w-5 text-red-500" />
                ) : isStarting ? (
                  <RefreshCw className="h-5 w-5 animate-spin text-yellow-600" />
                ) : (
                  <WifiOff className="h-5 w-5 text-text-secondary" />
                )}
              </div>
              <div>
                <p className="text-sm font-semibold">
                  {isConnected
                    ? "Terhubung"
                    : isScanning
                    ? "Scan QR Code"
                    : isStarting
                    ? "Memulai sesi..."
                    : isFailed
                    ? "Sesi Gagal"
                    : "Tidak Terhubung"
                  }
                </p>
                <p className="text-xs text-text-secondary">
                  {isConnected && statsQuery.data?.account
                    ? `Login sebagai ${statsQuery.data.account.pushName}`
                    : isScanning
                    ? "Scan QR code di bawah dengan WhatsApp di HP"
                    : isStarting
                    ? "Tunggu sebentar..."
                    : isFailed
                    ? "Klik Reconnect untuk reset sesi dan scan QR ulang"
                    : "Klik Start Session untuk memulai"
                  }
                </p>
              </div>
            </div>

            {/* Starting spinner */}
            {(isStarting || startMutation.isPending) && (
              <div className="flex flex-col items-center gap-3 rounded-lg border border-border bg-surface-muted/50 p-8">
                <RefreshCw className="h-8 w-8 animate-spin text-text-secondary" />
                <p className="text-sm text-text-secondary">Menginisialisasi sesi WhatsApp...</p>
              </div>
            )}

            {/* QR Code display — only when WAHA is in SCAN_QR_CODE state */}
            {isScanning && qrQuery.data?.qr && (
              <div className="flex flex-col items-center gap-4 rounded-lg border border-yellow-200 bg-yellow-50/50 p-6 dark:border-yellow-800 dark:bg-yellow-950/20">
                {qrQuery.data.qr.startsWith("data:") ? (
                  <img src={qrQuery.data.qr} alt="WhatsApp QR Code" className="h-[264px] w-[264px] rounded-lg bg-white p-2" />
                ) : (
                  <div className="flex h-[264px] w-[264px] items-center justify-center rounded-lg border-2 border-dashed border-border bg-white p-4">
                    <p className="break-all text-center text-xs font-mono text-text-secondary">{qrQuery.data.qr}</p>
                  </div>
                )}
                <div className="text-center">
                  <p className="text-sm font-medium text-yellow-800 dark:text-yellow-300">
                    Scan QR code ini
                  </p>
                  <p className="mt-1 text-xs text-text-secondary">
                    WhatsApp &rarr; Menu &rarr; Linked Devices &rarr; Link a Device
                  </p>
                </div>
              </div>
            )}
            {isScanning && qrQuery.isLoading && (
              <div className="flex flex-col items-center gap-3 rounded-lg border border-border bg-surface-muted/50 p-8">
                <RefreshCw className="h-8 w-8 animate-spin text-text-secondary" />
                <p className="text-sm text-text-secondary">Memuat QR code...</p>
              </div>
            )}
          </div>
        )}
      </Card>

      {/* Stats */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card className="p-4">
          <p className="text-xs font-medium uppercase tracking-wider text-text-secondary">Terkirim Hari Ini</p>
          <p className="mt-1 text-2xl font-bold">{summaryQuery.data?.sent_today ?? 0}</p>
          <div className="mt-2 h-1.5 rounded-full bg-surface-muted">
            <div
              className="h-1.5 rounded-full bg-green-500 transition-all"
              style={{ width: `${Math.min(100, ((summaryQuery.data?.sent_today ?? 0) / Math.max(1, summaryQuery.data?.daily_limit ?? 50)) * 100)}%` }}
            />
          </div>
          <p className="mt-1 text-xs text-text-secondary">
            / {summaryQuery.data?.daily_limit ?? 50} daily limit
          </p>
        </Card>
        <Card className="p-4">
          <p className="text-xs font-medium uppercase tracking-wider text-text-secondary">Gagal</p>
          <p className="mt-1 text-2xl font-bold text-red-500">{summaryQuery.data?.total_failed ?? 0}</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs font-medium uppercase tracking-wider text-text-secondary">Dilewati</p>
          <p className="mt-1 text-2xl font-bold text-yellow-500">{summaryQuery.data?.total_skipped ?? 0}</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs font-medium uppercase tracking-wider text-text-secondary">Total Terkirim</p>
          <p className="mt-1 text-2xl font-bold">{summaryQuery.data?.total_sent ?? 0}</p>
        </Card>
      </div>

      {/* Quick Send */}
      {canManage && (
        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">Quick Send</h3>
              <p className="text-sm text-text-secondary">Kirim pesan WhatsApp langsung ke nomor tertentu</p>
            </div>
            <Button size="sm" onClick={() => setQuickSendOpen(true)}>
              <Send className="mr-1.5 h-3.5 w-3.5" /> Kirim Pesan
            </Button>
          </div>
        </Card>
      )}

      <QuickSendDialog open={quickSendOpen} onOpenChange={setQuickSendOpen} />
    </div>
  );
}

function QuickSendDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (v: boolean) => void }) {
  const [phone, setPhone] = useState("");
  const [message, setMessage] = useState("");

  const mutation = useMutation({
    mutationFn: () => quickSend(phone, message),
    onSuccess: () => {
      onOpenChange(false);
      setPhone("");
      setMessage("");
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Quick Send WhatsApp</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div>
            <label className="text-sm font-medium text-text-primary">Nomor WhatsApp</label>
            <Input className="mt-1.5" placeholder="08xxxxxxxxxx" value={phone} onChange={(e) => setPhone(e.target.value)} />
            <p className="mt-1 text-xs text-text-secondary">Format: 08xxx, +628xxx, atau 628xxx</p>
          </div>
          <div>
            <label className="text-sm font-medium text-text-primary">Pesan</label>
            <textarea
              className="mt-1.5 w-full rounded-md border border-border bg-background px-3 py-2 text-sm min-h-[120px] focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder="Ketik pesan..."
              value={message}
              onChange={(e) => setMessage(e.target.value)}
            />
          </div>
          {mutation.isError && (
            <div className="rounded-md bg-red-50 p-3 text-sm text-red-600">
              Gagal mengirim pesan. Pastikan sesi WhatsApp aktif.
            </div>
          )}
          {mutation.isSuccess && (
            <div className="rounded-md bg-green-50 p-3 text-sm text-green-600">
              Pesan berhasil dikirim!
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Batal</Button>
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending || !phone.trim() || !message.trim()}>
            {mutation.isPending ? "Mengirim..." : "Kirim"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ===================== Templates Tab =====================

/** Map of available variables per template category/slug */
const TEMPLATE_VARIABLES: Record<string, { label: string; vars: string[] }> = {
  task_assigned: {
    label: "Task Assigned",
    vars: ["name", "task_title", "project_name", "due_date", "priority", "app_url"],
  },
  task_due_today: {
    label: "Task Due Today",
    vars: ["name", "task_title", "project_name", "due_date", "priority", "app_url"],
  },
  task_overdue: {
    label: "Task Overdue",
    vars: ["name", "task_title", "project_name", "due_date", "priority", "app_url"],
  },
  project_deadline_h3: {
    label: "Project Deadline H-3",
    vars: ["name", "project_name", "deadline", "project_status", "open_tasks_count", "total_tasks_count", "app_url"],
  },
  weekly_digest: {
    label: "Weekly Digest",
    vars: ["name", "week_start", "week_end", "completed_count", "open_count", "overdue_count", "app_url"],
  },
  reimbursement_status: {
    label: "Reimbursement Status",
    vars: ["name", "reimbursement_title", "amount", "new_status", "reviewer_notes_section", "app_url"],
  },
  _general: {
    label: "Umum (semua template)",
    vars: ["name", "app_url"],
  },
};

function TemplatesTab() {
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.operationalWAManage);
  const queryClient = useQueryClient();

  const [categoryFilter, setCategoryFilter] = useState("");
  const [triggerFilter, setTriggerFilter] = useState("");
  const [editTemplate, setEditTemplate] = useState<WATemplate | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [previewData, setPreviewData] = useState<{ text: string; name: string } | null>(null);

  const templatesQuery = useQuery({
    queryKey: waKeys.templates(categoryFilter, triggerFilter),
    queryFn: () => listTemplates(categoryFilter || undefined, triggerFilter || undefined),
  });

  const deleteMutation = useMutation({
    mutationFn: deleteTemplate,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: waKeys.all }),
  });

  const previewMutation = useMutation({
    mutationFn: previewTemplate,
  });

  const handlePreview = async (t: WATemplate) => {
    const result = await previewMutation.mutateAsync(t.id);
    setPreviewData({ text: result.preview, name: t.name });
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex gap-2">
          <select className="rounded-md border border-border bg-background px-3 py-1.5 text-sm" value={categoryFilter} onChange={(e) => setCategoryFilter(e.target.value)}>
            <option value="">Semua Kategori</option>
            <option value="operational">Operational</option>
            <option value="hris">HRIS</option>
            <option value="marketing">Marketing</option>
            <option value="general">General</option>
          </select>
          <select className="rounded-md border border-border bg-background px-3 py-1.5 text-sm" value={triggerFilter} onChange={(e) => setTriggerFilter(e.target.value)}>
            <option value="">Semua Trigger</option>
            <option value="auto_scheduled">Auto Scheduled</option>
            <option value="event_triggered">Event Triggered</option>
            <option value="manual">Manual</option>
          </select>
        </div>
        {canManage && (
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="mr-1.5 h-3.5 w-3.5" /> Buat Template
          </Button>
        )}
      </div>

      <div className="rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-surface-muted/50">
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Nama</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Slug</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Kategori</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Trigger</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Status</th>
              <th className="px-4 py-3 text-right font-medium text-text-secondary">Aksi</th>
            </tr>
          </thead>
          <tbody>
            {templatesQuery.data?.map((t) => (
              <tr key={t.id} className="border-b border-border/50 last:border-0">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    {t.is_system && <Lock className="h-3.5 w-3.5 text-text-secondary" />}
                    <span className="font-medium">{t.name}</span>
                  </div>
                  {t.description && <p className="mt-0.5 text-xs text-text-secondary">{t.description}</p>}
                </td>
                <td className="px-4 py-3 font-mono text-xs text-text-secondary">{t.slug}</td>
                <td className="px-4 py-3"><CategoryBadge category={t.category} /></td>
                <td className="px-4 py-3"><TriggerBadge trigger={t.trigger_type} /></td>
                <td className="px-4 py-3">
                  <span className={cn("inline-flex rounded-full px-2 py-0.5 text-xs font-medium",
                    t.is_active ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400" : "bg-surface-muted text-text-secondary"
                  )}>
                    {t.is_active ? "Aktif" : "Nonaktif"}
                  </span>
                </td>
                <td className="px-4 py-3 text-right">
                  <div className="flex justify-end gap-1">
                    <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => handlePreview(t)} title="Preview">
                      <Eye className="h-4 w-4" />
                    </Button>
                    {canManage && (
                      <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => setEditTemplate(t)} title="Edit">
                        <Pencil className="h-4 w-4" />
                      </Button>
                    )}
                    {canManage && !t.is_system && (
                      <Button size="icon" variant="ghost" className="h-8 w-8 text-red-500 hover:text-red-600" onClick={() => { if (confirm("Hapus template ini?")) deleteMutation.mutate(t.id); }} title="Hapus">
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {templatesQuery.data?.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-12 text-center text-text-secondary">Belum ada template</td></tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Preview Dialog */}
      <Dialog open={previewData !== null} onOpenChange={() => setPreviewData(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader><DialogTitle>Preview: {previewData?.name}</DialogTitle></DialogHeader>
          <div className="rounded-lg bg-surface-muted p-4">
            <pre className="whitespace-pre-wrap text-sm leading-relaxed">{previewData?.text}</pre>
          </div>
          <p className="text-xs text-text-secondary">
            * Variabel ditampilkan dengan data contoh. Pesan aktual akan menggunakan data real.
          </p>
        </DialogContent>
      </Dialog>

      {/* Create/Edit Dialog */}
      {(createOpen || editTemplate) && (
        <TemplateFormDialog
          template={editTemplate}
          onClose={() => { setCreateOpen(false); setEditTemplate(null); }}
        />
      )}
    </div>
  );
}

function TemplateFormDialog({ template, onClose }: { template: WATemplate | null; onClose: () => void }) {
  const queryClient = useQueryClient();
  const isEdit = !!template;
  const isSystem = template?.is_system ?? false;

  const [name, setName] = useState(template?.name ?? "");
  const [slug, setSlug] = useState(template?.slug ?? "");
  const [category, setCategory] = useState(template?.category ?? "operational");
  const [triggerType, setTriggerType] = useState(template?.trigger_type ?? "manual");
  const [bodyTemplate, setBodyTemplate] = useState(template?.body_template ?? "");
  const [description, setDescription] = useState(template?.description ?? "");
  const [isActive, setIsActive] = useState(template?.is_active ?? true);

  // Auto-generate slug from name (only for new templates)
  useEffect(() => {
    if (!isEdit && name) {
      setSlug(name.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_|_$/g, ""));
    }
  }, [name, isEdit]);

  const mutation = useMutation({
    mutationFn: () => {
      const data = {
        name, slug, category, trigger_type: triggerType,
        body_template: bodyTemplate, description: description || null,
        is_active: isActive, available_variables: availableVars,
      };
      return isEdit ? updateTemplate(template!.id, data) : createTemplate(data);
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: waKeys.all }); onClose(); },
  });

  // Determine available variables based on slug
  const varInfo = TEMPLATE_VARIABLES[slug] ?? TEMPLATE_VARIABLES._general;
  const availableVars = varInfo.vars;

  const insertVariable = (varName: string) => {
    setBodyTemplate((prev) => prev + `{{${varName}}}`);
  };

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent className="max-w-3xl max-h-[85vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit Template" : "Buat Template Baru"}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 overflow-y-auto flex-1 pr-1">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-sm font-medium text-text-primary">Nama Template</label>
              <Input className="mt-1.5" placeholder="e.g. Reminder Meeting" value={name} onChange={(e) => setName(e.target.value)} disabled={isSystem} />
            </div>
            <div>
              <label className="text-sm font-medium text-text-primary">Slug</label>
              <Input className="mt-1.5 font-mono text-sm" placeholder="e.g. reminder_meeting" value={slug} onChange={(e) => setSlug(e.target.value)} disabled={isEdit} />
              <p className="mt-1 text-xs text-text-secondary">Identifier unik, tidak bisa diubah setelah dibuat</p>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-sm font-medium text-text-primary">Kategori</label>
              <select className="mt-1.5 w-full rounded-md border border-border bg-background px-3 py-2 text-sm" value={category} onChange={(e) => setCategory(e.target.value)} disabled={isSystem}>
                <option value="operational">Operational</option>
                <option value="hris">HRIS</option>
                <option value="marketing">Marketing</option>
                <option value="general">General</option>
              </select>
            </div>
            <div>
              <label className="text-sm font-medium text-text-primary">Trigger Type</label>
              <select className="mt-1.5 w-full rounded-md border border-border bg-background px-3 py-2 text-sm" value={triggerType} onChange={(e) => setTriggerType(e.target.value)} disabled={isSystem}>
                <option value="auto_scheduled">Auto Scheduled</option>
                <option value="event_triggered">Event Triggered</option>
                <option value="manual">Manual</option>
              </select>
            </div>
          </div>

          <div>
            <label className="text-sm font-medium text-text-primary">Deskripsi</label>
            <Input className="mt-1.5" placeholder="Deskripsi singkat tentang template ini" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>

          {/* Available Variables */}
          <div className="rounded-lg border border-blue-200 bg-blue-50/50 p-3 dark:border-blue-800 dark:bg-blue-950/20">
            <div className="flex items-center gap-2 mb-2">
              <Info className="h-4 w-4 text-blue-600" />
              <span className="text-sm font-medium text-blue-700 dark:text-blue-400">Variabel yang tersedia</span>
            </div>
            <p className="text-xs text-blue-600 dark:text-blue-400 mb-2">
              Klik variabel untuk menyisipkan ke body template. Format: {"{{nama_variabel}}"}
            </p>
            <div className="flex flex-wrap gap-1.5">
              {availableVars.map((v) => (
                <button
                  key={v}
                  type="button"
                  onClick={() => insertVariable(v)}
                  className="inline-flex items-center gap-1 rounded-md border border-blue-300 bg-white px-2 py-1 text-xs font-mono text-blue-700 transition hover:bg-blue-100 dark:border-blue-700 dark:bg-blue-900/30 dark:text-blue-300 dark:hover:bg-blue-900/50"
                >
                  <Copy className="h-3 w-3" />
                  {`{{${v}}}`}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="text-sm font-medium text-text-primary">Body Template</label>
            <textarea
              className="mt-1.5 w-full rounded-md border border-border bg-background px-3 py-2 text-sm font-mono min-h-[180px] focus:outline-none focus:ring-2 focus:ring-ring leading-relaxed"
              placeholder={"Halo {{name}},\n\nIni adalah pesan template...\n\n{{app_url}}"}
              value={bodyTemplate}
              onChange={(e) => setBodyTemplate(e.target.value)}
            />
          </div>

          <label className="flex items-center gap-2.5 cursor-pointer">
            <input
              type="checkbox"
              checked={isActive}
              onChange={(e) => setIsActive(e.target.checked)}
              className="h-4 w-4 rounded border-border"
            />
            <span className="text-sm font-medium">Template aktif</span>
          </label>
        </div>

        <DialogFooter className="mt-4">
          <Button variant="outline" onClick={onClose}>Batal</Button>
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending || !name.trim() || !slug.trim() || !bodyTemplate.trim()}>
            {mutation.isPending ? "Menyimpan..." : "Simpan"}
          </Button>
        </DialogFooter>
        {mutation.isError && (
          <div className="rounded-md bg-red-50 p-3 text-sm text-red-600">
            Gagal menyimpan template. Pastikan slug unik dan data valid.
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

// ===================== Schedules Tab =====================

function SchedulesTab() {
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.operationalWAManage);
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [editSchedule, setEditSchedule] = useState<WASchedule | null>(null);

  const schedulesQuery = useQuery({ queryKey: waKeys.schedules(), queryFn: listSchedules });

  const deleteMutation = useMutation({
    mutationFn: deleteSchedule,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: waKeys.all }),
  });

  const triggerMutation = useMutation({
    mutationFn: triggerSchedule,
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) => toggleSchedule(id, active),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: waKeys.all }),
  });

  return (
    <div className="space-y-4">
      <Card className="border-blue-200 bg-blue-50/50 p-4 dark:border-blue-800 dark:bg-blue-950/20">
        <div className="flex gap-3">
          <Info className="h-5 w-5 text-blue-600 shrink-0 mt-0.5" />
          <p className="text-sm text-blue-700 dark:text-blue-400">
            Schedule bawaan (task due/overdue, project deadline H-3, weekly digest) berjalan otomatis sesuai konfigurasi server.
            Tab ini untuk membuat schedule custom tambahan.
          </p>
        </div>
      </Card>

      {canManage && (
        <div className="flex justify-end">
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="mr-1.5 h-3.5 w-3.5" /> Buat Schedule
          </Button>
        </div>
      )}

      <div className="rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-surface-muted/50">
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Nama</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Template</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Jadwal</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Target</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Status</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Terakhir Jalan</th>
              <th className="px-4 py-3 text-right font-medium text-text-secondary">Aksi</th>
            </tr>
          </thead>
          <tbody>
            {schedulesQuery.data?.map((s) => (
              <tr key={s.id} className="border-b border-border/50 last:border-0">
                <td className="px-4 py-3 font-medium">{s.name}</td>
                <td className="px-4 py-3 text-text-secondary">{s.template_name}</td>
                <td className="px-4 py-3 font-mono text-xs">{s.cron_expression ?? s.schedule_type}</td>
                <td className="px-4 py-3">
                  <span className="rounded-full bg-surface-muted px-2 py-0.5 text-xs">{s.target_type.replace(/_/g, " ")}</span>
                </td>
                <td className="px-4 py-3">
                  <button
                    onClick={() => canManage && toggleMutation.mutate({ id: s.id, active: !s.is_active })}
                    className={cn("inline-flex rounded-full px-2 py-0.5 text-xs font-medium transition",
                      s.is_active ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400" : "bg-surface-muted text-text-secondary",
                      canManage && "cursor-pointer hover:opacity-80"
                    )}
                    disabled={!canManage}
                  >
                    {s.is_active ? "Aktif" : "Nonaktif"}
                  </button>
                </td>
                <td className="px-4 py-3 text-xs text-text-secondary">
                  {s.last_run_at ? new Date(s.last_run_at).toLocaleString("id-ID") : "-"}
                </td>
                <td className="px-4 py-3 text-right">
                  <div className="flex justify-end gap-1">
                    {canManage && (
                      <>
                        <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => { if (confirm("Jalankan schedule sekarang?")) triggerMutation.mutate(s.id); }} title="Jalankan Sekarang">
                          <Play className="h-4 w-4" />
                        </Button>
                        <Button size="icon" variant="ghost" className="h-8 w-8" onClick={() => setEditSchedule(s)} title="Edit">
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button size="icon" variant="ghost" className="h-8 w-8 text-red-500 hover:text-red-600" onClick={() => { if (confirm("Hapus schedule ini?")) deleteMutation.mutate(s.id); }} title="Hapus">
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {schedulesQuery.data?.length === 0 && (
              <tr><td colSpan={7} className="px-4 py-12 text-center text-text-secondary">Belum ada custom schedule</td></tr>
            )}
          </tbody>
        </table>
      </div>

      {(createOpen || editSchedule) && (
        <ScheduleFormDialog
          schedule={editSchedule}
          onClose={() => { setCreateOpen(false); setEditSchedule(null); }}
        />
      )}
    </div>
  );
}

function ScheduleFormDialog({ schedule, onClose }: { schedule: WASchedule | null; onClose: () => void }) {
  const queryClient = useQueryClient();
  const isEdit = !!schedule;

  const [name, setName] = useState(schedule?.name ?? "");
  const [templateId, setTemplateId] = useState(schedule?.template_id ?? "");
  const [scheduleType, setScheduleType] = useState(schedule?.schedule_type ?? "daily");
  const [cronExpression, setCronExpression] = useState(schedule?.cron_expression ?? "");
  const [targetType, setTargetType] = useState(schedule?.target_type ?? "all_employees");
  const [isActive, setIsActive] = useState(schedule?.is_active ?? true);

  const templatesQuery = useQuery({
    queryKey: waKeys.templates(),
    queryFn: () => listTemplates(),
  });

  const mutation = useMutation({
    mutationFn: () => {
      const data = {
        name,
        template_id: templateId,
        schedule_type: scheduleType,
        cron_expression: cronExpression || null,
        target_type: targetType,
        target_config: null,
        is_active: isActive,
      };
      return isEdit ? updateSchedule(schedule!.id, data) : createSchedule(data);
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: waKeys.all }); onClose(); },
  });

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent className="max-w-lg">
        <DialogHeader><DialogTitle>{isEdit ? "Edit Schedule" : "Buat Schedule Baru"}</DialogTitle></DialogHeader>
        <div className="space-y-4 py-2">
          <div>
            <label className="text-sm font-medium text-text-primary">Nama Schedule</label>
            <Input className="mt-1.5" placeholder="e.g. Reminder Harian" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div>
            <label className="text-sm font-medium text-text-primary">Template</label>
            <select className="mt-1.5 w-full rounded-md border border-border bg-background px-3 py-2 text-sm" value={templateId} onChange={(e) => setTemplateId(e.target.value)}>
              <option value="">-- Pilih Template --</option>
              {templatesQuery.data?.map((t) => (
                <option key={t.id} value={t.id}>{t.name} ({t.slug})</option>
              ))}
            </select>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-sm font-medium text-text-primary">Tipe Jadwal</label>
              <select className="mt-1.5 w-full rounded-md border border-border bg-background px-3 py-2 text-sm" value={scheduleType} onChange={(e) => setScheduleType(e.target.value)}>
                <option value="daily">Harian</option>
                <option value="weekly">Mingguan</option>
                <option value="monthly">Bulanan</option>
                <option value="once">Sekali</option>
              </select>
            </div>
            <div>
              <label className="text-sm font-medium text-text-primary">Cron Expression</label>
              <Input className="mt-1.5 font-mono text-sm" placeholder="0 8 * * 1-5" value={cronExpression} onChange={(e) => setCronExpression(e.target.value)} />
              <p className="mt-1 text-xs text-text-secondary">Format: menit jam hari bulan hariMinggu</p>
            </div>
          </div>
          <div>
            <label className="text-sm font-medium text-text-primary">Target Penerima</label>
            <select className="mt-1.5 w-full rounded-md border border-border bg-background px-3 py-2 text-sm" value={targetType} onChange={(e) => setTargetType(e.target.value)}>
              <option value="all_employees">Semua Karyawan</option>
              <option value="department">Per Department</option>
              <option value="specific_users">User Tertentu</option>
              <option value="project_members">Anggota Project</option>
            </select>
          </div>
          <label className="flex items-center gap-2.5 cursor-pointer">
            <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} className="h-4 w-4 rounded border-border" />
            <span className="text-sm font-medium">Schedule aktif</span>
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Batal</Button>
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending || !name.trim() || !templateId}>
            {mutation.isPending ? "Menyimpan..." : "Simpan"}
          </Button>
        </DialogFooter>
        {mutation.isError && (
          <div className="mt-2 rounded-md bg-red-50 p-3 text-sm text-red-600">
            Gagal menyimpan schedule.
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

// ===================== Logs Tab =====================

function LogsTab() {
  const [filters, setFilters] = useState<WALogFilters>({ page: 1, perPage: 20 });
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const logsQuery = useQuery({
    queryKey: waKeys.logs(filters),
    queryFn: () => listLogs(filters),
  });
  const summaryQuery = useQuery({ queryKey: waKeys.logSummary(), queryFn: () => getLogSummary() });

  const totalPages = Math.ceil((logsQuery.data?.meta?.total ?? 0) / filters.perPage);

  return (
    <div className="space-y-4">
      {/* Summary cards */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card className="p-4">
          <p className="text-xs font-medium uppercase tracking-wider text-text-secondary">Total Terkirim</p>
          <p className="mt-1 text-xl font-bold text-green-600">{summaryQuery.data?.total_sent ?? 0}</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs font-medium uppercase tracking-wider text-text-secondary">Total Gagal</p>
          <p className="mt-1 text-xl font-bold text-red-600">{summaryQuery.data?.total_failed ?? 0}</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs font-medium uppercase tracking-wider text-text-secondary">Total Dilewati</p>
          <p className="mt-1 text-xl font-bold text-yellow-600">{summaryQuery.data?.total_skipped ?? 0}</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs font-medium uppercase tracking-wider text-text-secondary">Sisa Kuota Harian</p>
          <p className="mt-1 text-xl font-bold">{(summaryQuery.data?.daily_limit ?? 50) - (summaryQuery.data?.sent_today ?? 0)}</p>
        </Card>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-2">
        <select className="rounded-md border border-border bg-background px-3 py-1.5 text-sm" value={filters.triggerType ?? ""} onChange={(e) => setFilters({ ...filters, triggerType: e.target.value || undefined, page: 1 })}>
          <option value="">Semua Trigger</option>
          <option value="auto_scheduled">Auto</option>
          <option value="event_triggered">Event</option>
          <option value="manual_quick_send">Manual</option>
        </select>
        <select className="rounded-md border border-border bg-background px-3 py-1.5 text-sm" value={filters.status ?? ""} onChange={(e) => setFilters({ ...filters, status: e.target.value || undefined, page: 1 })}>
          <option value="">Semua Status</option>
          <option value="sent">Terkirim</option>
          <option value="failed">Gagal</option>
          <option value="skipped_no_phone">Dilewati (no phone)</option>
          <option value="skipped_no_wa">Dilewati (no WA)</option>
          <option value="daily_limit_reached">Limit Harian</option>
        </select>
        <Input
          className="w-48"
          placeholder="Cari nama/nomor..."
          value={filters.search ?? ""}
          onChange={(e) => setFilters({ ...filters, search: e.target.value || undefined, page: 1 })}
        />
        <Button size="icon" variant="outline" className="h-9 w-9" onClick={() => setFilters({ page: 1, perPage: 20 })} title="Reset filter">
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>

      {/* Table */}
      <div className="rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-surface-muted/50">
              <th className="w-8 px-2 py-3" />
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Waktu</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Trigger</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Template</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Penerima</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Status</th>
              <th className="px-4 py-3 text-left font-medium text-text-secondary">Error</th>
            </tr>
          </thead>
          <tbody>
            {logsQuery.data?.items?.map((log) => (
              <LogRow key={log.id} log={log} expanded={expandedId === log.id} onToggle={() => setExpandedId(expandedId === log.id ? null : log.id)} />
            ))}
            {(logsQuery.data?.items?.length ?? 0) === 0 && (
              <tr><td colSpan={7} className="px-4 py-12 text-center text-text-secondary">Belum ada log</td></tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-text-secondary">
            Halaman {filters.page} dari {totalPages} ({logsQuery.data?.meta?.total ?? 0} total)
          </p>
          <div className="flex gap-1">
            <Button size="sm" variant="outline" disabled={filters.page <= 1} onClick={() => setFilters({ ...filters, page: filters.page - 1 })}>
              Prev
            </Button>
            <Button size="sm" variant="outline" disabled={filters.page >= totalPages} onClick={() => setFilters({ ...filters, page: filters.page + 1 })}>
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function LogRow({ log, expanded, onToggle }: { log: import("@/services/wa-broadcast").WABroadcastLog; expanded: boolean; onToggle: () => void }) {
  return (
    <>
      <tr className="border-b border-border/50 last:border-0 cursor-pointer hover:bg-surface-muted/30" onClick={onToggle}>
        <td className="px-2 py-3">
          {expanded ? <ChevronDown className="h-3.5 w-3.5 text-text-secondary" /> : <ChevronRight className="h-3.5 w-3.5 text-text-secondary" />}
        </td>
        <td className="px-4 py-3 text-xs text-text-secondary">{new Date(log.created_at).toLocaleString("id-ID")}</td>
        <td className="px-4 py-3"><TriggerBadge trigger={log.trigger_type} /></td>
        <td className="px-4 py-3 font-mono text-xs text-text-secondary">{log.template_slug ?? "-"}</td>
        <td className="px-4 py-3">
          <div className="text-xs">
            {log.recipient_name && <span className="font-medium text-text-primary">{log.recipient_name} — </span>}
            <span className="text-text-secondary">{maskPhone(log.recipient_phone)}</span>
          </div>
        </td>
        <td className="px-4 py-3"><StatusBadge status={log.status} /></td>
        <td className="px-4 py-3 text-xs text-red-500 truncate max-w-[200px]">{log.error_message ?? ""}</td>
      </tr>
      {expanded && (
        <tr className="bg-surface-muted/20">
          <td colSpan={7} className="px-6 py-4">
            <p className="text-xs font-medium text-text-secondary mb-1.5">Isi Pesan:</p>
            <pre className="whitespace-pre-wrap rounded-lg bg-background border border-border p-4 text-sm leading-relaxed">{log.message_body || "(kosong)"}</pre>
            {log.reference_type && (
              <p className="mt-2 text-xs text-text-secondary">
                Referensi: {log.reference_type} / {log.reference_id}
              </p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

// ===================== Shared Components =====================

function CategoryBadge({ category }: { category: string }) {
  const colors: Record<string, string> = {
    operational: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
    hris: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
    marketing: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
    general: "bg-surface-muted text-text-secondary",
  };
  return (
    <span className={cn("inline-flex rounded-full px-2 py-0.5 text-xs font-medium capitalize", colors[category] ?? colors.general)}>
      {category}
    </span>
  );
}

function TriggerBadge({ trigger }: { trigger: string }) {
  const colors: Record<string, string> = {
    auto_scheduled: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    event_triggered: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
    manual: "bg-surface-muted text-text-secondary",
    manual_quick_send: "bg-surface-muted text-text-secondary",
  };
  const labels: Record<string, string> = {
    auto_scheduled: "auto",
    event_triggered: "event",
    manual: "manual",
    manual_quick_send: "manual",
  };
  return (
    <span className={cn("inline-flex rounded-full px-2 py-0.5 text-xs font-medium", colors[trigger] ?? colors.manual)}>
      {labels[trigger] ?? trigger}
    </span>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    sent: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    failed: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
    queued: "bg-blue-100 text-blue-700",
    skipped_no_phone: "bg-surface-muted text-text-secondary",
    skipped_no_wa: "bg-surface-muted text-text-secondary",
    daily_limit_reached: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  };
  const labels: Record<string, string> = {
    sent: "terkirim",
    failed: "gagal",
    queued: "antrian",
    skipped_no_phone: "no phone",
    skipped_no_wa: "no WA",
    daily_limit_reached: "limit",
  };
  return (
    <span className={cn("inline-flex rounded-full px-2 py-0.5 text-xs font-medium", colors[status] ?? "bg-surface-muted text-text-secondary")}>
      {labels[status] ?? status.replace(/_/g, " ")}
    </span>
  );
}

function maskPhone(phone: string): string {
  if (phone.length <= 6) return phone;
  return phone.slice(0, 4) + "****" + phone.slice(-4);
}

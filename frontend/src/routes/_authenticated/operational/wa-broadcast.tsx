import { useEffect, useState } from "react";
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
  Settings,
  Save,
} from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { StatusBadge as SharedStatusBadge } from "@/components/shared/status-badge";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogBody, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select } from "@/components/ui/select";
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
  getWAConfig,
  updateWAConfig,
  type WAConfig,
  type WABroadcastLog,
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

type TabKey = "dashboard" | "templates" | "schedules" | "logs" | "settings";

function WABroadcastPage() {
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.operationalWAManage);
  const [activeTab, setActiveTab] = useState<TabKey>("dashboard");

  const tabs: { key: TabKey; label: string }[] = [
    { key: "dashboard", label: "Dashboard" },
    { key: "templates", label: "Templates" },
    { key: "schedules", label: "Schedules" },
    { key: "logs", label: "Logs" },
    ...(canManage ? [{ key: "settings" as const, label: "Settings" }] : []),
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
      {activeTab === "settings" && <SettingsTab />}
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
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: waKeys.status() });
      await queryClient.invalidateQueries({ queryKey: [...waKeys.status(), "qr"] });
    },
  });
  const stopMutation = useMutation({
    mutationFn: stopWASession,
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: waKeys.status() }); },
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
              WA Broadcast belum dikonfigurasi. Buka tab <strong>Settings</strong> untuk mengatur koneksi WhatsApp.
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

const GENERAL_TEMPLATE_VARIABLES: { label: string; vars: string[] } = {
  label: "Umum (semua template)",
  vars: ["name", "app_url"],
};

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
  project_deadline_warning: {
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
  _general: GENERAL_TEMPLATE_VARIABLES,
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
  const [templateToDelete, setTemplateToDelete] = useState<WATemplate | null>(null);

  const templatesQuery = useQuery({
    queryKey: waKeys.templates(categoryFilter, triggerFilter),
    queryFn: () => listTemplates(categoryFilter || undefined, triggerFilter || undefined),
  });

  const deleteMutation = useMutation({
    mutationFn: deleteTemplate,
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: waKeys.all }); },
  });

  const previewMutation = useMutation({
    mutationFn: previewTemplate,
  });

  const handlePreview = async (t: WATemplate) => {
    const result = await previewMutation.mutateAsync(t.id);
    setPreviewData({ text: result.preview, name: t.name });
  };

  const columns: Array<DataTableColumn<WATemplate>> = [
    {
      id: "name",
      header: "Nama",
      mobilePrimary: true,
      cell: (template) => (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            {template.is_system ? <Lock className="h-3.5 w-3.5 text-text-secondary" /> : null}
            <span className="font-medium text-text-primary">{template.name}</span>
          </div>
          {template.description ? (
            <p className="text-xs text-text-secondary">{template.description}</p>
          ) : null}
        </div>
      ),
    },
    {
      id: "slug",
      header: "Slug",
      cell: (template) => (
        <span className="font-mono text-xs text-text-secondary">{template.slug}</span>
      ),
      hideOnMobile: true,
    },
    {
      id: "category",
      header: "Kategori",
      cell: (template) => <CategoryBadge category={template.category} />,
    },
    {
      id: "trigger_type",
      header: "Trigger",
      cell: (template) => (
        <SharedStatusBadge
          label={formatWATriggerLabel(template.trigger_type)}
          status={template.trigger_type}
          variant="wa-trigger"
        />
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (template) => (
        <SharedStatusBadge
          label={template.is_active ? "Aktif" : "Nonaktif"}
          status={template.is_active ? "active" : "inactive"}
        />
      ),
    },
    {
      id: "actions",
      header: "Aksi",
      align: "right",
      cell: (template) => (
        <div className="flex justify-end gap-1">
          <Button
            className="h-8 w-8"
            onClick={() => void handlePreview(template)}
            size="icon"
            title="Preview"
            type="button"
            variant="ghost"
          >
            <Eye className="h-4 w-4" />
          </Button>
          {canManage ? (
            <Button
              className="h-8 w-8"
              onClick={() => setEditTemplate(template)}
              size="icon"
              title="Edit"
              type="button"
              variant="ghost"
            >
              <Pencil className="h-4 w-4" />
            </Button>
          ) : null}
          {canManage && !template.is_system ? (
            <Button
              className="h-8 w-8 text-red-500 hover:text-red-600"
              onClick={() => setTemplateToDelete(template)}
              size="icon"
              title="Hapus"
              type="button"
              variant="ghost"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          ) : null}
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="grid gap-2 sm:grid-cols-2">
          <Select
            onValueChange={setCategoryFilter}
            options={[
              { value: "", label: "Semua Kategori" },
              { value: "operational", label: "Operational" },
              { value: "hris", label: "HRIS" },
              { value: "marketing", label: "Marketing" },
              { value: "general", label: "General" },
            ]}
            value={categoryFilter}
          />
          <Select
            onValueChange={setTriggerFilter}
            options={[
              { value: "", label: "Semua Trigger" },
              { value: "auto_scheduled", label: "Auto Scheduled" },
              { value: "event_triggered", label: "Event Triggered" },
              { value: "manual", label: "Manual" },
            ]}
            value={triggerFilter}
          />
        </div>
        {canManage && (
          <Button className="w-full sm:w-auto" size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="mr-1.5 h-3.5 w-3.5" /> Buat Template
          </Button>
        )}
      </div>

      <DataTable
        columns={columns}
        data={templatesQuery.data ?? []}
        emptyActionLabel={canManage ? "Buat Template" : undefined}
        emptyDescription="Template WhatsApp akan muncul di sini setelah dibuat atau diaktifkan."
        emptyTitle="Belum ada template WA"
        getRowId={(template) => template.id}
        loading={templatesQuery.isLoading}
        onEmptyAction={canManage ? () => setCreateOpen(true) : undefined}
      />

      {/* Preview Dialog */}
      <Dialog open={previewData !== null} onOpenChange={() => setPreviewData(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader className="flex items-start justify-between gap-4">
            <div>
              <DialogTitle>Preview: {previewData?.name}</DialogTitle>
              <DialogDescription>
                Pesan di bawah memakai data contoh. Saat dikirim, isi template akan memakai data aktual.
              </DialogDescription>
            </div>
            <DialogClose />
          </DialogHeader>
          <DialogBody className="space-y-3">
            <div className="rounded-lg bg-surface-muted p-4">
            <pre className="whitespace-pre-wrap text-sm leading-relaxed">{previewData?.text}</pre>
            </div>
            <p className="text-xs text-text-secondary">
              Klik area luar modal atau tombol tutup untuk kembali ke daftar template.
            </p>
          </DialogBody>
          <DialogFooter>
            <Button onClick={() => setPreviewData(null)} type="button" variant="ghost">
              Tutup
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create/Edit Dialog */}
      {(createOpen || editTemplate) && (
        <TemplateFormDialog
          template={editTemplate}
          onClose={() => { setCreateOpen(false); setEditTemplate(null); }}
        />
      )}

      <ConfirmDialog
        confirmLabel="Hapus template"
        description={
          templateToDelete
            ? `Template "${templateToDelete.name}" akan dihapus permanen dari tenant ini.`
            : ""
        }
        isLoading={deleteMutation.isPending}
        isOpen={templateToDelete !== null}
        onClose={() => setTemplateToDelete(null)}
        onConfirm={() => {
          if (!templateToDelete) {
            return;
          }
          deleteMutation.mutate(templateToDelete.id, {
            onSuccess: async () => {
              await queryClient.invalidateQueries({ queryKey: waKeys.all });
              setTemplateToDelete(null);
            },
          });
        }}
        title="Hapus template WA?"
      />
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
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: waKeys.all }); onClose(); },
  });

  // Determine available variables based on slug
  const varInfo = TEMPLATE_VARIABLES[slug] ?? GENERAL_TEMPLATE_VARIABLES;
  const availableVars = varInfo.vars;

  const insertVariable = (varName: string) => {
    setBodyTemplate((prev) => prev + `{{${varName}}}`);
  };

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent className="flex max-h-[92vh] flex-col sm:max-w-[860px]" size="xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit Template" : "Buat Template Baru"}</DialogTitle>
          <DialogDescription>
            <span className="sm:hidden">Rakit template WA yang rapi dan siap dipakai.</span>
            <span className="hidden sm:inline">Susun template WhatsApp dengan struktur yang jelas, slug yang rapi, dan body yang siap dipakai untuk automasi maupun kirim manual.</span>
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5 sm:max-h-[70vh]">
          <div className="grid gap-5 lg:grid-cols-[1.05fr,0.95fr]">
            <Card className="order-1 p-4 sm:p-5 lg:col-start-1">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium text-text-primary">Nama Template</label>
                  <Input className="mt-1.5" placeholder="Contoh: Reminder Meeting" value={name} onChange={(e) => setName(e.target.value)} disabled={isSystem} />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium text-text-primary">Slug Template</label>
                  <Input className="mt-1.5 font-mono text-sm" placeholder="contoh: reminder_meeting" value={slug} onChange={(e) => setSlug(e.target.value)} disabled={isEdit} />
                  <p className="text-xs leading-5 text-text-secondary">Identifier unik, tidak bisa diubah setelah dibuat.</p>
                </div>
              </div>

              <div className="mt-4 grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium text-text-primary">Kategori</label>
                  <Select
                    disabled={isSystem}
                    onValueChange={setCategory}
                    options={[
                      { value: "operational", label: "Operasional" },
                      { value: "hris", label: "HRIS" },
                      { value: "marketing", label: "Marketing" },
                      { value: "general", label: "Umum" },
                    ]}
                    value={category}
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium text-text-primary">Tipe Pemicu</label>
                  <Select
                    disabled={isSystem}
                    onValueChange={setTriggerType}
                    options={[
                      { value: "auto_scheduled", label: "Terjadwal Otomatis" },
                      { value: "event_triggered", label: "Dipicu Event" },
                      { value: "manual", label: "Manual" },
                    ]}
                    value={triggerType}
                  />
                </div>
              </div>

              <div className="mt-4 space-y-1.5">
                <label className="text-sm font-medium text-text-primary">Deskripsi</label>
                <Input className="mt-1.5" placeholder="Deskripsi singkat tentang template ini" value={description} onChange={(e) => setDescription(e.target.value)} />
              </div>
            </Card>

            <Card className="order-2 border-blue-200 bg-blue-50/70 p-4 dark:border-blue-800 dark:bg-blue-950/20 sm:p-5 lg:col-start-2 lg:row-start-1">
              <div className="mb-3 flex items-center gap-2">
                <Info className="h-4 w-4 text-blue-600" />
                <span className="text-sm font-semibold text-blue-700 dark:text-blue-400">Variabel yang tersedia</span>
              </div>
              <p className="mb-3 text-xs leading-5 text-blue-700/90 dark:text-blue-300">
                Klik variabel untuk menyisipkan ke body template. Format: {"{{nama_variabel}}"}
              </p>
              <div className="flex flex-wrap gap-2">
                {availableVars.map((v) => (
                  <button
                    key={v}
                    type="button"
                    onClick={() => insertVariable(v)}
                    className="inline-flex items-center gap-1.5 rounded-full border border-blue-300 bg-white px-3 py-1.5 text-xs font-mono text-blue-700 transition hover:bg-blue-100 dark:border-blue-700 dark:bg-blue-900/30 dark:text-blue-300 dark:hover:bg-blue-900/50"
                  >
                    <Copy className="h-3 w-3" />
                    {`{{${v}}}`}
                  </button>
                ))}
              </div>
            </Card>

            <Card className="order-3 p-4 sm:p-5 lg:col-start-1 lg:row-start-2">
              <div className="mb-3 flex items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold text-text-primary">Body Template</p>
                  <p className="mt-1 text-xs leading-5 text-text-secondary">Tulis pesan final yang akan dikirim. Variabel bisa disisipkan dari panel kanan.</p>
                </div>
                <div className="rounded-full bg-surface-muted px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.08em] text-text-secondary">
                  {bodyTemplate.length} chars
                </div>
              </div>
              <textarea
                className="min-h-[220px] w-full rounded-2xl border border-border/70 bg-surface-muted/90 px-4 py-3 font-mono text-sm leading-relaxed text-text-primary outline-none transition-all duration-150 focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
                placeholder={"Halo {{name}},\n\nIni adalah pesan template...\n\n{{app_url}}"}
                value={bodyTemplate}
                onChange={(e) => setBodyTemplate(e.target.value)}
              />
            </Card>

            <Card className="order-4 p-4 sm:p-5 lg:col-start-2 lg:row-start-2">
              <label className="flex cursor-pointer items-start gap-3">
                <input
                  type="checkbox"
                  checked={isActive}
                  onChange={(e) => setIsActive(e.target.checked)}
                  className="mt-1 h-4 w-4 rounded border-border"
                />
                <div>
                  <p className="text-sm font-semibold text-text-primary">Template aktif</p>
                  <p className="mt-1 text-xs leading-5 text-text-secondary">
                    Nonaktifkan jika template hanya ingin disimpan sebagai draft atau arsip.
                  </p>
                </div>
              </label>
            </Card>

            <Card className="order-5 hidden p-4 sm:block sm:p-5 lg:col-start-2 lg:row-start-3">
              <p className="text-sm font-semibold text-text-primary">Catatan cepat</p>
              <ul className="mt-3 space-y-2 text-xs leading-5 text-text-secondary">
                <li>Slug sebaiknya singkat dan stabil karena dipakai sebagai identifier template.</li>
                <li>Gunakan <span className="font-semibold text-text-primary">Manual</span> untuk template kirim cepat.</li>
                <li>Gunakan <span className="font-semibold text-text-primary">Event Triggered</span> atau <span className="font-semibold text-text-primary">Auto Scheduled</span> untuk automasi.</li>
              </ul>
            </Card>
          </div>
        </DialogBody>

        <DialogFooter className="grid grid-cols-2 gap-3 sm:flex sm:justify-end">
          <Button variant="outline" onClick={onClose}>Batal</Button>
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending || !name.trim() || !slug.trim() || !bodyTemplate.trim()}>
            {mutation.isPending ? "Menyimpan..." : "Simpan"}
          </Button>
        </DialogFooter>
        {mutation.isError ? (
          <div className="px-4 pb-4 sm:px-6 sm:pb-5">
            <div className="rounded-xl bg-red-50 p-3 text-sm text-red-600">
              Gagal menyimpan template. Pastikan slug unik dan data valid.
            </div>
          </div>
        ) : null}
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
  const [scheduleAction, setScheduleAction] = useState<
    | { kind: "trigger"; schedule: WASchedule }
    | { kind: "delete"; schedule: WASchedule }
    | null
  >(null);

  const schedulesQuery = useQuery({ queryKey: waKeys.schedules(), queryFn: listSchedules });

  const deleteMutation = useMutation({
    mutationFn: deleteSchedule,
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: waKeys.all }); },
  });

  const triggerMutation = useMutation({
    mutationFn: triggerSchedule,
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) => toggleSchedule(id, active),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: waKeys.all }); },
  });

  const columns: Array<DataTableColumn<WASchedule>> = [
    {
      id: "name",
      header: "Nama",
      mobilePrimary: true,
      cell: (schedule) => <span className="font-medium text-text-primary">{schedule.name}</span>,
    },
    {
      id: "template_name",
      header: "Template",
      cell: (schedule) => <span className="text-text-secondary">{schedule.template_name}</span>,
    },
    {
      id: "schedule",
      header: "Jadwal",
      cell: (schedule) => (
        <span className="font-mono text-xs text-text-secondary">
          {schedule.cron_expression ?? schedule.schedule_type}
        </span>
      ),
    },
    {
      id: "target_type",
      header: "Target",
      cell: (schedule) => (
        <span className="rounded-full bg-surface-muted px-2 py-0.5 text-xs text-text-secondary">
          {formatScheduleTargetLabel(schedule.target_type)}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (schedule) => (
        <button
          className={cn("inline-flex", canManage && "cursor-pointer")}
          disabled={!canManage}
          onClick={() => toggleMutation.mutate({ id: schedule.id, active: !schedule.is_active })}
          type="button"
        >
          <SharedStatusBadge
            label={schedule.is_active ? "Aktif" : "Nonaktif"}
            status={schedule.is_active ? "active" : "inactive"}
          />
        </button>
      ),
    },
    {
      id: "last_run_at",
      header: "Terakhir Jalan",
      cell: (schedule) => (
        <span className="text-xs text-text-secondary">
          {schedule.last_run_at ? new Date(schedule.last_run_at).toLocaleString("id-ID") : "-"}
        </span>
      ),
      hideOnMobile: true,
    },
    {
      id: "actions",
      header: "Aksi",
      align: "right",
      cell: (schedule) => (
        <div className="flex justify-end gap-1">
          {canManage ? (
            <>
              <Button
                className="h-8 w-8"
                onClick={() => setScheduleAction({ kind: "trigger", schedule })}
                size="icon"
                title="Jalankan Sekarang"
                type="button"
                variant="ghost"
              >
                <Play className="h-4 w-4" />
              </Button>
              <Button
                className="h-8 w-8"
                onClick={() => setEditSchedule(schedule)}
                size="icon"
                title="Edit"
                type="button"
                variant="ghost"
              >
                <Pencil className="h-4 w-4" />
              </Button>
              <Button
                className="h-8 w-8 text-red-500 hover:text-red-600"
                onClick={() => setScheduleAction({ kind: "delete", schedule })}
                size="icon"
                title="Hapus"
                type="button"
                variant="ghost"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </>
          ) : null}
        </div>
      ),
    },
  ];

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

      <DataTable
        columns={columns}
        data={schedulesQuery.data ?? []}
        emptyActionLabel={canManage ? "Buat Schedule" : undefined}
        emptyDescription="Schedule custom akan tampil di sini setelah dibuat untuk tenant ini."
        emptyTitle="Belum ada schedule custom"
        getRowId={(schedule) => schedule.id}
        loading={schedulesQuery.isLoading}
        onEmptyAction={canManage ? () => setCreateOpen(true) : undefined}
      />

      {(createOpen || editSchedule) && (
        <ScheduleFormDialog
          schedule={editSchedule}
          onClose={() => { setCreateOpen(false); setEditSchedule(null); }}
        />
      )}

      <ConfirmDialog
        confirmLabel={scheduleAction?.kind === "trigger" ? "Jalankan sekarang" : "Hapus schedule"}
        description={
          scheduleAction
            ? scheduleAction.kind === "trigger"
              ? `Schedule "${scheduleAction.schedule.name}" akan dijalankan manual sekarang.`
              : `Schedule "${scheduleAction.schedule.name}" akan dihapus permanen dari tenant ini.`
            : ""
        }
        isLoading={triggerMutation.isPending || deleteMutation.isPending}
        isOpen={scheduleAction !== null}
        onClose={() => setScheduleAction(null)}
        onConfirm={() => {
          if (!scheduleAction) {
            return;
          }

          if (scheduleAction.kind === "trigger") {
            triggerMutation.mutate(scheduleAction.schedule.id, {
              onSuccess: async () => {
                await queryClient.invalidateQueries({ queryKey: waKeys.all });
                setScheduleAction(null);
              },
            });
            return;
          }

          deleteMutation.mutate(scheduleAction.schedule.id, {
            onSuccess: async () => {
              await queryClient.invalidateQueries({ queryKey: waKeys.all });
              setScheduleAction(null);
            },
          });
        }}
        title={scheduleAction?.kind === "trigger" ? "Jalankan schedule sekarang?" : "Hapus schedule WA?"}
        tone={scheduleAction?.kind === "trigger" ? "warning" : "danger"}
      />
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
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: waKeys.all }); onClose(); },
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
            <div className="mt-1.5">
              <Select
                onValueChange={setTemplateId}
                options={[
                  { value: "", label: "-- Pilih Template --" },
                  ...(templatesQuery.data?.map((t) => ({
                    value: t.id,
                    label: t.name,
                    description: t.slug,
                  })) ?? []),
                ]}
                value={templateId}
              />
            </div>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <label className="text-sm font-medium text-text-primary">Tipe Jadwal</label>
              <div className="mt-1.5">
                <Select
                  onValueChange={setScheduleType}
                  options={[
                    { value: "daily", label: "Harian" },
                    { value: "weekly", label: "Mingguan" },
                    { value: "monthly", label: "Bulanan" },
                    { value: "once", label: "Sekali" },
                  ]}
                  value={scheduleType}
                />
              </div>
            </div>
            <div>
              <label className="text-sm font-medium text-text-primary">Cron Expression</label>
              <Input className="mt-1.5 font-mono text-sm" placeholder="0 8 * * 1-5" value={cronExpression} onChange={(e) => setCronExpression(e.target.value)} />
              <p className="mt-1 text-xs text-text-secondary">Format: menit jam hari bulan hariMinggu</p>
            </div>
          </div>
          <div>
            <label className="text-sm font-medium text-text-primary">Target Penerima</label>
            <div className="mt-1.5">
              <Select
                onValueChange={setTargetType}
                options={[
                  { value: "all_employees", label: "Semua Karyawan" },
                  { value: "department", label: "Per Department" },
                  { value: "specific_users", label: "User Tertentu" },
                  { value: "project_members", label: "Anggota Project" },
                ]}
                value={targetType}
              />
            </div>
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

  const columns: Array<DataTableColumn<WABroadcastLog>> = [
    {
      id: "expand",
      header: "",
      align: "center",
      widthClassName: "w-12",
      hideOnMobile: true,
      cell: (log) =>
        expandedId === log.id ? (
          <ChevronDown className="h-3.5 w-3.5 text-text-secondary" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 text-text-secondary" />
        ),
    },
    {
      id: "created_at",
      header: "Waktu",
      accessor: "created_at",
      sortable: true,
      cell: (log) => (
        <span className="text-xs text-text-secondary">
          {new Date(log.created_at).toLocaleString("id-ID")}
        </span>
      ),
      hideOnMobile: true,
    },
    {
      id: "recipient",
      header: "Penerima",
      mobilePrimary: true,
      cell: (log) => (
        <div className="space-y-1">
          <p className="font-medium text-text-primary">
            {log.recipient_name?.trim() || maskPhone(log.recipient_phone)}
          </p>
          {log.recipient_name ? (
            <p className="text-xs text-text-secondary">{maskPhone(log.recipient_phone)}</p>
          ) : null}
        </div>
      ),
    },
    {
      id: "trigger_type",
      header: "Trigger",
      accessor: "trigger_type",
      cell: (log) => (
        <SharedStatusBadge
          label={formatWATriggerLabel(log.trigger_type)}
          status={log.trigger_type}
          variant="wa-trigger"
        />
      ),
    },
    {
      id: "template_slug",
      header: "Template",
      accessor: "template_slug",
      cell: (log) => (
        <span className="font-mono text-xs text-text-secondary">{log.template_slug ?? "-"}</span>
      ),
      hideOnMobile: true,
    },
    {
      id: "status",
      header: "Status",
      accessor: "status",
      cell: (log) => (
        <SharedStatusBadge
          label={formatWALogStatusLabel(log.status)}
          status={log.status}
          variant="wa-log-status"
        />
      ),
    },
    {
      id: "error_message",
      header: "Error",
      accessor: "error_message",
      cell: (log) => (
        <span className="block max-w-[220px] truncate text-xs text-red-500">
          {log.error_message?.trim() || "-"}
        </span>
      ),
      hideOnMobile: true,
    },
  ];

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
      <div className="grid gap-2 sm:grid-cols-[180px_220px_minmax(0,1fr)_auto]">
        <Select
          onValueChange={(value) => setFilters({ ...filters, triggerType: value || undefined, page: 1 })}
          options={[
            { value: "", label: "Semua Trigger" },
            { value: "auto_scheduled", label: "Auto" },
            { value: "event_triggered", label: "Event" },
            { value: "manual_quick_send", label: "Manual" },
          ]}
          value={filters.triggerType ?? ""}
        />
        <Select
          onValueChange={(value) => setFilters({ ...filters, status: value || undefined, page: 1 })}
          options={[
            { value: "", label: "Semua Status" },
            { value: "sent", label: "Terkirim" },
            { value: "failed", label: "Gagal" },
            { value: "skipped_no_phone", label: "Dilewati (no phone)" },
            { value: "skipped_no_wa", label: "Dilewati (no WA)" },
            { value: "daily_limit_reached", label: "Limit Harian" },
          ]}
          value={filters.status ?? ""}
        />
        <Input
          className="w-full"
          placeholder="Cari nama/nomor..."
          value={filters.search ?? ""}
          onChange={(e) => setFilters({ ...filters, search: e.target.value || undefined, page: 1 })}
        />
        <Button size="icon" variant="outline" className="h-9 w-9" onClick={() => setFilters({ page: 1, perPage: 20 })} title="Reset filter">
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={logsQuery.data?.items ?? []}
        emptyDescription="Belum ada log yang cocok dengan filter aktif."
        emptyTitle="Log WhatsApp belum tersedia"
        getRowId={(row) => row.id}
        loading={logsQuery.isLoading}
        onRowClick={(row) => setExpandedId((current) => (current === row.id ? null : row.id))}
        pagination={
          logsQuery.data?.meta
            ? {
                page: logsQuery.data.meta.page,
                perPage: logsQuery.data.meta.per_page,
                total: logsQuery.data.meta.total,
                onPageChange: (page) => setFilters((current) => ({ ...current, page })),
              }
            : undefined
        }
        renderExpandedRow={(log) => (
          <div className="space-y-4">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <p className="text-sm font-semibold text-text-primary">
                  Detail broadcast untuk {log.recipient_name?.trim() || maskPhone(log.recipient_phone)}
                </p>
                <p className="text-xs text-text-secondary">
                  {new Date(log.created_at).toLocaleString("id-ID")} | {formatWATriggerLabel(log.trigger_type)}
                  {log.template_slug ? ` | ${log.template_slug}` : ""}
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <SharedStatusBadge
                  label={formatWATriggerLabel(log.trigger_type)}
                  status={log.trigger_type}
                  variant="wa-trigger"
                />
                <SharedStatusBadge
                  label={formatWALogStatusLabel(log.status)}
                  status={log.status}
                  variant="wa-log-status"
                />
              </div>
            </div>

            <div className="grid gap-3 text-xs text-text-secondary md:grid-cols-2">
              <div className="rounded-2xl border border-border/70 bg-surface px-4 py-3">
                <p className="font-semibold uppercase tracking-[0.08em] text-text-tertiary">Penerima</p>
                <p className="mt-1 text-sm text-text-primary">{log.recipient_name?.trim() || "-"}</p>
                <p className="mt-1">{maskPhone(log.recipient_phone)}</p>
              </div>
              <div className="rounded-2xl border border-border/70 bg-surface px-4 py-3">
                <p className="font-semibold uppercase tracking-[0.08em] text-text-tertiary">Referensi</p>
                <p className="mt-1 text-sm text-text-primary">
                  {log.reference_type ? `${log.reference_type} / ${log.reference_id ?? "-"}` : "Tidak ada referensi"}
                </p>
                {log.sent_at ? <p className="mt-1">Sent at {new Date(log.sent_at).toLocaleString("id-ID")}</p> : null}
              </div>
            </div>

            {log.error_message ? (
              <div className="rounded-2xl border border-error/20 bg-error-light px-4 py-3">
                <p className="text-xs font-semibold uppercase tracking-[0.08em] text-error">Error</p>
                <p className="mt-1 text-sm text-error">{log.error_message}</p>
              </div>
            ) : null}

            <div className="space-y-2">
              <p className="text-xs font-semibold uppercase tracking-[0.08em] text-text-tertiary">Isi Pesan</p>
              <pre className="whitespace-pre-wrap rounded-2xl border border-border/70 bg-background px-4 py-4 text-sm leading-relaxed text-text-primary">
                {log.message_body || "(kosong)"}
              </pre>
            </div>
          </div>
        )}
        selectedRowId={expandedId}
      />
    </div>
  );
}


// ===================== Settings Tab =====================

function SettingsTab() {
  const queryClient = useQueryClient();
  const configQuery = useQuery({ queryKey: waKeys.config(), queryFn: getWAConfig });

  const [form, setForm] = useState<WAConfig>({
    api_url: "http://localhost:3000",
    api_key: "",
    session_name: "default",
    enabled: false,
    max_daily_messages: 50,
    min_delay_ms: 2000,
    max_delay_ms: 5000,
    reminder_cron: "0 8 * * 1-5",
    weekly_digest_cron: "0 8 * * 1",
  });

  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (configQuery.data && !loaded) {
      setForm(configQuery.data);
      setLoaded(true);
    }
  }, [configQuery.data, loaded]);

  const saveMutation = useMutation({
    mutationFn: () => updateWAConfig(form),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: waKeys.config() });
      await queryClient.invalidateQueries({ queryKey: waKeys.status() });
      await queryClient.invalidateQueries({ queryKey: waKeys.stats() });
    },
  });

  const updateField = <K extends keyof WAConfig>(key: K, value: WAConfig[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  if (configQuery.isLoading) {
    return <Card className="p-6"><p className="text-sm text-text-secondary">Memuat konfigurasi...</p></Card>;
  }

  return (
    <div className="space-y-6">
      <Card className="p-6">
        <div className="flex items-center gap-3 mb-6">
          <Settings className="h-5 w-5 text-ops" />
          <h3 className="text-lg font-semibold">Konfigurasi WhatsApp</h3>
        </div>

        <div className="space-y-6">
          {/* Enable toggle */}
          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => updateField("enabled", e.target.checked)}
              className="h-4 w-4 rounded border-border text-ops focus:ring-ops"
            />
            <div>
              <span className="text-sm font-medium">Aktifkan WA Broadcast</span>
              <p className="text-xs text-text-secondary">Aktifkan pengiriman pesan WhatsApp untuk tenant ini</p>
            </div>
          </label>

          {/* WAHA Connection */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-text-secondary uppercase tracking-wider">Koneksi WAHA</h4>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1.5 block text-sm font-medium">API URL</label>
                <Input
                  value={form.api_url}
                  onChange={(e) => updateField("api_url", e.target.value)}
                  placeholder="http://localhost:3000"
                />
                <p className="mt-1 text-xs text-text-secondary">URL instance WAHA</p>
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium">API Key</label>
                <Input
                  type="password"
                  value={form.api_key}
                  onChange={(e) => updateField("api_key", e.target.value)}
                  placeholder="WAHA API key"
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium">Session Name</label>
                <Input
                  value={form.session_name}
                  onChange={(e) => updateField("session_name", e.target.value)}
                  placeholder="default"
                />
              </div>
            </div>
          </div>

          {/* Rate Limiting */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-text-secondary uppercase tracking-wider">Rate Limiting</h4>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <div>
                <label className="mb-1.5 block text-sm font-medium">Maks Pesan / Hari</label>
                <Input
                  type="number"
                  value={form.max_daily_messages}
                  onChange={(e) => updateField("max_daily_messages", parseInt(e.target.value) || 1)}
                  min={1}
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium">Min Delay (ms)</label>
                <Input
                  type="number"
                  value={form.min_delay_ms}
                  onChange={(e) => updateField("min_delay_ms", parseInt(e.target.value) || 0)}
                  min={0}
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium">Max Delay (ms)</label>
                <Input
                  type="number"
                  value={form.max_delay_ms}
                  onChange={(e) => updateField("max_delay_ms", parseInt(e.target.value) || 0)}
                  min={0}
                />
              </div>
            </div>
          </div>

          {/* Cron Schedules */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-text-secondary uppercase tracking-wider">Jadwal Otomatis</h4>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1.5 block text-sm font-medium">Reminder Cron</label>
                <Input
                  value={form.reminder_cron}
                  onChange={(e) => updateField("reminder_cron", e.target.value)}
                  placeholder="0 8 * * 1-5"
                />
                <p className="mt-1 text-xs text-text-secondary">Jadwal reminder harian (default: Senin-Jumat jam 8)</p>
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium">Weekly Digest Cron</label>
                <Input
                  value={form.weekly_digest_cron}
                  onChange={(e) => updateField("weekly_digest_cron", e.target.value)}
                  placeholder="0 8 * * 1"
                />
                <p className="mt-1 text-xs text-text-secondary">Jadwal digest mingguan (default: Senin jam 8)</p>
              </div>
            </div>
          </div>

          {/* Save button */}
          <div className="flex items-center gap-3 border-t border-border pt-4">
            <Button onClick={() => saveMutation.mutate()} disabled={saveMutation.isPending}>
              <Save className="mr-1.5 h-4 w-4" />
              {saveMutation.isPending ? "Menyimpan..." : "Simpan Konfigurasi"}
            </Button>
            {saveMutation.isSuccess && (
              <p className="text-sm text-green-600">Konfigurasi berhasil disimpan</p>
            )}
            {saveMutation.isError && (
              <p className="text-sm text-red-500">Gagal menyimpan konfigurasi</p>
            )}
          </div>
        </div>
      </Card>
    </div>
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

function formatWATriggerLabel(trigger: string) {
  const labels: Record<string, string> = {
    auto_scheduled: "Auto",
    event_triggered: "Event",
    manual: "Manual",
    manual_quick_send: "Manual",
  };
  return labels[trigger] ?? trigger.replace(/_/g, " ");
}

function formatWALogStatusLabel(status: string) {
  const labels: Record<string, string> = {
    sent: "Terkirim",
    failed: "Gagal",
    queued: "Antrian",
    skipped_no_phone: "No Phone",
    skipped_no_wa: "No WA",
    skipped_disabled: "Disabled",
    daily_limit_reached: "Limit Harian",
  };
  return labels[status] ?? status.replace(/_/g, " ");
}

function formatScheduleTargetLabel(targetType: string) {
  const labels: Record<string, string> = {
    all_employees: "Semua Karyawan",
    department: "Per Department",
    specific_users: "User Tertentu",
    project_members: "Anggota Project",
  };
  return labels[targetType] ?? targetType.replace(/_/g, " ");
}

function maskPhone(phone: string): string {
  if (phone.length <= 6) return phone;
  return phone.slice(0, 4) + "****" + phone.slice(-4);
}


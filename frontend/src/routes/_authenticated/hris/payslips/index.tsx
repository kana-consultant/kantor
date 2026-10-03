import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { FileClock, FilePen, FilePlus2, Info, MailCheck, Search, Send, TriangleAlert, Users, X } from "lucide-react";
import { z } from "zod";

import { MonthYearPicker } from "@/components/compensation-policy/month-year-picker";
import { GeneratePayslipsDialog, type GenerateTarget } from "@/components/payslips/generate-payslips-dialog";
import {
  formatAmount,
  formatDateTime,
  formatPayslipPeriod,
  PayslipProgress,
  PayslipStatusPill,
  WarningChips,
} from "@/components/payslips/payslip-display";
import { PayslipDrawer } from "@/components/payslips/payslip-drawer";
import { SendPayslipsDialog } from "@/components/payslips/send-payslips-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { StatCard } from "@/components/shared/stat-card";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { listPayslips, payslipsKeys } from "@/services/hris-payslips";
import type { PayslipListItem } from "@/types/documents";

const searchSchema = z.object({
  year: z.coerce.number().int().min(2000).max(2100).optional().catch(undefined),
  month: z.coerce.number().int().min(1).max(12).optional().catch(undefined),
  employee: z.string().max(64).optional().catch(undefined),
});

type StatusFilter = "" | "none" | "draft" | "sent" | "failed" | "warning";

const companyFieldLabels: Record<string, string> = {
  legal_name: "nama perusahaan",
  address: "alamat",
  hr_contact_email: "email kontak HR",
  doc_code: "kode dokumen",
};

export const Route = createFileRoute("/_authenticated/hris/payslips/")({
  validateSearch: searchSchema,
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisPayslipView);
    // Every payslip endpoint also needs salary access.
    await ensurePermission(permissions.hrisSalaryView);
  },
  component: PayslipsPage,
});

/** The current month in Asia/Jakarta (the payroll timezone). */
function currentPeriod() {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Jakarta",
    year: "numeric",
    month: "numeric",
  }).formatToParts(new Date());
  const read = (type: string) => Number(parts.find((part) => part.type === type)?.value);
  return { year: read("year"), month: read("month") };
}

/** A failed render, or a failed latest e-mail (a draft, or a 'Kirim ulang' of a sent slip). */
function isFailed(item: PayslipListItem) {
  return item.render_status === "failed" || item.last_delivery?.status === "failed";
}

/** Years before the current one the period picker offers (older slips stay reachable). */
const PAYSLIP_YEARS_BACK = 5;

function isSelectable(item: PayslipListItem) {
  return !(item.status === "none" && item.blocked);
}

function PayslipsPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.hrisPayslipManage);
  const canSend = hasPermission(permissions.hrisPayslipSend);
  const canOpenSettings = hasPermission(permissions.adminSettingsView);

  const today = useMemo(() => currentPeriod(), []);
  const year = search.year ?? today.year;
  const month = search.month ?? today.month;

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerEmployeeId, setDrawerEmployeeId] = useState<string | null>(null);
  const [generateState, setGenerateState] = useState<{
    targets: GenerateTarget[];
    payDate: string | null;
    title: string;
  } | null>(null);
  const [sendIds, setSendIds] = useState<string[] | null>(null);

  // Polls while a PDF renders or an e-mail is queued; otherwise static (each
  // read writes a salary access row).
  const listQuery = useQuery({
    queryKey: payslipsKeys.period(year, month),
    queryFn: () => listPayslips(year, month),
    refetchOnWindowFocus: false,
    refetchInterval: (current) => ((current.state.data?.summary.in_progress ?? 0) > 0 ? 3000 : false),
  });

  useEffect(() => {
    setSelected(new Set());
  }, [year, month]);

  const data = listQuery.data;
  const items = useMemo(() => data?.items ?? [], [data]);

  // Drop selections that no longer exist or became unselectable.
  useEffect(() => {
    setSelected((current) => {
      const allowed = new Set(items.filter(isSelectable).map((item) => item.employee_id));
      const next = new Set([...current].filter((id) => allowed.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [items]);

  const filterEmployee = search.employee ? items.find((item) => item.employee_id === search.employee) : undefined;

  const visibleItems = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return items.filter((item) => {
      if (search.employee && item.employee_id !== search.employee) {
        return false;
      }
      if (needle && !`${item.employee_name} ${item.department ?? ""} ${item.employee_code ?? ""} ${item.doc_number ?? ""}`.toLowerCase().includes(needle)) {
        return false;
      }
      switch (statusFilter) {
        case "none":
          return item.status === "none";
        case "draft":
          return item.status === "draft";
        case "sent":
          return item.status === "sent";
        case "failed":
          return isFailed(item);
        case "warning":
          return (item.warnings ?? []).length > 0;
        default:
          return true;
      }
    });
  }, [items, query, search.employee, statusFilter]);

  const selectedItems = items.filter((item) => selected.has(item.employee_id));
  const generateTargets: GenerateTarget[] = selectedItems
    .filter((item) => (item.status === "none" && !item.blocked) || item.status === "draft")
    .map((item) => ({ employee_id: item.employee_id, employee_name: item.employee_name, status: item.status }));
  const sendTargetIds = selectedItems
    .filter((item) => item.payslip_id && (item.status === "draft" || item.status === "sent"))
    .map((item) => item.payslip_id!);

  const selectableVisible = visibleItems.filter(isSelectable);
  const allVisibleSelected = selectableVisible.length > 0 && selectableVisible.every((item) => selected.has(item.employee_id));

  const toggle = (employeeId: string) =>
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(employeeId)) {
        next.delete(employeeId);
      } else {
        next.add(employeeId);
      }
      return next;
    });

  const toggleAllVisible = () =>
    setSelected((current) => {
      const next = new Set(current);
      if (allVisibleSelected) {
        selectableVisible.forEach((item) => next.delete(item.employee_id));
      } else {
        selectableVisible.forEach((item) => next.add(item.employee_id));
      }
      return next;
    });

  const setPeriod = (period: { year: number; month: number }) => {
    void navigate({ search: (previous) => ({ ...previous, year: period.year, month: period.month }) });
  };

  const drawerItem = drawerEmployeeId ? items.find((item) => item.employee_id === drawerEmployeeId) ?? null : null;

  const columns: Array<DataTableColumn<PayslipListItem>> = [
    {
      id: "select",
      header: "",
      widthClassName: "w-10",
      cell: (item) => (
        <input
          aria-label={`Pilih ${item.employee_name}`}
          checked={selected.has(item.employee_id)}
          className="h-4 w-4 cursor-pointer accent-primary disabled:cursor-not-allowed disabled:opacity-40"
          disabled={!isSelectable(item)}
          onChange={() => toggle(item.employee_id)}
          onClick={(event) => event.stopPropagation()}
          title={isSelectable(item) ? undefined : "Diblokir: lengkapi data gaji terlebih dahulu"}
          type="checkbox"
        />
      ),
    },
    {
      id: "employee",
      header: "Karyawan",
      accessor: "employee_name",
      sortable: true,
      mobilePrimary: true,
      cell: (item) => (
        <div className="min-w-[140px] space-y-0.5">
          <p className="font-semibold text-text-primary">{item.employee_name}</p>
          <p className="text-[12px] text-text-secondary">{item.department || "Tanpa departemen"}</p>
          {item.doc_number ? <p className="font-mono text-[11px] text-text-tertiary">{item.doc_number}</p> : null}
        </div>
      ),
    },
    {
      id: "status",
      header: "Status / Email",
      cell: (item) => <StatusEmailCell item={item} pdfAvailable={data?.pdf_available ?? true} />,
    },
    {
      id: "job_title",
      header: "Jabatan / Kode",
      accessor: "job_title",
      sortable: true,
      cell: (item) => (
        <div className="min-w-[96px] space-y-0.5">
          {item.job_title ? (
            <p className="text-[13px]">{item.job_title}</p>
          ) : (
            <p className="text-[12px] text-text-tertiary">Belum diisi</p>
          )}
          <p className="whitespace-nowrap font-mono text-[11px] text-text-secondary">{item.employee_code || "Kode belum ada"}</p>
        </div>
      ),
    },
    {
      id: "total_gaji",
      header: "Gaji / Reimbursement",
      align: "right",
      numeric: true,
      cell: (item) => (
        <div className="space-y-0.5 whitespace-nowrap text-[13px]">
          <p title="Total gaji">{formatAmount(item.totals.total_gaji)}</p>
          {item.totals.total_potongan > 0 ? (
            <p className="text-[11px] text-text-tertiary" title="Total potongan">
              −{formatAmount(item.totals.total_potongan)}
            </p>
          ) : null}
          {item.totals.total_reimbursement > 0 ? (
            <p className="text-[11px] text-text-secondary" title="Reimbursement">
              +{formatAmount(item.totals.total_reimbursement)} reimb.
            </p>
          ) : null}
        </div>
      ),
    },
    {
      id: "total_diterima",
      header: "Total diterima",
      align: "right",
      numeric: true,
      cell: (item) => (
        <span className="whitespace-nowrap text-[13px] font-semibold">{formatAmount(item.totals.total_diterima)}</span>
      ),
    },
    {
      id: "warnings",
      header: "Peringatan",
      cell: (item) => <WarningChips limit={2} warnings={item.warnings ?? []} />,
    },
  ];

  const summary = data?.summary;
  const missingCompany = data?.company.missing_fields ?? [];

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">HRIS slip gaji</p>
            <h3 className="text-[28px] font-[700] text-text-primary">Slip Gaji {formatPayslipPeriod(year, month)}</h3>
            <p className="mt-2 max-w-3xl text-[14px] leading-relaxed text-text-secondary">
              Buat draf slip dari data gaji, bonus yang disetujui, dan reimbursement yang sudah dibayar. Periksa
              pratinjau PDF, lalu kirim ke email login karyawan.
            </p>
          </div>
          <MonthYearPicker current={today} onChange={setPeriod} value={{ year, month }} yearsBack={PAYSLIP_YEARS_BACK} />
        </div>
      </Card>

      {data ? (
        <div className="space-y-3">
          {!data.pdf_available ? (
            <Banner tone="warning">
              Konverter PDF (LibreOffice) belum tersedia di server. Draf tetap dibuat sebagai DOCX, tetapi
              {data.allow_docx_send ? " email akan melampirkan DOCX." : " slip belum dapat dikirim lewat email."}
            </Banner>
          ) : null}
          {!data.document_mail_ready ? (
            <Banner tone="warning">
              Email Dokumen belum dikonfigurasi, jadi slip belum dapat dikirim.{" "}
              {canOpenSettings ? (
                <Link className="font-semibold underline" to="/admin/settings">
                  Atur di Admin &gt; Pengaturan
                </Link>
              ) : (
                "Minta Admin mengaktifkan Email Dokumen di Pengaturan."
              )}
            </Banner>
          ) : null}
          {missingCompany.length > 0 ? (
            <Banner tone="warning">
              Profil perusahaan belum lengkap ({missingCompany.map((field) => companyFieldLabels[field] ?? field).join(", ")}).
              Data ini dicetak di slip.
            </Banner>
          ) : null}
          {summary && summary.in_progress > 0 ? (
            <Banner tone="info">
              {summary.in_progress} slip sedang diproses (pembuatan PDF atau pengiriman email). Tabel diperbarui otomatis.
            </Banner>
          ) : null}
        </div>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
        <StatCard helper="Karyawan aktif/probation periode ini" icon={Users} label="Total" tone="hr" value={String(summary?.total ?? 0)} />
        <StatCard
          helper={summary && summary.blocked > 0 ? `${summary.blocked} diblokir data gaji` : "Belum ada draf"}
          icon={FileClock}
          label="Belum dibuat"
          tone="warning"
          value={String(summary?.not_generated ?? 0)}
        />
        <StatCard helper="Siap diperiksa dan dikirim" icon={FilePen} label="Draf" tone="info" value={String(summary?.draft ?? 0)} />
        <StatCard helper="Email slip terkirim" icon={MailCheck} label="Terkirim" tone="success" value={String(summary?.sent ?? 0)} />
        <StatCard helper="PDF atau email gagal" icon={TriangleAlert} label="Gagal" tone="error" value={String(summary?.failed ?? 0)} />
      </div>

      <Card className="space-y-4 p-6">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-1 flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative w-full sm:max-w-xs">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary" />
              <Input
                aria-label="Cari karyawan"
                className="h-10 pl-9"
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Cari nama, kode, nomor slip"
                value={query}
              />
            </div>
            <select
              aria-label="Filter status"
              className="field-select sm:max-w-[200px]"
              onChange={(event) => setStatusFilter(event.target.value as StatusFilter)}
              value={statusFilter}
            >
              <option value="">Semua status</option>
              <option value="none">Belum dibuat</option>
              <option value="draft">Draf</option>
              <option value="sent">Terkirim</option>
              <option value="failed">Gagal</option>
              <option value="warning">Ada peringatan</option>
            </select>
            {search.employee ? (
              <span className="inline-flex items-center gap-2 rounded-full bg-hr-light px-3 py-1 text-[12px] font-semibold text-hr">
                {filterEmployee?.employee_name ?? "Karyawan terpilih"}
                <button
                  aria-label="Tampilkan semua karyawan"
                  className="rounded-full p-0.5 hover:bg-hr/10"
                  onClick={() => void navigate({ search: (previous) => ({ ...previous, employee: undefined }) })}
                  type="button"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </span>
            ) : null}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <label className="flex cursor-pointer items-center gap-2 text-sm text-text-secondary">
              <input
                checked={allVisibleSelected}
                className="h-4 w-4 accent-primary"
                disabled={selectableVisible.length === 0}
                onChange={toggleAllVisible}
                type="checkbox"
              />
              Pilih semua ({selectableVisible.length})
            </label>
            {canManage ? (
              <Button
                disabled={generateTargets.length === 0}
                onClick={() =>
                  setGenerateState({ targets: generateTargets, payDate: null, title: "Generate Draft" })
                }
                size="sm"
                type="button"
              >
                <FilePlus2 className="h-4 w-4" />
                Generate Draft ({generateTargets.length})
              </Button>
            ) : null}
            {canSend ? (
              <Button
                disabled={sendTargetIds.length === 0}
                onClick={() => setSendIds(sendTargetIds)}
                size="sm"
                type="button"
                variant="outline"
              >
                <Send className="h-4 w-4" />
                Kirim Terpilih ({sendTargetIds.length})
              </Button>
            ) : null}
          </div>
        </div>

        {listQuery.error instanceof Error ? (
          <p className="rounded-xl bg-error-light px-4 py-3 text-sm text-error">{listQuery.error.message}</p>
        ) : (
          <DataTable
            columns={columns}
            data={visibleItems}
            emptyDescription={
              items.length === 0
                ? "Tidak ada karyawan aktif atau probation yang bergabung sampai akhir periode ini."
                : "Tidak ada baris yang cocok dengan filter."
            }
            emptyTitle="Tidak ada slip"
            getRowClassName={(item) => (selected.has(item.employee_id) ? "bg-hr-light/40" : undefined)}
            getRowId={(item) => item.employee_id}
            loading={listQuery.isLoading}
            onRowClick={(item) => {
              setDrawerEmployeeId(item.employee_id);
              setDrawerOpen(true);
            }}
            selectedRowId={drawerOpen ? drawerEmployeeId : null}
          />
        )}
      </Card>

      <PayslipDrawer
        allowDocxSend={data?.allow_docx_send ?? false}
        documentMailReady={data?.document_mail_ready ?? false}
        item={drawerItem}
        month={month}
        onOpenChange={setDrawerOpen}
        onRegenerate={(item, payDate) =>
          setGenerateState({
            targets: [{ employee_id: item.employee_id, employee_name: item.employee_name, status: item.status }],
            payDate,
            title: item.status === "draft" ? "Generate ulang draf" : "Generate draf",
          })
        }
        open={drawerOpen && drawerItem !== null}
        pdfAvailable={data?.pdf_available ?? false}
        year={year}
      />

      <GeneratePayslipsDialog
        defaultPayDate={generateState?.payDate ?? data?.default_pay_date ?? ""}
        month={month}
        onClose={() => setGenerateState(null)}
        onGenerated={() => setSelected(new Set())}
        open={generateState !== null}
        targets={generateState?.targets ?? []}
        title={generateState?.title}
        year={year}
      />

      <SendPayslipsDialog
        onClose={() => setSendIds(null)}
        onQueued={() => setSelected(new Set())}
        open={sendIds !== null}
        payslipIds={sendIds ?? []}
      />
    </div>
  );
}

/**
 * Status, progress and e-mail of a row. A sent slip shows when it was
 * actually sent (last_sent_at); a failed latest attempt is shown as such,
 * with its own time.
 */
function StatusEmailCell({ item, pdfAvailable }: { item: PayslipListItem; pdfAvailable: boolean }) {
  const delivery = item.last_delivery;
  const failed = delivery?.status === "failed";
  return (
    <div className="flex min-w-[112px] max-w-[170px] flex-col items-start gap-1">
      <PayslipStatusPill blocked={item.blocked} status={item.status} />
      <PayslipProgress item={item} pdfAvailable={pdfAvailable} />
      {delivery ? (
        <div className="w-full">
          <p className="truncate text-[12px] text-text-primary" title={delivery.recipient}>
            {delivery.recipient}
          </p>
          <p className="text-[11px] text-text-tertiary">
            {failed
              ? `Gagal ${formatDateTime(delivery.created_at)}`
              : item.status === "sent" && item.last_sent_at
                ? formatDateTime(item.last_sent_at)
                : formatDateTime(delivery.sent_at ?? delivery.created_at)}
          </p>
          {failed && item.status === "sent" && item.last_sent_at ? (
            <p className="text-[11px] text-text-tertiary">Terkirim {formatDateTime(item.last_sent_at)}</p>
          ) : null}
        </div>
      ) : item.status !== "none" ? (
        <p className="text-[11px] text-text-tertiary">Belum dikirim</p>
      ) : null}
    </div>
  );
}

function Banner({ tone, children }: { tone: "warning" | "info"; children: ReactNode }) {
  const Icon = tone === "warning" ? TriangleAlert : Info;
  return (
    <div
      className={
        tone === "warning"
          ? "flex items-start gap-2 rounded-xl border border-warning/30 bg-warning-light px-4 py-3 text-sm text-warning"
          : "flex items-start gap-2 rounded-xl border border-info/30 bg-info-light px-4 py-3 text-sm text-info"
      }
      role={tone === "warning" ? "alert" : "status"}
    >
      <Icon className="mt-0.5 h-4 w-4 shrink-0" />
      <p>{children}</p>
    </div>
  );
}

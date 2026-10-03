import { useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { CalendarClock, CalendarX, FilePen, FileSignature, Info, Plus, Search, TriangleAlert, X } from "lucide-react";
import { z } from "zod";

import {
  ContractDeadlineBadges,
  ContractStatusPill,
  ContractTypeBadge,
  contractStatusLabels,
  contractTypeLabels,
  formatContractPeriod,
} from "@/components/contracts/contract-display";
import { DeliveryStatusPill, formatDateTime, RenderStatusPill } from "@/components/payslips/payslip-display";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { StatCard } from "@/components/shared/stat-card";
import { buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { contractsKeys, listContracts } from "@/services/hris-contracts";
import type { ContractListItem, ContractStatus, ContractType } from "@/types/contracts";

const statusValues = ["draft", "generated", "sent", "signed", "ended", "cancelled"] as const;
const typeValues = ["PKWT", "PKWTT", "MAGANG"] as const;

const searchSchema = z.object({
  employee: z.string().max(64).optional().catch(undefined),
  status: z.enum(statusValues).optional().catch(undefined),
  type: z.enum(typeValues).optional().catch(undefined),
  attention: z.boolean().optional().catch(undefined),
});

export const Route = createFileRoute("/_authenticated/hris/contracts/")({
  validateSearch: searchSchema,
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisContractView);
  },
  component: ContractsPage,
});

/** A contract HR must act on: the notice deadline is near, or the end date has passed. */
function needsAttention(item: ContractListItem) {
  return item.expired || item.notice_alert;
}

function ContractsPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.hrisContractManage);
  const canOpenSettings = hasPermission(permissions.adminSettingsView);
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query.trim(), 350);

  const filters = {
    employee_id: search.employee,
    status: search.status ?? "",
    type: search.type ?? "",
    search: debouncedQuery,
    limit: 500,
  } as const;
  const listQuery = useQuery({
    queryKey: contractsKeys.list(filters),
    queryFn: () => listContracts(filters),
    refetchOnWindowFocus: false,
    // Follow a render or a send in progress.
    refetchInterval: (current) =>
      (current.state.data?.items ?? []).some(
        (item) =>
          item.render_status === "pending" ||
          item.render_status === "rendering" ||
          item.last_delivery?.status === "queued" ||
          item.last_delivery?.status === "sending",
      )
        ? 3000
        : false,
  });

  const data = listQuery.data;
  const items = useMemo(() => data?.items ?? [], [data]);
  const visibleItems = useMemo(() => (search.attention ? items.filter(needsAttention) : items), [items, search.attention]);
  const counts = useMemo(
    () => ({
      active: items.filter((item) => item.status === "signed" && !item.expired).length,
      inProgress: items.filter((item) => item.status === "draft" || item.status === "generated" || item.status === "sent").length,
      notice: items.filter((item) => item.notice_alert && !item.expired).length,
      expired: items.filter((item) => item.expired).length,
    }),
    [items],
  );
  const employeeName = search.employee ? items.find((item) => item.employee_id === search.employee)?.employee_name : undefined;

  const setSearch = (patch: Partial<z.infer<typeof searchSchema>>) =>
    void navigate({ search: (previous) => ({ ...previous, ...patch }) });

  const columns: Array<DataTableColumn<ContractListItem>> = [
    {
      id: "employee",
      header: "Karyawan",
      accessor: "employee_name",
      sortable: true,
      mobilePrimary: true,
      cell: (item) => (
        <div className="min-w-[150px] space-y-0.5">
          <p className="font-semibold text-text-primary">{item.employee_name}</p>
          <p className="text-[12px] text-text-secondary">
            {item.job_title}
            {item.employee_department ? ` · ${item.employee_department}` : ""}
          </p>
        </div>
      ),
    },
    {
      id: "type",
      header: "Jenis",
      accessor: "contract_type",
      sortable: true,
      cell: (item) => <ContractTypeBadge recordOnly={item.is_record_only} type={item.contract_type} />,
    },
    {
      id: "doc_number",
      header: "No. Dokumen",
      cell: (item) =>
        item.doc_number ? (
          <div className="space-y-0.5 font-mono text-[11px] text-text-secondary">
            <p className="whitespace-nowrap text-text-primary">{item.doc_number}</p>
            <p className="whitespace-nowrap">{item.nda_doc_number}</p>
            {item.revision > 0 ? <p className="font-sans text-[11px] text-hr">Revisi {item.revision}</p> : null}
          </div>
        ) : (
          <span className="text-[12px] text-text-tertiary">{item.is_record_only ? "Tanpa dokumen" : "Belum terbit"}</span>
        ),
    },
    {
      id: "period",
      header: "Periode",
      accessor: "end_date",
      sortable: true,
      cell: (item) => (
        <div className="min-w-[150px] space-y-1">
          <p className="whitespace-nowrap text-[13px] text-text-primary">{formatContractPeriod(item.start_date, item.end_date)}</p>
          <ContractDeadlineBadges item={item} />
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      accessor: "status",
      sortable: true,
      cell: (item) => (
        <div className="flex flex-col items-start gap-1">
          <ContractStatusPill status={item.status} />
          {item.render_status === "pending" || item.render_status === "rendering" || item.render_status === "failed" ? (
            <RenderStatusPill status={item.render_status} />
          ) : null}
          {item.last_delivery && item.last_delivery.status !== "sent" ? <DeliveryStatusPill delivery={item.last_delivery} /> : null}
        </div>
      ),
    },
    {
      id: "last_sent",
      header: "Terakhir dikirim",
      accessor: "last_sent_at",
      sortable: true,
      cell: (item) =>
        item.last_sent_at ? (
          <div className="max-w-[180px] space-y-0.5">
            <p className="whitespace-nowrap text-[12px] text-text-primary">{formatDateTime(item.last_sent_at)}</p>
            {item.last_sent_delivery?.recipient ? (
              <p className="truncate text-[11px] text-text-tertiary" title={item.last_sent_delivery.recipient}>
                {item.last_sent_delivery.recipient}
              </p>
            ) : null}
          </div>
        ) : (
          <span className="text-[12px] text-text-tertiary">-</span>
        ),
    },
  ];

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">HRIS kontrak kerja</p>
            <h3 className="text-[28px] font-[700] text-text-primary">Kontrak Kerja</h3>
            <p className="mt-2 max-w-3xl text-[14px] leading-relaxed text-text-secondary">
              Buat PKWT beserta NDA &amp; HKI dari data karyawan, kirim ke email login karyawan, lalu pantau tanda tangan,
              batas pemberitahuan, dan perpanjangan. PKWTT dan magang dicatat tanpa dokumen.
            </p>
          </div>
          {canManage ? (
            <Link
              className={buttonVariants({ variant: "hr" })}
              data-testid="contract-create-link"
              search={search.employee ? { employee: search.employee } : {}}
              to="/hris/contracts/new"
            >
              <Plus className="h-4 w-4" />
              Buat Kontrak
            </Link>
          ) : null}
        </div>
      </Card>

      {data ? (
        <div className="space-y-3">
          {!data.pdf_available ? (
            <Banner tone="warning">
              Konverter PDF (LibreOffice) belum tersedia di server. Kontrak tetap bisa di-generate dan diunduh sebagai DOCX.
            </Banner>
          ) : null}
          {!data.document_mail_ready ? (
            <Banner tone="warning">
              Email Dokumen belum dikonfigurasi, jadi kontrak belum dapat dikirim lewat email.{" "}
              {canOpenSettings ? (
                <Link className="font-semibold underline" to="/admin/settings">
                  Atur di Admin &gt; Pengaturan
                </Link>
              ) : (
                "Minta Admin mengaktifkan Email Dokumen di Pengaturan."
              )}
            </Banner>
          ) : null}
        </div>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard helper="Sudah ditandatangani dan berjalan" icon={FileSignature} label="Aktif" tone="success" value={String(counts.active)} />
        <StatCard helper="Draf, dokumen dibuat, atau terkirim" icon={FilePen} label="Dalam proses" tone="info" value={String(counts.inProgress)} />
        <StatCard helper="Masuk masa pemberitahuan" icon={CalendarClock} label="Batas pemberitahuan" tone="warning" value={String(counts.notice)} />
        <StatCard helper="Tanggal berakhir sudah lewat" icon={CalendarX} label="Kedaluwarsa" tone="error" value={String(counts.expired)} />
      </div>

      <Card className="space-y-4 p-6">
        <div className="flex flex-col gap-3 lg:flex-row lg:flex-wrap lg:items-center">
          <div className="relative w-full lg:max-w-xs">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-tertiary" />
            <Input
              aria-label="Cari kontrak"
              className="h-10 pl-9"
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Cari nama, jabatan, nomor dokumen"
              value={query}
            />
          </div>
          <select
            aria-label="Filter status"
            className="field-select lg:max-w-[200px]"
            onChange={(event) => setSearch({ status: (event.target.value || undefined) as ContractStatus | undefined })}
            value={search.status ?? ""}
          >
            <option value="">Semua status</option>
            {statusValues.map((status) => (
              <option key={status} value={status}>
                {contractStatusLabels[status]}
              </option>
            ))}
          </select>
          <select
            aria-label="Filter jenis"
            className="field-select lg:max-w-[180px]"
            onChange={(event) => setSearch({ type: (event.target.value || undefined) as ContractType | undefined })}
            value={search.type ?? ""}
          >
            <option value="">Semua jenis</option>
            {typeValues.map((type) => (
              <option key={type} value={type}>
                {contractTypeLabels[type]}
              </option>
            ))}
          </select>
          <label className="flex cursor-pointer items-center gap-2 text-sm text-text-secondary">
            <input
              checked={search.attention ?? false}
              className="h-4 w-4 accent-primary"
              onChange={(event) => setSearch({ attention: event.target.checked || undefined })}
              type="checkbox"
            />
            Perlu tindakan (pemberitahuan / kedaluwarsa)
          </label>
          {search.employee ? (
            <span className="inline-flex items-center gap-2 rounded-full bg-hr-light px-3 py-1 text-[12px] font-semibold text-hr">
              {employeeName ?? "Karyawan terpilih"}
              <button
                aria-label="Tampilkan semua karyawan"
                className="rounded-full p-0.5 hover:bg-hr/10"
                onClick={() => setSearch({ employee: undefined })}
                type="button"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </span>
          ) : null}
        </div>

        {listQuery.error instanceof Error ? (
          <p className="rounded-xl bg-error-light px-4 py-3 text-sm text-error">{listQuery.error.message}</p>
        ) : (
          <DataTable
            columns={columns}
            data={visibleItems}
            emptyActionLabel={canManage && items.length === 0 ? "Buat Kontrak" : undefined}
            emptyDescription={
              items.length === 0
                ? "Belum ada kontrak yang cocok. Buat PKWT baru atau catat PKWTT/magang yang sudah berjalan."
                : "Tidak ada kontrak yang perlu tindakan."
            }
            emptyTitle="Tidak ada kontrak"
            getRowClassName={(item) => (item.expired ? "bg-error-light/30" : undefined)}
            getRowId={(item) => item.id}
            loading={listQuery.isLoading}
            onEmptyAction={() =>
              void navigate({ to: "/hris/contracts/new", search: search.employee ? { employee: search.employee } : {} })
            }
            onRowClick={(item) => void navigate({ to: "/hris/contracts/$contractId", params: { contractId: item.id } })}
          />
        )}
      </Card>
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

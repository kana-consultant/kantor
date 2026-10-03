import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  ArrowRight,
  Ban,
  CalendarCheck2,
  CircleStop,
  Download,
  Eye,
  EyeOff,
  FileCog,
  Info,
  LoaderCircle,
  Pencil,
  Repeat,
  Send,
  TriangleAlert,
} from "lucide-react";

import {
  contractDurationMonths,
  ContractDeadlineBadges,
  ContractStatusPill,
  ContractTypeBadge,
  contractTypeDescriptions,
  contractWorkModeLabels,
  formatContractPeriod,
  formatShortDate,
  shortDigest,
} from "@/components/contracts/contract-display";
import { ContractForm } from "@/components/contracts/contract-form";
import { ContractPdfPreview } from "@/components/contracts/contract-pdf-preview";
import { GenerateContractDialog, PreflightSummary, usePreflight } from "@/components/contracts/contract-preflight";
import { EndContractDialog, SignContractDialog } from "@/components/contracts/contract-status-dialogs";
import { SendContractDialog } from "@/components/contracts/send-contract-dialog";
import { DeliveryHistory } from "@/components/payslips/delivery-history";
import {
  formatAmount,
  formatDateTime,
  isRenderInProgress,
  RenderStatusPill,
} from "@/components/payslips/payslip-display";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useRBAC } from "@/hooks/use-rbac";
import { triggerDownload } from "@/lib/download";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  contractsKeys,
  downloadContractDocx,
  downloadContractPDF,
  getContract,
  listContracts,
  renewContract,
  updateContractStatus,
} from "@/services/hris-contracts";
import { toast } from "@/stores/toast-store";
import type { ContractDetail, ContractPart } from "@/types/contracts";

export const Route = createFileRoute("/_authenticated/hris/contracts/$contractId")({
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisContractView);
  },
  component: ContractDetailPage,
});

const partLabels: Record<ContractPart, string> = { pkwt: "PKWT", nda: "NDA & HKI" };

const employeeStatusLabels: Record<string, string> = {
  active: "Active",
  probation: "Probation",
  resigned: "Resigned",
  terminated: "Terminated",
};

type DialogName = "generate" | "send" | "sign" | "end" | "cancel" | "renew" | "revise" | null;

function ContractDetailPage() {
  const { contractId } = Route.useParams();
  const navigate = Route.useNavigate();
  const queryClient = useQueryClient();
  const { hasPermission, hasAllPermissions } = useRBAC();
  const canManage = hasPermission(permissions.hrisContractManage);
  const canSend = hasPermission(permissions.hrisContractSend);
  const canReadPDF = hasAllPermissions(permissions.hrisContractView, permissions.hrisEmployeeIdentityView);
  const canReadDocx = hasAllPermissions(permissions.hrisContractManage, permissions.hrisEmployeeIdentityView);
  // The PKWT prints the compensation: it also needs hris:salary:view (the
  // NDA prints no amounts).
  const canViewSalary = hasPermission(permissions.hrisSalaryView);
  const readableParts = (["pkwt", "nda"] as const).filter((part) => part === "nda" || canViewSalary);
  const canOpenSettings = hasPermission(permissions.adminSettingsView);

  const [editing, setEditing] = useState(false);
  const [dialog, setDialog] = useState<DialogName>(null);
  const [previewPart, setPreviewPart] = useState<ContractPart | null>(null);
  const [downloading, setDownloading] = useState<string | null>(null);

  useEffect(() => {
    setEditing(false);
    setDialog(null);
    setPreviewPart(null);
  }, [contractId]);

  // Detail reads with salary access are access-logged: no focus refetch and
  // no polling here (the light list poll below follows renders and sends).
  const detailQuery = useQuery({
    queryKey: contractsKeys.detail(contractId),
    queryFn: () => getContract(contractId),
    refetchOnWindowFocus: false,
  });
  const detail = detailQuery.data;

  const renderBusy = detail ? isRenderInProgress(detail.render_status) : false;
  const sendBusy = detail?.last_delivery?.status === "queued" || detail?.last_delivery?.status === "sending";
  const pollQuery = useQuery({
    queryKey: [...contractsKeys.all, "poll", contractId],
    queryFn: () => listContracts({ employee_id: detail!.employee_id, limit: 500 }),
    enabled: Boolean(detail) && (renderBusy || sendBusy),
    refetchInterval: renderBusy || sendBusy ? 2500 : false,
    refetchOnWindowFocus: false,
  });
  useEffect(() => {
    const item = pollQuery.data?.items.find((candidate) => candidate.id === contractId);
    if (!item || !detail) {
      return;
    }
    if (
      item.render_status !== detail.render_status ||
      item.status !== detail.status ||
      item.last_delivery?.status !== detail.last_delivery?.status
    ) {
      void queryClient.invalidateQueries({ queryKey: contractsKeys.detail(contractId) });
      void queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "list"] });
    }
  }, [pollQuery.data, detail, contractId, queryClient]);

  const documents = detail ? detail.contract_type === "PKWT" && !detail.is_record_only : false;
  // The readiness card on drafts (each preflight logs an identity read).
  const showPreflight = Boolean(detail) && canManage && documents && detail?.status === "draft" && !editing;
  const preflightQuery = usePreflight(contractId, showPreflight);

  const statusMutation = useMutation({
    mutationFn: (note: string | undefined) =>
      updateContractStatus(contractId, { status: "cancelled", ...(note ? { end_notes: note } : {}) }),
    onSuccess: async (saved) => {
      queryClient.setQueryData(contractsKeys.detail(saved.id), saved);
      await queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "list"] });
      toast.success("Kontrak dibatalkan");
      setDialog(null);
    },
    onError: (error) => toast.error("Gagal membatalkan kontrak", error instanceof Error ? error.message : undefined),
  });

  const renewMutation = useMutation({
    mutationFn: () => renewContract(contractId),
    onSuccess: async (created) => {
      queryClient.setQueryData(contractsKeys.detail(created.id), created);
      await queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "list"] });
      await queryClient.invalidateQueries({ queryKey: contractsKeys.detail(contractId) });
      toast.success("Draf perpanjangan dibuat", `${formatContractPeriod(created.start_date, created.end_date)}`);
      setDialog(null);
      void navigate({ to: "/hris/contracts/$contractId", params: { contractId: created.id } });
    },
    onError: (error) => toast.error("Gagal membuat perpanjangan", error instanceof Error ? error.message : undefined),
  });

  const download = async (part: ContractPart, format: "pdf" | "docx") => {
    if (!detail) {
      return;
    }
    const key = `${part}-${format}`;
    setDownloading(key);
    try {
      const result =
        format === "pdf" ? await downloadContractPDF(detail.id, part, "attachment") : await downloadContractDocx(detail.id, part);
      triggerDownload(result.blob, result.filename ?? `${part}.${format}`);
    } catch (error) {
      toast.error("Unduhan gagal", error instanceof Error ? error.message : undefined);
    } finally {
      setDownloading(null);
    }
  };

  if (detailQuery.isLoading) {
    return (
      <Card className="space-y-4 p-8">
        <Skeleton className="h-8 w-64 rounded-lg" />
        <Skeleton className="h-5 w-96 rounded-lg" />
        <Skeleton className="h-40 rounded-lg" />
      </Card>
    );
  }
  if (detailQuery.error instanceof Error || !detail) {
    return (
      <Card className="space-y-3 p-8">
        <p className="text-error">{detailQuery.error instanceof Error ? detailQuery.error.message : "Kontrak tidak ditemukan"}</p>
        <Link className="text-sm font-semibold underline" to="/hris/contracts">
          Kembali ke Kontrak Kerja
        </Link>
      </Card>
    );
  }

  const status = detail.status;
  const numbered = Boolean(detail.doc_number);
  const signed = Boolean(detail.signed_at);
  const editable = status === "draft" || status === "generated" || (status === "sent" && !signed);
  const revision = status === "sent" && !signed;
  const generatable = documents && (status === "draft" || status === "generated");
  const snapshotCurrent = documents && numbered && status !== "draft" && status !== "cancelled";
  const pdfReady = detail.has_pdf && detail.render_status === "ready";
  const docxSendFallback = detail.allow_docx_send && !detail.pdf_available;
  const sendable = documents && (status === "generated" || status === "sent" || status === "signed" || status === "ended");
  // A generated contract is signed only once its documents exist (the PDF
  // worker never renders a signed contract).
  const documentsIssued = pdfReady || (!detail.pdf_available && detail.render_status === "none");
  const signable = detail.is_record_only ? status === "draft" : (status === "generated" && documentsIssued) || status === "sent";
  const endable = detail.is_record_only ? status === "draft" || status === "signed" : status === "signed";
  const cancellable = detail.is_record_only ? status === "draft" : status === "draft" || status === "generated" || status === "sent";
  const renewable = (status === "signed" || status === "ended") && Boolean(detail.end_date) && !detail.renewed_by_id;

  const anyAction =
    (canManage && (editable || generatable || signable || renewable || endable || cancellable)) ||
    (documents && canReadPDF && pdfReady && readableParts.length > 0) ||
    (canReadDocx && snapshotCurrent && readableParts.length > 0) ||
    (canSend && sendable);

  let sendDisabledReason: string | null = null;
  if (sendBusy) {
    sendDisabledReason = "Pengiriman sebelumnya masih berjalan";
  } else if (renderBusy) {
    sendDisabledReason = "Tunggu PDF selesai dibuat";
  } else if (!pdfReady && !docxSendFallback) {
    sendDisabledReason = detail.pdf_available ? "PDF belum siap; generate ulang" : "Konverter PDF belum tersedia";
  } else if (!detail.document_mail_ready) {
    sendDisabledReason = "Email Dokumen belum dikonfigurasi";
  }

  const duration = detail.end_date ? contractDurationMonths(detail.start_date, detail.end_date) : null;

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <Link
          className="mb-3 inline-flex items-center gap-1.5 text-[13px] font-semibold text-text-secondary hover:text-text-primary"
          to="/hris/contracts"
        >
          <ArrowLeft className="h-4 w-4" />
          Kontrak Kerja
        </Link>
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">
              {contractTypeDescriptions[detail.contract_type]}
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">
              <Link
                className="hover:underline"
                params={{ employeeId: detail.employee_id }}
                to="/hris/employees/$employeeId"
              >
                {detail.employee_name}
              </Link>
            </h3>
            <p className="mt-1 text-[14px] text-text-secondary">
              {[detail.job_title, detail.department ?? detail.employee_department].filter(Boolean).join(" · ")}
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2" data-testid="contract-header-badges">
              <ContractTypeBadge recordOnly={detail.is_record_only} type={detail.contract_type} />
              <ContractStatusPill status={status} />
              {detail.revision > 0 ? (
                <span className="rounded-full bg-hr-light px-2 py-0.5 text-[11px] font-semibold text-hr">Revisi {detail.revision}</span>
              ) : null}
              {documents && (renderBusy || detail.render_status === "failed") ? (
                <RenderStatusPill error={detail.render_error} status={detail.render_status} />
              ) : null}
              <ContractDeadlineBadges item={detail} />
            </div>
          </div>
          <dl className="grid shrink-0 gap-x-6 gap-y-2 text-[13px] sm:grid-cols-2 lg:text-right">
            <HeaderFact label="Periode" value={formatContractPeriod(detail.start_date, detail.end_date)} />
            <HeaderFact
              label="Durasi"
              value={duration ? (duration.days === 0 ? `${duration.months} bulan` : `${duration.months} bulan ${duration.days} hari`) : "-"}
            />
            {numbered ? (
              <>
                <HeaderFact label="No. PKWT" mono value={detail.doc_number ?? "-"} />
                <HeaderFact label="No. NDA & HKI" mono value={detail.nda_doc_number ?? "-"} />
              </>
            ) : null}
          </dl>
        </div>
        {detail.previous_contract_id || detail.renewed_by_id ? (
          <div className="mt-4 flex flex-wrap gap-3 text-[13px]">
            {detail.previous_contract_id ? (
              <Link
                className="inline-flex items-center gap-1.5 font-semibold text-hr hover:underline"
                params={{ contractId: detail.previous_contract_id }}
                to="/hris/contracts/$contractId"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                Perpanjangan dari {detail.previous_doc_number ?? "kontrak sebelumnya"}
                {detail.previous_end_date ? ` (berakhir ${formatShortDate(detail.previous_end_date)})` : ""}
              </Link>
            ) : null}
            {detail.renewed_by_id ? (
              <Link
                className="inline-flex items-center gap-1.5 font-semibold text-hr hover:underline"
                params={{ contractId: detail.renewed_by_id }}
                to="/hris/contracts/$contractId"
              >
                Sudah diperpanjang: buka kontrak lanjutan
                <ArrowRight className="h-3.5 w-3.5" />
              </Link>
            ) : null}
          </div>
        ) : null}
      </Card>

      {!editing && anyAction ? (
        <Card className="flex flex-wrap items-center gap-2 p-4" data-testid="contract-action-bar">
          {canManage && editable ? (
            <Button
              data-testid="contract-action-edit"
              disabled={sendBusy || renderBusy}
              onClick={() => (revision ? setDialog("revise") : setEditing(true))}
              size="sm"
              type="button"
              variant="outline"
            >
              <Pencil className="h-4 w-4" />
              {revision ? "Revisi" : "Edit"}
            </Button>
          ) : null}
          {canManage && generatable ? (
            <Button
              data-testid="contract-action-generate"
              disabled={renderBusy || sendBusy}
              onClick={() => setDialog("generate")}
              size="sm"
              type="button"
            >
              {renderBusy ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <FileCog className="h-4 w-4" />}
              {renderBusy ? "Membuat PDF..." : status === "generated" || numbered ? "Generate ulang" : "Generate"}
            </Button>
          ) : null}
          {documents && canReadPDF && pdfReady
            ? readableParts.map((part) => (
                <Button
                  data-testid={`contract-action-preview-${part}`}
                  key={part}
                  onClick={() => setPreviewPart((current) => (current === part ? null : part))}
                  size="sm"
                  type="button"
                  variant={previewPart === part ? "secondary" : "outline"}
                >
                  {previewPart === part ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  Preview {partLabels[part]}
                </Button>
              ))
            : null}
          {canReadDocx && snapshotCurrent
            ? readableParts.map((part) => (
                <Button
                  disabled={downloading !== null || renderBusy}
                  key={part}
                  onClick={() => void download(part, "docx")}
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  <Download className="h-4 w-4" />
                  {downloading === `${part}-docx` ? "Mengunduh..." : `DOCX ${partLabels[part]}`}
                </Button>
              ))
            : null}
          {canSend && sendable ? (
            <Button
              data-testid="contract-action-send"
              disabled={Boolean(sendDisabledReason)}
              onClick={() => setDialog("send")}
              size="sm"
              title={sendDisabledReason ?? undefined}
              type="button"
              variant={status === "generated" ? "default" : "outline"}
            >
              {sendBusy ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
              {status === "generated" ? "Kirim via Email" : "Kirim ulang"}
            </Button>
          ) : null}
          {canManage && signable ? (
            <Button
              data-testid="contract-action-sign"
              disabled={sendBusy || renderBusy}
              onClick={() => setDialog("sign")}
              size="sm"
              type="button"
              variant="outline"
            >
              <CalendarCheck2 className="h-4 w-4" />
              Tandai Ditandatangani
            </Button>
          ) : null}
          {canManage && renewable ? (
            <Button data-testid="contract-action-renew" onClick={() => setDialog("renew")} size="sm" type="button" variant="outline">
              <Repeat className="h-4 w-4" />
              Perpanjang
            </Button>
          ) : null}
          {canManage && endable ? (
            <Button data-testid="contract-action-end" onClick={() => setDialog("end")} size="sm" type="button" variant="outline">
              <CircleStop className="h-4 w-4" />
              Akhiri
            </Button>
          ) : null}
          {canManage && cancellable ? (
            <Button
              className="text-error hover:text-error"
              data-testid="contract-action-cancel"
              disabled={sendBusy}
              onClick={() => setDialog("cancel")}
              size="sm"
              type="button"
              variant="ghost"
            >
              <Ban className="h-4 w-4" />
              Batalkan
            </Button>
          ) : null}
          {sendDisabledReason && canSend && sendable && !sendBusy && !renderBusy ? (
            <span className="text-[12px] text-text-tertiary">{sendDisabledReason}</span>
          ) : null}
        </Card>
      ) : null}

      <div className="space-y-3">
        {documents && renderBusy ? (
          <Banner tone="info">
            <LoaderCircle className="mr-1 inline h-3.5 w-3.5 animate-spin" />
            PDF PKWT dan NDA sedang dibuat (sekitar 10-20 detik). Halaman diperbarui otomatis.
          </Banner>
        ) : null}
        {documents && detail.render_status === "failed" ? (
          <Banner tone="warning">
            Pembuatan PDF gagal{detail.render_error ? `: ${detail.render_error}` : ""}. Coba Generate ulang.
          </Banner>
        ) : null}
        {documents && !detail.pdf_available ? (
          <Banner tone="warning">
            Konverter PDF belum tersedia di server. Dokumen dapat diperiksa sebagai DOCX
            {detail.allow_docx_send ? " dan dikirim sebagai DOCX." : "; pengiriman email menunggu PDF."}
          </Banner>
        ) : null}
        {documents && sendable && !detail.document_mail_ready ? (
          <Banner tone="warning">
            Email Dokumen belum dikonfigurasi.{" "}
            {canOpenSettings ? (
              <Link className="font-semibold underline" to="/admin/settings">
                Atur di Admin &gt; Pengaturan
              </Link>
            ) : (
              "Minta Admin mengaktifkan Email Dokumen."
            )}
          </Banner>
        ) : null}
        {documents && status === "draft" && numbered && !editing ? (
          <Banner tone="info">
            Draf revisi {detail.revision}: nomor {detail.doc_number} tetap. Generate ulang lalu kirim; subjek email diberi
            &quot;(Revisi {detail.revision})&quot;.
          </Banner>
        ) : null}
        {detail.is_record_only && !editing ? (
          <Banner tone="info">
            Catatan tanpa dokumen: dipakai untuk status kerja dan jabatan di slip gaji serta batas 5 tahun PKWT.
          </Banner>
        ) : null}
        {detail.expired ? (
          <Banner tone="warning">
            Kontrak melewati tanggal berakhir ({formatShortDate(detail.end_date)}). Perpanjang atau akhiri kontrak ini.
          </Banner>
        ) : null}
      </div>

      {editing ? (
        <ContractForm
          detail={detail}
          mode="edit"
          onCancel={() => setEditing(false)}
          onSaved={() => {
            setEditing(false);
            setPreviewPart(null);
          }}
        />
      ) : (
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)]">
          <div className="space-y-6">
            {previewPart && pdfReady ? (
              <Card className="space-y-3 p-6">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h4 className="text-[12px] font-[700] uppercase tracking-[0.08em] text-text-secondary">
                    Pratinjau {partLabels[previewPart]}
                  </h4>
                  <div className="flex gap-2">
                    {readableParts.map((part) => (
                      <Button
                        key={part}
                        onClick={() => setPreviewPart(part)}
                        size="xs"
                        type="button"
                        variant={previewPart === part ? "secondary" : "ghost"}
                      >
                        {partLabels[part]}
                      </Button>
                    ))}
                    <Button
                      disabled={downloading !== null}
                      onClick={() => void download(previewPart, "pdf")}
                      size="xs"
                      type="button"
                      variant="outline"
                    >
                      <Download className="h-4 w-4" />
                      {downloading === `${previewPart}-pdf` ? "Mengunduh..." : "PDF"}
                    </Button>
                  </div>
                </div>
                <ContractPdfPreview
                  contractId={detail.id}
                  part={previewPart}
                  version={detail.pdf_sha256[previewPart === "pkwt" ? 0 : 1] ?? detail.updated_at}
                />
              </Card>
            ) : null}

            <TermsCard detail={detail} />
          </div>

          <div className="space-y-6">
            {showPreflight ? (
              <Card className="space-y-3 p-6">
                <SectionTitle>Kesiapan dokumen</SectionTitle>
                {preflightQuery.isLoading ? (
                  <p className="flex items-center gap-2 text-sm text-text-secondary">
                    <LoaderCircle className="h-4 w-4 animate-spin" />
                    Memeriksa kelengkapan data...
                  </p>
                ) : preflightQuery.error instanceof Error ? (
                  <p className="text-sm text-error">{preflightQuery.error.message}</p>
                ) : preflightQuery.data ? (
                  <PreflightSummary employeeId={detail.employee_id} preflight={preflightQuery.data} />
                ) : null}
              </Card>
            ) : null}

            {documents && detail.compensation_visible ? (
              <Card className="space-y-3 p-6" data-testid="contract-compensation">
                <SectionTitle>Kompensasi</SectionTitle>
                {detail.compensation ? (
                  <dl className="divide-y divide-border/60 rounded-xl border border-border/70 text-[13px]">
                    <AmountRow label="Gaji pokok" value={detail.compensation.base_salary} />
                    <AmountRow label="Tunjangan tetap" value={detail.compensation.fixed_allowance} />
                    <AmountRow
                      label="Total per bulan"
                      strong
                      value={detail.compensation.base_salary + detail.compensation.fixed_allowance}
                    />
                  </dl>
                ) : (
                  <p className="text-sm text-warning">Belum ada kompensasi. Isi lewat Edit sebelum generate.</p>
                )}
                <p className="text-[12px] text-text-tertiary">Dicetak di PKWT; tidak mengubah data gaji karyawan.</p>
              </Card>
            ) : null}

            <Card className="space-y-3 p-6">
              <SectionTitle>Riwayat status</SectionTitle>
              <dl className="space-y-1.5 text-[13px]">
                <TimelineRow label="Dibuat" value={formatDateTime(detail.created_at)} />
                {detail.generated_at ? <TimelineRow label="Dokumen dibuat" value={formatDateTime(detail.generated_at)} /> : null}
                {detail.document_date ? (
                  <TimelineRow
                    label="Tanggal dokumen"
                    value={`${formatShortDate(detail.document_date)}${detail.document_city ? `, ${detail.document_city}` : ""}`}
                  />
                ) : null}
                {detail.last_sent_at ? <TimelineRow label="Terakhir dikirim" value={formatDateTime(detail.last_sent_at)} /> : null}
                {detail.signed_at ? <TimelineRow label="Ditandatangani" value={formatShortDate(detail.signed_at)} /> : null}
                {detail.ended_at ? (
                  <TimelineRow label={status === "cancelled" ? "Dibatalkan" : "Berakhir"} value={formatShortDate(detail.ended_at)} />
                ) : null}
                {detail.end_notes ? <TimelineRow label="Catatan" value={detail.end_notes} /> : null}
                {detail.notice_deadline ? (
                  <TimelineRow
                    label="Batas pemberitahuan"
                    value={`${formatShortDate(detail.notice_deadline)} (${detail.notice_days} hari sebelum berakhir)`}
                  />
                ) : null}
              </dl>
              {pdfReady && detail.pdf_sha256.length === 2 ? (
                <div className="space-y-0.5 border-t border-border/60 pt-2 text-[11px] text-text-tertiary">
                  {(["pkwt", "nda"] as const).map((part, index) => (
                    <p className="font-mono" key={part} title={detail.pdf_sha256[index]}>
                      {partLabels[part]} sha256 {shortDigest(detail.pdf_sha256[index] ?? "")}
                    </p>
                  ))}
                </div>
              ) : null}
            </Card>

            {documents ? (
              <Card className="space-y-3 p-6">
                <SectionTitle>Riwayat pengiriman</SectionTitle>
                <DeliveryHistory
                  referenceId={detail.id}
                  referenceType="contract"
                  version={`${detail.last_delivery?.id ?? ""}:${detail.last_delivery?.status ?? ""}`}
                />
              </Card>
            ) : null}
          </div>
        </div>
      )}

      <GenerateContractDialog contract={detail} onClose={() => setDialog(null)} open={dialog === "generate"} />
      <SendContractDialog contract={detail} onClose={() => setDialog(null)} open={dialog === "send"} />
      <SignContractDialog contract={detail} onClose={() => setDialog(null)} open={dialog === "sign"} />
      <EndContractDialog contract={detail} onClose={() => setDialog(null)} open={dialog === "end"} />
      <ConfirmDialog
        confirmLabel="Batalkan kontrak"
        description={
          numbered
            ? `Nomor ${detail.doc_number} tidak dipakai ulang. Kontrak yang dibatalkan tidak dapat diaktifkan kembali.`
            : "Kontrak yang dibatalkan tidak dapat diaktifkan kembali."
        }
        isLoading={statusMutation.isPending}
        isOpen={dialog === "cancel"}
        noteLabel="Alasan (opsional)"
        notePlaceholder="mis. Kandidat tidak jadi bergabung"
        onClose={() => setDialog(null)}
        onConfirm={(note) => statusMutation.mutate(note || undefined)}
        title="Batalkan kontrak?"
      />
      <ConfirmDialog
        confirmLabel="Buat draf perpanjangan"
        description={
          detail.end_date
            ? `Draf baru mulai ${formatShortDate(nextDay(detail.end_date))} dengan durasi dan isian yang sama. Batas total PKWT berturut-turut 5 tahun diperiksa saat Generate.`
            : "Draf baru dibuat dengan isian yang sama."
        }
        isLoading={renewMutation.isPending}
        isOpen={dialog === "renew"}
        onClose={() => setDialog(null)}
        onConfirm={() => renewMutation.mutate()}
        title="Perpanjang kontrak?"
        tone="warning"
      />
      <ConfirmDialog
        confirmLabel="Mulai revisi"
        description={`Kontrak sudah dikirim. Menyimpan perubahan membuat Revisi ${detail.revision + 1}: status kembali Draf, nomor ${detail.doc_number ?? ""} tetap, lalu generate ulang dan kirim lagi.`}
        isOpen={dialog === "revise"}
        onClose={() => setDialog(null)}
        onConfirm={() => {
          setDialog(null);
          setPreviewPart(null);
          setEditing(true);
        }}
        title="Revisi kontrak?"
        tone="warning"
      />
    </div>
  );
}

function nextDay(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(year ?? 0, (month ?? 1) - 1, (day ?? 1) + 1);
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** Every term of the contract (read-only view). */
function TermsCard({ detail }: { detail: ContractDetail }) {
  const documents = detail.contract_type === "PKWT" && !detail.is_record_only;
  const benefits = detail.benefits ?? [];
  const priorWorks = detail.prior_works ?? [];
  return (
    <Card className="space-y-6 p-6" data-testid="contract-terms">
      <div className="space-y-3">
        <SectionTitle>Posisi</SectionTitle>
        <dl className="grid gap-2 text-[13px] sm:grid-cols-2">
          <Fact label="Jabatan" value={detail.job_title} />
          <Fact label="Departemen" value={detail.department ?? detail.employee_department ?? "-"} />
          <Fact label="Atasan langsung" value={detail.supervisor_name ?? "-"} />
          {documents ? <Fact label="Lokasi kerja" value={detail.work_location || "-"} /> : null}
          {documents ? (
            <Fact
              label="Mode kerja"
              value={`${contractWorkModeLabels[detail.work_mode]}${detail.work_mode_detail ? ` · ${detail.work_mode_detail}` : ""}`}
            />
          ) : null}
          <Fact label="Status karyawan" value={employeeStatusLabels[detail.employee_status] ?? detail.employee_status} />
          {documents ? <Fact label="Uraian tugas" value={detail.job_description || "-"} wide /> : null}
          {documents ? <Fact label="Dasar PKWT" value={detail.pkwt_basis ?? "-"} wide /> : null}
        </dl>
      </div>

      {documents ? (
        <>
          <div className="space-y-3">
            <SectionTitle>Jam kerja</SectionTitle>
            <dl className="grid gap-2 text-[13px] sm:grid-cols-2">
              <Fact label="Hari kerja" value={detail.work_days} />
              <Fact label="Jam kerja" value={detail.work_hours} />
              <Fact label="Jam per minggu" value={`${detail.weekly_hours} jam`} />
              <Fact label="Masa pemberitahuan" value={`${detail.notice_days} hari`} />
            </dl>
          </div>

          <div className="space-y-3">
            <SectionTitle>Benefit</SectionTitle>
            {benefits.length === 0 ? (
              <p className="text-[13px] text-text-secondary">Tidak ada (dokumen mencetak &quot;Tidak ada&quot;).</p>
            ) : (
              <ul className="divide-y divide-border/60 rounded-xl border border-border/70 text-[13px]">
                {benefits.map((item, index) => (
                  <li className="px-3 py-2" key={`${item.name}-${index}`}>
                    <p className="font-semibold text-text-primary">{item.name}</p>
                    <p className="text-[12px] text-text-secondary">{[item.value, item.notes].filter(Boolean).join(" · ") || "-"}</p>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="space-y-3">
            <SectionTitle>NDA &amp; HKI</SectionTitle>
            <dl className="grid gap-2 text-[13px] sm:grid-cols-3">
              <Fact label="Lapor insiden" value={`${detail.incident_report_hours} jam`} />
              <Fact label="Non-solicitation" value={`${detail.non_solicit_months} bulan`} />
              <Fact label="Kerahasiaan" value={`${detail.confidentiality_years} tahun`} />
            </dl>
            {priorWorks.length > 0 ? (
              <ul className="divide-y divide-border/60 rounded-xl border border-border/70 text-[13px]">
                {priorWorks.map((item, index) => (
                  <li className="px-3 py-2" key={`${item.title}-${index}`}>
                    <p className="font-semibold text-text-primary">
                      {item.title}
                      {item.year ? <span className="font-normal text-text-secondary"> ({item.year})</span> : null}
                    </p>
                    {item.description ? <p className="text-[12px] text-text-secondary">{item.description}</p> : null}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-[13px] text-text-secondary">Tidak ada karya terdahulu.</p>
            )}
          </div>
        </>
      ) : null}
    </Card>
  );
}

function SectionTitle({ children }: { children: ReactNode }) {
  return <h4 className="text-[12px] font-[700] uppercase tracking-[0.08em] text-text-secondary">{children}</h4>;
}

function Fact({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return (
    <div className={`rounded-xl bg-surface-muted/60 px-3 py-2${wide ? " sm:col-span-2" : ""}`}>
      <dt className="text-[11px] uppercase tracking-[0.08em] text-text-tertiary">{label}</dt>
      <dd className="mt-0.5 break-words font-medium text-text-primary">{value}</dd>
    </div>
  );
}

function HeaderFact({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <dt className="text-[11px] uppercase tracking-[0.08em] text-text-tertiary">{label}</dt>
      <dd className={mono ? "font-mono text-[12px] font-semibold text-text-primary" : "font-semibold text-text-primary"}>{value}</dd>
    </div>
  );
}

function TimelineRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-3">
      <dt className="w-36 shrink-0 text-text-secondary">{label}</dt>
      <dd className="min-w-0 break-words text-text-primary">{value}</dd>
    </div>
  );
}

function AmountRow({ label, value, strong = false }: { label: string; value: number; strong?: boolean }) {
  return (
    <div className="flex items-center justify-between px-3 py-1.5">
      <dt className={strong ? "font-semibold text-text-primary" : "text-text-secondary"}>{label}</dt>
      <dd className={strong ? "font-mono font-semibold tabular-nums text-text-primary" : "font-mono tabular-nums text-text-primary"}>
        {formatAmount(value)}
      </dd>
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

import { useState, type ReactNode } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Ban, Download, FileText, LoaderCircle, RefreshCw } from "lucide-react";

import { DeliveryHistory } from "@/components/payslips/delivery-history";
import {
  formatAmount,
  formatDateTime,
  formatLongDate,
  formatPayslipPeriod,
  isDeliveryInFlight,
  isRenderInProgress,
  PayslipProgress,
  PayslipStatusPill,
  WarningList,
} from "@/components/payslips/payslip-display";
import { PayslipEditForm } from "@/components/payslips/payslip-edit-form";
import { PayslipPdfPreview } from "@/components/payslips/payslip-pdf-preview";
import { PayslipSendPanel } from "@/components/payslips/payslip-send-panel";
import { VoidReissueDialog } from "@/components/payslips/void-reissue-dialog";
import { Drawer, DrawerBody, DrawerClose, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from "@/components/shared/drawer";
import { Button } from "@/components/ui/button";
import { useRBAC } from "@/hooks/use-rbac";
import { triggerDownload } from "@/lib/download";
import { permissions } from "@/lib/permissions";
import { downloadPayslipDocx, downloadPayslipPDF, getPayslip, payslipsKeys } from "@/services/hris-payslips";
import { toast } from "@/stores/toast-store";
import type { PayslipLine, PayslipListItem, PayslipTotals } from "@/types/documents";

/**
 * Row drawer of the Slip Gaji page: PDF preview, amounts, catatan and
 * manual lines (drafts), Generate ulang, Kirim, Batalkan & Terbitkan Ulang
 * and the delivery history. `item` is the live table row; the detail is
 * re-read whenever the row's render or delivery state changes, so the
 * drawer follows the page's polling without polling on its own (every
 * detail read is access-logged).
 */
export function PayslipDrawer({
  open,
  onOpenChange,
  item,
  year,
  month,
  pdfAvailable,
  documentMailReady,
  allowDocxSend,
  onRegenerate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  item: PayslipListItem | null;
  year: number;
  month: number;
  pdfAvailable: boolean;
  documentMailReady: boolean;
  allowDocxSend: boolean;
  /** Opens the Generate dialog for this employee (payDate: the slip's own date on regenerate). */
  onRegenerate: (item: PayslipListItem, payDate: string | null) => void;
}) {
  return (
    <Drawer onOpenChange={onOpenChange} open={open}>
      <DrawerContent size="lg">
        {item ? (
          <PayslipDrawerContent
            allowDocxSend={allowDocxSend}
            documentMailReady={documentMailReady}
            item={item}
            month={month}
            onRegenerate={onRegenerate}
            open={open}
            pdfAvailable={pdfAvailable}
            year={year}
          />
        ) : null}
      </DrawerContent>
    </Drawer>
  );
}

function PayslipDrawerContent({
  item,
  open,
  year,
  month,
  pdfAvailable,
  documentMailReady,
  allowDocxSend,
  onRegenerate,
}: {
  item: PayslipListItem;
  open: boolean;
  year: number;
  month: number;
  pdfAvailable: boolean;
  documentMailReady: boolean;
  allowDocxSend: boolean;
  onRegenerate: (item: PayslipListItem, payDate: string | null) => void;
}) {
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.hrisPayslipManage);
  const canSend = hasPermission(permissions.hrisPayslipSend);
  const [voidOpen, setVoidOpen] = useState(false);
  const [downloading, setDownloading] = useState<"pdf" | "docx" | null>(null);

  const payslipId = item.payslip_id;
  const detailQuery = useQuery({
    queryKey: [
      ...payslipsKeys.detail(payslipId ?? "none"),
      item.status,
      item.render_status,
      item.last_delivery?.id ?? "",
      item.last_delivery?.status ?? "",
    ],
    queryFn: () => getPayslip(payslipId!),
    enabled: open && Boolean(payslipId),
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
  });
  // keepPreviousData may still hold the slip this employee had before a
  // void & reissue; only use a detail that belongs to the current row.
  const detail = detailQuery.data && detailQuery.data.id === payslipId ? detailQuery.data : undefined;

  const download = async (format: "pdf" | "docx") => {
    if (!payslipId) {
      return;
    }
    setDownloading(format);
    try {
      const result = format === "pdf" ? await downloadPayslipPDF(payslipId, "attachment") : await downloadPayslipDocx(payslipId);
      triggerDownload(result.blob, result.filename ?? `${item.doc_number ?? "slip-gaji"}.${format}`);
    } catch (error) {
      toast.error("Unduhan gagal", error instanceof Error ? error.message : undefined);
    } finally {
      setDownloading(null);
    }
  };

  const warnings = detail?.warnings ?? item.warnings ?? [];
  const totals: PayslipTotals = detail?.totals ?? item.totals;
  const renderBusy = isRenderInProgress(item.render_status);
  const sendBusy = isDeliveryInFlight(item.last_delivery);
  const pdfReady = item.render_status === "ready" && item.has_pdf;

  let sendDisabledReason: string | null = null;
  if (sendBusy) {
    sendDisabledReason = "Pengiriman sebelumnya masih berjalan";
  } else if (renderBusy) {
    sendDisabledReason = "Tunggu PDF selesai dibuat";
  } else if (!pdfReady && !(allowDocxSend && !pdfAvailable)) {
    sendDisabledReason = pdfAvailable ? "PDF slip belum siap" : "Konverter PDF belum tersedia";
  }

  return (
    <>
      <DrawerHeader className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">
            Slip Gaji · {formatPayslipPeriod(year, month)}
          </p>
          <DrawerTitle className="mt-1 truncate">{item.employee_name}</DrawerTitle>
          <DrawerDescription className="mt-1">
            {[item.job_title || "Jabatan belum diisi", item.department, item.employee_code].filter(Boolean).join(" · ")}
          </DrawerDescription>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <PayslipStatusPill blocked={item.blocked} status={item.status} />
            <PayslipProgress item={item} pdfAvailable={pdfAvailable} />
            {item.doc_number ? <span className="font-mono text-[12px] text-text-secondary">{item.doc_number}</span> : null}
          </div>
        </div>
        <DrawerClose />
      </DrawerHeader>

      <DrawerBody className="space-y-6">
        {detail?.replaces_doc_number ? (
          <p className="rounded-xl bg-info-light px-3 py-2 text-[13px] text-info">
            Menggantikan slip No. {detail.replaces_doc_number} yang dibatalkan.
          </p>
        ) : null}

        {warnings.length > 0 ? (
          <Section title="Peringatan">
            <WarningList warnings={warnings} />
          </Section>
        ) : null}

        {item.status === "none" ? (
          <Section title="Belum dibuat">
            <p className="text-sm text-text-secondary">
              Angka di bawah adalah pratinjau dari data saat ini; belum ada slip yang tersimpan.
            </p>
            <TotalsTable totals={totals} />
            {canManage ? (
              <div className="mt-3 flex justify-end">
                <Button disabled={item.blocked} onClick={() => onRegenerate(item, null)} size="sm" type="button">
                  <FileText className="h-4 w-4" />
                  Generate draf
                </Button>
              </div>
            ) : null}
          </Section>
        ) : null}

        {payslipId ? (
          <>
            <Section
              action={
                <div className="flex flex-wrap gap-2">
                  <Button
                    disabled={!pdfReady || downloading !== null}
                    onClick={() => void download("pdf")}
                    size="xs"
                    type="button"
                    variant="outline"
                  >
                    <Download className="h-4 w-4" />
                    {downloading === "pdf" ? "Mengunduh..." : "PDF"}
                  </Button>
                  {canManage ? (
                    <Button
                      disabled={downloading !== null || renderBusy}
                      onClick={() => void download("docx")}
                      size="xs"
                      type="button"
                      variant="outline"
                    >
                      <Download className="h-4 w-4" />
                      {downloading === "docx" ? "Mengunduh..." : "DOCX"}
                    </Button>
                  ) : null}
                </div>
              }
              title="Pratinjau PDF"
            >
              {pdfReady && detail?.pdf_sha256 && open ? (
                <PayslipPdfPreview payslipId={payslipId} version={detail.pdf_sha256} />
              ) : renderBusy ? (
                <div className="flex h-[200px] items-center justify-center gap-2 rounded-2xl border border-border/70 bg-surface-muted/60 text-sm text-text-secondary">
                  <LoaderCircle className="h-4 w-4 animate-spin" />
                  PDF sedang dibuat...
                </div>
              ) : !pdfAvailable ? (
                <p className="rounded-xl bg-warning-light px-3 py-2 text-sm text-warning">
                  Konverter PDF belum tersedia di server. Unduh DOCX untuk memeriksa slip.
                </p>
              ) : item.render_status === "failed" ? (
                <p className="rounded-xl bg-error-light px-3 py-2 text-sm text-error">
                  Pembuatan PDF gagal{item.render_error ? `: ${item.render_error}` : ""}. Coba Generate ulang.
                </p>
              ) : item.render_status === "none" && item.status === "draft" ? (
                <p className="rounded-xl bg-surface-muted px-3 py-2 text-sm text-text-secondary">
                  PDF belum dibuat untuk draf ini. Generate ulang untuk membuat PDF.
                </p>
              ) : detailQuery.isLoading ? (
                <div className="h-[200px] animate-pulse rounded-2xl bg-surface-muted" />
              ) : null}
            </Section>

            {detail ? (
              <Section title="Rincian">
                <dl className="mb-3 grid gap-2 text-[13px] sm:grid-cols-2">
                  <Info label="Tanggal bayar" value={formatLongDate(detail.pay_date)} />
                  <Info label="Status kerja" value={detail.header.status_kerja || "-"} />
                  <Info label="Rekening" value={[detail.header.bank, detail.header.account_masked].filter(Boolean).join(" · ") || "-"} />
                  <Info label="Dibuat" value={formatDateTime(detail.generated_at)} />
                </dl>
                <LinesTable emptyLabel="-" lines={detail.earnings ?? []} title="Gaji" />
                <LinesTable emptyLabel="Tidak ada potongan" lines={detail.deductions ?? []} title="Potongan" />
                {(detail.reimbursements ?? []).length > 0 ? (
                  <LinesTable
                    emptyLabel="-"
                    lines={(detail.reimbursements ?? []).map((line) => ({
                      kind: "reimbursement",
                      label: line.title,
                      keterangan: line.category,
                      amount: line.amount,
                    }))}
                    title="Reimbursement"
                  />
                ) : null}
                <TotalsTable totals={totals} />
                {detail.header.total_in_words ? (
                  <p className="mt-2 text-[12px] italic text-text-secondary">Terbilang: {detail.header.total_in_words}</p>
                ) : null}
                {detail.status !== "draft" && detail.note ? (
                  <p className="mt-2 text-[13px] text-text-secondary">Catatan: {detail.note}</p>
                ) : null}
              </Section>
            ) : null}

            {detail && detail.status === "draft" && canManage ? (
              <Section
                action={
                  <Button
                    disabled={renderBusy}
                    onClick={() => onRegenerate(item, detail.pay_date)}
                    size="xs"
                    type="button"
                    variant="outline"
                  >
                    <RefreshCw className="h-4 w-4" />
                    Generate ulang
                  </Button>
                }
                title="Catatan & penyesuaian"
              >
                <PayslipEditForm detail={detail} />
              </Section>
            ) : null}

            {detail && canSend && detail.status !== "void" ? (
              <Section title={detail.status === "sent" ? "Kirim ulang" : "Kirim ke karyawan"}>
                <PayslipSendPanel
                  detail={detail}
                  disabledReason={sendDisabledReason}
                  documentMailReady={documentMailReady}
                />
              </Section>
            ) : null}

            {detail && detail.status === "sent" && canManage ? (
              <Section title="Koreksi slip terkirim">
                <p className="text-[13px] text-text-secondary">
                  Slip yang sudah terkirim tidak dapat diubah. Batalkan lalu terbitkan slip pengganti bila ada kesalahan.
                </p>
                <div className="mt-3 flex justify-end">
                  <Button disabled={sendBusy} onClick={() => setVoidOpen(true)} size="sm" type="button" variant="danger">
                    <Ban className="h-4 w-4" />
                    Batalkan & Terbitkan Ulang
                  </Button>
                </div>
                <VoidReissueDialog detail={detail} onClose={() => setVoidOpen(false)} open={voidOpen} />
              </Section>
            ) : null}

            <Section title="Riwayat pengiriman">
              <DeliveryHistory
                referenceId={payslipId}
                referenceType="payslip"
                version={`${item.last_delivery?.id ?? ""}:${item.last_delivery?.status ?? ""}`}
              />
            </Section>
          </>
        ) : null}
      </DrawerBody>
    </>
  );
}

function Section({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-[12px] font-[700] uppercase tracking-[0.08em] text-text-secondary">{title}</h3>
        {action}
      </div>
      {children}
    </section>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl bg-surface-muted/60 px-3 py-2">
      <dt className="text-[11px] uppercase tracking-[0.08em] text-text-tertiary">{label}</dt>
      <dd className="mt-0.5 font-medium text-text-primary">{value}</dd>
    </div>
  );
}

function LinesTable({ title, lines, emptyLabel }: { title: string; lines: PayslipLine[]; emptyLabel: string }) {
  return (
    <div className="mb-3 overflow-hidden rounded-xl border border-border/70">
      <p className="bg-surface-muted px-3 py-1.5 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-secondary">{title}</p>
      {lines.length === 0 ? (
        <p className="px-3 py-2 text-[13px] text-text-secondary">{emptyLabel}</p>
      ) : (
        <table className="w-full text-[13px]">
          <tbody>
            {lines.map((line, index) => (
              <tr className="border-t border-border/60 first:border-t-0" key={`${line.kind}-${line.label}-${index}`}>
                <td className="px-3 py-1.5 text-text-primary">
                  {line.label}
                  {line.kind === "manual" ? <span className="ml-1.5 text-[11px] text-hr">(penyesuaian)</span> : null}
                  {line.keterangan ? <span className="block text-[11px] text-text-tertiary">{line.keterangan}</span> : null}
                </td>
                <td className="px-3 py-1.5 text-right font-mono tabular-nums text-text-primary">{formatAmount(line.amount)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function TotalsTable({ totals }: { totals: PayslipTotals }) {
  const rows: Array<[string, number, boolean?]> = [
    ["Total gaji", totals.total_gaji],
    ["Total potongan", -totals.total_potongan],
    ["Total reimbursement", totals.total_reimbursement],
    ["Total diterima", totals.total_diterima, true],
  ];
  return (
    <dl className="divide-y divide-border/60 rounded-xl border border-border/70 text-[13px]" data-testid="payslip-totals">
      {rows.map(([label, value, strong]) => (
        <div className="flex items-center justify-between px-3 py-1.5" key={label}>
          <dt className={strong ? "font-semibold text-text-primary" : "text-text-secondary"}>{label}</dt>
          <dd className={strong ? "font-mono font-semibold tabular-nums text-text-primary" : "font-mono tabular-nums text-text-primary"}>
            {formatAmount(value)}
          </dd>
        </div>
      ))}
    </dl>
  );
}


import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeft, Ban, Download, FileCheck2, FileText, Info, Mail, Save, Trash2, TriangleAlert, Wand2 } from "lucide-react";

import { formatShortDate } from "@/components/contracts/contract-display";
import { LegalFieldForm } from "@/components/legal/legal-field-form";
import { LegalStatusPill } from "@/components/legal/legal-display";
import { DeliveryStatusPill, formatDateTime, RenderStatusPill } from "@/components/payslips/payslip-display";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { FormModal } from "@/components/shared/form-modal";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { useRBAC } from "@/hooks/use-rbac";
import { ApiError } from "@/lib/api-client";
import { triggerDownload } from "@/lib/download";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  cancelLegalDocument,
  deleteLegalDocument,
  downloadLegalDocx,
  downloadLegalPDF,
  downloadLegalSignedCopy,
  generateLegalDocument,
  getLegalDocument,
  legalKeys,
  listLegalDocTypes,
  markLegalDocumentSigned,
  sendLegalDocument,
  updateLegalDocument,
} from "@/services/legal-documents";
import { toast } from "@/stores/toast-store";
import type { LegalDocumentDetail, LegalFieldValues } from "@/types/legal";

export const Route = createFileRoute("/_authenticated/operational/legal-documents/$documentId")({
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalLegalView);
  },
  component: LegalDocumentPage,
});

interface DraftState {
  title: string;
  document_date: string;
  document_city: string;
  fields: LegalFieldValues;
}

function draftFrom(doc: LegalDocumentDetail): DraftState {
  return {
    title: doc.title,
    document_date: doc.document_date ?? "",
    document_city: doc.document_city ?? "",
    fields: doc.fields ?? {},
  };
}

function errorDetails(error: unknown): Record<string, string> {
  if (error instanceof ApiError && error.details && typeof error.details === "object" && !Array.isArray(error.details)) {
    const out: Record<string, string> = {};
    for (const [key, value] of Object.entries(error.details as Record<string, unknown>)) {
      if (typeof value === "string") {
        out[key] = value;
      }
    }
    return out;
  }
  return {};
}

function Notice({ tone, children }: { tone: "warning" | "info"; children: ReactNode }) {
  const Icon = tone === "warning" ? TriangleAlert : Info;
  return (
    <div
      className={
        tone === "warning"
          ? "flex items-start gap-2 rounded-xl border border-warning/30 bg-warning-light px-4 py-3 text-sm text-text-primary"
          : "flex items-start gap-2 rounded-xl border border-info/30 bg-info-light px-4 py-3 text-sm text-text-primary"
      }
      role={tone === "warning" ? "alert" : "status"}
    >
      <Icon className={tone === "warning" ? "mt-0.5 h-4 w-4 shrink-0 text-warning" : "mt-0.5 h-4 w-4 shrink-0 text-info"} />
      <div>{children}</div>
    </div>
  );
}

function LegalDocumentPage() {
  const { documentId } = Route.useParams();
  const navigate = Route.useNavigate();
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.operationalLegalManage);
  const canSend = hasPermission(permissions.operationalLegalSend);

  const typesQuery = useQuery({ queryKey: legalKeys.types(), queryFn: listLegalDocTypes, staleTime: Infinity });
  const docQuery = useQuery({
    queryKey: legalKeys.document(documentId),
    queryFn: () => getLegalDocument(documentId),
    refetchOnWindowFocus: false,
    refetchInterval: (current) => {
      const doc = current.state.data;
      return doc && (doc.render_status === "pending" || doc.render_status === "rendering" || doc.send_in_flight) ? 2500 : false;
    },
  });
  const doc = docQuery.data;
  const spec = typesQuery.data?.find((item) => item.type === doc?.doc_type);

  const [draft, setDraft] = useState<DraftState | null>(null);
  const [dirty, setDirty] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [sendOpen, setSendOpen] = useState(false);
  const [signOpen, setSignOpen] = useState(false);
  const [confirm, setConfirm] = useState<"cancel" | "delete" | null>(null);

  useEffect(() => {
    if (doc && !dirty) {
      setDraft(draftFrom(doc));
    }
  }, [doc, dirty]);

  const readOnly = !canManage || doc?.status === "signed" || doc?.status === "cancelled";
  const issued = doc?.status === "generated" || doc?.status === "sent";

  const applyDetail = (next: LegalDocumentDetail) => {
    queryClient.setQueryData(legalKeys.document(documentId), next);
    void queryClient.invalidateQueries({ queryKey: legalKeys.all, refetchType: "none" });
    setDirty(false);
    setDraft(draftFrom(next));
  };

  const save = async () => {
    if (!doc || !draft) {
      return doc;
    }
    const next = await updateLegalDocument(documentId, {
      title: draft.title,
      party_id: doc.party_id,
      related_document_id: doc.related?.id ?? null,
      fields: draft.fields,
      document_date: draft.document_date || null,
      document_city: draft.document_city || null,
    });
    applyDetail(next);
    return next;
  };

  const handleError = (title: string) => (error: unknown) => {
    const details = errorDetails(error);
    setFieldErrors(details);
    toast.error(title, error instanceof Error ? error.message : undefined);
  };

  const saveMutation = useMutation({
    mutationFn: save,
    onSuccess: () => {
      setFieldErrors({});
      toast.success(issued ? "Disimpan, dokumen kembali ke draf" : "Perubahan disimpan", issued ? "Generate ulang untuk membuat PDF baru." : undefined);
    },
    onError: handleError("Gagal menyimpan"),
  });

  const generateMutation = useMutation({
    mutationFn: async () => {
      if (dirty) {
        await save();
      }
      return generateLegalDocument(documentId);
    },
    onSuccess: (next) => {
      applyDetail(next);
      setFieldErrors({});
      toast.success("Dokumen digenerate", `No. ${next.doc_number ?? "-"}. PDF sedang dibuat.`);
    },
    onError: handleError("Belum bisa generate"),
  });

  const statusMutation = useMutation({
    mutationFn: async (action: "cancel" | "delete") => {
      if (action === "delete") {
        await deleteLegalDocument(documentId);
        return null;
      }
      return cancelLegalDocument(documentId);
    },
    onSuccess: (next, action) => {
      setConfirm(null);
      if (action === "delete" || !next) {
        void queryClient.invalidateQueries({ queryKey: legalKeys.all });
        toast.success("Draf dihapus");
        void navigate({ to: "/operational/legal-documents" });
        return;
      }
      applyDetail(next);
      toast.success("Dokumen dibatalkan");
    },
    onError: (error) => {
      setConfirm(null);
      toast.error("Gagal", error instanceof Error ? error.message : undefined);
    },
  });

  const download = async (kind: "pdf" | "docx" | "signed") => {
    try {
      const result =
        kind === "pdf" ? await downloadLegalPDF(documentId, "attachment") : kind === "docx" ? await downloadLegalDocx(documentId) : await downloadLegalSignedCopy(documentId);
      triggerDownload(result.blob, result.filename ?? `dokumen.${kind === "docx" ? "docx" : "pdf"}`);
    } catch (error) {
      toast.error("Gagal mengunduh", error instanceof Error ? error.message : undefined);
    }
  };

  if (docQuery.error) {
    return (
      <Card className="p-8">
        <p className="text-sm text-error-strong" role="alert">
          {docQuery.error instanceof Error ? docQuery.error.message : "Dokumen tidak dapat dimuat"}
        </p>
      </Card>
    );
  }
  if (!doc || !draft || !spec) {
    return <Card className="p-8 text-sm text-text-secondary">Memuat dokumen…</Card>;
  }

  const missingEntries = Object.entries(doc.missing ?? {});
  const companyMissing = doc.company_missing ?? [];
  const renderBusy = doc.render_status === "pending" || doc.render_status === "rendering";

  return (
    <div className="space-y-6">
      <Card className="p-6 sm:p-8">
        <Link className="mb-3 inline-flex items-center gap-1.5 text-[13px] text-text-secondary hover:text-text-primary" to="/operational/legal-documents">
          <ArrowLeft className="h-4 w-4" />
          Dokumen Legal
        </Link>
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0 space-y-1.5">
            <p className="text-[13px] font-medium text-ops">{spec.label}</p>
            <h3 className="text-[24px] font-[700] text-text-primary">{doc.title}</h3>
            <div className="flex flex-wrap items-center gap-2 text-[13px] text-text-secondary">
              <LegalStatusPill status={doc.status} />
              {doc.render_status !== "none" ? <RenderStatusPill error={doc.render_error} status={doc.render_status} /> : null}
              <span className="font-mono">{doc.doc_number ?? "Belum ada nomor"}</span>
              <span>·</span>
              <span>{doc.party.name}</span>
              {doc.signed_at ? <span>· ditandatangani {formatShortDate(doc.signed_at)}</span> : null}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            {canManage && !readOnly ? (
              <>
                <Button disabled={!dirty || saveMutation.isPending} onClick={() => saveMutation.mutate()} type="button" variant="outline">
                  <Save className="h-4 w-4" />
                  Simpan
                </Button>
                <Button disabled={generateMutation.isPending || renderBusy} onClick={() => generateMutation.mutate()} type="button" variant="ops">
                  <Wand2 className="h-4 w-4" />
                  {doc.doc_number ? "Generate ulang" : "Generate"}
                </Button>
              </>
            ) : null}
            {canSend && (issued || doc.status === "signed") ? (
              <Button disabled={!doc.pdf_ready || !doc.mail_ready || doc.send_in_flight || dirty} onClick={() => setSendOpen(true)} type="button" variant="outline">
                <Mail className="h-4 w-4" />
                Kirim email
              </Button>
            ) : null}
            {canManage && (issued || doc.status === "signed") ? (
              <Button disabled={dirty} onClick={() => setSignOpen(true)} type="button" variant="outline">
                <FileCheck2 className="h-4 w-4" />
                {doc.status === "signed" ? "Perbarui tanda tangan" : "Tandai ditandatangani"}
              </Button>
            ) : null}
          </div>
        </div>
      </Card>

      <div className="space-y-3">
        {companyMissing.length > 0 ? (
          <Notice tone="warning">
            Profil Perusahaan belum lengkap ({companyMissing.join(", ")}). Lengkapi di{" "}
            <Link className="font-semibold underline" to="/admin/settings">
              Admin &gt; Pengaturan
            </Link>{" "}
            sebelum generate.
          </Notice>
        ) : null}
        {!readOnly && missingEntries.length > 0 ? (
          <Notice tone="info">Masih ada {missingEntries.length} isian wajib yang kosong. Lengkapi sebelum generate.</Notice>
        ) : null}
        {issued && dirty ? (
          <Notice tone="info">Menyimpan perubahan mengembalikan dokumen ke draf; nomor tetap sama, lalu generate ulang untuk PDF baru.</Notice>
        ) : null}
        {doc.render_status === "failed" ? <Notice tone="warning">PDF gagal dibuat: {doc.render_error ?? "coba generate ulang"}.</Notice> : null}
        {!doc.pdf_available ? (
          <Notice tone="warning">Konverter PDF belum tersedia di server; dokumen tetap bisa diunduh sebagai DOCX.</Notice>
        ) : null}
        {canSend && issued && !doc.mail_ready ? (
          <Notice tone="info">Email Dokumen belum aktif, jadi dokumen belum bisa dikirim lewat email. Admin dapat mengaktifkannya di Pengaturan.</Notice>
        ) : null}
      </div>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_420px]">
        <Card className="space-y-8 p-6">
          <section className="space-y-4">
            <h3 className="border-b border-border pb-2 text-[15px] font-semibold text-text-primary">Dokumen</h3>
            <div className="grid gap-4 md:grid-cols-3">
              <div className="space-y-1.5 md:col-span-3">
                <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-title">
                  Judul (untuk daftar)
                </label>
                <Input
                  disabled={readOnly}
                  id="legal-title"
                  onChange={(event) => {
                    setDraft({ ...draft, title: event.target.value });
                    setDirty(true);
                  }}
                  value={draft.title}
                />
              </div>
              <div className="space-y-1.5">
                <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-date">
                  Tanggal dokumen
                </label>
                <Input
                  disabled={readOnly}
                  id="legal-date"
                  onChange={(event) => {
                    setDraft({ ...draft, document_date: event.target.value });
                    setDirty(true);
                  }}
                  type="date"
                  value={draft.document_date}
                />
                <p className="text-[12px] text-text-secondary">Kosong = tanggal saat generate</p>
              </div>
              <div className="space-y-1.5">
                <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-city">
                  Kota penandatanganan
                </label>
                <Input
                  disabled={readOnly}
                  id="legal-city"
                  onChange={(event) => {
                    setDraft({ ...draft, document_city: event.target.value });
                    setDirty(true);
                  }}
                  placeholder="Kota di Profil Perusahaan"
                  value={draft.document_city}
                />
              </div>
              <div className="space-y-1.5">
                <p className="block text-[13px] font-medium text-text-secondary">{spec.party_label}</p>
                <p className="flex h-10 items-center text-sm text-text-primary">{doc.party.name}</p>
              </div>
            </div>
            {doc.doc_type === "bast" && doc.related ? (
              <SowMilestonePicker
                disabled={readOnly}
                onApply={(patch) => {
                  setDraft({ ...draft, fields: { ...draft.fields, ...patch } });
                  setDirty(true);
                }}
                sowId={doc.related.id}
              />
            ) : null}
          </section>
          <LegalFieldForm
            disabled={readOnly}
            errors={fieldErrors}
            onChange={(fields) => {
              setDraft({ ...draft, fields });
              setDirty(true);
            }}
            spec={spec}
            values={draft.fields}
          />
          {canManage && !readOnly ? (
            <div className="flex flex-wrap justify-between gap-2 border-t border-border pt-4">
              {doc.doc_number ? (
                <Button onClick={() => setConfirm("cancel")} type="button" variant="ghost">
                  <Ban className="h-4 w-4" />
                  Batalkan dokumen
                </Button>
              ) : (
                <Button onClick={() => setConfirm("delete")} type="button" variant="ghost">
                  <Trash2 className="h-4 w-4" />
                  Hapus draf
                </Button>
              )}
              <Button disabled={!dirty || saveMutation.isPending} onClick={() => saveMutation.mutate()} type="button" variant="outline">
                <Save className="h-4 w-4" />
                Simpan perubahan
              </Button>
            </div>
          ) : null}
        </Card>

        <div className="space-y-6">
          <Card className="space-y-4 p-6">
            <h3 className="text-[15px] font-semibold text-text-primary">File</h3>
            {doc.pdf_ready ? (
              <LegalPdfPreview documentId={documentId} version={doc.updated_at} />
            ) : (
              <p className="text-sm text-text-secondary">
                {renderBusy ? "PDF sedang dibuat…" : doc.doc_number ? "PDF belum tersedia." : "Generate dokumen untuk membuat PDF."}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button disabled={!doc.pdf_ready} onClick={() => void download("pdf")} size="sm" type="button" variant="outline">
                <Download className="h-4 w-4" />
                PDF
              </Button>
              {canManage ? (
                <Button disabled={!doc.generated_at || doc.status === "draft"} onClick={() => void download("docx")} size="sm" type="button" variant="outline">
                  <FileText className="h-4 w-4" />
                  DOCX
                </Button>
              ) : null}
              {doc.has_signed_copy ? (
                <Button onClick={() => void download("signed")} size="sm" type="button" variant="outline">
                  <FileCheck2 className="h-4 w-4" />
                  Salinan bertanda tangan
                </Button>
              ) : null}
            </div>
          </Card>

          <Card className="space-y-3 p-6">
            <h3 className="text-[15px] font-semibold text-text-primary">Riwayat pengiriman</h3>
            {(doc.deliveries ?? []).length === 0 ? (
              <p className="text-sm text-text-secondary">Belum pernah dikirim.</p>
            ) : (
              <ul className="space-y-2">
                {(doc.deliveries ?? []).map((delivery) => (
                  <li className="flex items-start justify-between gap-3 rounded-xl border border-border px-3 py-2.5 text-[13px]" key={delivery.id}>
                    <div className="min-w-0">
                      <p className="truncate text-text-primary">{delivery.recipient}</p>
                      <p className="text-text-secondary">{formatDateTime(delivery.sent_at ?? delivery.created_at)}</p>
                    </div>
                    <DeliveryStatusPill delivery={{ status: delivery.status, error: delivery.error }} />
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>
      </div>

      {canSend ? <SendDialog doc={doc} isOpen={sendOpen} onClose={() => setSendOpen(false)} onSent={applyDetail} /> : null}
      {canManage ? <SignDialog doc={doc} isOpen={signOpen} onClose={() => setSignOpen(false)} onSigned={applyDetail} /> : null}
      <ConfirmDialog
        confirmLabel={confirm === "delete" ? "Hapus draf" : "Batalkan dokumen"}
        description={
          confirm === "delete"
            ? "Draf ini belum punya nomor dan akan dihapus."
            : "Dokumen tidak bisa diubah, dikirim, atau digenerate lagi setelah dibatalkan. Nomornya tetap tercatat."
        }
        isLoading={statusMutation.isPending}
        isOpen={confirm !== null}
        onClose={() => setConfirm(null)}
        onConfirm={() => confirm && statusMutation.mutate(confirm)}
        title={confirm === "delete" ? "Hapus draf?" : "Batalkan dokumen?"}
        tone="danger"
      />
    </div>
  );
}

/** Inline PDF preview; the object URL is revoked when the version changes. */
function LegalPdfPreview({ documentId, version }: { documentId: string; version: string }) {
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    let objectUrl: string | null = null;
    setUrl(null);
    setError(null);
    downloadLegalPDF(documentId, "inline")
      .then((result) => {
        const blob = result.blob.type === "application/pdf" ? result.blob : new Blob([result.blob], { type: "application/pdf" });
        objectUrl = URL.createObjectURL(blob);
        if (active) {
          setUrl(objectUrl);
        } else {
          URL.revokeObjectURL(objectUrl);
        }
      })
      .catch((err: unknown) => active && setError(err instanceof Error ? err.message : "PDF tidak dapat dimuat"));
    return () => {
      active = false;
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [documentId, version]);

  if (error) {
    return <p className="text-sm text-error-strong">{error}</p>;
  }
  if (!url) {
    return <p className="text-sm text-text-secondary">Memuat pratinjau…</p>;
  }
  return <iframe className="h-[560px] w-full rounded-xl border border-border bg-surface-muted" src={`${url}#navpanes=0&view=FitH`} title="Pratinjau PDF" />;
}

/** Fills a BAST from one milestone of its SOW: termin, name, value and the linked deliverables. */
function SowMilestonePicker({ sowId, disabled, onApply }: { sowId: string; disabled: boolean; onApply: (patch: LegalFieldValues) => void }) {
  const sowQuery = useQuery({ queryKey: legalKeys.document(sowId), queryFn: () => getLegalDocument(sowId) });
  const milestones = useMemo(() => (Array.isArray(sowQuery.data?.fields.milestone) ? (sowQuery.data?.fields.milestone as Array<Record<string, unknown>>) : []), [sowQuery.data]);
  const deliverables = useMemo(
    () => (Array.isArray(sowQuery.data?.fields.deliverable) ? (sowQuery.data?.fields.deliverable as Array<Record<string, unknown>>) : []),
    [sowQuery.data],
  );
  if (!sowQuery.data || milestones.length === 0) {
    return null;
  }
  const apply = (value: string) => {
    const index = Number(value);
    const milestone = milestones[index];
    if (!milestone) {
      return;
    }
    const linked = String(milestone.deliverable_terkait ?? "")
      .split(/[,\s]+/)
      .filter(Boolean);
    const items = deliverables
      .map((deliverable, position) => ({ deliverable, code: `D${position + 1}` }))
      .filter(({ code }) => linked.includes(code))
      .map(({ deliverable }) => ({ nama: deliverable.nama ?? "", deskripsi: deliverable.deskripsi ?? "", jumlah: "1 paket", status: "Diterima" }));
    onApply({
      termin_ke: String(index + 1),
      milestone_nama: milestone.nama ?? "",
      nilai_termin: typeof milestone.nilai === "number" ? milestone.nilai : null,
      ...(items.length ? { item_serah_terima: items } : {}),
    });
  };
  return (
    <div className="space-y-1.5 rounded-xl border border-border bg-surface-muted/60 p-4">
      <label className="block text-[13px] font-medium text-text-secondary" htmlFor="bast-milestone">
        Ambil dari milestone SOW {sowQuery.data.doc_number}
      </label>
      <Select
        disabled={disabled}
        id="bast-milestone"
        onValueChange={apply}
        options={milestones.map((milestone, index) => ({ value: String(index), label: `Termin ${index + 1} — ${String(milestone.nama ?? "")}` }))}
        placeholder="Pilih milestone"
      />
      <p className="text-[12px] text-text-secondary">Mengisi termin, nama milestone, nilai tagihan, dan item serah terima.</p>
    </div>
  );
}

function SendDialog({ doc, isOpen, onClose, onSent }: { doc: LegalDocumentDetail; isOpen: boolean; onClose: () => void; onSent: (doc: LegalDocumentDetail) => void }) {
  const [recipient, setRecipient] = useState(doc.default_recipient);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (isOpen) {
      setRecipient(doc.default_recipient);
      setError(null);
    }
  }, [isOpen, doc.default_recipient]);
  const mutation = useMutation({
    mutationFn: () => sendLegalDocument(doc.id, recipient.trim()),
    onSuccess: (result) => {
      onSent(result.document);
      if (result.sent) {
        toast.success("Email terkirim", result.delivery.recipient);
        onClose();
      } else {
        setError(result.error_message ?? "Email gagal dikirim");
      }
    },
    onError: (err) => setError(err instanceof Error ? err.message : "Email gagal dikirim"),
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!recipient.trim()) {
      setError("Isi alamat email penerima");
      return;
    }
    mutation.mutate();
  };
  return (
    <FormModal
      error={error}
      isLoading={mutation.isPending}
      isOpen={isOpen}
      onClose={onClose}
      onSubmit={submit}
      submitLabel="Kirim"
      subtitle={`PDF ${doc.doc_number ?? ""} dikirim sebagai lampiran.`}
      title="Kirim dokumen"
    >
      <div className="space-y-1.5">
        <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-recipient">
          Email penerima
        </label>
        <Input id="legal-recipient" onChange={(event) => setRecipient(event.target.value)} type="email" value={recipient} />
        <p className="text-[12px] text-text-secondary">Bawaan: email {doc.party.name} di Daftar Pihak. Periksa lagi sebelum mengirim.</p>
      </div>
    </FormModal>
  );
}

function todayISO() {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 10);
}

function SignDialog({ doc, isOpen, onClose, onSigned }: { doc: LegalDocumentDetail; isOpen: boolean; onClose: () => void; onSigned: (doc: LegalDocumentDetail) => void }) {
  const [signedAt, setSignedAt] = useState(doc.signed_at ?? todayISO());
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (isOpen) {
      setSignedAt(doc.signed_at ?? todayISO());
      setFile(null);
      setError(null);
    }
  }, [isOpen, doc.signed_at]);
  const mutation = useMutation({
    mutationFn: () => markLegalDocumentSigned(doc.id, signedAt, file),
    onSuccess: (next) => {
      onSigned(next);
      toast.success("Ditandai sudah ditandatangani", file ? "Salinan bertanda tangan tersimpan." : undefined);
      onClose();
    },
    onError: (err) => setError(err instanceof Error ? err.message : "Gagal menyimpan"),
  });
  return (
    <FormModal
      error={error}
      isLoading={mutation.isPending}
      isOpen={isOpen}
      onClose={onClose}
      onSubmit={(event) => {
        event.preventDefault();
        mutation.mutate();
      }}
      submitLabel="Simpan"
      subtitle="Catat tanggal tanda tangan dan, bila ada, unggah scan dokumen yang sudah ditandatangani."
      title="Dokumen ditandatangani"
    >
      <div className="space-y-4">
        <div className="space-y-1.5">
          <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-signed-at">
            Tanggal tanda tangan
          </label>
          <Input id="legal-signed-at" max={todayISO()} onChange={(event) => setSignedAt(event.target.value)} required type="date" value={signedAt} />
        </div>
        <div className="space-y-1.5">
          <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-signed-file">
            Scan bertanda tangan (PDF, maks. 15 MB, opsional)
          </label>
          <input
            accept="application/pdf,.pdf"
            className="block w-full text-sm text-text-secondary file:mr-3 file:rounded-lg file:border-0 file:bg-surface-muted file:px-3 file:py-2 file:text-sm file:font-medium file:text-text-primary"
            id="legal-signed-file"
            onChange={(event) => setFile(event.target.files?.[0] ?? null)}
            type="file"
          />
          {doc.has_signed_copy ? <p className="text-[12px] text-text-secondary">Sudah ada salinan; unggah baru untuk menggantinya.</p> : null}
        </div>
      </div>
    </FormModal>
  );
}

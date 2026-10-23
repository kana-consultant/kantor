import { useMemo, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Plus, Search, Trash2, UserPlus } from "lucide-react";
import { z } from "zod";

import { formatShortDate } from "@/components/contracts/contract-display";
import { LegalStatusPill, legalStatusLabels, legalStatusValues, partyTypeLabels } from "@/components/legal/legal-display";
import { PartyFormDialog } from "@/components/legal/party-form-dialog";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { FormModal } from "@/components/shared/form-modal";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { cn } from "@/lib/utils";
import {
  createLegalDocument,
  deleteLegalParty,
  getLegalDocument,
  legalKeys,
  listLegalDocTypes,
  listLegalDocuments,
  listLegalParties,
} from "@/services/legal-documents";
import { toast } from "@/stores/toast-store";
import type { LegalDocSpec, LegalDocType, LegalDocumentListItem, LegalFieldValues, LegalParty } from "@/types/legal";

const docTypes = ["freelance", "subcontract", "sow", "sla", "bast"] as const;

const searchSchema = z.object({
  tab: z.enum(["documents", "parties"]).optional().catch(undefined),
  type: z.enum(docTypes).optional().catch(undefined),
  status: z.enum(legalStatusValues).optional().catch(undefined),
});

export const Route = createFileRoute("/_authenticated/operational/legal-documents/")({
  validateSearch: searchSchema,
  beforeLoad: async () => {
    await ensureModuleAccess("operational");
    await ensurePermission(permissions.operationalLegalView);
  },
  component: LegalDocumentsPage,
});

function LegalDocumentsPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.operationalLegalManage);
  const tab = search.tab ?? "documents";
  const [createOpen, setCreateOpen] = useState(false);

  const typesQuery = useQuery({ queryKey: legalKeys.types(), queryFn: listLegalDocTypes, staleTime: Infinity });
  const specs = useMemo(() => typesQuery.data ?? [], [typesQuery.data]);
  const specByType = useMemo(() => new Map(specs.map((spec) => [spec.type, spec])), [specs]);

  const setSearch = (patch: Partial<z.infer<typeof searchSchema>>) =>
    void navigate({ search: (previous) => ({ ...previous, ...patch }) });

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <h3 className="text-[28px] font-[700] text-text-primary">Dokumen Legal</h3>
            <p className="mt-2 max-w-3xl text-[14px] leading-relaxed text-text-secondary">
              Buat perjanjian kemitraan freelance, subkontrak, lingkup kerja (SOW), SLA, dan berita acara serah terima (BAST)
              dari template perusahaan. Generate PDF, kirim ke email pihak terkait, lalu simpan salinan yang sudah ditandatangani.
            </p>
          </div>
          {canManage ? (
            <Button onClick={() => setCreateOpen(true)} type="button" variant="ops">
              <Plus className="h-4 w-4" />
              Dokumen baru
            </Button>
          ) : null}
        </div>
      </Card>

      <div className="flex gap-1 border-b border-border" role="tablist">
        {(
          [
            ["documents", "Dokumen"],
            ["parties", "Daftar pihak"],
          ] as const
        ).map(([value, label]) => (
          <button
            aria-selected={tab === value}
            className={cn(
              "-mb-px border-b-2 px-4 py-2.5 text-sm font-semibold transition-colors",
              tab === value ? "border-ops text-ops" : "border-transparent text-text-secondary hover:text-text-primary",
            )}
            key={value}
            onClick={() => setSearch({ tab: value === "documents" ? undefined : value })}
            role="tab"
            type="button"
          >
            {label}
          </button>
        ))}
      </div>

      {tab === "documents" ? (
        <DocumentsTab
          canManage={canManage}
          onCreate={() => setCreateOpen(true)}
          onOpen={(item) => void navigate({ to: "/operational/legal-documents/$documentId", params: { documentId: item.id } })}
          onSearch={setSearch}
          specByType={specByType}
          status={search.status}
          type={search.type}
        />
      ) : (
        <PartiesTab canManage={canManage} />
      )}

      {canManage ? (
        <CreateDocumentDialog
          isOpen={createOpen}
          onClose={() => setCreateOpen(false)}
          onCreated={(id) => void navigate({ to: "/operational/legal-documents/$documentId", params: { documentId: id } })}
          specs={specs}
        />
      ) : null}
    </div>
  );
}

function DocumentsTab({
  canManage,
  type,
  status,
  specByType,
  onSearch,
  onCreate,
  onOpen,
}: {
  canManage: boolean;
  type?: LegalDocType;
  status?: (typeof legalStatusValues)[number];
  specByType: Map<LegalDocType, LegalDocSpec>;
  onSearch: (patch: { type?: LegalDocType; status?: (typeof legalStatusValues)[number] }) => void;
  onCreate: () => void;
  onOpen: (item: LegalDocumentListItem) => void;
}) {
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query.trim(), 350);
  const filters = { type: type ?? "", status: status ?? "", search: debouncedQuery } as const;
  const listQuery = useQuery({
    queryKey: legalKeys.documents(filters),
    queryFn: () => listLegalDocuments(filters),
    refetchOnWindowFocus: false,
    refetchInterval: (current) =>
      (current.state.data ?? []).some((item) => item.render_status === "pending" || item.render_status === "rendering") ? 3000 : false,
  });

  const columns: Array<DataTableColumn<LegalDocumentListItem>> = [
    {
      id: "title",
      header: "Dokumen",
      accessor: "title",
      sortable: true,
      mobilePrimary: true,
      cell: (item) => (
        <div className="min-w-[180px] space-y-0.5">
          <p className="font-semibold text-text-primary">{item.title}</p>
          <p className="text-[12px] text-text-secondary">{specByType.get(item.doc_type)?.short_label ?? item.doc_type}</p>
        </div>
      ),
    },
    { id: "party", header: "Pihak", accessor: "party_name", sortable: true },
    {
      id: "number",
      header: "No. dokumen",
      cell: (item) =>
        item.doc_number ? (
          <span className="whitespace-nowrap font-mono text-[12px] text-text-primary">{item.doc_number}</span>
        ) : (
          <span className="text-[12px] text-text-secondary">Belum terbit</span>
        ),
    },
    { id: "date", header: "Tanggal", cell: (item) => <span className="whitespace-nowrap">{formatShortDate(item.document_date)}</span>, hideOnMobile: true },
    { id: "status", header: "Status", cell: (item) => <LegalStatusPill status={item.status} /> },
  ];

  return (
    <Card className="space-y-4 p-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:flex-wrap lg:items-center">
        <div className="relative w-full lg:max-w-xs">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-secondary" />
          <Input
            aria-label="Cari dokumen"
            className="h-10 pl-9"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Cari judul, pihak, nomor"
            value={query}
          />
        </div>
        <div className="w-full lg:w-[220px]">
          <Select
            aria-label="Filter jenis"
            onValueChange={(value) => onSearch({ type: (value || undefined) as LegalDocType | undefined })}
            options={[{ value: "", label: "Semua jenis" }, ...docTypes.map((value) => ({ value, label: specByType.get(value)?.short_label ?? value }))]}
            value={type ?? ""}
          />
        </div>
        <div className="w-full lg:w-[200px]">
          <Select
            aria-label="Filter status"
            onValueChange={(value) => onSearch({ status: (value || undefined) as (typeof legalStatusValues)[number] | undefined })}
            options={[{ value: "", label: "Semua status" }, ...legalStatusValues.map((value) => ({ value, label: legalStatusLabels[value] }))]}
            value={status ?? ""}
          />
        </div>
      </div>
      {listQuery.error instanceof Error ? (
        <p className="rounded-xl bg-error-light px-4 py-3 text-sm text-error-strong" role="alert">
          {listQuery.error.message}
        </p>
      ) : (
        <DataTable
          columns={columns}
          data={listQuery.data ?? []}
          emptyActionLabel={canManage ? "Dokumen baru" : undefined}
          emptyDescription="Belum ada dokumen yang cocok. Buat dokumen baru dari salah satu template."
          emptyTitle="Belum ada dokumen"
          getRowId={(item) => item.id}
          loading={listQuery.isLoading}
          onEmptyAction={onCreate}
          onRowClick={onOpen}
        />
      )}
    </Card>
  );
}

function PartiesTab({ canManage }: { canManage: boolean }) {
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query.trim(), 350);
  const [editing, setEditing] = useState<LegalParty | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState<LegalParty | null>(null);
  const partiesQuery = useQuery({ queryKey: legalKeys.parties(debouncedQuery), queryFn: () => listLegalParties(debouncedQuery) });

  const deleteMutation = useMutation({
    mutationFn: (party: LegalParty) => deleteLegalParty(party.id),
    onSuccess: async (_, party) => {
      toast.success("Pihak dihapus", party.name);
      setDeleting(null);
      await queryClient.invalidateQueries({ queryKey: legalKeys.all });
    },
    onError: (error) => {
      setDeleting(null);
      toast.error("Gagal menghapus pihak", error instanceof Error ? error.message : undefined);
    },
  });

  const columns: Array<DataTableColumn<LegalParty>> = [
    {
      id: "name",
      header: "Nama",
      accessor: "name",
      sortable: true,
      mobilePrimary: true,
      cell: (item) => (
        <div className="min-w-[160px] space-y-0.5">
          <p className="font-semibold text-text-primary">{item.name}</p>
          <p className="text-[12px] text-text-secondary">{partyTypeLabels[item.party_type]}</p>
        </div>
      ),
    },
    { id: "email", header: "Email", accessor: "email", cell: (item) => item.email || "-" },
    { id: "bank", header: "Rekening", cell: (item) => (item.bank_account_masked ? `${item.bank_name} ${item.bank_account_masked}` : "-"), hideOnMobile: true },
    { id: "documents", header: "Dokumen", accessor: "documents", numeric: true },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (item) =>
        canManage && item.documents === 0 ? (
          <Button
            aria-label={`Hapus ${item.name}`}
            onClick={(event) => {
              event.stopPropagation();
              setDeleting(item);
            }}
            size="icon"
            type="button"
            variant="ghost"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        ) : null,
    },
  ];

  return (
    <Card className="space-y-4 p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative w-full sm:max-w-xs">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-secondary" />
          <Input
            aria-label="Cari pihak"
            className="h-10 pl-9"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Cari nama atau email"
            value={query}
          />
        </div>
        {canManage ? (
          <Button
            onClick={() => {
              setEditing(null);
              setDialogOpen(true);
            }}
            type="button"
            variant="outline"
          >
            <UserPlus className="h-4 w-4" />
            Pihak baru
          </Button>
        ) : null}
      </div>
      <DataTable
        columns={columns}
        data={partiesQuery.data ?? []}
        emptyDescription="Tambahkan mitra freelance, subkontraktor, atau klien sebelum membuat dokumen."
        emptyTitle="Belum ada pihak"
        getRowId={(item) => item.id}
        loading={partiesQuery.isLoading}
        onRowClick={
          canManage
            ? (item) => {
                setEditing(item);
                setDialogOpen(true);
              }
            : undefined
        }
      />
      {canManage ? <PartyFormDialog isOpen={dialogOpen} onClose={() => setDialogOpen(false)} party={editing} /> : null}
      <ConfirmDialog
        confirmLabel="Hapus"
        description={`${deleting?.name ?? ""} akan dihapus dari daftar pihak.`}
        isLoading={deleteMutation.isPending}
        isOpen={deleting !== null}
        onClose={() => setDeleting(null)}
        onConfirm={() => deleting && deleteMutation.mutate(deleting)}
        title="Hapus pihak?"
        tone="danger"
      />
    </Card>
  );
}

/** BAST built on a SOW starts from the SOW's reference data. */
async function prefillFromRelated(docType: LegalDocType, relatedId: string): Promise<LegalFieldValues> {
  if (docType !== "bast" || !relatedId) {
    return {};
  }
  const sow = await getLegalDocument(relatedId);
  return {
    kontrak_nomor: sow.doc_number ?? "",
    kontrak_tanggal: sow.document_date ?? "",
    proyek_nama: typeof sow.fields.proyek_nama === "string" ? sow.fields.proyek_nama : "",
    mata_uang: typeof sow.fields.mata_uang === "string" ? sow.fields.mata_uang : "IDR",
  };
}

function CreateDocumentDialog({
  isOpen,
  onClose,
  onCreated,
  specs,
}: {
  isOpen: boolean;
  onClose: () => void;
  onCreated: (id: string) => void;
  specs: LegalDocSpec[];
}) {
  const queryClient = useQueryClient();
  const [docType, setDocType] = useState<LegalDocType>("freelance");
  const [partyId, setPartyId] = useState("");
  const [title, setTitle] = useState("");
  const [relatedId, setRelatedId] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [partyDialogOpen, setPartyDialogOpen] = useState(false);
  const spec = specs.find((item) => item.type === docType);

  const partiesQuery = useQuery({ queryKey: legalKeys.parties(""), queryFn: () => listLegalParties(""), enabled: isOpen });
  const relatedQuery = useQuery({
    queryKey: legalKeys.documents({ type: "sow" }),
    queryFn: () => listLegalDocuments({ type: "sow" }),
    enabled: isOpen && docType === "bast",
  });
  const parties = (partiesQuery.data ?? []).filter((party) => spec?.party_types.includes(party.party_type) ?? true);
  const sows = (relatedQuery.data ?? []).filter(
    (item) => item.doc_number && item.status !== "cancelled" && (!partyId || item.party_id === partyId),
  );

  const mutation = useMutation({
    mutationFn: async () => {
      const fields = await prefillFromRelated(docType, relatedId);
      return createLegalDocument({
        doc_type: docType,
        title: title.trim(),
        party_id: partyId,
        related_document_id: relatedId || null,
        fields,
      });
    },
    onSuccess: async (doc) => {
      await queryClient.invalidateQueries({ queryKey: legalKeys.all });
      toast.success("Draf dibuat", doc.title);
      onClose();
      onCreated(doc.id);
    },
    onError: (err) => setError(err instanceof Error ? err.message : "Gagal membuat dokumen"),
  });

  const reset = () => {
    setPartyId("");
    setTitle("");
    setRelatedId("");
    setError(null);
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!partyId) {
      setError(`Pilih ${spec?.party_label.toLowerCase() ?? "pihak"} dulu`);
      return;
    }
    if (!title.trim()) {
      setError("Judul dokumen wajib diisi");
      return;
    }
    mutation.mutate();
  };

  return (
    <>
      <FormModal
        error={error}
        isLoading={mutation.isPending}
        isOpen={isOpen}
        onClose={() => {
          reset();
          onClose();
        }}
        onSubmit={submit}
        submitLabel="Buat draf"
        subtitle="Isian lengkap diisi di halaman berikutnya."
        title="Dokumen baru"
      >
        <div className="space-y-4">
          <div className="space-y-1.5">
            <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-new-type">
              Jenis dokumen
            </label>
            <Select
              id="legal-new-type"
              onValueChange={(value) => {
                setDocType(value as LegalDocType);
                setPartyId("");
                setRelatedId("");
              }}
              options={specs.map((item) => ({ value: item.type, label: item.label, description: item.description }))}
              value={docType}
            />
            {spec ? <p className="text-[12px] text-text-secondary">{spec.description}</p> : null}
          </div>
          <div className="space-y-1.5">
            <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-new-party">
              {spec?.party_label ?? "Pihak"}
            </label>
            <div className="flex gap-2">
              <div className="min-w-0 flex-1">
                <Select
                  id="legal-new-party"
                  onValueChange={setPartyId}
                  options={parties.map((party) => ({ value: party.id, label: party.name, description: partyTypeLabels[party.party_type] }))}
                  placeholder={parties.length ? "Pilih pihak" : "Belum ada pihak"}
                  value={partyId}
                />
              </div>
              <Button onClick={() => setPartyDialogOpen(true)} type="button" variant="outline">
                <UserPlus className="h-4 w-4" />
                Baru
              </Button>
            </div>
          </div>
          {docType === "bast" ? (
            <div className="space-y-1.5">
              <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-new-related">
                SOW acuan (opsional)
              </label>
              <Select
                id="legal-new-related"
                onValueChange={setRelatedId}
                options={[{ value: "", label: "Tanpa SOW" }, ...sows.map((item) => ({ value: item.id, label: `${item.doc_number} — ${item.title}` }))]}
                value={relatedId}
              />
              <p className="text-[12px] text-text-secondary">Nomor kontrak, tanggal, proyek, dan milestone bisa diambil dari SOW.</p>
            </div>
          ) : null}
          <div className="space-y-1.5">
            <label className="block text-[13px] font-medium text-text-secondary" htmlFor="legal-new-title">
              Judul (untuk daftar)
            </label>
            <Input
              id="legal-new-title"
              maxLength={200}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="mis. SOW Aplikasi Internal — PT Klien"
              value={title}
            />
          </div>
        </div>
      </FormModal>
      <PartyFormDialog
        defaultType={spec?.party_types[0] ?? "person"}
        isOpen={partyDialogOpen}
        onClose={() => setPartyDialogOpen(false)}
        onSaved={(party) => setPartyId(party.id)}
        party={null}
      />
    </>
  );
}

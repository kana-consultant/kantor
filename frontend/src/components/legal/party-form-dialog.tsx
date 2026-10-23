import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { FormModal } from "@/components/shared/form-modal";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api-client";
import { createLegalParty, legalKeys, updateLegalParty } from "@/services/legal-documents";
import { toast } from "@/stores/toast-store";
import type { LegalParty, LegalPartyPayload, LegalPartyType } from "@/types/legal";

const jurisdictionIndonesia = "Negara Republik Indonesia / the Republic of Indonesia";

type FormState = Omit<LegalPartyPayload, "id_number" | "tax_id" | "bank_account_number"> & {
  id_number: string;
  tax_id: string;
  bank_account_number: string;
};

function initialState(party: LegalParty | null, partyType: LegalPartyType): FormState {
  return {
    party_type: party?.party_type ?? partyType,
    name: party?.name ?? "",
    address: party?.address ?? "",
    jurisdiction: party?.jurisdiction ?? "",
    id_number: "",
    tax_id: "",
    representative_name: party?.representative_name ?? "",
    representative_title: party?.representative_title ?? "",
    contact_name: party?.contact_name ?? "",
    email: party?.email ?? "",
    phone: party?.phone ?? "",
    bank_name: party?.bank_name ?? "",
    bank_account_number: "",
    bank_account_holder: party?.bank_account_holder ?? "",
    bank_swift: party?.bank_swift ?? "",
    notes: party?.notes ?? "",
  };
}

function Field({ id, label, hint, children }: { id: string; label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <label className="block text-[13px] font-medium text-text-secondary" htmlFor={id}>
        {label}
      </label>
      {children}
      {hint ? <p className="text-[12px] text-text-secondary">{hint}</p> : null}
    </div>
  );
}

/**
 * Create or edit a party (freelancer, subcontractor, client). The identity,
 * NPWP and account numbers are never shown in full: on an edit they stay as
 * they are unless a new value is typed.
 */
export function PartyFormDialog({
  isOpen,
  onClose,
  party,
  defaultType = "person",
  onSaved,
}: {
  isOpen: boolean;
  onClose: () => void;
  party: LegalParty | null;
  defaultType?: LegalPartyType;
  onSaved?: (party: LegalParty) => void;
}) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<FormState>(() => initialState(party, defaultType));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      setForm(initialState(party, defaultType));
      setError(null);
    }
  }, [isOpen, party, defaultType]);

  const mutation = useMutation({
    mutationFn: (payload: LegalPartyPayload) => (party ? updateLegalParty(party.id, payload) : createLegalParty(payload)),
    onSuccess: async (saved) => {
      toast.success(party ? "Pihak diperbarui" : "Pihak ditambahkan", saved.name);
      await queryClient.invalidateQueries({ queryKey: legalKeys.all });
      onSaved?.(saved);
      onClose();
    },
    onError: (err) => setError(err instanceof ApiError || err instanceof Error ? err.message : "Gagal menyimpan pihak"),
  });

  const set = (key: keyof FormState, value: string) => setForm((current) => ({ ...current, [key]: value }));
  const isCompany = form.party_type === "company";

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!form.name.trim()) {
      setError("Nama pihak wajib diisi");
      return;
    }
    const payload: LegalPartyPayload = {
      ...form,
      jurisdiction: isCompany ? form.jurisdiction.trim() || jurisdictionIndonesia : "",
      // Empty on an edit = keep the stored number.
      id_number: form.id_number.trim() || (party ? undefined : ""),
      tax_id: form.tax_id.trim() || (party ? undefined : ""),
      bank_account_number: form.bank_account_number.trim() || (party ? undefined : ""),
    };
    mutation.mutate(payload);
  };

  const keepHint = (masked: string | undefined) =>
    party && masked ? `Tersimpan: ${masked}. Kosongkan jika tidak diubah.` : undefined;

  return (
    <FormModal
      error={error}
      isLoading={mutation.isPending}
      isOpen={isOpen}
      onClose={onClose}
      onSubmit={submit}
      size="lg"
      submitLabel={party ? "Simpan perubahan" : "Tambah pihak"}
      subtitle="Mitra freelance, subkontraktor, atau klien. NIK, NPWP, dan nomor rekening disimpan terenkripsi."
      title={party ? "Edit pihak" : "Pihak baru"}
    >
      <div className="grid gap-4 md:grid-cols-2">
        <Field id="party-type" label="Jenis pihak">
          <Select
            id="party-type"
            onValueChange={(value) => set("party_type", value)}
            options={[
              { value: "person", label: "Perorangan" },
              { value: "company", label: "Badan usaha (PT, CV, dll.)" },
            ]}
            value={form.party_type}
          />
        </Field>
        <Field id="party-name" label={isCompany ? "Nama badan usaha" : "Nama lengkap"}>
          <Input id="party-name" onChange={(event) => set("name", event.target.value)} required value={form.name} />
        </Field>
        <div className="md:col-span-2">
          <Field id="party-address" label="Alamat">
            <Textarea className="min-h-[72px]" id="party-address" onChange={(event) => set("address", event.target.value)} value={form.address} />
          </Field>
        </div>
        {isCompany ? (
          <>
            <Field id="party-rep-name" label="Nama penanda tangan" hint="Direksi atau pejabat yang berwenang menandatangani">
              <Input id="party-rep-name" onChange={(event) => set("representative_name", event.target.value)} value={form.representative_name} />
            </Field>
            <Field id="party-rep-title" label="Jabatan penanda tangan">
              <Input id="party-rep-title" onChange={(event) => set("representative_title", event.target.value)} value={form.representative_title} />
            </Field>
            <Field id="party-jurisdiction" label="Didirikan berdasarkan hukum" hint="Kosongkan untuk badan usaha Indonesia">
              <Input
                id="party-jurisdiction"
                onChange={(event) => set("jurisdiction", event.target.value)}
                placeholder={jurisdictionIndonesia}
                value={form.jurisdiction}
              />
            </Field>
            <Field id="party-id" label="NIB / nomor registrasi" hint={keepHint(party?.id_number_masked)}>
              <Input id="party-id" onChange={(event) => set("id_number", event.target.value)} value={form.id_number} />
            </Field>
          </>
        ) : (
          <Field id="party-id" label="NIK" hint={keepHint(party?.id_number_masked)}>
            <Input id="party-id" inputMode="numeric" onChange={(event) => set("id_number", event.target.value)} value={form.id_number} />
          </Field>
        )}
        <Field id="party-npwp" label="NPWP" hint={keepHint(party?.tax_id_masked)}>
          <Input id="party-npwp" onChange={(event) => set("tax_id", event.target.value)} value={form.tax_id} />
        </Field>
        <Field id="party-contact" label="Nama kontak (PIC)">
          <Input id="party-contact" onChange={(event) => set("contact_name", event.target.value)} value={form.contact_name} />
        </Field>
        <Field id="party-email" label="Email" hint="Alamat bawaan saat dokumen dikirim">
          <Input id="party-email" onChange={(event) => set("email", event.target.value)} type="email" value={form.email} />
        </Field>
        <Field id="party-phone" label="Telepon">
          <Input id="party-phone" onChange={(event) => set("phone", event.target.value)} value={form.phone} />
        </Field>
        <Field id="party-bank" label="Bank">
          <Input id="party-bank" onChange={(event) => set("bank_name", event.target.value)} value={form.bank_name} />
        </Field>
        <Field id="party-account" label="Nomor rekening" hint={keepHint(party?.bank_account_masked)}>
          <Input id="party-account" inputMode="numeric" onChange={(event) => set("bank_account_number", event.target.value)} value={form.bank_account_number} />
        </Field>
        <Field id="party-holder" label="Atas nama rekening">
          <Input id="party-holder" onChange={(event) => set("bank_account_holder", event.target.value)} value={form.bank_account_holder} />
        </Field>
        <Field id="party-swift" label="Kode SWIFT" hint="Hanya untuk bank luar negeri">
          <Input id="party-swift" onChange={(event) => set("bank_swift", event.target.value)} value={form.bank_swift} />
        </Field>
        <div className="md:col-span-2">
          <Field id="party-notes" label="Catatan">
            <Textarea className="min-h-[64px]" id="party-notes" onChange={(event) => set("notes", event.target.value)} value={form.notes} />
          </Field>
        </div>
      </div>
    </FormModal>
  );
}

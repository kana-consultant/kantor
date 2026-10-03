import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LoaderCircle, Mail, Paperclip, Plus, Send, X } from "lucide-react";
import { z } from "zod";

import { formatDateTime, Pill, recipientSourceLabels } from "@/components/payslips/payslip-display";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { emailDeliveriesKeys } from "@/services/email-deliveries";
import { contractsKeys, getContractRecipient, sendContract } from "@/services/hris-contracts";
import { toast } from "@/stores/toast-store";
import type { ContractDetail, ContractRecipientChoice } from "@/types/contracts";

const CC_MAX = 5;
const emailSchema = z.email();

const choiceLabels: Record<ContractRecipientChoice, string> = {
  default: "Otomatis (email login / email karyawan)",
  login: "Email login karyawan",
  employee: "Email data karyawan",
  personal: "Email pribadi (profil HR) — mis. karyawan baru",
};

/** Client-side mirror of the server CC rule: a valid address on the company domain. */
export function ccError(address: string, domain: string, current: string[], recipient: string | undefined) {
  const value = address.trim().toLowerCase();
  if (!emailSchema.safeParse(value).success) {
    return "Format email tidak valid";
  }
  if (!domain) {
    return "CC belum diizinkan: isi email kontak HR di Profil Perusahaan";
  }
  if (value.split("@")[1] !== domain) {
    return `CC hanya untuk alamat @${domain}`;
  }
  if (current.includes(value)) {
    return "Alamat sudah ada di CC";
  }
  if (recipient && value === recipient.toLowerCase()) {
    return "Alamat ini sudah menjadi penerima";
  }
  if (current.length >= CC_MAX) {
    return `Maksimal ${CC_MAX} alamat CC`;
  }
  return null;
}

/**
 * Kirim via Email / Kirim ulang: one e-mail with the PKWT and the NDA PDFs.
 * HR picks the recipient (login e-mail by default; personal e-mail for a
 * new hire whose work mailbox does not exist yet) and may CC addresses on
 * the company domain. A resend needs an explicit confirmation.
 */
export function SendContractDialog({
  contract,
  open,
  onClose,
}: {
  contract: ContractDetail;
  open: boolean;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const { hasAllPermissions } = useRBAC();
  // CC recipients get both documents (full NIK, account number, salary):
  // the server allows CC only to callers who may read them.
  const canCc = hasAllPermissions(
    permissions.hrisContractView,
    permissions.hrisEmployeeIdentityView,
    permissions.hrisSalaryView,
  );
  const [choice, setChoice] = useState<ContractRecipientChoice>("default");
  const [cc, setCc] = useState<string[]>([]);
  const [ccInput, setCcInput] = useState("");
  const [ccMessage, setCcMessage] = useState<string | null>(null);
  const [confirmResend, setConfirmResend] = useState(false);

  useEffect(() => {
    if (!open) {
      setChoice("default");
      setCc([]);
      setCcInput("");
      setCcMessage(null);
      setConfirmResend(false);
    }
  }, [open]);

  // The default resolution says which other sources exist (linked login,
  // personal e-mail); the chosen one is shown before sending.
  const baseQuery = useQuery({
    queryKey: contractsKeys.recipient(contract.id, "default"),
    queryFn: () => getContractRecipient(contract.id, "default"),
    enabled: open,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const recipientQuery = useQuery({
    queryKey: contractsKeys.recipient(contract.id, choice),
    queryFn: () => getContractRecipient(contract.id, choice),
    enabled: open,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const recipient = recipientQuery.data;
  const base = baseQuery.data;
  const domain = canCc ? (base?.cc_domain ?? contract.company.cc_domain) : "";
  // A generated contract (first issue or a new revision) is a first send;
  // a sent, signed or ended one is a resend of the same documents.
  const resend = contract.status !== "generated";

  const choices: ContractRecipientChoice[] = ["default"];
  if (base?.linked) {
    choices.push("login", "employee");
  }
  if (base?.personal_available) {
    choices.push("personal");
  }
  if (!choices.includes(choice)) {
    choices.push(choice);
  }

  const addCc = () => {
    if (ccInput.trim() === "") {
      return;
    }
    const problem = ccError(ccInput, domain, cc, recipient?.recipient);
    if (problem) {
      setCcMessage(problem);
      return;
    }
    setCc((current) => [...current, ccInput.trim().toLowerCase()]);
    setCcInput("");
    setCcMessage(null);
  };

  const mutation = useMutation({
    mutationFn: () =>
      sendContract(contract.id, {
        ...(choice === "default" ? {} : { recipient_source: choice }),
        ...(cc.length > 0 ? { cc } : {}),
      }),
    onSuccess: async (result) => {
      if (result.contract) {
        queryClient.setQueryData(contractsKeys.detail(contract.id), result.contract);
      } else {
        await queryClient.invalidateQueries({ queryKey: contractsKeys.detail(contract.id) });
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "list"] }),
        queryClient.invalidateQueries({ queryKey: emailDeliveriesKeys.all }),
        queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "recipient", contract.id] }),
      ]);
      if (result.sent) {
        toast.success(`Kontrak ${contract.doc_number ?? ""} terkirim`, `Ke ${result.delivery.recipient}`);
        onClose();
      } else {
        toast.error("Email gagal dikirim", result.error_message ?? result.delivery.error ?? undefined);
      }
    },
    onError: (error) => {
      toast.error("Gagal mengirim kontrak", error instanceof Error ? error.message : undefined);
    },
  });

  const pendingInput = ccInput.trim() !== "";
  const blocked =
    !contract.document_mail_ready
      ? "Email Dokumen belum dikonfigurasi di Admin > Pengaturan."
      : recipientQuery.error instanceof Error
        ? recipientQuery.error.message
        : null;
  const canSend = !blocked && Boolean(recipient) && !mutation.isPending && (!resend || confirmResend) && !pendingInput;

  return (
    <Dialog dismissible={!mutation.isPending} onOpenChange={(next) => (!next ? onClose() : undefined)} open={open}>
      <DialogContent size="md">
        <DialogHeader className="flex items-start justify-between gap-4">
          <div>
            <DialogTitle>{resend ? "Kirim ulang kontrak" : "Kirim kontrak via email"}</DialogTitle>
            <DialogDescription>
              Satu email berisi PDF PKWT dan NDA &amp; HKI
              {contract.revision > 0 ? `; subjek diberi "(Revisi ${contract.revision})"` : ""}. Isi email netral tanpa
              nominal.
            </DialogDescription>
          </div>
          <DialogClose disabled={mutation.isPending} />
        </DialogHeader>
        <DialogBody className="space-y-4">
          {!resend && contract.revision > 0 && contract.last_sent_at ? (
            <p className="rounded-xl border border-info/30 bg-info-light px-3 py-2.5 text-[13px] text-info">
              Revisi {contract.revision} menggantikan dokumen yang dikirim {formatDateTime(contract.last_sent_at)}.
            </p>
          ) : null}
          {resend ? (
            <div className="space-y-2 rounded-xl border border-warning/30 bg-warning-light px-3 py-2.5 text-[13px] text-warning">
              <p>
                Kontrak ini sudah dikirim {formatDateTime(contract.last_sent_at)}
                {contract.last_sent_delivery?.recipient ? ` ke ${contract.last_sent_delivery.recipient}` : ""}.
              </p>
              <label className="flex cursor-pointer items-center gap-2 font-semibold">
                <input
                  checked={confirmResend}
                  className="h-4 w-4 accent-primary"
                  data-testid="contract-resend-confirm"
                  onChange={(event) => setConfirmResend(event.target.checked)}
                  type="checkbox"
                />
                Ya, kirim ulang dokumen yang sama
              </label>
            </div>
          ) : null}

          <div className="space-y-1.5">
            <label className="block text-sm font-medium text-text-primary" htmlFor="contract-recipient-source">
              Penerima
            </label>
            <select
              className="field-select w-full"
              disabled={mutation.isPending}
              id="contract-recipient-source"
              onChange={(event) => setChoice(event.target.value as ContractRecipientChoice)}
              value={choice}
            >
              {choices.map((option) => (
                <option key={option} value={option}>
                  {choiceLabels[option]}
                </option>
              ))}
            </select>
            {base && !base.personal_available ? (
              <p className="text-[12px] text-text-tertiary">
                Email pribadi belum diisi di Data Identitas karyawan.
              </p>
            ) : null}
          </div>

          <div className="rounded-xl border border-border/70 bg-surface-muted/40 px-3 py-2.5">
            {recipientQuery.isLoading ? (
              <p className="flex items-center gap-2 text-sm text-text-secondary">
                <LoaderCircle className="h-4 w-4 animate-spin" />
                Memeriksa alamat penerima...
              </p>
            ) : recipientQuery.error instanceof Error ? (
              <p className="text-sm text-error">{recipientQuery.error.message}</p>
            ) : recipient ? (
              <div className="space-y-1">
                <p className="flex flex-wrap items-center gap-2 text-sm">
                  <Mail className="h-4 w-4 text-text-tertiary" />
                  <span className="font-semibold text-text-primary" data-testid="contract-recipient">
                    {recipient.recipient}
                  </span>
                  {recipient.is_new ? <Pill tone="warning">alamat baru</Pill> : null}
                  {recipient.recipient_source === "personal" ? <Pill tone="warning">email pribadi</Pill> : null}
                </p>
                <p className="text-[12px] text-text-secondary">
                  {recipientSourceLabels[recipient.recipient_source]}
                  {recipient.is_new && recipient.previous_recipient
                    ? ` · dokumen sebelumnya dikirim ke ${recipient.previous_recipient}`
                    : ""}
                </p>
              </div>
            ) : null}
          </div>

          <div className="space-y-1.5">
            <label className="block text-sm font-medium text-text-primary" htmlFor="contract-cc">
              CC (opsional)
            </label>
            <div className="flex gap-2">
              <Input
                autoComplete="off"
                disabled={mutation.isPending || !domain || cc.length >= CC_MAX}
                id="contract-cc"
                onChange={(event) => {
                  setCcInput(event.target.value);
                  setCcMessage(null);
                }}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === ",") {
                    event.preventDefault();
                    addCc();
                  }
                }}
                placeholder={domain ? `nama@${domain}` : "CC tidak tersedia"}
                type="email"
                value={ccInput}
              />
              <Button
                disabled={mutation.isPending || !domain || ccInput.trim() === ""}
                onClick={addCc}
                type="button"
                variant="outline"
              >
                <Plus className="h-4 w-4" />
                Tambah
              </Button>
            </div>
            {ccMessage ? (
              <p className="text-[12px] text-error" data-testid="contract-cc-error">
                {ccMessage}
              </p>
            ) : pendingInput ? (
              <p className="text-[12px] text-warning">Tekan Tambah untuk memasukkan alamat ini ke CC.</p>
            ) : (
              <p className="text-[12px] text-text-tertiary">
                {!canCc
                  ? "CC memerlukan izin melihat kontrak, identitas karyawan, dan gaji."
                  : domain
                    ? `Hanya alamat @${domain} (domain email kontak HR), maksimal ${CC_MAX}.`
                    : "CC tidak tersedia: email kontak HR di Profil Perusahaan belum diisi atau memakai domain email publik (mis. gmail.com)."}
              </p>
            )}
            {cc.length > 0 ? (
              <ul className="flex flex-wrap gap-1.5">
                {cc.map((address) => (
                  <li
                    className="inline-flex items-center gap-1 rounded-full bg-hr-light px-2.5 py-1 text-[12px] font-semibold text-hr"
                    key={address}
                  >
                    {address}
                    <button
                      aria-label={`Hapus ${address}`}
                      className="rounded-full p-0.5 hover:bg-hr/10"
                      disabled={mutation.isPending}
                      onClick={() => setCc((current) => current.filter((item) => item !== address))}
                      type="button"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>

          <p className="flex items-center gap-1.5 text-[12px] text-text-secondary">
            <Paperclip className="h-3.5 w-3.5" />
            {contract.doc_number} · {contract.nda_doc_number}
          </p>

          {blocked ? <p className="text-[13px] text-warning">{blocked}</p> : null}
        </DialogBody>
        <DialogFooter>
          <Button disabled={mutation.isPending} onClick={onClose} type="button" variant="ghost">
            Batal
          </Button>
          <Button data-testid="contract-send-confirm" disabled={!canSend} onClick={() => mutation.mutate()} type="button">
            <Send className="h-4 w-4" />
            {mutation.isPending ? "Mengirim..." : resend ? "Kirim ulang" : "Kirim"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

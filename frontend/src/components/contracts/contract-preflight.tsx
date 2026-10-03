import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { CircleAlert, CircleCheck, FileCog, LoaderCircle, TriangleAlert, UserCheck } from "lucide-react";

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
import { useRBAC } from "@/hooks/use-rbac";
import { ApiError } from "@/lib/api-client";
import { permissions } from "@/lib/permissions";
import { contractsKeys, generateContract, getContractPreflight } from "@/services/hris-contracts";
import { employeesKeys, getEmployee, patchEmployeeFields } from "@/services/hris-employees";
import { toast } from "@/stores/toast-store";
import type { ContractDetail, ContractMissingField, ContractPreflight } from "@/types/contracts";

const scopeLabels: Record<string, { title: string; hint: string }> = {
  company: { title: "Profil perusahaan", hint: "Admin > Pengaturan > Profil Perusahaan" },
  identity: { title: "Data identitas karyawan", hint: "Edit kontrak > Data Legal (izin ubah data identitas)" },
  employee: { title: "Data karyawan", hint: "Edit kontrak > Data Legal, atau halaman karyawan" },
  contract: { title: "Isian kontrak", hint: "Edit kontrak ini" },
};

const scopeOrder = ["contract", "identity", "employee", "company"];

function groupMissing(missing: ContractMissingField[]) {
  const groups = new Map<string, ContractMissingField[]>();
  for (const item of missing) {
    groups.set(item.scope, [...(groups.get(item.scope) ?? []), item]);
  }
  const rank = (scope: string) => {
    const index = scopeOrder.indexOf(scope);
    return index === -1 ? scopeOrder.length : index;
  };
  return [...groups.entries()].sort(([left], [right]) => rank(left) - rank(right));
}

export function usePreflight(contractId: string, enabled: boolean) {
  return useQuery({
    queryKey: contractsKeys.preflight(contractId),
    queryFn: () => getContractPreflight(contractId),
    enabled,
    staleTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

/**
 * What blocks Generate (missing fields by where they are fixed) and the
 * warnings that do not: an employee still on Probation (offers to set
 * Active), a PKWT chain above five years, a duration that is not whole
 * months.
 */
export function PreflightSummary({ preflight, employeeId }: { preflight: ContractPreflight; employeeId: string }) {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canOpenSettings = hasPermission(permissions.adminSettingsView);
  const canEditEmployee = hasPermission(permissions.hrisEmployeeEdit);

  const activateMutation = useMutation({
    mutationFn: async (employeeId: string) => {
      const employee = await getEmployee(employeeId);
      return patchEmployeeFields(employee, { employment_status: "active" });
    },
    onSuccess: async (saved) => {
      queryClient.setQueryData(employeesKeys.detail(saved.id), saved);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: employeesKeys.all }),
        queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "preflight"] }),
        queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "detail"] }),
      ]);
      toast.success(`Status ${saved.full_name} kini Active`);
    },
    onError: (error) => {
      toast.error("Gagal mengubah status karyawan", error instanceof Error ? error.message : undefined);
    },
  });

  return (
    <div className="space-y-3" data-testid="contract-preflight">
      {preflight.missing.length === 0 ? (
        <p className="flex items-center gap-2 rounded-xl bg-success-light px-3 py-2 text-[13px] font-semibold text-success">
          <CircleCheck className="h-4 w-4" />
          Semua data yang dicetak di PKWT dan NDA sudah lengkap.
        </p>
      ) : (
        <div className="space-y-2">
          <p className="flex items-center gap-2 text-[13px] font-semibold text-error">
            <CircleAlert className="h-4 w-4" />
            {preflight.missing.length} data belum lengkap. Generate baru bisa dilakukan setelah semuanya terisi.
          </p>
          {groupMissing(preflight.missing).map(([scope, items]) => (
            <div className="rounded-xl border border-border/70 px-3 py-2.5" data-scope={scope} key={scope}>
              <p className="text-[12px] font-[700] uppercase tracking-[0.08em] text-text-secondary">
                {scopeLabels[scope]?.title ?? scope}
              </p>
              <ul className="mt-1.5 flex flex-wrap gap-1.5">
                {items.map((item) => (
                  <li
                    className="rounded-full bg-error-light px-2 py-0.5 text-[12px] font-semibold text-error"
                    data-field={`${item.scope}.${item.field}`}
                    key={item.field}
                  >
                    {item.label}
                  </li>
                ))}
              </ul>
              <p className="mt-1.5 text-[12px] text-text-tertiary">
                {scope === "company" && canOpenSettings ? (
                  <Link className="font-semibold underline" to="/admin/settings">
                    Buka Profil Perusahaan
                  </Link>
                ) : (
                  `Dilengkapi di: ${scopeLabels[scope]?.hint ?? "-"}`
                )}
              </p>
            </div>
          ))}
        </div>
      )}

      {preflight.warnings.length > 0 ? (
        <ul className="space-y-2">
          {preflight.warnings.map((warning) => (
            <li
              className="flex flex-col gap-2 rounded-xl bg-warning-light px-3 py-2 text-[13px] text-warning sm:flex-row sm:items-start sm:justify-between"
              data-code={warning.code}
              key={warning.code}
            >
              <span className="flex items-start gap-2">
                <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
                {warning.message}
              </span>
              {warning.action === "set_employee_active" && canEditEmployee ? (
                <Button
                  className="shrink-0"
                  disabled={activateMutation.isPending}
                  onClick={() => activateMutation.mutate(employeeId)}
                  size="xs"
                  type="button"
                  variant="outline"
                >
                  <UserCheck className="h-4 w-4" />
                  {activateMutation.isPending ? "Menyimpan..." : "Jadikan Active"}
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}

      {preflight.chain_pkwt_months > 0 ? (
        <p className="text-[12px] text-text-secondary">
          Total PKWT berturut-turut (termasuk perpanjangan): {preflight.chain_pkwt_months} bulan dari batas 60 bulan.
        </p>
      ) : null}
    </div>
  );
}

/**
 * Generate: shows the preflight first, then assigns the numbers (first time)
 * and queues the PKWT + NDA render. The detail page polls until the PDFs
 * are ready.
 */
export function GenerateContractDialog({
  contract,
  open,
  onClose,
}: {
  contract: ContractDetail;
  open: boolean;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const preflightQuery = usePreflight(contract.id, open);
  const preflight = preflightQuery.data;

  const mutation = useMutation({
    mutationFn: () => generateContract(contract.id),
    onSuccess: async (saved) => {
      queryClient.setQueryData(contractsKeys.detail(saved.id), saved);
      await queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "list"] });
      toast.success(
        contract.doc_number ? "Dokumen dibuat ulang" : `Nomor ${saved.doc_number} diterbitkan`,
        saved.render_status === "pending" || saved.render_status === "rendering"
          ? "PDF PKWT dan NDA sedang dibuat (sekitar 10-20 detik)."
          : undefined,
      );
      onClose();
    },
    onError: async (error) => {
      if (error instanceof ApiError && error.code === "CONTRACT_INCOMPLETE") {
        await preflightQuery.refetch();
      }
      toast.error("Generate gagal", error instanceof Error ? error.message : undefined);
    },
  });

  const regenerate = Boolean(contract.doc_number);
  return (
    <Dialog dismissible={!mutation.isPending} onOpenChange={(next) => (!next ? onClose() : undefined)} open={open}>
      <DialogContent size="lg">
        <DialogHeader className="flex items-start justify-between gap-4">
          <div>
            <DialogTitle>{regenerate ? "Generate ulang PKWT & NDA" : "Generate PKWT & NDA"}</DialogTitle>
            <DialogDescription>
              {regenerate
                ? `Nomor ${contract.doc_number} dan ${contract.nda_doc_number} tetap; PDF dibuat ulang dari data terbaru.`
                : "Nomor PKWT dan NDA/HKI diterbitkan sekali dari penomoran bulanan, lalu PDF dibuat di latar belakang."}
            </DialogDescription>
          </div>
          <DialogClose disabled={mutation.isPending} />
        </DialogHeader>
        <DialogBody>
          {preflightQuery.isLoading ? (
            <p className="flex items-center gap-2 text-sm text-text-secondary">
              <LoaderCircle className="h-4 w-4 animate-spin" />
              Memeriksa kelengkapan data...
            </p>
          ) : preflightQuery.error instanceof Error ? (
            <p className="text-sm text-error">{preflightQuery.error.message}</p>
          ) : preflight ? (
            <PreflightSummary employeeId={contract.employee_id} preflight={preflight} />
          ) : null}
          {!contract.pdf_available ? (
            <p className="mt-3 rounded-xl bg-warning-light px-3 py-2 text-[13px] text-warning">
              Konverter PDF belum tersedia di server: dokumen hanya dapat diunduh sebagai DOCX.
            </p>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button disabled={mutation.isPending} onClick={onClose} type="button" variant="ghost">
            Batal
          </Button>
          <Button
            data-testid="contract-generate-confirm"
            disabled={mutation.isPending || !preflight?.ready}
            onClick={() => mutation.mutate()}
            type="button"
          >
            <FileCog className="h-4 w-4" />
            {mutation.isPending ? "Memproses..." : regenerate ? "Generate ulang" : "Generate"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

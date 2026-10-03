import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { z } from "zod";

import { ContractForm } from "@/components/contracts/contract-form";
import { Card } from "@/components/ui/card";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";

const searchSchema = z.object({
  employee: z.string().max(64).optional().catch(undefined),
});

export const Route = createFileRoute("/_authenticated/hris/contracts/new")({
  validateSearch: searchSchema,
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisContractManage);
  },
  component: NewContractPage,
});

function NewContractPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <Link
          className="mb-3 inline-flex items-center gap-1.5 text-[13px] font-semibold text-text-secondary hover:text-text-primary"
          search={search.employee ? { employee: search.employee } : {}}
          to="/hris/contracts"
        >
          <ArrowLeft className="h-4 w-4" />
          Kontrak Kerja
        </Link>
        <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">HRIS kontrak kerja</p>
        <h3 className="text-[28px] font-[700] text-text-primary">Buat Kontrak</h3>
        <p className="mt-2 max-w-3xl text-[14px] leading-relaxed text-text-secondary">
          Pilih karyawan dan jenis kontrak. Untuk PKWT, lengkapi Data Legal dan isian kontrak, simpan sebagai draf, lalu
          Generate untuk menerbitkan nomor dan PDF PKWT serta NDA &amp; HKI. PKWTT dan magang hanya dicatat.
        </p>
      </Card>

      <ContractForm
        initialEmployeeId={search.employee ?? ""}
        mode="create"
        onCancel={() =>
          void navigate({ to: "/hris/contracts", search: search.employee ? { employee: search.employee } : {} })
        }
        onSaved={(saved) => void navigate({ to: "/hris/contracts/$contractId", params: { contractId: saved.id } })}
      />
    </div>
  );
}

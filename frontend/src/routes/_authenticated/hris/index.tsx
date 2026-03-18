import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";

import { Card } from "@/components/ui/card";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { fetchModuleOverview } from "@/services/foundation";

export const Route = createFileRoute("/_authenticated/hris/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.hrisOverview);
  },
  component: HrisPage,
});

function HrisPage() {
  const overviewQuery = useQuery({
    queryKey: ["hris", "overview", "page"],
    queryFn: () => fetchModuleOverview("hris"),
  });

  return (
    <div className="space-y-6">
      <Card className="p-8 border-none bg-gradient-to-br from-hr/5 to-surface shadow-md">
        <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-hr mb-2">HRIS</p>
        <h3 className="mt-2 text-[32px] font-[700] leading-tight text-text-primary">People operations workspace</h3>
        <p className="mt-4 max-w-2xl text-[14px] text-text-secondary leading-relaxed">
          HRIS sekarang punya fondasi data karyawan dan struktur department. Salary,
          finance, subscription, dan reimbursement akan dibangun di atas struktur ini.
        </p>
        <div className="mt-6 rounded-[12px] border border-border bg-surface-muted p-4 text-[13px] font-[500] text-text-tertiary">
          {overviewQuery.isLoading
            ? "Memuat protected overview..."
            : overviewQuery.error instanceof Error
              ? overviewQuery.error.message
              : overviewQuery.data?.message}
        </div>
      </Card>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card className="p-6 flex flex-col items-start border border-border bg-surface shadow-sm transition-all hover:border-hr/30">
          <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary mb-1">Employee directory</p>
          <h4 className="text-[20px] font-[700] text-text-primary">Manage employees</h4>
          <p className="mt-2 text-[13px] text-text-secondary leading-relaxed flex-1">
            Buat profil karyawan, cari berdasarkan nama, department, dan status kerja.
          </p>
          <Link
            className="mt-6 inline-flex h-[44px] items-center justify-center rounded-[6px] bg-hr px-5 text-[14px] font-[600] text-white transition hover:opacity-90 shadow-sm"
            to="/hris/employees"
          >
            Open employees
          </Link>
        </Card>

        <Card className="p-6 flex flex-col items-start border border-border bg-surface shadow-sm transition-all hover:border-hr/30">
          <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary mb-1">Organization</p>
          <h4 className="text-[20px] font-[700] text-text-primary">Manage departments</h4>
          <p className="mt-2 text-[13px] text-text-secondary leading-relaxed flex-1">
            Atur struktur department, deskripsi fungsi, dan penunjukan head untuk tiap team.
          </p>
          <Link
            className="mt-6 inline-flex h-[44px] items-center justify-center rounded-[6px] border border-border bg-surface-muted px-5 text-[14px] font-[600] text-text-primary transition hover:bg-border/50 shadow-sm"
            to="/hris/departments"
          >
            Open departments
          </Link>
        </Card>

        <Card className="p-6 flex flex-col items-start border border-border bg-surface shadow-sm transition-all hover:border-hr/30">
          <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary mb-1">Finance</p>
          <h4 className="text-[20px] font-[700] text-text-primary">Income and outcome</h4>
          <p className="mt-2 text-[13px] text-text-secondary leading-relaxed flex-1">
            Lihat record bulanan, approval flow, dashboard tren, dan export CSV.
          </p>
          <Link
            className="mt-6 inline-flex h-[44px] items-center justify-center rounded-[6px] border border-border bg-surface-muted px-5 text-[14px] font-[600] text-text-primary transition hover:bg-border/50 shadow-sm"
            to="/hris/finance"
          >
            Open finance
          </Link>
        </Card>

        <Card className="p-6 flex flex-col items-start border border-border bg-surface shadow-sm transition-all hover:border-hr/30">
          <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary mb-1">Reimbursements</p>
          <h4 className="text-[20px] font-[700] text-text-primary">Claims and approval</h4>
          <p className="mt-2 text-[13px] text-text-secondary leading-relaxed flex-1">
            Submit reimbursement, upload bukti, dan pantau status manager review hingga payout.
          </p>
          <Link
            className="mt-6 inline-flex h-[44px] items-center justify-center rounded-[6px] border border-border bg-surface-muted px-5 text-[14px] font-[600] text-text-primary transition hover:bg-border/50 shadow-sm"
            to="/hris/reimbursements"
          >
            Open reimbursements
          </Link>
        </Card>
      </div>
    </div>
  );
}

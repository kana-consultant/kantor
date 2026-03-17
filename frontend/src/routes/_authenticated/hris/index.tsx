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
      <Card className="p-8">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">HRIS</p>
        <h3 className="mt-3 text-3xl font-bold">People operations workspace</h3>
        <p className="mt-3 max-w-2xl text-muted-foreground">
          HRIS sekarang punya fondasi data karyawan dan struktur department. Salary,
          finance, subscription, dan reimbursement akan dibangun di atas struktur ini.
        </p>
        <div className="mt-6 rounded-3xl border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
          {overviewQuery.isLoading
            ? "Memuat protected overview..."
            : overviewQuery.error instanceof Error
              ? overviewQuery.error.message
              : overviewQuery.data?.message}
        </div>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="p-6">
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Employee directory</p>
          <h4 className="mt-2 text-2xl font-bold">Manage employees</h4>
          <p className="mt-2 text-sm text-muted-foreground">
            Buat profil karyawan, cari berdasarkan nama, department, dan status kerja.
          </p>
          <Link
            className="mt-5 inline-flex h-11 items-center justify-center rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition hover:opacity-95"
            to="/hris/employees"
          >
            Open employees
          </Link>
        </Card>

        <Card className="p-6">
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Organization</p>
          <h4 className="mt-2 text-2xl font-bold">Manage departments</h4>
          <p className="mt-2 text-sm text-muted-foreground">
            Atur struktur department, deskripsi fungsi, dan penunjukan head untuk tiap team.
          </p>
          <Link
            className="mt-5 inline-flex h-11 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted"
            to="/hris/departments"
          >
            Open departments
          </Link>
        </Card>

        <Card className="p-6">
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Finance</p>
          <h4 className="mt-2 text-2xl font-bold">Income and outcome</h4>
          <p className="mt-2 text-sm text-muted-foreground">
            Lihat record bulanan, approval flow, dashboard tren, dan export CSV.
          </p>
          <Link
            className="mt-5 inline-flex h-11 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted"
            to="/hris/finance"
          >
            Open finance
          </Link>
        </Card>

        <Card className="p-6">
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Reimbursements</p>
          <h4 className="mt-2 text-2xl font-bold">Claims and approval</h4>
          <p className="mt-2 text-sm text-muted-foreground">
            Submit reimbursement, upload bukti, dan pantau status manager review hingga payout.
          </p>
          <Link
            className="mt-5 inline-flex h-11 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted"
            to="/hris/reimbursements"
          >
            Open reimbursements
          </Link>
        </Card>
      </div>
    </div>
  );
}

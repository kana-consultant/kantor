import { useMemo, useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { formatIDR } from "@/lib/currency";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { employeesKeys, listEmployees } from "@/services/hris-employees";
import {
  createReimbursement,
  getReimbursementSummary,
  listReimbursements,
  reimbursementsKeys,
  uploadReimbursementAttachments,
} from "@/services/hris-reimbursements";
import type { ReimbursementFilters, ReimbursementFormValues } from "@/types/hris";

const reimbursementSchema = z.object({
  employee_id: z.string().min(1),
  title: z.string().min(2).max(200),
  category: z.string().min(2).max(120),
  amount: z.coerce.number().min(0),
  transaction_date: z.string().min(1),
  description: z.string().min(2).max(2000),
});

const defaultFilters: ReimbursementFilters = {
  page: 1,
  perPage: 20,
  status: "",
  employee: "",
  month: String(new Date().getMonth() + 1),
  year: String(new Date().getFullYear()),
};

export const Route = createFileRoute("/_authenticated/hris/reimbursements/")({
  beforeLoad: async () => {
    await ensurePermission(permissions.hrisReimbursementView);
  },
  component: ReimbursementsPage,
});

function ReimbursementsPage() {
  const queryClient = useQueryClient();
  const { hasPermission, hasRole } = useRBAC();
  const [filters, setFilters] = useState<ReimbursementFilters>(defaultFilters);
  const [showForm, setShowForm] = useState(false);
  const [files, setFiles] = useState<File[]>([]);

  const form = useForm<ReimbursementFormValues>({
    resolver: zodResolver(reimbursementSchema),
    defaultValues: {
      employee_id: "",
      title: "",
      category: "",
      amount: 0,
      transaction_date: new Date().toISOString().slice(0, 10),
      description: "",
    },
  });

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list({ page: 1, perPage: 100, search: "", department: "", status: "" }),
    queryFn: () => listEmployees({ page: 1, perPage: 100, search: "", department: "", status: "" }),
  });

  const reimbursementsQuery = useQuery({
    queryKey: reimbursementsKeys.list(filters),
    queryFn: () => listReimbursements(filters),
  });

  const summaryQuery = useQuery({
    queryKey: reimbursementsKeys.summary(filters.month, filters.year),
    queryFn: () => getReimbursementSummary(filters.month, filters.year),
  });

  const createMutation = useMutation({
    mutationFn: async (values: ReimbursementFormValues) => {
      const created = await createReimbursement(values);
      if (files.length > 0) {
        await uploadReimbursementAttachments(created.id, files);
      }
      return created;
    },
    onSuccess: async () => {
      form.reset({
        employee_id: "",
        title: "",
        category: "",
        amount: 0,
        transaction_date: new Date().toISOString().slice(0, 10),
        description: "",
      });
      setFiles([]);
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: reimbursementsKeys.all });
    },
  });

  const employees = employeesQuery.data?.items ?? [];
  const canReview = hasPermission(permissions.hrisReimbursementApprove);
  const canMarkPaid = hasRole("manager", "hris") || hasRole("admin", "hris") || hasRole("super_admin");

  const handleFiles = (incomingFiles: FileList | File[]) => {
    const nextFiles = Array.from(incomingFiles).filter((file) => {
      const validType = file.type.startsWith("image/") || file.type === "application/pdf";
      const validSize = file.size <= 10 * 1024 * 1024;
      return validType && validSize;
    });
    setFiles(nextFiles);
  };

  const fileSummary = useMemo(
    () => files.map((file) => `${file.name} (${Math.round(file.size / 1024)} KB)`),
    [files],
  );

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">HRIS reimbursements</p>
            <h3 className="mt-2 text-3xl font-bold">Reimbursement workflow</h3>
            <p className="mt-2 max-w-3xl text-muted-foreground">
              Submit claim, upload bukti gambar atau PDF, dan pantau status sampai dibayar.
            </p>
          </div>
          <PermissionGate permission={permissions.hrisReimbursementCreate}>
            <Button onClick={() => setShowForm((value) => !value)}>
              {showForm ? "Close form" : "Submit reimbursement"}
            </Button>
          </PermissionGate>
        </div>
      </Card>

      <div className="grid gap-4 lg:grid-cols-3">
        <SummaryCard label="Awaiting review" value={String(summaryQuery.data?.counts_by_status?.submitted ?? 0)} />
        <SummaryCard label="Approved requests" value={String(summaryQuery.data?.counts_by_status?.approved ?? 0)} />
        <SummaryCard label="Approved this month" value={formatIDR(summaryQuery.data?.approved_amount_month ?? 0)} />
      </div>

      <Card className="p-6">
        <div className="grid gap-3 lg:grid-cols-5">
          <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((prev) => ({ ...prev, status: event.target.value, page: 1 }))} value={filters.status}>
            <option value="">All statuses</option>
            <option value="submitted">Submitted</option>
            <option value="approved">Approved</option>
            <option value="rejected">Rejected</option>
            <option value="paid">Paid</option>
          </select>
          <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((prev) => ({ ...prev, employee: event.target.value, page: 1 }))} value={filters.employee}>
            <option value="">All employees</option>
            {employees.map((employee) => (
              <option key={employee.id} value={employee.id}>
                {employee.full_name}
              </option>
            ))}
          </select>
          <Input onChange={(event) => setFilters((prev) => ({ ...prev, month: event.target.value, page: 1 }))} placeholder="Month" type="number" value={filters.month} />
          <Input onChange={(event) => setFilters((prev) => ({ ...prev, year: event.target.value, page: 1 }))} placeholder="Year" type="number" value={filters.year} />
          <div className="rounded-2xl border border-dashed border-border/70 bg-background/70 px-4 py-3 text-sm text-muted-foreground">
            {canMarkPaid ? "Approval and payout access active" : canReview ? "Approval access active" : "Self-service mode"}
          </div>
        </div>
      </Card>

      {showForm ? (
        <Card className="p-6">
          <div className="mb-4">
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">New reimbursement</p>
            <h4 className="mt-2 text-2xl font-bold">Submit claim</h4>
          </div>
          <form className="grid gap-4 lg:grid-cols-2" onSubmit={form.handleSubmit((values) => createMutation.mutate(values))}>
            <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...form.register("employee_id")}>
              <option value="">Select employee</option>
              {employees.map((employee) => (
                <option key={employee.id} value={employee.id}>
                  {employee.full_name}
                </option>
              ))}
            </select>
            <Input {...form.register("category")} placeholder="Category" />
            <Input {...form.register("title")} placeholder="Title" />
            <Controller
              control={form.control}
              name="amount"
              render={({ field }) => (
                <CurrencyInput
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  ref={field.ref}
                  value={field.value}
                />
              )}
            />
            <Input {...form.register("transaction_date")} type="date" />
            <Input className="lg:col-span-2" {...form.register("description")} placeholder="Description" />
            <div
              className="lg:col-span-2 rounded-[24px] border border-dashed border-border/70 bg-background/70 p-6"
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault();
                handleFiles(event.dataTransfer.files);
              }}
            >
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <p className="text-sm font-semibold">Attachment drop zone</p>
                  <p className="text-xs text-muted-foreground">
                    Drag-and-drop file gambar atau PDF. Maksimum 10MB per file.
                  </p>
                </div>
                <label className="inline-flex h-11 cursor-pointer items-center justify-center rounded-full border border-border bg-card px-5 text-sm font-medium">
                  Choose files
                  <input
                    className="hidden"
                    multiple
                    onChange={(event) => handleFiles(event.target.files ?? [])}
                    type="file"
                  />
                </label>
              </div>
              {fileSummary.length > 0 ? (
                <div className="mt-4 space-y-2">
                  {fileSummary.map((item) => (
                    <div className="rounded-[18px] border border-border/60 bg-card/70 px-4 py-3 text-sm" key={item}>
                      {item}
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
            <div className="lg:col-span-2 flex flex-wrap gap-3">
              <Button disabled={createMutation.isPending} type="submit">
                Submit claim
              </Button>
              <Button onClick={() => setShowForm(false)} type="button" variant="ghost">
                Cancel
              </Button>
            </div>
          </form>
        </Card>
      ) : null}

      <div className="grid gap-4">
        {(reimbursementsQuery.data?.items ?? []).map((item) => (
          <Card className="p-6" key={item.id}>
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">{item.employee_name}</p>
                <h4 className="mt-2 text-xl font-bold">{item.title}</h4>
                <p className="mt-2 text-sm text-muted-foreground">
                  {item.category} - {new Date(item.transaction_date).toLocaleDateString("id-ID")}
                </p>
              </div>
              <div className="text-right">
                <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Amount</p>
                <p className="mt-2 text-2xl font-bold">{formatIDR(item.amount)}</p>
                <span className={`mt-3 inline-flex rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] ${statusTone(item.status)}`}>
                  {item.status}
                </span>
              </div>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">{item.description}</p>
            <div className="mt-5 flex flex-wrap gap-3">
              <Link className="inline-flex h-10 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted" params={{ reimbursementId: item.id }} to="/hris/reimbursements/$reimbursementId">
                Open detail
              </Link>
              <div className="inline-flex h-10 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm text-muted-foreground">
                {item.attachments.length} attachment(s)
              </div>
            </div>
          </Card>
        ))}
      </div>
    </div>
  );
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-6">
      <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <h4 className="mt-3 text-2xl font-bold">{value}</h4>
    </Card>
  );
}

function statusTone(status: string) {
  switch (status) {
    case "paid":
      return "bg-sky-100 text-sky-700";
    case "approved":
      return "bg-emerald-100 text-emerald-700";
    case "rejected":
      return "bg-red-100 text-red-700";
    default:
      return "bg-amber-100 text-amber-700";
  }
}

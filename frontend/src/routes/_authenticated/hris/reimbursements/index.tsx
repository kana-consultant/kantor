import { useMemo, useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Controller, useForm } from "react-hook-form";
import { CircleDollarSign, Plus, Receipt, TimerReset } from "lucide-react";
import { z } from "zod";

import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
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
import type { Reimbursement, ReimbursementFilters, ReimbursementFormValues } from "@/types/hris";

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
    resolver: zodResolver(reimbursementSchema) as never,
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
  const meta = reimbursementsQuery.data?.meta;
  const canReview = hasPermission(permissions.hrisReimbursementApprove);
  const canMarkPaid = hasRole("manager", "hris") || hasRole("admin", "hris") || hasRole("super_admin");
  const reimbursements = reimbursementsQuery.data?.items ?? [];

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

  const columns: Array<DataTableColumn<Reimbursement>> = [
    {
      id: "title",
      header: "Request",
      accessor: "title",
      sortable: true,
      cell: (item) => (
        <div className="space-y-1">
          <p className="font-semibold text-text-primary">{item.title}</p>
          <p className="text-[13px] text-text-secondary">{item.employee_name}</p>
        </div>
      ),
    },
    {
      id: "category",
      header: "Category",
      accessor: "category",
      sortable: true,
      cell: (item) => (
        <div className="space-y-1">
          <p className="text-sm text-text-primary">{item.category}</p>
          <p className="text-[13px] text-text-secondary">
            {new Date(item.transaction_date).toLocaleDateString("id-ID")}
          </p>
        </div>
      ),
    },
    {
      id: "amount",
      header: "Amount",
      accessor: "amount",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (item) => <span className="font-mono tabular-nums">{formatIDR(item.amount)}</span>,
    },
    {
      id: "status",
      header: "Status",
      accessor: "status",
      sortable: true,
      cell: (item) => <StatusBadge status={item.status} variant="reimbursement-status" />,
    },
    {
      id: "attachments",
      header: "Attachments",
      accessor: "attachments",
      align: "right",
      cell: (item) => <span className="font-mono tabular-nums text-text-secondary">{item.attachments.length}</span>,
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (item) => (
        <Link
          className="inline-flex h-9 items-center justify-center rounded-md bg-module px-4 text-sm font-semibold text-white transition hover:brightness-95"
          params={{ reimbursementId: item.id }}
          to="/hris/reimbursements/$reimbursementId"
        >
          Open detail
        </Link>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 border-b border-border pb-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">
              HRIS reimbursements
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">Reimbursement workflow</h3>
            <p className="mt-2 max-w-3xl text-[14px] leading-relaxed text-text-secondary">
              Submit claims, attach supporting files, and track approval through payout.
            </p>
          </div>
          <PermissionGate permission={permissions.hrisReimbursementCreate}>
            <Button onClick={() => setShowForm(true)} type="button">
              <Plus className="h-4 w-4" />
              Submit reimbursement
            </Button>
          </PermissionGate>
        </div>
      </Card>

      <div className="grid gap-4 lg:grid-cols-3">
        <StatCard
          helper="Requests waiting for approval"
          icon={TimerReset}
          label="Awaiting review"
          tone="warning"
          value={String(summaryQuery.data?.counts_by_status?.submitted ?? 0)}
        />
        <StatCard
          helper="Requests already approved"
          icon={Receipt}
          label="Approved requests"
          tone="success"
          value={String(summaryQuery.data?.counts_by_status?.approved ?? 0)}
        />
        <StatCard
          helper={canMarkPaid ? "Approval and payout access active" : canReview ? "Approval access active" : "Self-service mode"}
          icon={CircleDollarSign}
          label="Approved this month"
          mono
          tone="hr"
          value={formatIDR(summaryQuery.data?.approved_amount_month ?? 0)}
        />
      </div>

      <Card className="p-6">
        <div className="grid gap-3 lg:grid-cols-5">
          <select
            className="field-select"
            onChange={(event) => setFilters((prev) => ({ ...prev, status: event.target.value, page: 1 }))}
            value={filters.status}
          >
            <option value="">All statuses</option>
            <option value="submitted">Submitted</option>
            <option value="approved">Approved</option>
            <option value="rejected">Rejected</option>
            <option value="paid">Paid</option>
          </select>
          <select
            className="field-select"
            onChange={(event) => setFilters((prev) => ({ ...prev, employee: event.target.value, page: 1 }))}
            value={filters.employee}
          >
            <option value="">All employees</option>
            {employees.map((employee) => (
              <option key={employee.id} value={employee.id}>
                {employee.full_name}
              </option>
            ))}
          </select>
          <Input onChange={(event) => setFilters((prev) => ({ ...prev, month: event.target.value, page: 1 }))} placeholder="Month" type="number" value={filters.month} />
          <Input onChange={(event) => setFilters((prev) => ({ ...prev, year: event.target.value, page: 1 }))} placeholder="Year" type="number" value={filters.year} />
          <select
            className="field-select"
            onChange={(event) => setFilters((prev) => ({ ...prev, perPage: Number(event.target.value), page: 1 }))}
            value={filters.perPage}
          >
            <option value={10}>10 per page</option>
            <option value={20}>20 per page</option>
            <option value={50}>50 per page</option>
          </select>
        </div>
      </Card>

      <FormModal
        isLoading={createMutation.isPending}
        isOpen={showForm}
        onClose={() => setShowForm(false)}
        onSubmit={form.handleSubmit((values) => createMutation.mutate(values))}
        size="lg"
        submitLabel="Submit claim"
        title="Submit reimbursement"
        subtitle="Capture the expense detail, transaction date, and supporting evidence in one focused dialog."
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <select className="field-select" {...form.register("employee_id")}>
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
            className="lg:col-span-2 rounded-md border border-dashed border-border bg-surface-muted p-6"
            onDragOver={(event) => event.preventDefault()}
            onDrop={(event) => {
              event.preventDefault();
              handleFiles(event.dataTransfer.files);
            }}
          >
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <p className="font-semibold text-text-primary">Attachment drop zone</p>
                <p className="text-xs text-text-secondary">
                  Drag image or PDF files here. Maximum 10MB per file.
                </p>
              </div>
              <label className="inline-flex h-10 cursor-pointer items-center justify-center rounded-md border border-border bg-surface px-4 text-sm font-semibold text-text-primary transition hover:bg-surface-muted">
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
                  <div className="rounded-md border border-border bg-surface px-4 py-3 text-sm text-text-secondary" key={item}>
                    {item}
                  </div>
                ))}
              </div>
            ) : null}
          </div>
        </div>
      </FormModal>

      {reimbursementsQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-error">{reimbursementsQuery.error.message}</Card>
      ) : null}

      <DataTable
        columns={columns}
        data={reimbursements}
        emptyActionLabel={hasPermission(permissions.hrisReimbursementCreate) ? "Submit reimbursement" : undefined}
        emptyDescription="No reimbursement requests match the current filter."
        emptyTitle="No reimbursements found"
        getRowId={(item) => item.id}
        loading={reimbursementsQuery.isLoading}
        loadingRows={6}
        onEmptyAction={hasPermission(permissions.hrisReimbursementCreate) ? () => setShowForm(true) : undefined}
        pagination={
          meta
            ? {
                page: meta.page,
                perPage: meta.per_page,
                total: meta.total,
                onPageChange: (page) => setFilters((current) => ({ ...current, page })),
              }
            : undefined
        }
      />
    </div>
  );
}

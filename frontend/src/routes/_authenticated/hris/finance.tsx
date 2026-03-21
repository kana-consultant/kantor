import { useMemo, useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Controller, useForm } from "react-hook-form";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { z } from "zod";
import { ArrowDownCircle, ArrowUpCircle, Plus, Scale } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { ExportButton } from "@/components/shared/export-button";
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
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  createFinanceCategory,
  createFinanceRecord,
  deleteFinanceCategory,
  deleteFinanceRecord,
  financeKeys,
  getFinanceSummary,
  listFinanceCategories,
  listFinanceRecords,
  reviewFinanceRecord,
  submitFinanceRecord,
  updateFinanceCategory,
  updateFinanceRecord,
} from "@/services/hris-finance";
import type {
  FinanceCategory,
  FinanceCategoryFormValues,
  FinanceRecord,
  FinanceRecordFilters,
  FinanceRecordFormValues,
} from "@/types/hris";

const recordSchema = z.object({
  category_id: z.string().min(1),
  type: z.enum(["income", "outcome"]),
  amount: z.coerce.number().min(0),
  description: z.string().min(2).max(2000),
  record_date: z.string().min(1),
});

const categorySchema = z.object({
  name: z.string().min(2).max(120),
  type: z.enum(["income", "outcome"]),
});

const defaultFilters: FinanceRecordFilters = {
  page: 1,
  perPage: 20,
  type: "",
  category: "",
  month: "",
  year: String(new Date().getFullYear()),
  status: "",
};

const pieColors = ["#36B37E", "#FF5630", "#0065FF", "#6554C0", "#FF8B00", "#00B8D9"];

export const Route = createFileRoute("/_authenticated/hris/finance")({
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisFinanceView);
  },
  component: FinancePage,
});

function FinancePage() {
  const queryClient = useQueryClient();
  const { hasPermission, hasRole } = useRBAC();
  const canManageCategories = hasRole("admin", "hris") || hasRole("super_admin");
  const [activeTab, setActiveTab] = useState<"records" | "dashboard">("records");
  const [filters, setFilters] = useState<FinanceRecordFilters>(defaultFilters);
  const [showRecordForm, setShowRecordForm] = useState(false);
  const [showCategoryForm, setShowCategoryForm] = useState(false);
  const [editingRecord, setEditingRecord] = useState<FinanceRecord | null>(null);
  const [editingCategory, setEditingCategory] = useState<FinanceCategory | null>(null);
  const [recordToDelete, setRecordToDelete] = useState<FinanceRecord | null>(null);
  const [categoryToDelete, setCategoryToDelete] = useState<FinanceCategory | null>(null);
  const [recordToReject, setRecordToReject] = useState<FinanceRecord | null>(null);

  const categoriesQuery = useQuery({
    queryKey: financeKeys.categories(),
    queryFn: () => listFinanceCategories(),
  });

  const recordsQuery = useQuery({
    queryKey: financeKeys.records(filters),
    queryFn: () => listFinanceRecords(filters),
  });

  const summaryYear = Number(filters.year || new Date().getFullYear());
  const summaryQuery = useQuery({
    queryKey: financeKeys.summary(summaryYear),
    queryFn: () => getFinanceSummary(summaryYear),
  });

  const recordForm = useForm<FinanceRecordFormValues>({
    resolver: zodResolver(recordSchema) as never,
    defaultValues: {
      category_id: "",
      type: "income",
      amount: 0,
      description: "",
      record_date: new Date().toISOString().slice(0, 10),
    },
  });

  const categoryForm = useForm<FinanceCategoryFormValues>({
    resolver: zodResolver(categorySchema),
    defaultValues: {
      name: "",
      type: "income",
    },
  });

  const createRecordMutation = useMutation({
    mutationFn: createFinanceRecord,
    onSuccess: async () => {
      resetRecordForm(recordForm);
      setShowRecordForm(false);
      setEditingRecord(null);
      await invalidateFinance(queryClient, summaryYear);
    },
  });

  const updateRecordMutation = useMutation({
    mutationFn: (payload: { recordId: string; values: FinanceRecordFormValues }) =>
      updateFinanceRecord(payload.recordId, payload.values),
    onSuccess: async () => {
      resetRecordForm(recordForm);
      setShowRecordForm(false);
      setEditingRecord(null);
      await invalidateFinance(queryClient, summaryYear);
    },
  });

  const deleteRecordMutation = useMutation({
    mutationFn: deleteFinanceRecord,
    onSuccess: async () => {
      setRecordToDelete(null);
      await invalidateFinance(queryClient, summaryYear);
    },
  });

  const submitRecordMutation = useMutation({
    mutationFn: submitFinanceRecord,
    onSuccess: async () => {
      await invalidateFinance(queryClient, summaryYear);
    },
  });

  const reviewRecordMutation = useMutation({
    mutationFn: (payload: { recordId: string; decision: "approved" | "rejected" }) =>
      reviewFinanceRecord(payload.recordId, payload.decision),
    onSuccess: async () => {
      setRecordToReject(null);
      await invalidateFinance(queryClient, summaryYear);
    },
  });

  const createCategoryMutation = useMutation({
    mutationFn: createFinanceCategory,
    onSuccess: async () => {
      categoryForm.reset({ name: "", type: "income" });
      setShowCategoryForm(false);
      await queryClient.invalidateQueries({ queryKey: financeKeys.categories() });
    },
  });

  const updateCategoryMutation = useMutation({
    mutationFn: (payload: { categoryId: string; values: FinanceCategoryFormValues }) =>
      updateFinanceCategory(payload.categoryId, payload.values),
    onSuccess: async () => {
      setEditingCategory(null);
      setShowCategoryForm(false);
      categoryForm.reset({ name: "", type: "income" });
      await queryClient.invalidateQueries({ queryKey: financeKeys.categories() });
    },
  });

  const deleteCategoryMutation = useMutation({
    mutationFn: deleteFinanceCategory,
    onSuccess: async () => {
      setCategoryToDelete(null);
      await queryClient.invalidateQueries({ queryKey: financeKeys.categories() });
    },
  });

  const recordType = recordForm.watch("type");
  const categories = categoriesQuery.data ?? [];
  const records = recordsQuery.data?.items ?? [];
  const meta = recordsQuery.data?.meta;
  const visibleCategories = useMemo(
    () => categories.filter((item) => item.type === recordType),
    [categories, recordType],
  );
  const byCategoryRows = Object.entries(summaryQuery.data?.by_category ?? {});

  const recordColumns: Array<DataTableColumn<FinanceRecord>> = [
    {
      id: "description",
      header: "Record",
      accessor: "description",
      sortable: true,
      cell: (record) => (
        <div className="space-y-1">
          <p className="font-semibold text-text-primary">{record.description}</p>
          <p className="text-[13px] text-text-secondary">{record.category_name}</p>
        </div>
      ),
    },
    {
      id: "type",
      header: "Type",
      accessor: "type",
      sortable: true,
      cell: (record) => (
        <span className={record.type === "income" ? "text-success" : "text-error"}>
          {record.type === "income" ? "Income" : "Outcome"}
        </span>
      ),
    },
    {
      id: "record_date",
      header: "Date",
      accessor: "record_date",
      sortable: true,
      cell: (record) => (
        <span className="text-sm text-text-secondary">
          {new Date(record.record_date).toLocaleDateString("id-ID")}
        </span>
      ),
    },
    {
      id: "amount",
      header: "Amount",
      accessor: "amount",
      numeric: true,
      align: "right",
      sortable: true,
      cell: (record) => (
        <span className={record.type === "income" ? "font-mono tabular-nums text-success" : "font-mono tabular-nums text-error"}>
          {record.type === "income" ? "+" : "-"}
          {formatIDR(record.amount)}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      accessor: "approval_status",
      sortable: true,
      cell: (record) => (
        <StatusBadge status={record.approval_status} variant="finance-status" />
      ),
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (record) => (
        <div className="flex justify-end gap-2">
          {hasPermission(permissions.hrisFinanceEdit) ? (
            <>
              <Button
                onClick={() => {
                  setEditingRecord(record);
                  setShowRecordForm(true);
                  recordForm.reset({
                    category_id: record.category_id,
                    type: record.type,
                    amount: record.amount,
                    description: record.description,
                    record_date: record.record_date.slice(0, 10),
                  });
                }}
                size="sm"
                type="button"
                variant="outline"
              >
                Edit
              </Button>
              <Button
                disabled={deleteRecordMutation.isPending && deleteRecordMutation.variables === record.id}
                onClick={() => setRecordToDelete(record)}
                size="sm"
                type="button"
                variant="ghost"
              >
                Delete
              </Button>
            </>
          ) : null}
          {hasPermission(permissions.hrisFinanceCreate) && record.approval_status === "draft" ? (
            <Button onClick={() => submitRecordMutation.mutate(record.id)} size="sm" type="button" variant="outline">
              Submit
            </Button>
          ) : null}
          {hasPermission(permissions.hrisFinanceApprove) && record.approval_status === "pending_review" ? (
            <>
              <Button
                onClick={() => reviewRecordMutation.mutate({ recordId: record.id, decision: "approved" })}
                size="sm"
                type="button"
              >
                Approve
              </Button>
              <Button
                onClick={() => setRecordToReject(record)}
                size="sm"
                type="button"
                variant="ghost"
              >
                Reject
              </Button>
            </>
          ) : null}
        </div>
      ),
    },
  ];

  const handleRecordSubmit = recordForm.handleSubmit((values) => {
    if (editingRecord) {
      updateRecordMutation.mutate({ recordId: editingRecord.id, values });
      return;
    }
    createRecordMutation.mutate(values);
  });

  const handleCategorySubmit = categoryForm.handleSubmit((values) => {
    if (editingCategory) {
      updateCategoryMutation.mutate({ categoryId: editingCategory.id, values });
      return;
    }
    createCategoryMutation.mutate(values);
  });

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 border-b border-border pb-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-hr">
              HRIS finance
            </p>
            <h3 className="text-[28px] font-[700] text-text-primary">Income and outcome control</h3>
            <p className="mt-2 max-w-3xl text-[14px] leading-relaxed text-text-secondary">
              Maintain monthly records, move entries through approval, and review the yearly trend in one place.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Button onClick={() => setActiveTab("records")} variant={activeTab === "records" ? undefined : "outline"}>
              Records
            </Button>
            <Button onClick={() => setActiveTab("dashboard")} variant={activeTab === "dashboard" ? undefined : "outline"}>
              Dashboard
            </Button>
            <PermissionGate permission={permissions.hrisFinanceView}>
              <ExportButton
                endpoint="/hris/finance/export"
                filename="finance-report"
                filters={{
                  category: filters.category,
                  month: filters.month,
                  status: filters.status,
                  type: filters.type,
                  year: filters.year,
                }}
                formats={["csv", "pdf", "xlsx"]}
              />
            </PermissionGate>
          </div>
        </div>
      </Card>

      {activeTab === "records" ? (
        <div className="space-y-6">
          <Card className="p-6">
            <div className="grid gap-3 lg:grid-cols-6">
              <select
                className="field-select"
                onChange={(event) => setFilters((prev) => ({ ...prev, type: event.target.value, page: 1 }))}
                value={filters.type}
              >
                <option value="">All types</option>
                <option value="income">Income</option>
                <option value="outcome">Outcome</option>
              </select>
              <select
                className="field-select"
                onChange={(event) => setFilters((prev) => ({ ...prev, category: event.target.value, page: 1 }))}
                value={filters.category}
              >
                <option value="">All categories</option>
                {categories.map((category) => (
                  <option key={category.id} value={category.id}>
                    {category.name}
                  </option>
                ))}
              </select>
              <Input onChange={(event) => setFilters((prev) => ({ ...prev, month: event.target.value, page: 1 }))} placeholder="Month" type="number" value={filters.month} />
              <Input onChange={(event) => setFilters((prev) => ({ ...prev, year: event.target.value, page: 1 }))} placeholder="Year" type="number" value={filters.year} />
              <select
                className="field-select"
                onChange={(event) => setFilters((prev) => ({ ...prev, status: event.target.value, page: 1 }))}
                value={filters.status}
              >
                <option value="">All statuses</option>
                <option value="draft">Draft</option>
                <option value="pending_review">Pending review</option>
                <option value="approved">Approved</option>
                <option value="rejected">Rejected</option>
              </select>
            </div>
            <div className="mt-4">
              <PermissionGate permission={permissions.hrisFinanceCreate}>
                <Button
                  onClick={() => {
                    setEditingRecord(null);
                    resetRecordForm(recordForm);
                    setShowRecordForm(true);
                  }}
                  type="button"
                >
                  <Plus className="h-4 w-4" />
                  New record
                </Button>
              </PermissionGate>
            </div>
          </Card>

          {recordsQuery.error instanceof Error ? (
            <Card className="p-6 text-sm text-error">{recordsQuery.error.message}</Card>
          ) : null}

          <DataTable
            columns={recordColumns}
            data={records}
            emptyDescription="No finance records match the current filter."
            emptyTitle="No finance records found"
            getRowId={(record) => record.id}
            loading={recordsQuery.isLoading}
            loadingRows={6}
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

          <PermissionGate fallback={null} permission={permissions.hrisFinanceApprove}>
            {canManageCategories ? (
              <Card className="p-6">
                <div className="mb-4">
                  <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
                    Finance categories
                  </p>
                  <h4 className="text-[20px] font-[700] text-text-primary">Manage categories</h4>
                </div>
                <div className="flex justify-end">
                  <Button
                    onClick={() => {
                      setEditingCategory(null);
                      categoryForm.reset({ name: "", type: "income" });
                      setShowCategoryForm(true);
                    }}
                    type="button"
                  >
                    <Plus className="h-4 w-4" />
                    Add category
                  </Button>
                </div>
                <div className="mt-5 grid gap-3 lg:grid-cols-2">
                  {categories.map((category) => (
                    <div className="rounded-md border border-border bg-surface-muted p-4" key={category.id}>
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        <div>
                          <p className="font-semibold text-text-primary">{category.name}</p>
                          <p className="text-[12px] capitalize text-text-secondary">{category.type}</p>
                        </div>
                        {category.is_default ? (
                          <StatusBadge status="default" />
                        ) : (
                          <div className="flex gap-2">
                            <Button
                              onClick={() => {
                                setEditingCategory(category);
                                categoryForm.reset({ name: category.name, type: category.type });
                                setShowCategoryForm(true);
                              }}
                              size="sm"
                              type="button"
                              variant="outline"
                            >
                              Edit
                            </Button>
                            <Button onClick={() => setCategoryToDelete(category)} size="sm" type="button" variant="ghost">
                              Delete
                            </Button>
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </Card>
            ) : null}
          </PermissionGate>
        </div>
      ) : (
        <div className="space-y-6">
          <div className="grid gap-4 lg:grid-cols-3">
            <StatCard
              helper="Approved income this year"
              icon={ArrowUpCircle}
              label="Total income"
              mono
              tone="success"
              value={formatIDR(summaryQuery.data?.total_income ?? 0)}
            />
            <StatCard
              helper="Approved outcome this year"
              icon={ArrowDownCircle}
              label="Total outcome"
              mono
              tone="error"
              value={formatIDR(summaryQuery.data?.total_outcome ?? 0)}
            />
            <StatCard
              helper="Current month net position"
              icon={Scale}
              mono
              tone={(summaryQuery.data?.net_profit_this_month ?? 0) >= 0 ? "success" : "error"}
              label="Net this month"
              value={formatIDR(summaryQuery.data?.net_profit_this_month ?? 0)}
            />
          </div>

          <div className="grid gap-6 xl:grid-cols-[2fr,1fr]">
            <Card className="p-6">
              <div className="flex items-center justify-between gap-3 border-b border-border pb-4">
                <div>
                  <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
                    Monthly trend
                  </p>
                  <h4 className="text-[20px] font-[700] text-text-primary">Income vs outcome</h4>
                </div>
                <Input
                  className="w-32"
                  onChange={(event) => setFilters((prev) => ({ ...prev, year: event.target.value || String(new Date().getFullYear()) }))}
                  type="number"
                  value={filters.year}
                />
              </div>
              <div className="mt-6 h-[320px]">
                <ResponsiveContainer height="100%" width="100%">
                  <BarChart data={summaryQuery.data?.monthly ?? []}>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} />
                    <XAxis dataKey="month" />
                    <YAxis tickFormatter={(value) => `${Math.round(Number(value) / 1000000)} jt`} />
                    <Tooltip formatter={(value) => formatIDR(Number(value))} />
                    <Bar dataKey="income" fill="#36B37E" radius={[4, 4, 0, 0]} />
                    <Bar dataKey="outcome" fill="#FF5630" radius={[4, 4, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </Card>

            <Card className="p-6">
              <div className="border-b border-border pb-4">
                <p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
                  Category mix
                </p>
                <h4 className="text-[20px] font-[700] text-text-primary">Breakdown</h4>
              </div>
              <div className="mt-6 h-[260px]">
                <ResponsiveContainer height="100%" width="100%">
                  <PieChart>
                    <Pie
                      cx="50%"
                      cy="50%"
                      data={byCategoryRows.map(([name, value]) => ({ name, value }))}
                      dataKey="value"
                      innerRadius={55}
                      outerRadius={90}
                      paddingAngle={3}
                    >
                      {byCategoryRows.map(([name], index) => (
                        <Cell fill={pieColors[index % pieColors.length]} key={name} />
                      ))}
                    </Pie>
                    <Tooltip formatter={(value) => formatIDR(Number(value))} />
                  </PieChart>
                </ResponsiveContainer>
              </div>
              <div className="space-y-2">
                {byCategoryRows.map(([name, value], index) => (
                  <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface-muted px-4 py-3" key={name}>
                    <div className="flex items-center gap-3">
                      <span className="h-3 w-3 rounded-full" style={{ backgroundColor: pieColors[index % pieColors.length] }} />
                      <span className="text-sm font-medium text-text-primary">{name}</span>
                    </div>
                    <span className="font-mono text-sm tabular-nums text-text-secondary">{formatIDR(value)}</span>
                  </div>
                ))}
              </div>
            </Card>
          </div>
        </div>
      )}

      <FormModal
        isLoading={createRecordMutation.isPending || updateRecordMutation.isPending}
        isOpen={showRecordForm}
        onClose={() => {
          setShowRecordForm(false);
          setEditingRecord(null);
          resetRecordForm(recordForm);
        }}
        onSubmit={handleRecordSubmit}
        size="lg"
        submitLabel={editingRecord ? "Save record" : "Create record"}
        title={editingRecord ? "Edit record" : "Create record"}
        subtitle="Capture the category, amount, and posting date without pushing the records table down."
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <select className="field-select" {...recordForm.register("type")}>
            <option value="income">Income</option>
            <option value="outcome">Outcome</option>
          </select>
          <select className="field-select" {...recordForm.register("category_id")}>
            <option value="">Select category</option>
            {visibleCategories.map((category) => (
              <option key={category.id} value={category.id}>
                {category.name}
              </option>
            ))}
          </select>
          <Controller
            control={recordForm.control}
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
          <Input {...recordForm.register("record_date")} type="date" />
          <Input className="lg:col-span-2" {...recordForm.register("description")} placeholder="Description" />
        </div>
      </FormModal>

      <FormModal
        isLoading={createCategoryMutation.isPending || updateCategoryMutation.isPending}
        isOpen={showCategoryForm}
        onClose={() => {
          setShowCategoryForm(false);
          setEditingCategory(null);
          categoryForm.reset({ name: "", type: "income" });
        }}
        onSubmit={handleCategorySubmit}
        size="sm"
        submitLabel={editingCategory ? "Save category" : "Add category"}
        title={editingCategory ? "Edit finance category" : "Add finance category"}
        subtitle="Keep income and outcome categories tidy so records stay easy to classify."
      >
        <div className="grid gap-4">
          <Input {...categoryForm.register("name")} placeholder="Category name" />
          <select className="field-select" {...categoryForm.register("type")}>
            <option value="income">Income</option>
            <option value="outcome">Outcome</option>
          </select>
        </div>
      </FormModal>

      <ConfirmDialog
        confirmLabel="Delete record"
        description={recordToDelete ? `Record "${recordToDelete.description}" will be removed permanently.` : ""}
        isLoading={deleteRecordMutation.isPending}
        isOpen={Boolean(recordToDelete)}
        onClose={() => setRecordToDelete(null)}
        onConfirm={() => {
          if (recordToDelete) {
            deleteRecordMutation.mutate(recordToDelete.id);
          }
        }}
        title={recordToDelete ? "Delete finance record?" : "Delete finance record?"}
      />

      <ConfirmDialog
        confirmLabel="Reject record"
        description={recordToReject ? `Record "${recordToReject.description}" will be marked as rejected.` : ""}
        isLoading={reviewRecordMutation.isPending}
        isOpen={Boolean(recordToReject)}
        onClose={() => setRecordToReject(null)}
        onConfirm={() => {
          if (recordToReject) {
            reviewRecordMutation.mutate({ recordId: recordToReject.id, decision: "rejected" });
          }
        }}
        title={recordToReject ? "Reject finance record?" : "Reject finance record?"}
      />

      <ConfirmDialog
        confirmLabel="Delete category"
        description={categoryToDelete ? `Category "${categoryToDelete.name}" will be removed from finance settings.` : ""}
        isLoading={deleteCategoryMutation.isPending}
        isOpen={Boolean(categoryToDelete)}
        onClose={() => setCategoryToDelete(null)}
        onConfirm={() => {
          if (categoryToDelete) {
            deleteCategoryMutation.mutate(categoryToDelete.id);
          }
        }}
        title={categoryToDelete ? `Delete ${categoryToDelete.name}?` : "Delete category?"}
      />
    </div>
  );
}

function resetRecordForm(form: Pick<ReturnType<typeof useForm<FinanceRecordFormValues>>, "reset">) {
  form.reset({
    category_id: "",
    type: "income",
    amount: 0,
    description: "",
    record_date: new Date().toISOString().slice(0, 10),
  });
}

async function invalidateFinance(queryClient: ReturnType<typeof useQueryClient>, year: number) {
  await queryClient.invalidateQueries({ queryKey: financeKeys.all });
  await queryClient.invalidateQueries({ queryKey: financeKeys.summary(year) });
}

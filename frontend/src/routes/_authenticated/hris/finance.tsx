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

import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { formatIDR } from "@/lib/currency";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import {
  createFinanceCategory,
  createFinanceRecord,
  deleteFinanceCategory,
  deleteFinanceRecord,
  exportFinanceCSV,
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

const pieColors = ["#16a34a", "#ea580c", "#2563eb", "#7c3aed", "#ca8a04", "#0f766e"];

export const Route = createFileRoute("/_authenticated/hris/finance")({
  beforeLoad: async () => {
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
  const [editingRecord, setEditingRecord] = useState<FinanceRecord | null>(null);
  const [editingCategory, setEditingCategory] = useState<FinanceCategory | null>(null);

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
    resolver: zodResolver(recordSchema),
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
      await invalidateFinance(queryClient, summaryYear);
    },
  });

  const createCategoryMutation = useMutation({
    mutationFn: createFinanceCategory,
    onSuccess: async () => {
      categoryForm.reset({ name: "", type: "income" });
      await queryClient.invalidateQueries({ queryKey: financeKeys.categories() });
    },
  });

  const updateCategoryMutation = useMutation({
    mutationFn: (payload: { categoryId: string; values: FinanceCategoryFormValues }) =>
      updateFinanceCategory(payload.categoryId, payload.values),
    onSuccess: async () => {
      setEditingCategory(null);
      categoryForm.reset({ name: "", type: "income" });
      await queryClient.invalidateQueries({ queryKey: financeKeys.categories() });
    },
  });

  const deleteCategoryMutation = useMutation({
    mutationFn: deleteFinanceCategory,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: financeKeys.categories() });
    },
  });

  const recordType = recordForm.watch("type");
  const categories = categoriesQuery.data ?? [];
  const visibleCategories = useMemo(
    () => categories.filter((item) => item.type === recordType),
    [categories, recordType],
  );

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

  const downloadExport = async () => {
    const blob = await exportFinanceCSV(summaryYear, filters.month || undefined);
    const url = window.URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `finance-${summaryYear}${filters.month ? `-${filters.month}` : ""}.csv`;
    anchor.click();
    window.URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">HRIS finance</p>
            <h3 className="mt-2 text-3xl font-bold">Income and outcome control</h3>
            <p className="mt-2 max-w-3xl text-muted-foreground">
              Kelola record keuangan, approval flow, dashboard bulanan, dan export CSV tanpa pindah modul.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Button onClick={() => setActiveTab("records")} variant={activeTab === "records" ? "default" : "outline"}>
              Records
            </Button>
            <Button onClick={() => setActiveTab("dashboard")} variant={activeTab === "dashboard" ? "default" : "outline"}>
              Dashboard
            </Button>
          </div>
        </div>
      </Card>

      {activeTab === "records" ? (
        <div className="space-y-6">
          <Card className="p-6">
            <div className="grid gap-3 lg:grid-cols-6">
              <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((prev) => ({ ...prev, type: event.target.value, page: 1 }))} value={filters.type}>
                <option value="">All types</option>
                <option value="income">Income</option>
                <option value="outcome">Outcome</option>
              </select>
              <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((prev) => ({ ...prev, category: event.target.value, page: 1 }))} value={filters.category}>
                <option value="">All categories</option>
                {categories.map((category) => (
                  <option key={category.id} value={category.id}>
                    {category.name}
                  </option>
                ))}
              </select>
              <Input onChange={(event) => setFilters((prev) => ({ ...prev, month: event.target.value, page: 1 }))} placeholder="Month" type="number" value={filters.month} />
              <Input onChange={(event) => setFilters((prev) => ({ ...prev, year: event.target.value, page: 1 }))} placeholder="Year" type="number" value={filters.year} />
              <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" onChange={(event) => setFilters((prev) => ({ ...prev, status: event.target.value, page: 1 }))} value={filters.status}>
                <option value="">All statuses</option>
                <option value="draft">Draft</option>
                <option value="pending_review">Pending review</option>
                <option value="approved">Approved</option>
                <option value="rejected">Rejected</option>
              </select>
              <Button onClick={() => void downloadExport()} variant="outline">
                Export CSV
              </Button>
            </div>
            <div className="mt-4">
              <PermissionGate permission={permissions.hrisFinanceCreate}>
                <Button
                  onClick={() => {
                    setEditingRecord(null);
                    resetRecordForm(recordForm);
                    setShowRecordForm((value) => !value);
                  }}
                >
                  {showRecordForm ? "Close form" : "New record"}
                </Button>
              </PermissionGate>
            </div>
          </Card>

          {showRecordForm ? (
            <Card className="p-6">
              <div className="mb-4">
                <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Finance record</p>
                <h4 className="mt-2 text-2xl font-bold">{editingRecord ? "Edit record" : "Create record"}</h4>
              </div>
              <form className="grid gap-4 lg:grid-cols-2" onSubmit={handleRecordSubmit}>
                <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...recordForm.register("type")}>
                  <option value="income">Income</option>
                  <option value="outcome">Outcome</option>
                </select>
                <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...recordForm.register("category_id")}>
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
                <div className="lg:col-span-2 flex flex-wrap gap-3">
                  <Button disabled={createRecordMutation.isPending || updateRecordMutation.isPending} type="submit">
                    {editingRecord ? "Save record" : "Create record"}
                  </Button>
                  <Button
                    onClick={() => {
                      setEditingRecord(null);
                      setShowRecordForm(false);
                    }}
                    type="button"
                    variant="ghost"
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            </Card>
          ) : null}

          <div className="grid gap-4">
            {(recordsQuery.data?.items ?? []).map((record) => (
              <Card className="p-6" key={record.id}>
                <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
                  <div>
                    <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">{record.category_name}</p>
                    <h4 className="mt-2 text-xl font-bold">{record.description}</h4>
                    <p className="mt-2 text-sm text-muted-foreground">
                      {new Date(record.record_date).toLocaleDateString("id-ID")} • {record.type}
                    </p>
                  </div>
                  <div className="text-right">
                    <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Amount</p>
                    <p className="mt-2 text-2xl font-bold">{formatIDR(record.amount)}</p>
                    <span className="mt-3 inline-flex rounded-full bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-secondary-foreground">
                      {record.approval_status}
                    </span>
                  </div>
                </div>
                <div className="mt-5 flex flex-wrap gap-3">
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
                        variant="outline"
                      >
                        Edit
                      </Button>
                      <Button
                        disabled={deleteRecordMutation.isPending && deleteRecordMutation.variables === record.id}
                        onClick={() => deleteRecordMutation.mutate(record.id)}
                        variant="ghost"
                      >
                        Delete
                      </Button>
                    </>
                  ) : null}
                  {hasPermission(permissions.hrisFinanceCreate) && record.approval_status === "draft" ? (
                    <Button onClick={() => submitRecordMutation.mutate(record.id)} variant="outline">
                      Submit
                    </Button>
                  ) : null}
                  {hasPermission(permissions.hrisFinanceApprove) && record.approval_status === "pending_review" ? (
                    <>
                      <Button onClick={() => reviewRecordMutation.mutate({ recordId: record.id, decision: "approved" })}>
                        Approve
                      </Button>
                      <Button onClick={() => reviewRecordMutation.mutate({ recordId: record.id, decision: "rejected" })} variant="ghost">
                        Reject
                      </Button>
                    </>
                  ) : null}
                </div>
              </Card>
            ))}
          </div>

          {(recordsQuery.data?.items ?? []).length === 0 ? (
            <Card className="p-8 text-center text-sm text-muted-foreground">Belum ada finance record yang tercatat.</Card>
          ) : null}

          <PermissionGate fallback={null} permission={permissions.hrisFinanceApprove}>
            {canManageCategories ? (
              <Card className="p-6">
                <div className="mb-4">
                  <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Finance categories</p>
                  <h4 className="mt-2 text-2xl font-bold">Manage categories</h4>
                </div>
                <form className="grid gap-4 lg:grid-cols-3" onSubmit={handleCategorySubmit}>
                  <Input {...categoryForm.register("name")} placeholder="Category name" />
                  <select className="h-12 rounded-2xl border border-input bg-card/80 px-4 text-sm" {...categoryForm.register("type")}>
                    <option value="income">Income</option>
                    <option value="outcome">Outcome</option>
                  </select>
                  <Button type="submit">{editingCategory ? "Save category" : "Add category"}</Button>
                </form>
                <div className="mt-5 grid gap-3 lg:grid-cols-2">
                  {categories.map((category) => (
                    <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={category.id}>
                      <div className="flex items-center justify-between gap-3">
                        <div>
                          <p className="text-sm font-semibold">{category.name}</p>
                          <p className="text-xs text-muted-foreground">{category.type}</p>
                        </div>
                        {category.is_default ? (
                          <span className="rounded-full bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-secondary-foreground">
                            default
                          </span>
                        ) : (
                          <div className="flex gap-2">
                            <Button
                              onClick={() => {
                                setEditingCategory(category);
                                categoryForm.reset({ name: category.name, type: category.type });
                              }}
                              size="sm"
                              type="button"
                              variant="outline"
                            >
                              Edit
                            </Button>
                            <Button onClick={() => deleteCategoryMutation.mutate(category.id)} size="sm" type="button" variant="ghost">
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
            <SummaryCard label="Total income" value={formatIDR(summaryQuery.data?.total_income ?? 0)} />
            <SummaryCard label="Total outcome" value={formatIDR(summaryQuery.data?.total_outcome ?? 0)} />
            <SummaryCard label="Net this month" value={formatIDR(summaryQuery.data?.net_profit_this_month ?? 0)} />
          </div>

          <div className="grid gap-6 xl:grid-cols-[2fr,1fr]">
            <Card className="p-6">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Monthly trend</p>
                  <h4 className="mt-2 text-2xl font-bold">Income vs outcome</h4>
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
                    <Tooltip formatter={(value: number) => formatIDR(value)} />
                    <Bar dataKey="income" fill="#16a34a" radius={[8, 8, 0, 0]} />
                    <Bar dataKey="outcome" fill="#ea580c" radius={[8, 8, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </Card>

            <Card className="p-6">
              <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Category mix</p>
              <h4 className="mt-2 text-2xl font-bold">Breakdown</h4>
              <div className="mt-6 h-[260px]">
                <ResponsiveContainer height="100%" width="100%">
                  <PieChart>
                    <Pie
                      cx="50%"
                      cy="50%"
                      data={Object.entries(summaryQuery.data?.by_category ?? {}).map(([name, value]) => ({ name, value }))}
                      dataKey="value"
                      innerRadius={55}
                      outerRadius={90}
                      paddingAngle={3}
                    >
                      {Object.entries(summaryQuery.data?.by_category ?? {}).map(([name], index) => (
                        <Cell fill={pieColors[index % pieColors.length]} key={name} />
                      ))}
                    </Pie>
                    <Tooltip formatter={(value: number) => formatIDR(value)} />
                  </PieChart>
                </ResponsiveContainer>
              </div>
              <div className="space-y-2">
                {Object.entries(summaryQuery.data?.by_category ?? {}).map(([name, value], index) => (
                  <div className="flex items-center justify-between gap-3 rounded-[18px] border border-border/70 bg-background/70 px-4 py-3" key={name}>
                    <div className="flex items-center gap-3">
                      <span className="h-3 w-3 rounded-full" style={{ backgroundColor: pieColors[index % pieColors.length] }} />
                      <span className="text-sm font-medium">{name}</span>
                    </div>
                    <span className="text-sm text-muted-foreground">{formatIDR(value)}</span>
                  </div>
                ))}
              </div>
            </Card>
          </div>
        </div>
      )}
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

function resetRecordForm(form: ReturnType<typeof useForm<FinanceRecordFormValues>>) {
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

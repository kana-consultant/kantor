import { useEffect, useMemo, useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Controller, useForm } from "react-hook-form";
import { CircleDollarSign, Pencil, Plus, Receipt, TimerReset, Trash2 } from "lucide-react";
import { z } from "zod";

import { DataTable, type DataTableColumn, type SortState } from "@/components/shared/data-table";
import { ExportButton } from "@/components/shared/export-button";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { formatIDR } from "@/lib/currency";
import { extractDateInputValue, formatCalendarDate, formatDateInputValue } from "@/lib/date";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import { employeesKeys, getMyEmployee, listEmployees } from "@/services/hris-employees";
import {
  bulkMarkReimbursementsPaid,
  bulkReviewReimbursements,
  createReimbursement,
  deleteReimbursement,
  getReimbursementSummary,
  listReimbursements,
  reimbursementsKeys,
  updateReimbursement,
  uploadReimbursementAttachments,
} from "@/services/hris-reimbursements";
import { toast } from "@/stores/toast-store";
import type { Reimbursement, ReimbursementFilters, ReimbursementFormValues } from "@/types/hris";

const reimbursementSchema = z.object({
  employee_id: z.string().min(1, "Karyawan wajib dipilih"),
  title: z.string().min(2, "Judul minimal 2 karakter").max(200),
  category: z.string().min(2, "Kategori wajib diisi").max(120),
  amount: z.coerce.number().min(1, "Jumlah wajib diisi"),
  transaction_date: z.string().min(1, "Tanggal transaksi wajib diisi"),
  description: z.string().max(2000),
});

const editReimbursementSchema = z.object({
  title: z.string().min(2, "Judul minimal 2 karakter").max(200),
  category: z.string().min(2, "Kategori wajib diisi").max(120),
  amount: z.coerce.number().min(1, "Jumlah wajib diisi"),
  transaction_date: z.string().min(1, "Tanggal transaksi wajib diisi"),
  description: z.string().max(2000),
});

type EditReimbursementFormValues = z.infer<typeof editReimbursementSchema>;

const defaultFilters: ReimbursementFilters = {
  page: 1,
  perPage: 20,
  status: "",
  employee: "",
  month: String(new Date().getMonth() + 1),
  year: String(new Date().getFullYear()),
  sortBy: "created_at",
  sortOrder: "desc",
};

const maxReimbursementAttachmentFiles = 5;
const maxReimbursementAttachmentSizeBytes = 10 * 1024 * 1024;
const acceptedReimbursementAttachmentTypes = "image/*,application/pdf";

export const Route = createFileRoute("/_authenticated/hris/reimbursements/")({
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
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
  const [editingReimbursement, setEditingReimbursement] = useState<Reimbursement | null>(null);
  const [keptAttachments, setKeptAttachments] = useState<string[]>([]);
  const [editFiles, setEditFiles] = useState<File[]>([]);
  const [reimbursementToDelete, setReimbursementToDelete] = useState<Reimbursement | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [bulkApproveOpen, setBulkApproveOpen] = useState(false);
  const [bulkMarkPaidOpen, setBulkMarkPaidOpen] = useState(false);
  const [bulkNotes, setBulkNotes] = useState("");

  const form = useForm<ReimbursementFormValues>({
    resolver: zodResolver(reimbursementSchema) as never,
    defaultValues: {
      employee_id: "",
      title: "",
      category: "",
      amount: 0,
      transaction_date: formatDateInputValue(),
      description: "",
    },
  });

  const editForm = useForm<EditReimbursementFormValues>({
    resolver: zodResolver(editReimbursementSchema) as never,
    defaultValues: { title: "", category: "", amount: 0, transaction_date: formatDateInputValue(), description: "" },
  });

  useEffect(() => {
    if (editingReimbursement) {
      editForm.reset({
        title: editingReimbursement.title,
        category: editingReimbursement.category,
        amount: editingReimbursement.amount,
        transaction_date: extractDateInputValue(editingReimbursement.transaction_date),
        description: editingReimbursement.description ?? "",
      });
      setKeptAttachments(editingReimbursement.attachments ?? []);
      setEditFiles([]);
    }
  }, [editingReimbursement]); // eslint-disable-line react-hooks/exhaustive-deps

  const canViewAllEmployees = hasPermission(permissions.hrisEmployeeView);

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list({ page: 1, perPage: 100, search: "", department: "", status: "" }),
    queryFn: () => listEmployees({ page: 1, perPage: 100, search: "", department: "", status: "" }),
    enabled: canViewAllEmployees,
  });

  const myEmployeeQuery = useQuery({
    queryKey: [...employeesKeys.all, "me"],
    queryFn: getMyEmployee,
    enabled: !canViewAllEmployees,
  });

  useEffect(() => {
    if (myEmployeeQuery.data && !canViewAllEmployees) {
      form.setValue("employee_id", myEmployeeQuery.data.id);
    }
  }, [myEmployeeQuery.data, canViewAllEmployees]); // eslint-disable-line react-hooks/exhaustive-deps

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
      let attachmentUploadWarning: string | null = null;
      if (files.length > 0) {
        try {
          await uploadReimbursementAttachments(created.id, files);
        } catch (error) {
          attachmentUploadWarning = error instanceof Error
            ? error.message
            : "Reimbursement berhasil dibuat, tetapi upload lampiran gagal.";
        }
      }
      return { created, attachmentUploadWarning };
    },
    onSuccess: async ({ attachmentUploadWarning }) => {
      form.reset({
        employee_id: "",
        title: "",
        category: "",
        amount: 0,
        transaction_date: formatDateInputValue(),
        description: "",
      });
      setFiles([]);
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: reimbursementsKeys.all });
      if (attachmentUploadWarning) {
        toast.warning("Reimbursement berhasil dibuat", attachmentUploadWarning);
        return;
      }
      toast.success("Reimbursement berhasil dibuat");
    },
    onError: (error) => {
      toast.error("Gagal membuat reimbursement", error instanceof Error ? error.message : undefined);
    },
  });

  const updateMutation = useMutation({
    mutationFn: async (values: EditReimbursementFormValues) => {
      const updated = await updateReimbursement(editingReimbursement!.id, values, keptAttachments);
      let attachmentUploadWarning: string | null = null;
      if (editFiles.length > 0) {
        try {
          await uploadReimbursementAttachments(updated.id, editFiles);
        } catch (error) {
          attachmentUploadWarning = error instanceof Error
            ? error.message
            : "Perubahan berhasil disimpan, tetapi upload lampiran baru gagal.";
        }
      }
      return { updated, attachmentUploadWarning };
    },
    onSuccess: async ({ attachmentUploadWarning }) => {
      setEditingReimbursement(null);
      setKeptAttachments([]);
      setEditFiles([]);
      await queryClient.invalidateQueries({ queryKey: reimbursementsKeys.all });
      if (attachmentUploadWarning) {
        toast.warning("Perubahan reimbursement tersimpan", attachmentUploadWarning);
        return;
      }
      toast.success("Reimbursement berhasil diperbarui");
    },
    onError: (error) => {
      toast.error("Gagal memperbarui reimbursement", error instanceof Error ? error.message : undefined);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteReimbursement(reimbursementToDelete!.id),
    onSuccess: async () => {
      setReimbursementToDelete(null);
      await queryClient.invalidateQueries({ queryKey: reimbursementsKeys.all });
      toast.success("Reimbursement berhasil dihapus");
    },
    onError: (error) => {
      toast.error("Gagal menghapus reimbursement", error instanceof Error ? error.message : undefined);
    },
  });

  const bulkApproveMutation = useMutation({
    mutationFn: () => bulkReviewReimbursements([...selectedIds], "approved", bulkNotes || undefined),
    onSuccess: async () => {
      setSelectedIds(new Set());
      setBulkApproveOpen(false);
      setBulkNotes("");
      await queryClient.invalidateQueries({ queryKey: reimbursementsKeys.all });
    },
  });

  const bulkMarkPaidMutation = useMutation({
    mutationFn: () => bulkMarkReimbursementsPaid([...selectedIds], bulkNotes || undefined),
    onSuccess: async () => {
      setSelectedIds(new Set());
      setBulkMarkPaidOpen(false);
      setBulkNotes("");
      await queryClient.invalidateQueries({ queryKey: reimbursementsKeys.all });
    },
  });

  const employees = employeesQuery.data?.items ?? [];
  const meta = reimbursementsQuery.data?.meta;
  const canReview = hasPermission(permissions.hrisReimbursementApprove);
  const reimbursementSortState = useMemo<SortState>(() => {
    if (!filters.sortBy || filters.sortBy === "created_at" || !filters.sortOrder) {
      return null;
    }

    return {
      columnId: filters.sortBy,
      direction: filters.sortOrder,
    };
  }, [filters.sortBy, filters.sortOrder]);
  const canMarkPaid = hasRole("manager", "hris") || hasRole("admin", "hris") || hasRole("super_admin");
  const reimbursements = reimbursementsQuery.data?.items ?? [];

  const selectedItems = reimbursements.filter((r) => selectedIds.has(r.id));
  const allSelectedSubmitted = selectedItems.length > 0 && selectedItems.every((r) => r.status === "submitted");
  const allSelectedApproved = selectedItems.length > 0 && selectedItems.every((r) => r.status === "approved");
  const selectedTotal = selectedItems.reduce((sum, r) => sum + r.amount, 0);
  const toggleSelect = (id: string) =>
    setSelectedIds((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  const selectAll = () => setSelectedIds(new Set(reimbursements.map((r) => r.id)));
  const clearSelection = () => setSelectedIds(new Set());

  const handleFiles = (incomingFiles: FileList | File[]) => {
    const result = mergeAttachmentFiles(files, incomingFiles, 0);
    setFiles(result.files);
    reportAttachmentWarnings(result.warnings);
  };

  const handleEditFiles = (incomingFiles: FileList | File[]) => {
    const result = mergeAttachmentFiles(editFiles, incomingFiles, keptAttachments.length);
    setEditFiles(result.files);
    reportAttachmentWarnings(result.warnings);
  };

  const columns: Array<DataTableColumn<Reimbursement>> = [
    {
      id: "select",
      header: "",
      widthClassName: "w-10",
      cell: (item) => (
        <input
          checked={selectedIds.has(item.id)}
          className="h-4 w-4 cursor-pointer accent-primary"
          onChange={() => toggleSelect(item.id)}
          onClick={(e) => e.stopPropagation()}
          type="checkbox"
        />
      ),
    },
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
            {formatCalendarDate(item.transaction_date)}
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
        <div className="flex items-center justify-end gap-2">
          {item.status === "submitted" && hasPermission(permissions.hrisReimbursementEdit) ? (
            <>
              <Button
                className="h-8 w-8 p-0"
                onClick={() => setEditingReimbursement(item)}
                title="Edit"
                type="button"
                variant="ghost"
              >
                <Pencil className="h-4 w-4" />
              </Button>
              <Button
                className="h-8 w-8 p-0 text-error hover:text-error"
                onClick={() => setReimbursementToDelete(item)}
                title="Hapus"
                type="button"
                variant="ghost"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </>
          ) : null}
          <Link
            className={buttonVariants({ size: "sm" })}
            params={{ reimbursementId: item.id }}
            to="/hris/reimbursements/$reimbursementId"
          >
            Open detail
          </Link>
        </div>
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
          <div className="flex flex-wrap gap-3">
            <PermissionGate permission={permissions.hrisReimbursementView}>
              <ExportButton
                endpoint="/hris/reimbursements/export"
                filename="reimbursements-report"
                filters={{
                  employee: filters.employee,
                  month: filters.month,
                  status: filters.status,
                  year: filters.year,
                }}
                formats={["pdf", "xlsx"]}
              />
            </PermissionGate>
            <PermissionGate permission={permissions.hrisReimbursementCreate}>
              <Button onClick={() => setShowForm(true)} type="button">
                <Plus className="h-4 w-4" />
                Submit reimbursement
              </Button>
            </PermissionGate>
          </div>
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
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Karyawan<span className="ml-0.5 text-priority-high">*</span>
            </label>
            {canViewAllEmployees ? (
              <>
                <select className="field-select" {...form.register("employee_id")}>
                  <option value="">Pilih karyawan</option>
                  {employees.map((employee) => (
                    <option key={employee.id} value={employee.id}>
                      {employee.full_name}
                    </option>
                  ))}
                </select>
                {form.formState.errors.employee_id ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{form.formState.errors.employee_id.message}</p> : null}
              </>
            ) : (
              <p className="flex h-9 items-center rounded-md border border-border bg-surface-muted px-3 text-sm text-text-primary">
                {myEmployeeQuery.data?.full_name ?? "Memuat..."}
              </p>
            )}
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Kategori<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input {...form.register("category")} placeholder="Kategori" />
            {form.formState.errors.category ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{form.formState.errors.category.message}</p> : null}
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Judul<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input {...form.register("title")} placeholder="Judul" />
            {form.formState.errors.title ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{form.formState.errors.title.message}</p> : null}
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Jumlah<span className="ml-0.5 text-priority-high">*</span>
            </label>
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
            {form.formState.errors.amount ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{form.formState.errors.amount.message}</p> : null}
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Tanggal transaksi<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input {...form.register("transaction_date")} type="date" />
            {form.formState.errors.transaction_date ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{form.formState.errors.transaction_date.message}</p> : null}
          </div>
          <div className="lg:col-span-2">
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Deskripsi
            </label>
            <Input {...form.register("description")} placeholder="Deskripsi (opsional)" />
            {form.formState.errors.description ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{form.formState.errors.description.message}</p> : null}
          </div>
          <div className="lg:col-span-2">
            <AttachmentPicker
              files={files}
              helper="Tambahkan bukti reimbursement dalam bentuk gambar atau PDF. Maksimal 5 file dan 10MB per file."
              onFilesSelected={handleFiles}
              onRemoveFile={(signature) => setFiles((current) => current.filter((file) => fileSignature(file) !== signature))}
              title="Lampiran reimbursement"
            />
          </div>
        </div>
      </FormModal>

      <FormModal
        isLoading={updateMutation.isPending}
        isOpen={!!editingReimbursement}
        onClose={() => setEditingReimbursement(null)}
        onSubmit={editForm.handleSubmit((values) => updateMutation.mutate(values))}
        size="lg"
        submitLabel="Simpan perubahan"
        title="Edit reimbursement"
        subtitle="Ubah detail klaim. Hanya reimbursement yang belum diproses yang dapat diedit."
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Kategori<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input {...editForm.register("category")} placeholder="Kategori" />
            {editForm.formState.errors.category ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{editForm.formState.errors.category.message}</p> : null}
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Judul<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input {...editForm.register("title")} placeholder="Judul" />
            {editForm.formState.errors.title ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{editForm.formState.errors.title.message}</p> : null}
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Jumlah<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Controller
              control={editForm.control}
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
            {editForm.formState.errors.amount ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{editForm.formState.errors.amount.message}</p> : null}
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Tanggal transaksi<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input {...editForm.register("transaction_date")} type="date" />
            {editForm.formState.errors.transaction_date ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{editForm.formState.errors.transaction_date.message}</p> : null}
          </div>
          <div className="lg:col-span-2">
            <label className="mb-1 block text-sm font-medium text-text-primary">
              Deskripsi
            </label>
            <Input {...editForm.register("description")} placeholder="Deskripsi (opsional)" />
            {editForm.formState.errors.description ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{editForm.formState.errors.description.message}</p> : null}
          </div>
          <div className="lg:col-span-2">
            <label className="mb-1 block text-sm font-medium text-text-primary">Lampiran saat ini</label>
            {keptAttachments.length > 0 ? (
              <div className="mt-2 grid gap-2 md:grid-cols-2">
                {keptAttachments.map((path) => (
                  <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface px-4 py-3" key={path}>
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium text-text-primary">{path.split("/").pop()}</p>
                      <p className="text-xs text-text-secondary">Tetap disimpan</p>
                    </div>
                    <Button
                      className="ml-3 h-7 shrink-0 text-xs"
                      onClick={() => setKeptAttachments((prev) => prev.filter((p) => p !== path))}
                      size="sm"
                      type="button"
                      variant="ghost"
                    >
                      <Trash2 className="h-3.5 w-3.5 text-error" />
                    </Button>
                  </div>
                ))}
              </div>
            ) : (
              <p className="mt-1 text-sm text-text-secondary">Tidak ada lampiran.</p>
            )}
          </div>
          <div className="lg:col-span-2">
            <AttachmentPicker
              existingCount={keptAttachments.length}
              files={editFiles}
              helper="Slot lampiran yang tersedia mengikuti file lama yang masih disimpan. Maksimal total 5 file per reimbursement."
              onFilesSelected={handleEditFiles}
              onRemoveFile={(signature) => setEditFiles((current) => current.filter((file) => fileSignature(file) !== signature))}
              title="Tambah lampiran baru"
            />
          </div>
        </div>
      </FormModal>

      {reimbursementToDelete ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <Card className="w-full max-w-md p-6">
            <h3 className="text-lg font-semibold text-text-primary">Hapus reimbursement?</h3>
            <p className="mt-2 text-sm text-text-secondary">
              Reimbursement <span className="font-medium text-text-primary">"{reimbursementToDelete.title}"</span> akan dihapus permanen. Tindakan ini tidak dapat dibatalkan.
            </p>
            <div className="mt-6 flex justify-end gap-3">
              <Button
                disabled={deleteMutation.isPending}
                onClick={() => setReimbursementToDelete(null)}
                type="button"
                variant="outline"
              >
                Batal
              </Button>
              <Button
                disabled={deleteMutation.isPending}
                onClick={() => deleteMutation.mutate()}
                type="button"
                variant="danger"
              >
                {deleteMutation.isPending ? "Menghapus..." : "Hapus"}
              </Button>
            </div>
          </Card>
        </div>
      ) : null}

      {reimbursementsQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-error">{reimbursementsQuery.error.message}</Card>
      ) : null}

      {selectedIds.size > 0 ? (
        <Card className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between border-primary">
          <div className="flex items-center gap-3">
            <span className="text-sm font-semibold text-text-primary">{selectedIds.size} item dipilih</span>
            <button className="text-xs text-text-secondary underline" onClick={selectAll} type="button">
              Pilih semua ({reimbursements.length})
            </button>
            <button className="text-xs text-text-secondary underline" onClick={clearSelection} type="button">
              Batalkan
            </button>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {!allSelectedSubmitted && !allSelectedApproved ? (
              <p className="text-xs text-text-secondary">Pilih item dengan status yang sama untuk aksi bulk.</p>
            ) : null}
            {allSelectedSubmitted && canReview ? (
              <Button onClick={() => { setBulkNotes(""); setBulkApproveOpen(true); }} size="sm" type="button">
                Approve {selectedIds.size} item
              </Button>
            ) : null}
            {allSelectedApproved && canMarkPaid ? (
              <Button onClick={() => { setBulkNotes(""); setBulkMarkPaidOpen(true); }} size="sm" type="button">
                Tandai lunas {selectedIds.size} item
              </Button>
            ) : null}
          </div>
        </Card>
      ) : null}

      {bulkApproveOpen ? (
        <BulkConfirmDialog
          confirmLabel="Approve semua"
          isPending={bulkApproveMutation.isPending}
          items={selectedItems}
          notes={bulkNotes}
          onClose={() => setBulkApproveOpen(false)}
          onConfirm={() => bulkApproveMutation.mutate()}
          onNotesChange={setBulkNotes}
          notesPlaceholder="Catatan untuk semua item..."
          title={`Approve ${selectedIds.size} reimbursement?`}
          subtitle="Semua item yang dipilih akan disetujui sekaligus."
          total={selectedTotal}
        />
      ) : null}

      {bulkMarkPaidOpen ? (
        <BulkConfirmDialog
          confirmLabel="Tandai lunas semua"
          isPending={bulkMarkPaidMutation.isPending}
          items={selectedItems}
          notes={bulkNotes}
          onClose={() => setBulkMarkPaidOpen(false)}
          onConfirm={() => bulkMarkPaidMutation.mutate()}
          onNotesChange={setBulkNotes}
          notesPlaceholder="Misal: Transfer batch Maret via BCA..."
          title={`Tandai lunas ${selectedIds.size} reimbursement?`}
          subtitle="Semua item yang dipilih akan ditandai sebagai sudah dibayar."
          total={selectedTotal}
        />
      ) : null}

      <DataTable
        columns={columns}
        data={reimbursements}
        manualSorting
        sortState={reimbursementSortState}
        onSortChange={(nextSort) =>
          setFilters((current) => ({
            ...current,
            page: 1,
            sortBy: nextSort?.columnId ? (nextSort.columnId as ReimbursementFilters["sortBy"]) : "created_at",
            sortOrder: nextSort?.direction ?? "desc",
          }))
        }
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

function AttachmentPicker({
  title,
  helper,
  files,
  existingCount = 0,
  onFilesSelected,
  onRemoveFile,
}: {
  title: string;
  helper: string;
  files: File[];
  existingCount?: number;
  onFilesSelected: (files: FileList | File[]) => void;
  onRemoveFile: (signature: string) => void;
}) {
  const usedSlots = existingCount + files.length;
  const slotsRemaining = Math.max(maxReimbursementAttachmentFiles - usedSlots, 0);
  const inputDisabled = slotsRemaining === 0;

  return (
    <div
      className="rounded-md border border-dashed border-border bg-surface-muted p-6"
      onDragOver={(event) => event.preventDefault()}
      onDrop={(event) => {
        event.preventDefault();
        if (!inputDisabled) {
          onFilesSelected(event.dataTransfer.files);
        }
      }}
    >
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <p className="font-semibold text-text-primary">{title}</p>
          <p className="mt-1 text-xs text-text-secondary">{helper}</p>
          <p className="mt-2 text-xs font-medium text-text-secondary">
            {usedSlots}/{maxReimbursementAttachmentFiles} file digunakan
          </p>
        </div>
        <label className={`inline-flex h-10 items-center justify-center rounded-md border px-4 text-sm font-semibold transition ${
          inputDisabled
            ? "cursor-not-allowed border-border bg-surface text-text-tertiary"
            : "cursor-pointer border-border bg-surface text-text-primary hover:bg-surface-muted"
        }`}>
          Pilih file
          <input
            accept={acceptedReimbursementAttachmentTypes}
            className="hidden"
            disabled={inputDisabled}
            multiple
            onChange={(event) => onFilesSelected(event.target.files ?? [])}
            type="file"
          />
        </label>
      </div>

      {files.length > 0 ? (
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          {files.map((file) => (
            <div className="flex items-start justify-between gap-3 rounded-md border border-border bg-surface px-4 py-3" key={fileSignature(file)}>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-text-primary">{file.name}</p>
                <p className="text-xs text-text-secondary">{formatAttachmentSize(file.size)}</p>
              </div>
              <Button
                className="h-7 w-7 shrink-0 p-0"
                onClick={() => onRemoveFile(fileSignature(file))}
                size="sm"
                type="button"
                variant="ghost"
              >
                <Trash2 className="h-3.5 w-3.5 text-error" />
              </Button>
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-4 rounded-md border border-border bg-surface px-4 py-3 text-sm text-text-secondary">
          {inputDisabled
            ? "Batas lampiran sudah penuh. Hapus lampiran yang ada dulu jika ingin menambah file baru."
            : "Drag gambar/PDF ke area ini atau pilih file secara manual."}
        </div>
      )}
    </div>
  );
}

function mergeAttachmentFiles(current: File[], incoming: FileList | File[], existingCount: number) {
  const warnings: string[] = [];
  const nextFiles = [...current];
  const seen = new Set(nextFiles.map(fileSignature));

  for (const file of Array.from(incoming)) {
    const signature = fileSignature(file);
    if (seen.has(signature)) {
      warnings.push(`File ${file.name} sudah dipilih.`);
      continue;
    }
    if (!(file.type.startsWith("image/") || file.type === "application/pdf")) {
      warnings.push(`File ${file.name} harus berupa gambar atau PDF.`);
      continue;
    }
    if (file.size > maxReimbursementAttachmentSizeBytes) {
      warnings.push(`File ${file.name} melebihi 10MB.`);
      continue;
    }
    if (existingCount + nextFiles.length >= maxReimbursementAttachmentFiles) {
      warnings.push(`Maksimal ${maxReimbursementAttachmentFiles} lampiran per reimbursement.`);
      break
    }

    seen.add(signature);
    nextFiles.push(file);
  }

  return {
    files: nextFiles,
    warnings,
  };
}

function reportAttachmentWarnings(warnings: string[]) {
  if (warnings.length === 0) {
    return;
  }

  toast.warning(
    "Sebagian lampiran tidak ditambahkan",
    warnings.slice(0, 2).join(" "),
  );
}

function fileSignature(file: File) {
  return `${file.name}:${file.size}:${file.lastModified}`;
}

function formatAttachmentSize(size: number) {
  if (size >= 1024 * 1024) {
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  }
  return `${Math.round(size / 1024)} KB`;
}

function BulkConfirmDialog({
  title,
  subtitle,
  confirmLabel,
  isPending,
  items,
  notes,
  notesPlaceholder,
  onClose,
  onConfirm,
  onNotesChange,
  total,
}: {
  title: string;
  subtitle: string;
  confirmLabel: string;
  isPending: boolean;
  items: Reimbursement[];
  notes: string;
  notesPlaceholder: string;
  onClose: () => void;
  onConfirm: () => void;
  onNotesChange: (v: string) => void;
  total: number;
}) {
  const itemsWithAttachments = items.filter((r) => r.attachments.length > 0);

  // Group by employee for per-person breakdown
  const byEmployee = items.reduce<Record<string, { name: string; count: number; total: number; items: Reimbursement[] }>>(
    (acc, r) => {
      const key = r.employee_id;
      if (!acc[key]) acc[key] = { name: r.employee_name ?? r.employee_id, count: 0, total: 0, items: [] };
      acc[key].count++;
      acc[key].total += r.amount;
      acc[key].items.push(r);
      return acc;
    },
    {},
  );
  const employeeGroups = Object.values(byEmployee);
  const hasMultipleEmployees = employeeGroups.length > 1;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <Card className="flex w-full max-w-lg flex-col gap-0 p-6">
        <h3 className="text-lg font-semibold text-text-primary">{title}</h3>
        <p className="mt-1 text-sm text-text-secondary">{subtitle}</p>

        <div className="mt-3 flex items-center justify-between rounded-md bg-surface-muted px-4 py-3">
          <span className="text-sm text-text-secondary">Total nominal</span>
          <span className="font-mono text-sm font-semibold tabular-nums text-text-primary">{formatIDR(total)}</span>
        </div>

        {hasMultipleEmployees ? (
          <div className="mt-3 space-y-1">
            {employeeGroups.map((g) => (
              <div className="flex items-center justify-between px-4 py-1.5 text-sm" key={g.name}>
                <span className="text-text-secondary">{g.name} <span className="text-text-tertiary">({g.count} item)</span></span>
                <span className="font-mono tabular-nums text-text-primary">{formatIDR(g.total)}</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="mt-2 px-4 text-sm text-text-secondary">
            Karyawan: <span className="font-medium text-text-primary">{employeeGroups[0]?.name}</span>
          </div>
        )}

        {itemsWithAttachments.length > 0 ? (
          <div className="mt-4">
            <p className="mb-2 text-sm font-medium text-text-primary">
              Lampiran tersedia ({itemsWithAttachments.length} item)
            </p>
            <div className="max-h-[180px] space-y-2 overflow-y-auto pr-1">
              {itemsWithAttachments.map((item) => (
                <div className="flex items-center justify-between rounded-md border border-border bg-surface px-3 py-2" key={item.id}>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-text-primary">{item.title}</p>
                    <p className="text-xs text-text-secondary">{item.employee_name} · {formatIDR(item.amount)}</p>
                  </div>
                  <a
                    className="ml-3 shrink-0 text-xs font-semibold text-primary hover:underline"
                    href={`/hris/reimbursements/${item.id}`}
                    rel="noopener noreferrer"
                    target="_blank"
                  >
                    Lihat {item.attachments.length} lampiran →
                  </a>
                </div>
              ))}
            </div>
          </div>
        ) : null}

        <div className="mt-4">
          <label className="mb-1 block text-sm font-medium text-text-primary">Catatan (opsional)</label>
          <textarea
            className="field-input w-full resize-none"
            onChange={(e) => onNotesChange(e.target.value)}
            placeholder={notesPlaceholder}
            rows={3}
            value={notes}
          />
        </div>

        <div className="mt-6 flex justify-end gap-3">
          <Button disabled={isPending} onClick={onClose} type="button" variant="outline">Batal</Button>
          <Button disabled={isPending} onClick={onConfirm} type="button">
            {isPending ? "Memproses..." : confirmLabel}
          </Button>
        </div>
      </Card>
    </div>
  );
}






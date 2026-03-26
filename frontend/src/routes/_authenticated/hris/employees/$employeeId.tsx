import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { CircleDollarSign, Gift, Plus } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { EmployeeForm } from "@/components/shared/employee-form";
import { EmptyState } from "@/components/shared/empty-state";
import { ExportButton } from "@/components/shared/export-button";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useRBAC } from "@/hooks/use-rbac";
import { ApiError } from "@/lib/api-client";
import { formatIDR } from "@/lib/currency";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  approveBonus,
  compensationKeys,
  createBonus,
  createSalary,
  deleteBonus,
  listBonuses,
  updateBonus,
  listSalaries,
  rejectBonus,
} from "@/services/hris-compensation";
import { departmentsKeys, listDepartments } from "@/services/hris-departments";
import { employeesKeys, getEmployee, updateEmployee, uploadEmployeeAvatar } from "@/services/hris-employees";
import { listReimbursements, reimbursementsKeys } from "@/services/hris-reimbursements";
import type {
  BonusFormValues,
  BonusRecord,
  Employee,
  EmployeeFormValues,
  Reimbursement,
  SalaryFormValues,
  SalaryRecord,
} from "@/types/hris";

const salarySchema = z.object({
  base_salary: z.coerce.number().min(1, "Gaji pokok wajib diisi"),
  allowances: z.string(),
  deductions: z.string(),
  effective_date: z.string().min(1, "Tanggal efektif wajib diisi"),
});

const bonusSchema = z.object({
  amount: z.coerce.number().min(1, "Jumlah bonus wajib diisi"),
  reason: z.string().min(3, "Alasan minimal 3 karakter"),
  period_month: z.coerce.number().min(1, "Bulan wajib diisi (1-12)").max(12, "Bulan maksimal 12"),
  period_year: z.coerce.number().min(2000, "Tahun minimal 2000").max(2100, "Tahun maksimal 2100"),
});

export const Route = createFileRoute("/_authenticated/hris/employees/$employeeId")({
  beforeLoad: async () => {
    await ensureModuleAccess("hris");
    await ensurePermission(permissions.hrisEmployeeView);
  },
  component: EmployeeDetailPage,
});

function EmployeeDetailPage() {
  const { employeeId } = Route.useParams();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<"profile" | "salary" | "bonus" | "reimbursements">("profile");
  const [isEditing, setIsEditing] = useState(false);
  const [pendingAvatarFile, setPendingAvatarFile] = useState<File | null>(null);
  const [isSalaryModalOpen, setIsSalaryModalOpen] = useState(false);
  const [isBonusModalOpen, setIsBonusModalOpen] = useState(false);
  const [editingBonus, setEditingBonus] = useState<BonusRecord | null>(null);
  const [bonusToDelete, setBonusToDelete] = useState<BonusRecord | null>(null);
  const [bonusToReject, setBonusToReject] = useState<BonusRecord | null>(null);

  const salaryForm = useForm<SalaryFormValues>({
    resolver: zodResolver(salarySchema) as never,
    defaultValues: {
      base_salary: 0,
      allowances: "",
      deductions: "",
      effective_date: "",
    },
  });

  const bonusForm = useForm<BonusFormValues>({
    resolver: zodResolver(bonusSchema) as never,
    defaultValues: {
      amount: 0,
      reason: "",
      period_month: new Date().getMonth() + 1,
      period_year: new Date().getFullYear(),
    },
  });

  const employeeQuery = useQuery({
    queryKey: employeesKeys.detail(employeeId),
    queryFn: () => getEmployee(employeeId),
  });

  const departmentsQuery = useQuery({
    queryKey: departmentsKeys.list(),
    queryFn: listDepartments,
  });

  const salaryHistoryQuery = useQuery({
    enabled: tab === "salary",
    queryKey: compensationKeys.salaries(employeeId),
    queryFn: () => listSalaries(employeeId),
  });

  const bonusesQuery = useQuery({
    enabled: tab === "bonus",
    queryKey: compensationKeys.bonuses(employeeId),
    queryFn: () => listBonuses(employeeId),
  });

  const reimbursementsQuery = useQuery({
    enabled: tab === "reimbursements",
    queryKey: reimbursementsKeys.list({
      page: 1,
      perPage: 20,
      status: "",
      employee: employeeId,
      month: "",
      year: "",
    }),
    queryFn: () =>
      listReimbursements({
        page: 1,
        perPage: 20,
        status: "",
        employee: employeeId,
        month: "",
        year: "",
      }),
  });

  const updateEmployeeMutation = useMutation({
    mutationFn: async ({ values, avatarFile }: { values: EmployeeFormValues; avatarFile: File | null }) => {
      const updatedEmployee = await updateEmployee(employeeId, values);
      if (!avatarFile) {
        return updatedEmployee;
      }

      return uploadEmployeeAvatar(employeeId, avatarFile);
    },
    onSuccess: async () => {
      setPendingAvatarFile(null);
      setIsEditing(false);
      await queryClient.invalidateQueries({ queryKey: employeesKeys.detail(employeeId) });
      await queryClient.invalidateQueries({ queryKey: employeesKeys.all });
    },
  });

  const createSalaryMutation = useMutation({
    mutationFn: (values: SalaryFormValues) => createSalary(employeeId, values),
    onSuccess: async () => {
      salaryForm.reset({
        base_salary: 0,
        allowances: "",
        deductions: "",
        effective_date: "",
      });
      setIsSalaryModalOpen(false);
      await queryClient.invalidateQueries({ queryKey: compensationKeys.currentSalary(employeeId) });
      await queryClient.invalidateQueries({ queryKey: compensationKeys.salaries(employeeId) });
    },
  });

  const createBonusMutation = useMutation({
    mutationFn: (values: BonusFormValues) => createBonus(employeeId, values),
    onSuccess: async () => {
      resetBonusForm(bonusForm);
      setIsBonusModalOpen(false);
      await queryClient.invalidateQueries({ queryKey: compensationKeys.bonuses(employeeId) });
    },
  });

  const updateBonusMutation = useMutation({
    mutationFn: ({ bonusId, values }: { bonusId: string; values: BonusFormValues }) => updateBonus(bonusId, values),
    onSuccess: async () => {
      setEditingBonus(null);
      resetBonusForm(bonusForm);
      setIsBonusModalOpen(false);
      await queryClient.invalidateQueries({ queryKey: compensationKeys.bonuses(employeeId) });
    },
  });

  const deleteBonusMutation = useMutation({
    mutationFn: deleteBonus,
    onSuccess: async (_, bonusId) => {
      if (editingBonus?.id === bonusId) {
        setEditingBonus(null);
        resetBonusForm(bonusForm);
      }
      setBonusToDelete(null);
      await queryClient.invalidateQueries({ queryKey: compensationKeys.bonuses(employeeId) });
    },
  });

  const approveBonusMutation = useMutation({
    mutationFn: approveBonus,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: compensationKeys.bonuses(employeeId) });
    },
  });

  const rejectBonusMutation = useMutation({
    mutationFn: rejectBonus,
    onSuccess: async () => {
      setBonusToReject(null);
      await queryClient.invalidateQueries({ queryKey: compensationKeys.bonuses(employeeId) });
    },
  });

  const employee = employeeQuery.data;

  if (employeeQuery.isLoading) {
    return (
      <Card className="space-y-4 p-8">
        <Skeleton className="h-8 w-56 rounded-lg" />
        <Skeleton className="h-5 w-80 rounded-lg" />
        <Skeleton className="h-32 rounded-lg" />
      </Card>
    );
  }

  if (employeeQuery.error instanceof Error || !employee) {
    return <Card className="p-8 text-error">{employeeQuery.error instanceof Error ? employeeQuery.error.message : "Employee not found"}</Card>;
  }

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex items-start gap-4">
            <ProtectedAvatar
              alt={employee.full_name}
              avatarUrl={employee.avatar_url}
              className="h-16 w-16 rounded-3xl border border-border/70 shadow-sm"
            />
            <div>
              <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">Employee profile</p>
              <h3 className="mt-2 text-3xl font-bold">{employee.full_name}</h3>
              <p className="mt-2 text-sm text-muted-foreground">
                {employee.position} | {employee.department || "No department"} | {employee.email}
              </p>
            </div>
          </div>

          <div className="flex flex-wrap gap-3">
            <PermissionGate permission={permissions.hrisSalaryView}>
              <ExportButton
                endpoint={`/hris/employees/${employeeId}/export`}
                filename={`employee-${employeeId}`}
                formats={["pdf"]}
              />
            </PermissionGate>
            <PermissionGate permission={permissions.hrisEmployeeEdit}>
              <Button onClick={() => setIsEditing(true)}>Edit profile</Button>
            </PermissionGate>
          </div>
        </div>
      </Card>

      <EmployeeForm
        avatarFile={pendingAvatarFile}
        defaultValues={toEmployeeFormValues(employee)}
        departments={departmentsQuery.data ?? []}
        description="Perbarui data profil karyawan tanpa meninggalkan halaman detail."
        existingAvatarPath={employee.avatar_url}
        isOpen={isEditing}
        isSubmitting={updateEmployeeMutation.isPending}
        onAvatarFileChange={setPendingAvatarFile}
        onCancel={() => {
          setPendingAvatarFile(null);
          setIsEditing(false);
        }}
        onSubmit={(values) => updateEmployeeMutation.mutate({ values, avatarFile: pendingAvatarFile })}
        submitLabel="Save changes"
        title="Edit employee"
      />

      <div className="flex flex-wrap gap-3">
        <Button onClick={() => setTab("profile")} variant={tab === "profile" ? "default" : "outline"}>Profile</Button>
        <Button onClick={() => setTab("salary")} variant={tab === "salary" ? "default" : "outline"}>Salary</Button>
        <Button onClick={() => setTab("bonus")} variant={tab === "bonus" ? "default" : "outline"}>Bonus</Button>
        <Button onClick={() => setTab("reimbursements")} variant={tab === "reimbursements" ? "default" : "outline"}>Reimbursements</Button>
      </div>

      {tab === "profile" ? <ProfileTab employee={employee} /> : null}

      {tab === "salary" ? (
        <PermissionGate
          fallback={<Card className="p-6 text-sm text-muted-foreground">Anda tidak punya akses melihat data salary.</Card>}
          permission={permissions.hrisSalaryView}
        >
          <SalaryTab
            createMutation={createSalaryMutation}
            currentSalary={salaryHistoryQuery.data?.[0]}
            currentSalaryError={salaryHistoryQuery.error}
            form={salaryForm}
            history={salaryHistoryQuery.data ?? []}
            isLoading={salaryHistoryQuery.isLoading}
            isModalOpen={isSalaryModalOpen}
            onModalOpenChange={setIsSalaryModalOpen}
          />
        </PermissionGate>
      ) : null}

      {tab === "bonus" ? (
        <PermissionGate
          fallback={<Card className="p-6 text-sm text-muted-foreground">Anda tidak punya akses melihat data bonus.</Card>}
          permission={permissions.hrisBonusView}
        >
          <BonusTab
            approveMutation={approveBonusMutation}
            bonuses={bonusesQuery.data ?? []}
            bonusForm={bonusForm}
            createMutation={createBonusMutation}
            deleteMutation={deleteBonusMutation}
            editingBonus={editingBonus}
            isModalOpen={isBonusModalOpen}
            rejectMutation={rejectBonusMutation}
            bonusToDelete={bonusToDelete}
            bonusToReject={bonusToReject}
            setEditingBonus={setEditingBonus}
            setBonusToDelete={setBonusToDelete}
            setBonusToReject={setBonusToReject}
            setIsModalOpen={setIsBonusModalOpen}
            updateMutation={updateBonusMutation}
          />
        </PermissionGate>
      ) : null}

      {tab === "reimbursements" ? (
        <ReimbursementsTab
          employeeName={employee.full_name}
          items={reimbursementsQuery.data?.items ?? []}
          loading={reimbursementsQuery.isLoading}
        />
      ) : null}
    </div>
  );
}

function ProfileTab({ employee }: { employee: Employee }) {
  const items = [
    { label: "Tipe kepegawaian", value: employee.position || "-" },
    { label: "Telepon", value: employee.phone || "-" },
    { label: "Departemen", value: employee.department || "-" },
    { label: "Status", value: employee.employment_status },
    { label: "Tanggal bergabung", value: new Date(employee.date_joined).toLocaleDateString() },
    { label: "Kontak darurat", value: employee.emergency_contact || "-" },
    { label: "Nomor rekening", value: employee.bank_account_number || "-" },
    { label: "Nama bank / E-Wallet", value: employee.bank_name || "-" },
    { label: "Profil LinkedIn", value: employee.linkedin_profile || "-" },
  ];

  return (
    <div className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
      <Card className="p-6">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Identity</p>
        <div className="mt-4 grid gap-4 md:grid-cols-2">
          {items.map((item) => (
            <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={item.label}>
              <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{item.label}</p>
              <div className="mt-2">
                {item.label === "Status" ? (
                  <StatusBadge status={item.value} variant="employee-status" />
                ) : (
                  <p className="text-sm font-semibold">{item.value}</p>
                )}
              </div>
            </div>
          ))}
        </div>
      </Card>

      <div className="space-y-6">
        <Card className="p-6">
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Address</p>
          <p className="mt-4 text-sm text-muted-foreground">{employee.address || "Belum ada alamat yang tercatat."}</p>
        </Card>

        <Card className="p-6">
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">SSH Keys</p>
          <pre className="mt-4 whitespace-pre-wrap break-all rounded-[16px] border border-border/70 bg-background/70 p-4 text-xs text-muted-foreground">
            {employee.ssh_keys || "Belum ada SSH key yang tercatat."}
          </pre>
        </Card>
      </div>
    </div>
  );
}

function SalaryTab({
  currentSalary,
  currentSalaryError,
  history,
  isLoading,
  form,
  createMutation,
  isModalOpen,
  onModalOpenChange,
}: {
  currentSalary?: SalaryRecord;
  currentSalaryError: unknown;
  history: SalaryRecord[];
  isLoading: boolean;
  form: ReturnType<typeof useForm<SalaryFormValues>>;
  createMutation: ReturnType<typeof useMutation<SalaryRecord, Error, SalaryFormValues>>;
  isModalOpen: boolean;
  onModalOpenChange: (value: boolean) => void;
}) {
  const {
    control,
    register,
    handleSubmit,
    formState: { errors },
  } = form;

  const noCurrentSalary = !isLoading && !currentSalary && !currentSalaryError;

  return (
    <div className="space-y-6">
      <Card className="p-6">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Current salary</p>
        <h4 className="mt-2 text-2xl font-bold">
          {currentSalary ? formatIDR(currentSalary.net_salary) : noCurrentSalary ? "Belum ada data salary" : "Loading..."}
        </h4>
        {currentSalary ? (
          <div className="mt-4 grid gap-3 md:grid-cols-3">
            <SalaryMetric label="Base salary" value={formatIDR(currentSalary.base_salary)} />
            <SalaryMetric label="Allowances" value={formatIDR(sumAmountMap(currentSalary.allowances))} />
            <SalaryMetric label="Deductions" value={formatIDR(sumAmountMap(currentSalary.deductions))} />
          </div>
        ) : null}
      </Card>

      <PermissionGate permission={permissions.hrisSalaryCreate}>
        <Card className="p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Add salary record</p>
              <p className="mt-2 text-sm text-muted-foreground">
                Create a new compensation snapshot without shifting the salary history below it.
              </p>
            </div>
            <Button onClick={() => onModalOpenChange(true)} type="button">
              <Plus className="h-4 w-4" />
              Add salary
            </Button>
          </div>
        </Card>
      </PermissionGate>

      <Card className="p-6">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Salary history</p>
        <div className="mt-4 space-y-3">
          {history.length === 0 ? (
            <EmptyState
              className="border-border/70"
              description="Salary records for this employee will appear here after the first compensation entry is saved."
              icon={CircleDollarSign}
              title="No salary history yet"
            />
          ) : (
            history.map((item) => (
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={item.id}>
                <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                  <div>
                    <p className="text-sm font-semibold">{formatIDR(item.net_salary)}</p>
                    <p className="text-xs text-muted-foreground">Effective {new Date(item.effective_date).toLocaleDateString()}</p>
                  </div>
                  <div className="text-sm text-muted-foreground">
                    Base {formatIDR(item.base_salary)} | Allowances {formatIDR(sumAmountMap(item.allowances))} | Deductions {formatIDR(sumAmountMap(item.deductions))}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </Card>

      <FormModal
        isLoading={createMutation.isPending}
        isOpen={isModalOpen}
        onClose={() => onModalOpenChange(false)}
        onSubmit={handleSubmit((values) => createMutation.mutate(values))}
        size="md"
        submitLabel="Add salary"
        title="Add salary record"
        subtitle="Store the latest salary composition and effective date for this employee."
      >
        <div className="grid gap-4 md:grid-cols-2">
          <Field error={errors.base_salary?.message} label="Gaji pokok" required>
            <Controller
              control={control}
              name="base_salary"
              render={({ field }) => (
                <CurrencyInput
                  onBlur={field.onBlur}
                  onValueChange={field.onChange}
                  ref={field.ref}
                  value={field.value}
                />
              )}
            />
          </Field>
          <Field error={errors.effective_date?.message} label="Tanggal efektif" required>
            <Input {...register("effective_date")} type="date" />
          </Field>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <Field
            error={errors.allowances?.message}
            hint="Format per baris: Transport: Rp 500.000"
            label="Tunjangan"
          >
            <textarea
              className="min-h-28 w-full rounded-[6px] border-[1.5px] border-transparent bg-surface-muted px-3 py-2 text-sm outline-none transition-all duration-150 placeholder:text-text-tertiary focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
              {...register("allowances")}
            />
          </Field>
          <Field
            error={errors.deductions?.message}
            hint="Format per baris: BPJS: Rp 250.000"
            label="Potongan"
          >
            <textarea
              className="min-h-28 w-full rounded-[6px] border-[1.5px] border-transparent bg-surface-muted px-3 py-2 text-sm outline-none transition-all duration-150 placeholder:text-text-tertiary focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
              {...register("deductions")}
            />
          </Field>
        </div>
      </FormModal>
    </div>
  );
}

function BonusTab({
  bonuses,
  bonusForm,
  createMutation,
  updateMutation,
  deleteMutation,
  approveMutation,
  rejectMutation,
  editingBonus,
  isModalOpen,
  bonusToDelete,
  bonusToReject,
  setEditingBonus,
  setBonusToDelete,
  setBonusToReject,
  setIsModalOpen,
}: {
  bonuses: BonusRecord[];
  bonusForm: ReturnType<typeof useForm<BonusFormValues>>;
  createMutation: ReturnType<typeof useMutation<BonusRecord, Error, BonusFormValues>>;
  updateMutation: ReturnType<typeof useMutation<BonusRecord, Error, { bonusId: string; values: BonusFormValues }>>;
  deleteMutation: ReturnType<typeof useMutation<{ message: string }, Error, string>>;
  approveMutation: ReturnType<typeof useMutation<BonusRecord, Error, string>>;
  rejectMutation: ReturnType<typeof useMutation<BonusRecord, Error, string>>;
  editingBonus: BonusRecord | null;
  setEditingBonus: (bonus: BonusRecord | null) => void;
  isModalOpen: boolean;
  bonusToDelete: BonusRecord | null;
  bonusToReject: BonusRecord | null;
  setBonusToDelete: (bonus: BonusRecord | null) => void;
  setBonusToReject: (bonus: BonusRecord | null) => void;
  setIsModalOpen: (value: boolean) => void;
}) {
  const { hasPermission } = useRBAC();
  const {
    control,
    register,
    handleSubmit,
    formState: { errors },
  } = bonusForm;

  return (
    <div className="space-y-6">
      {hasPermission(permissions.hrisBonusCreate) || (Boolean(editingBonus) && hasPermission(permissions.hrisBonusEdit)) ? (
        <Card className="p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Bonus records</p>
              <p className="mt-2 text-sm text-muted-foreground">
                Create or revise pending bonus requests in a focused dialog.
              </p>
            </div>
            <Button
              onClick={() => {
                setEditingBonus(null);
                resetBonusForm(bonusForm);
                setIsModalOpen(true);
              }}
              type="button"
            >
              <Plus className="h-4 w-4" />
              Add bonus
            </Button>
          </div>
        </Card>
      ) : null}

      <Card className="p-6">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Bonus history</p>
        <div className="mt-4 space-y-3">
          {bonuses.length === 0 ? (
            <EmptyState
              className="border-border/70"
              description="Bonus history will appear here after the first bonus record is added for this employee."
              icon={Gift}
              title="No bonus history yet"
            />
          ) : (
            bonuses.map((bonus) => (
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={bonus.id}>
                <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                  <div>
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-semibold">{formatIDR(bonus.amount)}</p>
                      <StatusBadge status={bonus.approval_status} />
                    </div>
                    <p className="mt-2 text-sm text-muted-foreground">{bonus.reason}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Period {bonus.period_month}/{bonus.period_year}
                    </p>
                  </div>

                  <div className="flex flex-wrap gap-3">
                    <PermissionGate permission={permissions.hrisBonusEdit}>
                      {bonus.approval_status === "pending" ? (
                        <>
                          <Button
                            onClick={() => {
                              setEditingBonus(bonus);
                              bonusForm.reset({
                                amount: bonus.amount,
                                reason: bonus.reason,
                                period_month: bonus.period_month,
                                period_year: bonus.period_year,
                              });
                              setIsModalOpen(true);
                            }}
                            size="sm"
                            type="button"
                            variant="outline"
                          >
                            Edit
                          </Button>
                          <Button
                            disabled={deleteMutation.isPending}
                            onClick={() => setBonusToDelete(bonus)}
                            size="sm"
                            type="button"
                            variant="ghost"
                          >
                            Delete
                          </Button>
                        </>
                      ) : null}
                    </PermissionGate>

                    <PermissionGate permission={permissions.hrisBonusApprove}>
                      {bonus.approval_status === "pending" ? (
                      <div className="flex gap-3">
                        <Button
                          disabled={approveMutation.isPending}
                          onClick={() => approveMutation.mutate(bonus.id)}
                          size="sm"
                          variant="outline"
                        >
                          Approve
                        </Button>
                        <Button
                          disabled={rejectMutation.isPending}
                          onClick={() => setBonusToReject(bonus)}
                          size="sm"
                          variant="ghost"
                        >
                          Reject
                        </Button>
                      </div>
                      ) : null}
                    </PermissionGate>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </Card>

      <FormModal
        isLoading={createMutation.isPending || updateMutation.isPending}
        isOpen={isModalOpen}
        onClose={() => {
          setIsModalOpen(false);
          setEditingBonus(null);
          resetBonusForm(bonusForm);
        }}
        onSubmit={handleSubmit((values) => {
          if (editingBonus) {
            updateMutation.mutate({ bonusId: editingBonus.id, values });
            return;
          }
          createMutation.mutate(values);
        })}
        size="md"
        submitLabel={editingBonus ? "Save bonus" : "Add bonus"}
        title={editingBonus ? "Edit bonus" : "Add bonus"}
        subtitle="Capture the amount, reason, and payout period before approval."
      >
        <div className="grid gap-4 md:grid-cols-2">
          <Field error={errors.amount?.message} label="Jumlah" required>
            <Controller
              control={control}
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
          </Field>
          <Field error={errors.reason?.message} label="Alasan" required>
            <Input {...register("reason")} placeholder="Project launch performance" />
          </Field>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <Field error={errors.period_month?.message} label="Bulan periode">
            <Input {...register("period_month", { valueAsNumber: true })} max={12} min={1} type="number" />
          </Field>
          <Field error={errors.period_year?.message} label="Tahun periode">
            <Input {...register("period_year", { valueAsNumber: true })} max={2100} min={2000} type="number" />
          </Field>
        </div>
      </FormModal>

      <ConfirmDialog
        confirmLabel="Delete bonus"
        description={bonusToDelete ? `Pending bonus "${bonusToDelete.reason}" will be removed.` : ""}
        isLoading={deleteMutation.isPending}
        isOpen={Boolean(bonusToDelete)}
        onClose={() => setBonusToDelete(null)}
        onConfirm={() => {
          if (bonusToDelete) {
            deleteMutation.mutate(bonusToDelete.id);
          }
        }}
        title={bonusToDelete ? `Delete bonus ${bonusToDelete.reason}?` : "Delete bonus?"}
      />

      <ConfirmDialog
        confirmLabel="Reject bonus"
        description={bonusToReject ? `Bonus "${bonusToReject.reason}" will be marked as rejected.` : ""}
        isLoading={rejectMutation.isPending}
        isOpen={Boolean(bonusToReject)}
        onClose={() => setBonusToReject(null)}
        onConfirm={() => {
          if (bonusToReject) {
            rejectMutation.mutate(bonusToReject.id);
          }
        }}
        title={bonusToReject ? `Reject bonus ${bonusToReject.reason}?` : "Reject bonus?"}
      />
    </div>
  );
}

function ReimbursementsTab({
  employeeName,
  items,
  loading,
}: {
  employeeName: string;
  items: Reimbursement[];
  loading: boolean;
}) {
  const columns: Array<DataTableColumn<Reimbursement>> = [
    {
      id: "title",
      header: "Title",
      accessor: "title",
      sortable: true,
      cell: (item) => (
        <div className="space-y-1">
          <p className="font-semibold text-text-primary">{item.title}</p>
          <p className="text-sm text-text-secondary">{item.category}</p>
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
      id: "date",
      header: "Transaction date",
      accessor: "transaction_date",
      sortable: true,
      cell: (item) => new Date(item.transaction_date).toLocaleDateString("id-ID"),
    },
    {
      id: "status",
      header: "Status",
      accessor: "status",
      sortable: true,
      cell: (item) => <StatusBadge status={item.status} variant="reimbursement-status" />,
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      cell: (item) => (
        <Link
          className={buttonVariants({ size: "sm", variant: "outline" })}
          params={{ reimbursementId: item.id }}
          to="/hris/reimbursements/$reimbursementId"
        >
          Open
        </Link>
      ),
    },
  ];

  return (
    <Card className="p-6">
      <div className="mb-5">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Reimbursements</p>
        <h4 className="mt-2 text-2xl font-bold">Reimbursement history</h4>
        <p className="mt-2 text-sm text-muted-foreground">All reimbursement requests submitted for {employeeName}.</p>
      </div>

      <DataTable
        columns={columns}
        data={items}
        emptyDescription="No reimbursement history has been recorded for this employee yet."
        emptyTitle="No reimbursements found"
        getRowId={(item) => item.id}
        loading={loading}
      />
    </Card>
  );
}

function SalaryMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
      <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{label}</p>
      <p className="mt-2 text-sm font-semibold">{value}</p>
    </div>
  );
}

function Field({
  label,
  error,
  hint,
  required,
  children,
}: {
  label: string;
  error?: string;
  hint?: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">
        {label}
        {required ? <span className="ml-0.5 text-priority-high">*</span> : null}
      </label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      {error ? <p className="text-sm text-error">{error}</p> : null}
    </div>
  );
}

function toEmployeeFormValues(employee: Employee): EmployeeFormValues {
  return {
    full_name: employee.full_name,
    email: employee.email,
    phone: employee.phone ?? "",
    position: employee.position,
    department: employee.department ?? "",
    date_joined: employee.date_joined.slice(0, 10),
    employment_status: employee.employment_status,
    address: employee.address ?? "",
    emergency_contact: employee.emergency_contact ?? "",
    avatar_url: employee.avatar_url ?? "",
    bank_account_number: employee.bank_account_number ?? "",
    bank_name: employee.bank_name ?? "",
    linkedin_profile: employee.linkedin_profile ?? "",
    ssh_keys: employee.ssh_keys ?? "",
  };
}

function sumAmountMap(values: Record<string, number>) {
  return Object.values(values).reduce((total, amount) => total + amount, 0);
}

function resetBonusForm(form: Pick<ReturnType<typeof useForm<BonusFormValues>>, "reset">) {
  form.reset({
    amount: 0,
    reason: "",
    period_month: new Date().getMonth() + 1,
    period_year: new Date().getFullYear(),
  });
}

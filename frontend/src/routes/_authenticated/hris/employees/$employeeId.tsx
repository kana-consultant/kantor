import { useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { EmployeeForm } from "@/components/shared/employee-form";
import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api-client";
import { formatIDR } from "@/lib/currency";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import {
  approveBonus,
  compensationKeys,
  createBonus,
  createSalary,
  getCurrentSalary,
  listBonuses,
  listSalaries,
  rejectBonus,
} from "@/services/hris-compensation";
import { departmentsKeys, listDepartments } from "@/services/hris-departments";
import { employeesKeys, getEmployee, updateEmployee } from "@/services/hris-employees";
import type {
  BonusFormValues,
  BonusRecord,
  Employee,
  EmployeeFormValues,
  SalaryFormValues,
  SalaryRecord,
} from "@/types/hris";

const salarySchema = z.object({
  base_salary: z.coerce.number().min(0, "Base salary harus 0 atau lebih"),
  allowances: z.string(),
  deductions: z.string(),
  effective_date: z.string().min(1, "Effective date wajib diisi"),
});

const bonusSchema = z.object({
  amount: z.coerce.number().min(0, "Bonus harus 0 atau lebih"),
  reason: z.string().min(3, "Reason minimal 3 karakter"),
  period_month: z.coerce.number().min(1).max(12),
  period_year: z.coerce.number().min(2000).max(2100),
});

export const Route = createFileRoute("/_authenticated/hris/employees/$employeeId")({
  beforeLoad: async () => {
    await ensurePermission(permissions.hrisEmployeeView);
  },
  component: EmployeeDetailPage,
});

function EmployeeDetailPage() {
  const { employeeId } = Route.useParams();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<"profile" | "salary" | "bonus" | "reimbursements">("profile");
  const [isEditing, setIsEditing] = useState(false);

  const salaryForm = useForm<SalaryFormValues>({
    resolver: zodResolver(salarySchema),
    defaultValues: {
      base_salary: 0,
      allowances: "",
      deductions: "",
      effective_date: "",
    },
  });

  const bonusForm = useForm<BonusFormValues>({
    resolver: zodResolver(bonusSchema),
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

  const currentSalaryQuery = useQuery({
    queryKey: compensationKeys.currentSalary(employeeId),
    queryFn: () => getCurrentSalary(employeeId),
  });

  const salaryHistoryQuery = useQuery({
    queryKey: compensationKeys.salaries(employeeId),
    queryFn: () => listSalaries(employeeId),
  });

  const bonusesQuery = useQuery({
    queryKey: compensationKeys.bonuses(employeeId),
    queryFn: () => listBonuses(employeeId),
  });

  const updateEmployeeMutation = useMutation({
    mutationFn: (values: EmployeeFormValues) => updateEmployee(employeeId, values),
    onSuccess: async () => {
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
      await queryClient.invalidateQueries({ queryKey: compensationKeys.currentSalary(employeeId) });
      await queryClient.invalidateQueries({ queryKey: compensationKeys.salaries(employeeId) });
    },
  });

  const createBonusMutation = useMutation({
    mutationFn: (values: BonusFormValues) => createBonus(employeeId, values),
    onSuccess: async () => {
      bonusForm.reset({
        amount: 0,
        reason: "",
        period_month: new Date().getMonth() + 1,
        period_year: new Date().getFullYear(),
      });
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
      await queryClient.invalidateQueries({ queryKey: compensationKeys.bonuses(employeeId) });
    },
  });

  const employee = employeeQuery.data;

  if (employeeQuery.isLoading) {
    return <Card className="p-8">Loading employee profile...</Card>;
  }

  if (employeeQuery.error instanceof Error || !employee) {
    return <Card className="p-8 text-red-700">{employeeQuery.error instanceof Error ? employeeQuery.error.message : "Employee not found"}</Card>;
  }

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex items-start gap-4">
            <div className="flex h-16 w-16 items-center justify-center rounded-3xl bg-primary/15 text-lg font-semibold text-primary">
              {initials(employee.full_name)}
            </div>
            <div>
              <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">Employee profile</p>
              <h3 className="mt-2 text-3xl font-bold">{employee.full_name}</h3>
              <p className="mt-2 text-sm text-muted-foreground">
                {employee.position} · {employee.department || "No department"} · {employee.email}
              </p>
            </div>
          </div>

          <PermissionGate permission={permissions.hrisEmployeeEdit}>
            <Button onClick={() => setIsEditing((value) => !value)}>{isEditing ? "Close edit" : "Edit profile"}</Button>
          </PermissionGate>
        </div>
      </Card>

      {isEditing ? (
        <EmployeeForm
          defaultValues={toEmployeeFormValues(employee)}
          departments={departmentsQuery.data ?? []}
          description="Perbarui data profil karyawan tanpa meninggalkan halaman detail."
          isSubmitting={updateEmployeeMutation.isPending}
          onCancel={() => setIsEditing(false)}
          onSubmit={(values) => updateEmployeeMutation.mutate(values)}
          submitLabel="Save changes"
          title="Edit employee"
        />
      ) : null}

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
            currentSalary={currentSalaryQuery.data}
            currentSalaryError={currentSalaryQuery.error}
            form={salaryForm}
            history={salaryHistoryQuery.data ?? []}
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
            rejectMutation={rejectBonusMutation}
          />
        </PermissionGate>
      ) : null}

      {tab === "reimbursements" ? (
        <Card className="p-6">
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Reimbursements</p>
          <h4 className="mt-2 text-2xl font-bold">Reimbursement history</h4>
          <p className="mt-2 text-sm text-muted-foreground">
            Placeholder untuk history reimbursement karyawan. Workflow reimbursement akan dibangun pada step HRIS berikutnya.
          </p>
        </Card>
      ) : null}
    </div>
  );
}

function ProfileTab({ employee }: { employee: Employee }) {
  const items = [
    { label: "Phone", value: employee.phone || "-" },
    { label: "Department", value: employee.department || "-" },
    { label: "Status", value: employee.employment_status },
    { label: "Date joined", value: new Date(employee.date_joined).toLocaleDateString() },
    { label: "Emergency contact", value: employee.emergency_contact || "-" },
    { label: "Avatar URL", value: employee.avatar_url || "-" },
  ];

  return (
    <div className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
      <Card className="p-6">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Identity</p>
        <div className="mt-4 grid gap-4 md:grid-cols-2">
          {items.map((item) => (
            <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={item.label}>
              <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{item.label}</p>
              <p className="mt-2 text-sm font-semibold">{item.value}</p>
            </div>
          ))}
        </div>
      </Card>

      <Card className="p-6">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Address</p>
        <p className="mt-4 text-sm text-muted-foreground">{employee.address || "Belum ada alamat yang tercatat."}</p>
      </Card>
    </div>
  );
}

function SalaryTab({
  currentSalary,
  currentSalaryError,
  history,
  form,
  createMutation,
}: {
  currentSalary?: SalaryRecord;
  currentSalaryError: unknown;
  history: SalaryRecord[];
  form: ReturnType<typeof useForm<SalaryFormValues>>;
  createMutation: ReturnType<typeof useMutation<SalaryRecord, Error, SalaryFormValues>>;
}) {
  const {
    control,
    register,
    handleSubmit,
    formState: { errors },
  } = form;

  const noCurrentSalary = currentSalaryError instanceof ApiError && currentSalaryError.status === 404;

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
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Add salary record</p>
          <form className="mt-4 space-y-4" onSubmit={handleSubmit((values) => createMutation.mutate(values))}>
            <div className="grid gap-4 md:grid-cols-2">
              <Field error={errors.base_salary?.message} label="Base salary">
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
              <Field error={errors.effective_date?.message} label="Effective date">
                <Input {...register("effective_date")} type="date" />
              </Field>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <Field
                error={errors.allowances?.message}
                hint="Format per baris: Transport: Rp 500.000"
                label="Allowances"
              >
                <textarea
                  className="min-h-28 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
                  {...register("allowances")}
                />
              </Field>
              <Field
                error={errors.deductions?.message}
                hint="Format per baris: BPJS: Rp 250.000"
                label="Deductions"
              >
                <textarea
                  className="min-h-28 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
                  {...register("deductions")}
                />
              </Field>
            </div>
            <Button disabled={createMutation.isPending} type="submit">
              {createMutation.isPending ? "Saving..." : "Add salary"}
            </Button>
          </form>
        </Card>
      </PermissionGate>

      <Card className="p-6">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Salary history</p>
        <div className="mt-4 space-y-3">
          {history.length === 0 ? (
            <div className="rounded-[22px] border border-dashed border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
              Belum ada riwayat salary.
            </div>
          ) : (
            history.map((item) => (
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={item.id}>
                <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                  <div>
                    <p className="text-sm font-semibold">{formatIDR(item.net_salary)}</p>
                    <p className="text-xs text-muted-foreground">Effective {new Date(item.effective_date).toLocaleDateString()}</p>
                  </div>
                  <div className="text-sm text-muted-foreground">
                    Base {formatIDR(item.base_salary)} · Allowances {formatIDR(sumAmountMap(item.allowances))} · Deductions {formatIDR(sumAmountMap(item.deductions))}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </Card>
    </div>
  );
}

function BonusTab({
  bonuses,
  bonusForm,
  createMutation,
  approveMutation,
  rejectMutation,
}: {
  bonuses: BonusRecord[];
  bonusForm: ReturnType<typeof useForm<BonusFormValues>>;
  createMutation: ReturnType<typeof useMutation<BonusRecord, Error, BonusFormValues>>;
  approveMutation: ReturnType<typeof useMutation<BonusRecord, Error, string>>;
  rejectMutation: ReturnType<typeof useMutation<BonusRecord, Error, string>>;
}) {
  const {
    control,
    register,
    handleSubmit,
    formState: { errors },
  } = bonusForm;

  return (
    <div className="space-y-6">
      <PermissionGate permission={permissions.hrisBonusCreate}>
        <Card className="p-6">
          <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Add bonus</p>
          <form className="mt-4 space-y-4" onSubmit={handleSubmit((values) => createMutation.mutate(values))}>
            <div className="grid gap-4 md:grid-cols-2">
              <Field error={errors.amount?.message} label="Amount">
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
              <Field error={errors.reason?.message} label="Reason">
                <Input {...register("reason")} placeholder="Project launch performance" />
              </Field>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <Field error={errors.period_month?.message} label="Period month">
                <Input {...register("period_month", { valueAsNumber: true })} max={12} min={1} type="number" />
              </Field>
              <Field error={errors.period_year?.message} label="Period year">
                <Input {...register("period_year", { valueAsNumber: true })} max={2100} min={2000} type="number" />
              </Field>
            </div>
            <Button disabled={createMutation.isPending} type="submit">
              {createMutation.isPending ? "Saving..." : "Add bonus"}
            </Button>
          </form>
        </Card>
      </PermissionGate>

      <Card className="p-6">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Bonus history</p>
        <div className="mt-4 space-y-3">
          {bonuses.length === 0 ? (
            <div className="rounded-[22px] border border-dashed border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
              Belum ada riwayat bonus.
            </div>
          ) : (
            bonuses.map((bonus) => (
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4" key={bonus.id}>
                <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                  <div>
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-semibold">{formatIDR(bonus.amount)}</p>
                      <span className="rounded-full bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-secondary-foreground">
                        {bonus.approval_status}
                      </span>
                    </div>
                    <p className="mt-2 text-sm text-muted-foreground">{bonus.reason}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Period {bonus.period_month}/{bonus.period_year}
                    </p>
                  </div>

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
                          onClick={() => rejectMutation.mutate(bonus.id)}
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
            ))
          )}
        </div>
      </Card>
    </div>
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
  children,
}: {
  label: string;
  error?: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">{label}</label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
    </div>
  );
}

function toEmployeeFormValues(employee: Employee): EmployeeFormValues {
  return {
    user_id: employee.user_id ?? "",
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
  };
}

function sumAmountMap(values: Record<string, number>) {
  return Object.values(values).reduce((total, amount) => total + amount, 0);
}

function initials(value: string) {
  return value
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

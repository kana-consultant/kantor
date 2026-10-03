import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Info, Lock, Plus, Save, Trash2, TriangleAlert } from "lucide-react";
import { Controller, useFieldArray, useForm, type FieldPath } from "react-hook-form";
import { z } from "zod";

import {
  contractDurationMonths,
  contractTypeDescriptions,
  contractTypeLabels,
  contractWorkModeLabels,
  endDateForMonths,
} from "@/components/contracts/contract-display";
import { DataLegalPanel } from "@/components/contracts/data-legal-panel";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { ApiError } from "@/lib/api-client";
import { permissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { contractsKeys, createContract, updateContract } from "@/services/hris-contracts";
import { departmentsKeys, listDepartments } from "@/services/hris-departments";
import { employeesKeys, getEmployee, listEmployees } from "@/services/hris-employees";
import { getHRProfile, hrProfileKeys } from "@/services/hris-hr-profile";
import { toast } from "@/stores/toast-store";
import type {
  ContractDetail,
  ContractFieldsPayload,
  ContractType,
  ContractWorkMode,
} from "@/types/contracts";

// Limits mirrored from backend/internal/dto/hris/contract.go.
const BENEFITS_MAX = 10;
const PRIOR_WORKS_MAX = 10;

const singleLine = (max: number) =>
  z
    .string()
    .trim()
    .max(max, `Maksimal ${max} karakter`)
    .refine((value) => !/[\r\n]/.test(value), "Hanya satu baris");

const wholeNumber = (min: number, max: number) =>
  z
    .number({ error: "Wajib diisi" })
    .int("Harus bilangan bulat")
    .min(min, `Minimal ${min}`)
    .max(max, `Maksimal ${max}`);

const benefitSchema = z.object({
  name: singleLine(80).pipe(z.string().min(1, "Nama benefit wajib diisi")),
  value: singleLine(60),
  notes: singleLine(80),
});

const priorWorkSchema = z.object({
  title: singleLine(100).pipe(z.string().min(1, "Judul karya wajib diisi")),
  description: singleLine(200),
  year: z
    .string()
    .trim()
    .refine((value) => value === "" || /^\d{4}$/.test(value), "Tahun 4 digit"),
});

const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

function buildSchema(mode: "create" | "edit", compensationVisible: boolean) {
  return z
    .object({
      employee_id: z.string(),
      contract_type: z.enum(["PKWT", "PKWTT", "MAGANG"]),
      is_record_only: z.boolean(),
      start_date: z.string().trim(),
      end_date: z.string().trim(),
      job_title: singleLine(120).pipe(z.string().min(1, "Jabatan wajib diisi")),
      department: singleLine(120),
      supervisor_name: singleLine(120),
      work_location: singleLine(200),
      work_mode: z.enum(["wfo", "hybrid", "remote"]),
      work_mode_detail: singleLine(120),
      pkwt_basis: singleLine(400),
      job_description: singleLine(600),
      work_days: singleLine(80),
      work_hours: singleLine(80),
      weekly_hours: wholeNumber(1, 60),
      notice_days: wholeNumber(0, 180),
      compensation_mode: z.enum(["auto", "manual"]),
      base_salary: z.number().int().min(0),
      fixed_allowance: z.number().int().min(0),
      benefits: z.array(benefitSchema).max(BENEFITS_MAX, `Maksimal ${BENEFITS_MAX} benefit`),
      incident_report_hours: wholeNumber(1, 720),
      non_solicit_months: wholeNumber(0, 60),
      confidentiality_years: wholeNumber(0, 30),
      prior_works: z.array(priorWorkSchema).max(PRIOR_WORKS_MAX, `Maksimal ${PRIOR_WORKS_MAX} karya`),
      document_date: z.string().trim(),
      document_city: singleLine(80),
    })
    .superRefine((values, ctx) => {
      if (mode === "create" && values.employee_id === "") {
        ctx.addIssue({ code: "custom", path: ["employee_id"], message: "Pilih karyawan" });
      }
      if (!DATE_PATTERN.test(values.start_date)) {
        ctx.addIssue({ code: "custom", path: ["start_date"], message: "Tanggal mulai wajib diisi" });
      }
      if (values.end_date !== "" && !DATE_PATTERN.test(values.end_date)) {
        ctx.addIssue({ code: "custom", path: ["end_date"], message: "Tanggal berakhir tidak valid" });
      }
      if (values.contract_type === "PKWT" && values.end_date === "") {
        ctx.addIssue({ code: "custom", path: ["end_date"], message: "PKWT wajib memiliki tanggal berakhir" });
      }
      if (values.end_date !== "" && DATE_PATTERN.test(values.start_date) && values.end_date < values.start_date) {
        ctx.addIssue({ code: "custom", path: ["end_date"], message: "Tanggal berakhir sebelum tanggal mulai" });
      }
      if (values.document_date !== "" && !DATE_PATTERN.test(values.document_date)) {
        ctx.addIssue({ code: "custom", path: ["document_date"], message: "Tanggal dokumen tidak valid" });
      }
      if (mode === "create" && compensationVisible && values.compensation_mode === "manual" && values.base_salary <= 0) {
        ctx.addIssue({ code: "custom", path: ["base_salary"], message: "Gaji pokok wajib diisi" });
      }
    });
}

type ContractFormValues = z.infer<ReturnType<typeof buildSchema>>;

const WORK_MODE_DETAIL_SUGGESTIONS = [
  "3 hari WFO, 2 hari WFH",
  "2 hari WFO, 3 hari WFH",
  "WFO setiap hari kerja",
  "Remote penuh dari domisili karyawan",
];

const BENEFIT_SUGGESTIONS: Array<{ name: string; value: string; notes: string }> = [
  { name: "Laptop kerja / Work laptop", value: "Pinjam pakai", notes: "Dikembalikan saat kontrak berakhir" },
  { name: "Tunjangan internet / Internet allowance", value: "", notes: "Dibayar bersama gaji" },
  { name: "BPJS Kesehatan / Health insurance", value: "Sesuai ketentuan", notes: "" },
  { name: "BPJS Ketenagakerjaan / Employment insurance", value: "Sesuai ketentuan", notes: "" },
];

function emptyValues(employeeId: string): ContractFormValues {
  return {
    employee_id: employeeId,
    contract_type: "PKWT",
    is_record_only: false,
    start_date: "",
    end_date: "",
    job_title: "",
    department: "",
    supervisor_name: "",
    work_location: "",
    work_mode: "wfo",
    work_mode_detail: "",
    pkwt_basis: "",
    job_description: "",
    work_days: "Senin–Jumat / Monday–Friday",
    work_hours: "09.00–18.00 WIB",
    weekly_hours: 40,
    notice_days: 30,
    compensation_mode: "auto",
    base_salary: 0,
    fixed_allowance: 0,
    benefits: [],
    incident_report_hours: 24,
    non_solicit_months: 12,
    confidentiality_years: 3,
    prior_works: [],
    document_date: "",
    document_city: "",
  };
}

function detailValues(detail: ContractDetail): ContractFormValues {
  return {
    employee_id: detail.employee_id,
    contract_type: detail.contract_type,
    is_record_only: detail.is_record_only,
    start_date: detail.start_date,
    end_date: detail.end_date ?? "",
    job_title: detail.job_title,
    department: detail.department ?? "",
    supervisor_name: detail.supervisor_name ?? "",
    work_location: detail.work_location,
    work_mode: detail.work_mode,
    work_mode_detail: detail.work_mode_detail ?? "",
    pkwt_basis: detail.pkwt_basis ?? "",
    job_description: detail.job_description,
    work_days: detail.work_days,
    work_hours: detail.work_hours,
    weekly_hours: detail.weekly_hours,
    notice_days: detail.notice_days,
    compensation_mode: "manual",
    base_salary: detail.compensation?.base_salary ?? 0,
    fixed_allowance: detail.compensation?.fixed_allowance ?? 0,
    benefits: (detail.benefits ?? []).map((item) => ({ ...item })),
    incident_report_hours: detail.incident_report_hours,
    non_solicit_months: detail.non_solicit_months,
    confidentiality_years: detail.confidentiality_years,
    prior_works: (detail.prior_works ?? []).map((item) => ({ ...item })),
    document_date: detail.document_date ?? "",
    document_city: detail.document_city ?? "",
  };
}

const optional = (value: string) => (value.trim() === "" ? null : value.trim());

function toPayload(
  values: ContractFormValues,
  options: { mode: "create" | "edit"; compensationVisible: boolean; detail?: ContractDetail },
): ContractFieldsPayload {
  const payload: ContractFieldsPayload = {
    contract_type: values.contract_type,
    is_record_only: values.contract_type === "PKWT" ? values.is_record_only : true,
    start_date: values.start_date,
    end_date: optional(values.end_date),
    job_title: values.job_title.trim(),
    department: optional(values.department),
    supervisor_name: optional(values.supervisor_name),
    work_location: values.work_location.trim(),
    work_mode: values.work_mode,
    work_mode_detail: optional(values.work_mode_detail),
    pkwt_basis: optional(values.pkwt_basis),
    job_description: values.job_description.trim(),
    work_days: optional(values.work_days),
    work_hours: optional(values.work_hours),
    weekly_hours: values.weekly_hours,
    notice_days: values.notice_days,
    benefits: values.benefits.map((item) => ({
      name: item.name.trim(),
      value: item.value.trim(),
      notes: item.notes.trim(),
    })),
    incident_report_hours: values.incident_report_hours,
    non_solicit_months: values.non_solicit_months,
    confidentiality_years: values.confidentiality_years,
    prior_works: values.prior_works.map((item) => ({
      title: item.title.trim(),
      description: item.description.trim(),
      year: item.year.trim(),
    })),
    // "" clears the stored city (the company city is used) and, while no
    // number is assigned, the stored date (the day of the first Generate).
    document_city: values.document_city.trim(),
  };
  if (values.document_date !== undefined) {
    payload.document_date = values.document_date;
  }
  if (options.compensationVisible) {
    const compensation = { base_salary: values.base_salary, fixed_allowance: values.fixed_allowance };
    if (options.mode === "create") {
      // "auto": omitted, the server prefills it from the salary in force at the start date.
      if (values.compensation_mode === "manual") {
        payload.compensation = compensation;
      }
    } else if (options.detail?.has_compensation || values.base_salary > 0 || values.fixed_allowance > 0) {
      payload.compensation = compensation;
    }
  }
  return payload;
}

// Server validation detail keys -> form fields.
const serverFieldMap: Record<string, FieldPath<ContractFormValues>> = {
  employee_id: "employee_id",
  contract_type: "contract_type",
  start_date: "start_date",
  end_date: "end_date",
  job_title: "job_title",
  document_date: "document_date",
  compensation: "base_salary",
  work_mode: "work_mode",
};

/**
 * The contract form shared by /hris/contracts/new and the detail page's
 * Edit / Revisi. Sections: 1 Karyawan, 2 Data Legal, 3 Posisi, 4 Jangka
 * Waktu ('Catat saja' forced on for PKWTT/Magang), 5 Jam Kerja, 6 Kompensasi
 * (only with hris:salary:view), 7 Benefit, 8 NDA & HKI, 9 Dokumen. A
 * record-only entry only needs sections 1, 3 and 4.
 */
export function ContractForm({
  mode,
  detail,
  initialEmployeeId = "",
  onSaved,
  onCancel,
}: {
  mode: "create" | "edit";
  detail?: ContractDetail;
  initialEmployeeId?: string;
  onSaved: (contract: ContractDetail) => void;
  onCancel: () => void;
}) {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const compensationVisible = hasPermission(permissions.hrisSalaryView);
  const canListDepartments = hasPermission(permissions.hrisDepartmentView);
  const schema = useMemo(() => buildSchema(mode, compensationVisible), [mode, compensationVisible]);

  const form = useForm<ContractFormValues>({
    resolver: zodResolver(schema),
    defaultValues: detail ? detailValues(detail) : emptyValues(initialEmployeeId),
  });
  const {
    control,
    formState: { errors, isSubmitting },
    getValues,
    handleSubmit,
    register,
    setError,
    setValue,
    watch,
  } = form;
  const benefits = useFieldArray({ control, name: "benefits" });
  const priorWorks = useFieldArray({ control, name: "prior_works" });

  const employeeId = watch("employee_id");
  const contractType = watch("contract_type");
  const recordOnly = watch("is_record_only") || contractType !== "PKWT";
  const startDate = watch("start_date");
  const endDate = watch("end_date");
  const workMode = watch("work_mode");
  const compensationMode = watch("compensation_mode");
  const numbered = Boolean(detail?.doc_number);
  const revising = mode === "edit" && detail?.status === "sent";

  // PKWTT and Magang never generate documents.
  useEffect(() => {
    if (contractType !== "PKWT" && !getValues("is_record_only")) {
      setValue("is_record_only", true);
    }
  }, [contractType, getValues, setValue]);

  const employeesQuery = useQuery({
    queryKey: employeesKeys.list({ page: 1, perPage: 100, search: "", department: "", status: "" }),
    queryFn: () => listEmployees({ page: 1, perPage: 100, search: "", department: "", status: "" }),
    enabled: mode === "create",
  });
  // The preselected employee (?employee=) is only shown once its option
  // exists: re-apply the value after the list loads.
  useEffect(() => {
    const current = getValues("employee_id");
    if (employeesQuery.data && current !== "") {
      setValue("employee_id", current);
    }
  }, [employeesQuery.data, getValues, setValue]);
  const departmentsQuery = useQuery({
    queryKey: departmentsKeys.list(),
    queryFn: listDepartments,
    enabled: mode === "create" && canListDepartments,
    retry: false,
  });
  const employeeQuery = useQuery({
    queryKey: employeesKeys.detail(employeeId),
    queryFn: () => getEmployee(employeeId),
    enabled: mode === "create" && employeeId !== "",
  });
  const profileQuery = useQuery({
    queryKey: hrProfileKeys.detail(employeeId),
    queryFn: () => getHRProfile(employeeId),
    enabled: mode === "create" && employeeId !== "",
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  });

  // Prefill jabatan (HR profile), departemen and atasan (department head)
  // once per picked employee, without overwriting what HR typed. A value
  // prefilled for a previously picked employee is replaced (or cleared).
  const prefilledFor = useRef<string | null>(null);
  const autoFilled = useRef<Partial<Record<"job_title" | "department" | "supervisor_name", string>>>({});
  useEffect(() => {
    if (mode !== "create" || employeeId === "" || prefilledFor.current === employeeId) {
      return;
    }
    const employee = employeeQuery.data;
    const profile = profileQuery.data;
    if (!employee || employee.id !== employeeId || !profile || profile.employee_id !== employeeId) {
      return;
    }
    if (canListDepartments && departmentsQuery.isLoading) {
      return;
    }
    prefilledFor.current = employeeId;
    const prefill = (field: "job_title" | "department" | "supervisor_name", value: string | null | undefined) => {
      const current = getValues(field).trim();
      if (current !== "" && current !== autoFilled.current[field]) {
        return; // typed by HR
      }
      const next = (value ?? "").trim();
      autoFilled.current[field] = next;
      if (next !== current) {
        setValue(field, next, { shouldDirty: true });
      }
    };
    prefill("job_title", profile.job_title);
    prefill("department", employee.department);
    const head = (departmentsQuery.data ?? []).find((item) => item.name === employee.department)?.head_name;
    prefill("supervisor_name", head !== employee.full_name ? head : "");
  }, [
    mode,
    employeeId,
    employeeQuery.data,
    profileQuery.data,
    departmentsQuery.data,
    departmentsQuery.isLoading,
    canListDepartments,
    getValues,
    setValue,
  ]);

  const mutation = useMutation({
    mutationFn: async (values: ContractFormValues) => {
      const payload = toPayload(values, { mode, compensationVisible, detail });
      if (mode === "create") {
        return createContract({ employee_id: values.employee_id, ...payload });
      }
      return updateContract(detail!.id, payload);
    },
    onSuccess: async (saved) => {
      queryClient.setQueryData(contractsKeys.detail(saved.id), saved);
      await queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "list"] });
      await queryClient.invalidateQueries({ queryKey: [...contractsKeys.all, "preflight"] });
      if (mode === "create") {
        toast.success("Kontrak dibuat", saved.is_record_only ? "Tersimpan sebagai catatan." : "Draf siap dilengkapi dan di-generate.");
      } else if (detail && saved.revision !== detail.revision) {
        toast.success(`Revisi ${saved.revision} disimpan`, "Status kembali Draf; generate ulang lalu kirim.");
      } else {
        toast.success("Perubahan kontrak disimpan");
      }
      onSaved(saved);
    },
    onError: (error) => {
      let mapped = false;
      if (error instanceof ApiError && error.details && typeof error.details === "object") {
        for (const key of Object.keys(error.details as Record<string, unknown>)) {
          const field = serverFieldMap[key];
          if (field) {
            setError(field, { message: error.message });
            mapped = true;
          }
        }
      }
      toast.error("Gagal menyimpan kontrak", mapped ? undefined : error instanceof Error ? error.message : undefined);
    },
  });

  const duration = startDate && endDate ? contractDurationMonths(startDate, endDate) : null;
  const employees = useMemo(
    () =>
      [...(employeesQuery.data?.items ?? [])].sort((left, right) => {
        const leftGone = left.employment_status === "resigned" || left.employment_status === "terminated";
        const rightGone = right.employment_status === "resigned" || right.employment_status === "terminated";
        return Number(leftGone) - Number(rightGone) || left.full_name.localeCompare(right.full_name, "id");
      }),
    [employeesQuery.data],
  );
  const pending = mutation.isPending || isSubmitting;
  const submit = handleSubmit((values) => mutation.mutate(values));
  // Visible sections are numbered in order (some are hidden for record-only
  // entries or without salary access).
  let sectionCount = 0;
  const nextSection = () => {
    sectionCount += 1;
    return sectionCount;
  };
  const submitLabel = mode === "create" ? "Simpan Kontrak" : revising ? "Simpan Revisi" : "Simpan Perubahan";

  return (
    // A div, not a form: the Data Legal panel holds its own forms (identity,
    // phone and bank), and nested forms would submit the contract too.
    <div className="space-y-6" data-testid="contract-form">
      {revising && detail ? (
        <Notice tone="warning">
          Kontrak ini sudah dikirim dan belum ditandatangani. Menyimpan perubahan membuat Revisi {detail.revision + 1}:
          status kembali Draf, nomor dokumen tetap, lalu generate ulang dan kirim (subjek email diberi &quot;(Revisi{" "}
          {detail.revision + 1})&quot;).
        </Notice>
      ) : null}

      <Section number={nextSection()} title="Karyawan">
        {mode === "create" ? (
          <Field error={errors.employee_id?.message} htmlFor="contract-employee" label="Karyawan">
            <select className="field-select w-full" disabled={pending} id="contract-employee" {...register("employee_id")}>
              <option value="">{employeesQuery.isLoading ? "Memuat karyawan..." : "Pilih karyawan"}</option>
              {employees.map((employee) => (
                <option key={employee.id} value={employee.id}>
                  {employee.full_name}
                  {employee.department ? ` · ${employee.department}` : ""}
                  {employee.employment_status !== "active" ? ` (${employee.employment_status})` : ""}
                </option>
              ))}
            </select>
            {employeesQuery.error instanceof Error ? (
              <p className="text-[12px] text-error">{employeesQuery.error.message}</p>
            ) : null}
          </Field>
        ) : (
          <p className="text-sm text-text-primary">
            <span className="font-semibold">{detail?.employee_name}</span>
            {detail?.employee_department ? <span className="text-text-secondary"> · {detail.employee_department}</span> : null}
          </p>
        )}
      </Section>

      {!recordOnly && employeeId ? (
        <Section number={nextSection()} title="Data Legal">
          {/* Keyed by employee: switching the employee closes (remounts) the
              inline identity / bank forms, which hold the previous employee's
              values. */}
          <DataLegalPanel employeeId={employeeId} key={employeeId} />
        </Section>
      ) : null}

      <Section number={nextSection()} title="Posisi">
        <div className="grid gap-4 md:grid-cols-2">
          <Field error={errors.job_title?.message} hint="Diisi dari jabatan di profil HR." htmlFor="contract-job-title" label="Jabatan">
            <Input disabled={pending} id="contract-job-title" maxLength={120} placeholder="Backend Engineer" {...register("job_title")} />
          </Field>
          <Field error={errors.department?.message} htmlFor="contract-department" label="Departemen">
            <Input disabled={pending} id="contract-department" maxLength={120} {...register("department")} />
          </Field>
          <Field
            error={errors.supervisor_name?.message}
            hint="Atasan langsung, dicetak di PKWT."
            htmlFor="contract-supervisor"
            label="Atasan langsung"
          >
            <Input disabled={pending} id="contract-supervisor" maxLength={120} placeholder="Nama atasan" {...register("supervisor_name")} />
          </Field>
          {!recordOnly ? (
            <Field error={errors.work_location?.message} htmlFor="contract-location" label="Lokasi kerja">
              <Input disabled={pending} id="contract-location" maxLength={200} placeholder="Jakarta" {...register("work_location")} />
            </Field>
          ) : null}
        </div>
        {!recordOnly ? (
          <>
            <div className="space-y-2">
              <p className="text-sm font-medium text-text-primary">Mode kerja</p>
              <Controller
                control={control}
                name="work_mode"
                render={({ field }) => (
                  <div className="flex flex-wrap gap-2" role="radiogroup">
                    {(Object.keys(contractWorkModeLabels) as ContractWorkMode[]).map((option) => (
                      <button
                        aria-checked={field.value === option}
                        className={cn(
                          "rounded-xl border px-3.5 py-2 text-[13px] font-semibold transition-colors",
                          field.value === option
                            ? "border-hr bg-hr-light text-hr"
                            : "border-border/70 bg-surface text-text-secondary hover:bg-surface-muted",
                        )}
                        disabled={pending}
                        key={option}
                        onClick={() => field.onChange(option)}
                        role="radio"
                        type="button"
                      >
                        {contractWorkModeLabels[option]}
                      </button>
                    ))}
                  </div>
                )}
              />
            </div>
            <Field error={errors.work_mode_detail?.message} htmlFor="contract-work-mode-detail" label="Rincian mode kerja">
              <Input
                disabled={pending}
                id="contract-work-mode-detail"
                list="contract-work-mode-suggestions"
                maxLength={120}
                placeholder={workMode === "hybrid" ? "3 hari WFO, 2 hari WFH" : workMode === "remote" ? "Remote penuh" : "WFO setiap hari kerja"}
                {...register("work_mode_detail")}
              />
              <datalist id="contract-work-mode-suggestions">
                {WORK_MODE_DETAIL_SUGGESTIONS.map((item) => (
                  <option key={item} value={item} />
                ))}
              </datalist>
            </Field>
            <Field
              error={errors.job_description?.message}
              hint="Satu kalimat dalam Bahasa Indonesia (Pasal 12.4: teks Bahasa Indonesia yang berlaku)."
              htmlFor="contract-job-description"
              label="Uraian tugas"
            >
              <Input
                disabled={pending}
                id="contract-job-description"
                maxLength={600}
                placeholder="merancang, membangun, dan memelihara layanan backend perusahaan"
                {...register("job_description")}
              />
            </Field>
            <Field
              error={errors.pkwt_basis?.message}
              hint="Alasan pekerjaan bersifat waktu tertentu, satu kalimat dalam Bahasa Indonesia."
              htmlFor="contract-pkwt-basis"
              label="Dasar PKWT"
            >
              <Input
                disabled={pending}
                id="contract-pkwt-basis"
                maxLength={400}
                placeholder="pekerjaan proyek yang penyelesaiannya diperkirakan dalam waktu tertentu"
                {...register("pkwt_basis")}
              />
            </Field>
          </>
        ) : null}
      </Section>

      <Section number={nextSection()} title="Jangka Waktu">
        <div className="grid gap-4 md:grid-cols-2">
          <Field
            error={errors.contract_type?.message}
            hint={numbered ? "Nomor dokumen sudah terbit; jenis kontrak terkunci." : contractTypeDescriptions[contractType]}
            htmlFor="contract-type"
            label="Jenis kontrak"
          >
            <select className="field-select w-full" disabled={pending || numbered} id="contract-type" {...register("contract_type")}>
              {(Object.keys(contractTypeLabels) as ContractType[]).map((option) => (
                <option key={option} value={option}>
                  {contractTypeLabels[option]} — {contractTypeDescriptions[option]}
                </option>
              ))}
            </select>
          </Field>
          <div className="space-y-2">
            <p className="text-sm font-medium text-text-primary">Dokumen</p>
            <label
              className={cn(
                "flex items-start gap-3 rounded-xl border border-border/70 px-3.5 py-3 text-sm",
                contractType !== "PKWT" || numbered ? "cursor-not-allowed opacity-80" : "cursor-pointer",
              )}
            >
              <input
                className="mt-0.5 h-4 w-4 accent-primary"
                disabled={pending || contractType !== "PKWT" || numbered}
                id="contract-record-only"
                type="checkbox"
                {...register("is_record_only")}
              />
              <span>
                <span className="font-semibold text-text-primary">Catat saja (tanpa dokumen)</span>
                <span className="block text-[12px] text-text-secondary">
                  {contractType !== "PKWT"
                    ? "Selalu aktif untuk PKWTT dan Magang: hanya dicatat untuk slip gaji dan riwayat."
                    : "Untuk kontrak kertas lama. Tidak membuat PKWT/NDA, tetap dihitung di batas 5 tahun PKWT."}
                </span>
              </span>
            </label>
          </div>
          <Field error={errors.start_date?.message} htmlFor="contract-start" label="Tanggal mulai">
            <Input disabled={pending} id="contract-start" type="date" {...register("start_date", { deps: ["end_date"] })} />
          </Field>
          <Field
            error={errors.end_date?.message}
            hint={contractType === "PKWT" ? undefined : "Opsional untuk PKWTT; isi untuk magang berjangka."}
            htmlFor="contract-end"
            label="Tanggal berakhir"
          >
            <Input disabled={pending} id="contract-end" min={startDate || undefined} type="date" {...register("end_date")} />
          </Field>
        </div>
        {contractType === "PKWT" ? (
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[12px] text-text-secondary">Isi cepat:</span>
            {[3, 6, 12, 24].map((months) => (
              <Button
                disabled={pending || !DATE_PATTERN.test(startDate)}
                key={months}
                onClick={() => setValue("end_date", endDateForMonths(startDate, months), { shouldDirty: true, shouldValidate: true })}
                size="xs"
                type="button"
                variant="outline"
              >
                {months} bulan
              </Button>
            ))}
            {duration ? (
              <span
                className={cn("ml-auto text-[13px] font-semibold", duration.days === 0 ? "text-text-primary" : "text-warning")}
                data-testid="contract-duration"
              >
                {duration.days === 0
                  ? `Durasi ${duration.months} bulan`
                  : `Durasi ${duration.months} bulan ${duration.days} hari (bukan bulan penuh)`}
              </span>
            ) : null}
          </div>
        ) : null}
      </Section>

      {!recordOnly ? (
        <>
          <Section number={nextSection()} title="Jam Kerja">
            <div className="grid gap-4 md:grid-cols-2">
              <Field error={errors.work_days?.message} htmlFor="contract-work-days" label="Hari kerja">
                <Input disabled={pending} id="contract-work-days" maxLength={80} {...register("work_days")} />
              </Field>
              <Field error={errors.work_hours?.message} htmlFor="contract-work-hours" label="Jam kerja">
                <Input disabled={pending} id="contract-work-hours" maxLength={80} {...register("work_hours")} />
              </Field>
              <Field error={errors.weekly_hours?.message} htmlFor="contract-weekly-hours" label="Jam per minggu">
                <Input
                  disabled={pending}
                  id="contract-weekly-hours"
                  max={60}
                  min={1}
                  type="number"
                  {...register("weekly_hours", { valueAsNumber: true })}
                />
              </Field>
              <Field
                error={errors.notice_days?.message}
                hint="Pemberitahuan perpanjangan atau pengakhiran sebelum kontrak berakhir."
                htmlFor="contract-notice-days"
                label="Masa pemberitahuan (hari)"
              >
                <Input
                  disabled={pending}
                  id="contract-notice-days"
                  max={180}
                  min={0}
                  type="number"
                  {...register("notice_days", { valueAsNumber: true })}
                />
              </Field>
            </div>
          </Section>

          {compensationVisible ? (
            <Section number={nextSection()} title="Kompensasi">
              {mode === "create" ? (
                <div className="flex flex-wrap gap-4 text-sm">
                  <label className="flex cursor-pointer items-center gap-2">
                    <input className="h-4 w-4 accent-primary" type="radio" value="auto" {...register("compensation_mode")} />
                    Ambil dari data gaji
                  </label>
                  <label className="flex cursor-pointer items-center gap-2">
                    <input className="h-4 w-4 accent-primary" type="radio" value="manual" {...register("compensation_mode")} />
                    Isi manual
                  </label>
                </div>
              ) : null}
              {mode === "create" && compensationMode === "auto" ? (
                <p className="rounded-xl bg-surface-muted px-3 py-2 text-[13px] text-text-secondary">
                  Gaji pokok dan tunjangan tetap diambil dari data gaji yang berlaku pada tanggal mulai. Angkanya bisa
                  diubah setelah draf tersimpan. Kontrak tidak mengubah data gaji.
                </p>
              ) : (
                <div className="grid gap-4 md:grid-cols-2">
                  <Field error={errors.base_salary?.message} htmlFor="contract-base-salary" label="Gaji pokok per bulan">
                    <Controller
                      control={control}
                      name="base_salary"
                      render={({ field }) => (
                        <CurrencyInput
                          disabled={pending}
                          id="contract-base-salary"
                          onBlur={field.onBlur}
                          onValueChange={field.onChange}
                          ref={field.ref}
                          value={field.value}
                        />
                      )}
                    />
                  </Field>
                  <Field error={errors.fixed_allowance?.message} htmlFor="contract-fixed-allowance" label="Tunjangan tetap per bulan">
                    <Controller
                      control={control}
                      name="fixed_allowance"
                      render={({ field }) => (
                        <CurrencyInput
                          disabled={pending}
                          id="contract-fixed-allowance"
                          onBlur={field.onBlur}
                          onValueChange={field.onChange}
                          ref={field.ref}
                          value={field.value}
                        />
                      )}
                    />
                  </Field>
                </div>
              )}
              {mode === "edit" && detail && !detail.has_compensation ? (
                <p className="text-[12px] text-warning">Belum ada kompensasi tersimpan; isi gaji pokok sebelum generate.</p>
              ) : null}
            </Section>
          ) : null}

          <Section
            action={
              <Button
                disabled={pending || benefits.fields.length >= BENEFITS_MAX}
                onClick={() => benefits.append({ name: "", value: "", notes: "" })}
                size="xs"
                type="button"
                variant="outline"
              >
                <Plus className="h-4 w-4" />
                Tambah benefit
              </Button>
            }
            number={nextSection()}
            title="Benefit"
          >
            {benefits.fields.length === 0 ? (
              <p className="rounded-xl bg-surface-muted px-3 py-2 text-[13px] text-text-secondary">
                Belum ada benefit; dokumen mencetak &quot;Tidak ada&quot;. Label singkat dwibahasa, mis. &quot;Laptop kerja /
                Work laptop&quot;.
              </p>
            ) : (
              <div className="space-y-3">
                {benefits.fields.map((field, index) => {
                  const rowErrors = errors.benefits?.[index];
                  return (
                    <div
                      className="grid gap-2 rounded-xl border border-border/70 bg-surface-muted/40 p-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,0.8fr)_minmax(0,1fr)_auto]"
                      data-testid="benefit-row"
                      key={field.id}
                    >
                      <div>
                        <Input
                          aria-label={`Nama benefit ${index + 1}`}
                          disabled={pending}
                          maxLength={80}
                          placeholder="Laptop kerja / Work laptop"
                          {...register(`benefits.${index}.name`)}
                        />
                        {rowErrors?.name?.message ? <p className="mt-1 text-[12px] text-error">{rowErrors.name.message}</p> : null}
                      </div>
                      <div>
                        <Input
                          aria-label={`Nilai benefit ${index + 1}`}
                          disabled={pending}
                          maxLength={60}
                          placeholder="Nilai, mis. Rp300.000/bulan"
                          {...register(`benefits.${index}.value`)}
                        />
                        {rowErrors?.value?.message ? <p className="mt-1 text-[12px] text-error">{rowErrors.value.message}</p> : null}
                      </div>
                      <div>
                        <Input
                          aria-label={`Keterangan benefit ${index + 1}`}
                          disabled={pending}
                          maxLength={80}
                          placeholder="Keterangan (opsional)"
                          {...register(`benefits.${index}.notes`)}
                        />
                        {rowErrors?.notes?.message ? <p className="mt-1 text-[12px] text-error">{rowErrors.notes.message}</p> : null}
                      </div>
                      <Button
                        aria-label={`Hapus benefit ${index + 1}`}
                        className="text-error hover:text-error"
                        disabled={pending}
                        onClick={() => benefits.remove(index)}
                        size="icon"
                        type="button"
                        variant="ghost"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  );
                })}
              </div>
            )}
            {benefits.fields.length < BENEFITS_MAX ? (
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-[12px] text-text-secondary">Contoh:</span>
                {BENEFIT_SUGGESTIONS.filter(
                  (suggestion) => !watch("benefits").some((item) => item.name === suggestion.name),
                ).map((suggestion) => (
                  <button
                    className="rounded-full border border-border/70 px-2.5 py-1 text-[12px] text-text-secondary transition-colors hover:bg-surface-muted"
                    disabled={pending}
                    key={suggestion.name}
                    onClick={() => benefits.append({ ...suggestion })}
                    type="button"
                  >
                    + {suggestion.name}
                  </button>
                ))}
              </div>
            ) : null}
          </Section>

          <Section number={nextSection()} title="NDA & HKI">
            <div className="grid gap-4 md:grid-cols-3">
              <Field
                error={errors.incident_report_hours?.message}
                hint="Batas waktu melapor kebocoran data."
                htmlFor="contract-incident-hours"
                label="Lapor insiden (jam)"
              >
                <Input
                  disabled={pending}
                  id="contract-incident-hours"
                  max={720}
                  min={1}
                  type="number"
                  {...register("incident_report_hours", { valueAsNumber: true })}
                />
              </Field>
              <Field
                error={errors.non_solicit_months?.message}
                hint="Larangan merekrut karyawan/klien setelah berakhir."
                htmlFor="contract-non-solicit"
                label="Non-solicitation (bulan)"
              >
                <Input
                  disabled={pending}
                  id="contract-non-solicit"
                  max={60}
                  min={0}
                  type="number"
                  {...register("non_solicit_months", { valueAsNumber: true })}
                />
              </Field>
              <Field
                error={errors.confidentiality_years?.message}
                hint="Kewajiban kerahasiaan setelah kontrak berakhir."
                htmlFor="contract-confidentiality"
                label="Kerahasiaan (tahun)"
              >
                <Input
                  disabled={pending}
                  id="contract-confidentiality"
                  max={30}
                  min={0}
                  type="number"
                  {...register("confidentiality_years", { valueAsNumber: true })}
                />
              </Field>
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between gap-2">
                <div>
                  <p className="text-sm font-medium text-text-primary">Karya terdahulu</p>
                  <p className="text-[12px] text-text-secondary">Karya milik karyawan yang dikecualikan dari pengalihan HKI (Pasal 6).</p>
                </div>
                <Button
                  disabled={pending || priorWorks.fields.length >= PRIOR_WORKS_MAX}
                  onClick={() => priorWorks.append({ title: "", description: "", year: "" })}
                  size="xs"
                  type="button"
                  variant="outline"
                >
                  <Plus className="h-4 w-4" />
                  Tambah karya
                </Button>
              </div>
              {priorWorks.fields.length === 0 ? (
                <p className="rounded-xl bg-surface-muted px-3 py-2 text-[13px] text-text-secondary">
                  Tidak ada karya terdahulu; dokumen mencetak &quot;Tidak ada&quot;.
                </p>
              ) : (
                priorWorks.fields.map((field, index) => {
                  const rowErrors = errors.prior_works?.[index];
                  return (
                    <div
                      className="grid gap-2 rounded-xl border border-border/70 bg-surface-muted/40 p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_100px_auto]"
                      data-testid="prior-work-row"
                      key={field.id}
                    >
                      <div>
                        <Input
                          aria-label={`Judul karya ${index + 1}`}
                          disabled={pending}
                          maxLength={100}
                          placeholder="Judul karya"
                          {...register(`prior_works.${index}.title`)}
                        />
                        {rowErrors?.title?.message ? <p className="mt-1 text-[12px] text-error">{rowErrors.title.message}</p> : null}
                      </div>
                      <div>
                        <Input
                          aria-label={`Deskripsi karya ${index + 1}`}
                          disabled={pending}
                          maxLength={200}
                          placeholder="Deskripsi singkat"
                          {...register(`prior_works.${index}.description`)}
                        />
                        {rowErrors?.description?.message ? (
                          <p className="mt-1 text-[12px] text-error">{rowErrors.description.message}</p>
                        ) : null}
                      </div>
                      <div>
                        <Input
                          aria-label={`Tahun karya ${index + 1}`}
                          disabled={pending}
                          inputMode="numeric"
                          maxLength={4}
                          placeholder="Tahun"
                          {...register(`prior_works.${index}.year`)}
                        />
                        {rowErrors?.year?.message ? <p className="mt-1 text-[12px] text-error">{rowErrors.year.message}</p> : null}
                      </div>
                      <Button
                        aria-label={`Hapus karya ${index + 1}`}
                        className="text-error hover:text-error"
                        disabled={pending}
                        onClick={() => priorWorks.remove(index)}
                        size="icon"
                        type="button"
                        variant="ghost"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  );
                })
              )}
            </div>
          </Section>

          <Section number={nextSection()} title="Dokumen">
            <div className="grid gap-4 md:grid-cols-2">
              <Field
                error={errors.document_date?.message}
                hint={
                  numbered
                    ? "Nomor dokumen sudah terbit dengan bulan tanggal ini; tanggal terkunci."
                    : "Kosongkan: tanggal saat Generate pertama (WIB). Bulannya menentukan nomor dokumen."
                }
                htmlFor="contract-document-date"
                label="Tanggal dokumen"
              >
                <Input disabled={pending || numbered} id="contract-document-date" type="date" {...register("document_date")} />
              </Field>
              <Field
                error={errors.document_city?.message}
                hint={detail?.company.city ? `Kosongkan untuk memakai kota perusahaan (${detail.company.city}).` : "Kosongkan untuk memakai kota perusahaan."}
                htmlFor="contract-document-city"
                label="Kota penandatanganan"
              >
                <Input disabled={pending} id="contract-document-city" maxLength={80} {...register("document_city")} />
              </Field>
            </div>
            {numbered && detail ? (
              <p className="flex items-center gap-2 text-[13px] text-text-secondary">
                <Lock className="h-3.5 w-3.5" />
                <span className="font-mono">{detail.doc_number}</span>
                <span>·</span>
                <span className="font-mono">{detail.nda_doc_number}</span>
              </p>
            ) : null}
          </Section>
        </>
      ) : (
        <Notice tone="info">
          Catatan tanpa dokumen hanya menyimpan jenis, periode, dan jabatan. Data ini dipakai untuk status kerja dan
          jabatan di slip gaji serta batas 5 tahun PKWT. Statusnya bisa langsung ditandai ditandatangani atau berakhir.
        </Notice>
      )}

      <div className="sticky bottom-0 z-10 -mx-1 flex flex-wrap items-center justify-end gap-2 rounded-2xl border border-border/70 bg-surface/95 px-4 py-3 shadow-[0_-12px_32px_-24px_rgba(15,23,42,0.4)] backdrop-blur">
        {Object.keys(errors).length > 0 ? (
          <p className="mr-auto flex items-center gap-1.5 text-[13px] text-error">
            <TriangleAlert className="h-4 w-4" />
            Periksa kembali isian yang ditandai.
          </p>
        ) : null}
        <Button disabled={pending} onClick={onCancel} type="button" variant="ghost">
          Batal
        </Button>
        <Button data-testid="contract-form-submit" disabled={pending} onClick={() => void submit()} type="button">
          <Save className="h-4 w-4" />
          {pending ? "Menyimpan..." : submitLabel}
        </Button>
      </div>
    </div>
  );
}

function Section({
  number,
  title,
  action,
  children,
}: {
  number: number;
  title: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Card className="space-y-4 p-6">
      <div className="flex items-center justify-between gap-3">
        <h3 className="flex items-center gap-2.5 text-[16px] font-[700] text-text-primary">
          <span className="flex h-6 w-6 items-center justify-center rounded-full bg-hr-light text-[12px] font-[700] text-hr">
            {number}
          </span>
          {title}
        </h3>
        {action}
      </div>
      {children}
    </Card>
  );
}

function Field({
  children,
  error,
  hint,
  htmlFor,
  label,
}: {
  children: ReactNode;
  error?: string;
  hint?: string;
  htmlFor: string;
  label: string;
}) {
  return (
    <div className="space-y-1.5">
      <label className="text-sm font-medium text-text-primary" htmlFor={htmlFor}>
        {label}
      </label>
      {children}
      {error ? <p className="text-[12px] text-error">{error}</p> : hint ? <p className="text-xs text-text-secondary">{hint}</p> : null}
    </div>
  );
}

function Notice({ tone, children }: { tone: "warning" | "info"; children: ReactNode }) {
  const Icon = tone === "warning" ? TriangleAlert : Info;
  return (
    <div
      className={
        tone === "warning"
          ? "flex items-start gap-2 rounded-xl border border-warning/30 bg-warning-light px-4 py-3 text-sm text-warning"
          : "flex items-start gap-2 rounded-xl border border-info/30 bg-info-light px-4 py-3 text-sm text-info"
      }
    >
      <Icon className="mt-0.5 h-4 w-4 shrink-0" />
      <p>{children}</p>
    </div>
  );
}

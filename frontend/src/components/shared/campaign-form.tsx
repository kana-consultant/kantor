import { useEffect, useMemo, useRef } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm, type FieldPath } from "react-hook-form";
import { z } from "zod";

import { CurrencyInput } from "@/components/ui/currency-input";
import { FieldError, RequiredMark, formHintClassName, formLabelClassName, formSubLabelClassName } from "@/components/shared/form-field";
import { FormModal } from "@/components/shared/form-modal";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { formatDateInputValue } from "@/lib/date";
import { campaignChannelOptions, campaignChannels, campaignStageOptions, campaignStatuses, describeMarketingError } from "@/lib/marketing";
import type { CampaignColumn, CampaignFormValues, CampaignPICOption } from "@/types/marketing";

// Limits mirror the backend DTO (dto/marketing/campaign.go).
const campaignFormSchema = z
  .object({
    name: z.string().trim().min(3, "Nama campaign minimal 3 karakter").max(180, "Nama campaign maksimal 180 karakter"),
    description: z.string().max(5000, "Deskripsi maksimal 5.000 karakter"),
    channel: z.enum(campaignChannels, { message: "Pilih kanal" }),
    budget_amount: z.number({ message: "Anggaran harus berupa angka" }).min(0, "Anggaran tidak boleh negatif"),
    budget_currency: z.string().trim().min(3).max(8),
    pic_employee_id: z.string(),
    start_date: z.string().min(1, "Tanggal mulai wajib diisi"),
    end_date: z.string().min(1, "Tanggal selesai wajib diisi"),
    brief_text: z.string().max(20000, "Teks brief maksimal 20.000 karakter"),
    status: z.enum(campaignStatuses, { message: "Pilih tahap" }),
  })
  .refine((value) => value.end_date >= value.start_date, {
    message: "Tanggal selesai tidak boleh sebelum tanggal mulai",
    path: ["end_date"],
  });

// Meta Ads is what the team runs most, so a new campaign starts there.
function createDefaultValues(status: CampaignFormValues["status"] = "planning"): CampaignFormValues {
  return {
    name: "",
    description: "",
    channel: "meta_ads",
    budget_amount: 0,
    budget_currency: "IDR",
    pic_employee_id: "",
    start_date: formatDateInputValue(),
    end_date: "",
    brief_text: "",
    status,
  };
}

// In the order the fields appear in the form.
const formFields = [
  "name",
  "channel",
  "status",
  "budget_amount",
  "pic_employee_id",
  "start_date",
  "end_date",
  "description",
  "brief_text",
] as const satisfies readonly FieldPath<CampaignFormValues>[];

interface CampaignFormProps {
  isOpen: boolean;
  title: string;
  description: string;
  submitLabel: string;
  picOptions: CampaignPICOption[];
  // True when the PIC list could not be loaded; the picker is then disabled
  // with a hint instead of silently offering only "Belum ada PIC".
  picOptionsUnavailable?: boolean;
  // The campaign's current PIC, so an edit never shows a blank PIC when that
  // person is missing from the options.
  currentPic?: { id: string; name: string } | null;
  // Board columns, to label the stage options with the lane names.
  columns?: CampaignColumn[];
  isSubmitting: boolean;
  // The last submit error (a mutation error); shown as a banner and, for
  // validation errors, next to the fields. The values typed stay in place.
  error?: unknown;
  defaultValues?: CampaignFormValues;
  // Stage a new campaign starts in (create from a lane); ignored when
  // defaultValues is given.
  defaultStatus?: CampaignFormValues["status"];
  onSubmit: (values: CampaignFormValues) => void;
  onCancel: () => void;
}

export function CampaignForm({
  isOpen,
  title,
  description,
  submitLabel,
  picOptions,
  picOptionsUnavailable = false,
  currentPic,
  columns,
  isSubmitting,
  error,
  defaultValues: initialValues,
  defaultStatus,
  onSubmit,
  onCancel,
}: CampaignFormProps) {
  const form = useForm<CampaignFormValues>({
    resolver: zodResolver(campaignFormSchema),
    defaultValues: initialValues ?? createDefaultValues(defaultStatus),
  });

  const {
    control,
    register,
    handleSubmit,
    reset,
    setError,
    watch,
    formState: { errors },
  } = form;

  // Reset only when the dialog opens. Re-renders of the parent (mutation
  // pending/error, background refetch) must never overwrite what the user
  // typed, so the initial values are read through a ref.
  const initialValuesRef = useRef(initialValues);
  initialValuesRef.current = initialValues;
  const defaultStatusRef = useRef(defaultStatus);
  defaultStatusRef.current = defaultStatus;
  const wasOpenRef = useRef(false);
  useEffect(() => {
    if (isOpen && !wasOpenRef.current) {
      reset(initialValuesRef.current ?? createDefaultValues(defaultStatusRef.current));
    }
    wasOpenRef.current = isOpen;
  }, [isOpen, reset]);

  const errorInfo = useMemo(() => (error ? describeMarketingError(error, columns) : null), [columns, error]);

  useEffect(() => {
    if (!errorInfo) {
      return;
    }
    // The first field the server rejected gets the focus (and with it the
    // scroll), so its message is on screen next to the field.
    let focused = false;
    for (const field of formFields) {
      const message = errorInfo.fieldErrors[field];
      if (message) {
        setError(field, { type: "server", message }, { shouldFocus: !focused });
        focused = true;
      }
    }
  }, [errorInfo, setError]);

  const startDate = watch("start_date");

  // Placeholders in text-secondary: text-tertiary is below 4.5:1.
  const inputClass = "placeholder:text-text-secondary aria-[invalid=true]:border-error";
  const selectTriggerClass = "aria-[invalid=true]:border-error";
  const labelClass = formLabelClassName;
  const required = <RequiredMark />;

  const picSelectOptions = useMemo(() => {
    const options = [
      { value: "", label: "Belum ada PIC" },
      ...picOptions.map((option) => ({
        value: option.id,
        label: option.full_name,
        description: option.position || undefined,
      })),
    ];
    if (currentPic && !picOptions.some((option) => option.id === currentPic.id)) {
      options.push({ value: currentPic.id, label: currentPic.name, description: undefined });
    }
    return options;
  }, [currentPic, picOptions]);

  const stageOptions = useMemo(() => campaignStageOptions(columns), [columns]);

  const errorText = (message: string | undefined, id: string) => <FieldError id={id} message={message} />;
  // The error id while the field has an error, so the control is described
  // by its message.
  const errorId = (field: keyof typeof errors, id: string) => (errors[field] ? id : undefined);

  return (
    <FormModal
      error={errorInfo?.message ?? null}
      isLoading={isSubmitting}
      isOpen={isOpen}
      onClose={onCancel}
      onSubmit={handleSubmit(onSubmit)}
      size="lg"
      submitLabel={submitLabel}
      title={title}
      subtitle={description}
    >
      <div className="flex flex-col gap-1.5">
        <label className={labelClass} htmlFor="campaign-name">
          Nama campaign{required}
        </label>
        <Input
          aria-describedby={errors.name ? "campaign-name-error" : undefined}
          aria-invalid={Boolean(errors.name)}
          className={inputClass}
          id="campaign-name"
          placeholder="Contoh: Promo akhir tahun - Traffic"
          {...register("name")}
        />
        {errorText(errors.name?.message, "campaign-name-error")}
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <label className={labelClass} htmlFor="campaign-channel">
            Kanal{required}
          </label>
          <Controller
            control={control}
            name="channel"
            render={({ field }) => (
              <Select
                aria-describedby={errorId("channel", "campaign-channel-error")}
                aria-invalid={Boolean(errors.channel)}
                id="campaign-channel"
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                options={campaignChannelOptions}
                ref={field.ref}
                triggerClassName={selectTriggerClass}
                value={field.value}
              />
            )}
          />
          {errorText(errors.channel?.message, "campaign-channel-error")}
        </div>

        <div className="flex flex-col gap-1.5">
          <label className={labelClass} htmlFor="campaign-status">
            Tahap{required}
          </label>
          <Controller
            control={control}
            name="status"
            render={({ field }) => (
              <Select
                aria-describedby={errorId("status", "campaign-status-error")}
                aria-invalid={Boolean(errors.status)}
                id="campaign-status"
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                options={stageOptions}
                ref={field.ref}
                triggerClassName={selectTriggerClass}
                value={field.value}
              />
            )}
          />
          {errorText(errors.status?.message, "campaign-status-error")}
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <label className={labelClass} htmlFor="campaign-budget">
            Anggaran
          </label>
          <Controller
            control={control}
            name="budget_amount"
            render={({ field }) => (
              <CurrencyInput
                aria-describedby={errors.budget_amount ? "campaign-budget-error" : "campaign-budget-hint"}
                aria-invalid={Boolean(errors.budget_amount)}
                className={inputClass}
                id="campaign-budget"
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                value={field.value}
                variant="field"
              />
            )}
          />
          {errors.budget_amount ? errorText(errors.budget_amount.message, "campaign-budget-error") : (
            <p className={formHintClassName} id="campaign-budget-hint">Isi 0 jika belum ada anggaran</p>
          )}
        </div>

        <div className="flex flex-col gap-1.5">
          <label className={labelClass} htmlFor="campaign-pic">
            PIC
          </label>
          <Controller
            control={control}
            name="pic_employee_id"
            render={({ field }) => (
              <Select
                aria-describedby={errors.pic_employee_id ? "campaign-pic-error" : picOptionsUnavailable ? "campaign-pic-hint" : undefined}
                aria-invalid={Boolean(errors.pic_employee_id)}
                disabled={picOptionsUnavailable && picOptions.length === 0}
                id="campaign-pic"
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                options={picSelectOptions}
                ref={field.ref}
                triggerClassName={selectTriggerClass}
                value={field.value}
              />
            )}
          />
          {errors.pic_employee_id ? errorText(errors.pic_employee_id.message, "campaign-pic-error") : picOptionsUnavailable ? (
            <p className={formHintClassName} id="campaign-pic-hint">Daftar PIC gagal dimuat. Campaign tetap bisa disimpan tanpa PIC.</p>
          ) : null}
        </div>
      </div>

      <fieldset className="flex min-w-0 flex-col gap-1.5">
        <legend className={`${labelClass} mb-1.5`}>
          Periode{required}
        </legend>
        {/* Same gap and breakpoint as the rows above, so the columns line up. */}
        <div className="grid gap-4 md:grid-cols-2">
          <div className="flex min-w-0 flex-col gap-1">
            <label className={formSubLabelClassName} htmlFor="campaign-start-date">
              Tanggal mulai
            </label>
            <Input
              aria-describedby={errors.start_date ? "campaign-start-date-error" : undefined}
              aria-invalid={Boolean(errors.start_date)}
              className={inputClass}
              id="campaign-start-date"
              type="date"
              {...register("start_date")}
            />
            {errorText(errors.start_date?.message, "campaign-start-date-error")}
          </div>
          <div className="flex min-w-0 flex-col gap-1">
            <label className={formSubLabelClassName} htmlFor="campaign-end-date">
              Tanggal selesai
            </label>
            <Input
              aria-describedby={errors.end_date ? "campaign-end-date-error" : undefined}
              aria-invalid={Boolean(errors.end_date)}
              className={inputClass}
              id="campaign-end-date"
              min={startDate || undefined}
              type="date"
              {...register("end_date")}
            />
            {errorText(errors.end_date?.message, "campaign-end-date-error")}
          </div>
        </div>
      </fieldset>

      <div className="flex flex-col gap-1.5">
        <label className={labelClass} htmlFor="campaign-description">
          Deskripsi
        </label>
        <Textarea
          aria-describedby={errorId("description", "campaign-description-error")}
          aria-invalid={Boolean(errors.description)}
          className={inputClass}
          id="campaign-description"
          placeholder="Tujuan utama, target audiens, dan konteks peluncuran"
          {...register("description")}
        />
        {errorText(errors.description?.message, "campaign-description-error")}
      </div>

      <div className="flex flex-col gap-1.5">
        <label className={labelClass} htmlFor="campaign-brief">
          Teks brief
        </label>
        <Textarea
          aria-describedby={errorId("brief_text", "campaign-brief-error")}
          aria-invalid={Boolean(errors.brief_text)}
          className={`${inputClass} min-h-[128px]`}
          id="campaign-brief"
          placeholder="Catatan copy, arahan aset, CTA, referensi landing page, atau checklist peluncuran"
          {...register("brief_text")}
        />
        {errorText(errors.brief_text?.message, "campaign-brief-error")}
      </div>
    </FormModal>
  );
}

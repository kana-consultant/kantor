import { useEffect } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { CurrencyInput } from "@/components/ui/currency-input";
import { FormModal } from "@/components/shared/form-modal";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { campaignChannelOptions, campaignStatusOptions } from "@/lib/marketing";
import type { Employee } from "@/types/hris";
import type { CampaignFormValues } from "@/types/marketing";
import { cn } from "@/lib/utils";

const campaignFormSchema = z
  .object({
    name: z.string().trim().min(3, "Nama campaign minimal 3 karakter").max(180),
    description: z.string(),
    channel: z.enum(["instagram", "facebook", "google_ads", "tiktok", "youtube", "email", "other"]),
    budget_amount: z.number().min(0, "Budget tidak boleh negatif"),
    budget_currency: z.string().trim().min(3).max(8),
    pic_employee_id: z.string(),
    start_date: z.string().min(1, "Tanggal mulai wajib diisi"),
    end_date: z.string().min(1, "Tanggal selesai wajib diisi"),
    brief_text: z.string(),
    status: z.enum(["ideation", "planning", "in_production", "live", "completed", "archived"]),
  })
  .refine((value) => value.end_date >= value.start_date, {
    message: "Tanggal selesai harus setelah tanggal mulai",
    path: ["end_date"],
  });

const defaultValues: CampaignFormValues = {
  name: "",
  description: "",
  channel: "instagram",
  budget_amount: 0,
  budget_currency: "IDR",
  pic_employee_id: "",
  start_date: "",
  end_date: "",
  brief_text: "",
  status: "planning",
};

interface CampaignFormProps {
  isOpen: boolean;
  title: string;
  description: string;
  submitLabel: string;
  employees: Employee[];
  isSubmitting: boolean;
  defaultValues?: CampaignFormValues;
  onSubmit: (values: CampaignFormValues) => void;
  onCancel: () => void;
}

export function CampaignForm({
  isOpen,
  title,
  description,
  submitLabel,
  employees,
  isSubmitting,
  defaultValues: initialValues,
  onSubmit,
  onCancel,
}: CampaignFormProps) {
  const form = useForm<CampaignFormValues>({
    resolver: zodResolver(campaignFormSchema) as never,
    defaultValues: initialValues ?? defaultValues,
  });

  const {
    control,
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = form;

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-marketing focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-marketing/10 disabled:cursor-not-allowed disabled:opacity-50";
  const textareaClass = cn(formControlClass, "h-auto py-3");
  const picOptions = [
    { value: "", label: "No PIC yet" },
    ...employees.map((employee) => ({
      value: employee.id,
      label: employee.full_name,
      description: employee.position,
    })),
  ];

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    reset(initialValues ?? defaultValues);
  }, [initialValues, isOpen, reset]);

  return (
    <FormModal
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
        <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-name">
          Nama campaign<span className="ml-0.5 text-priority-high">*</span>
        </label>
        <Input className="focus-visible:border-marketing focus-visible:ring-marketing/10" id="campaign-name" placeholder="Q2 retargeting push" {...register("name")} />
        {errors.name ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{errors.name.message}</p> : null}
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <label className="text-[13px] font-[500] text-text-secondary">Kanal<span className="ml-0.5 text-priority-high">*</span></label>
          <Controller
            control={control}
            name="channel"
            render={({ field }) => (
              <Select
                aria-label="Campaign channel"
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                options={campaignChannelOptions}
                triggerClassName="focus-visible:border-marketing focus-visible:ring-marketing/10"
                value={field.value}
              />
            )}
          />
          {errors.channel ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{errors.channel.message}</p> : null}
        </div>

        <div className="flex flex-col gap-1.5">
          <label className="text-[13px] font-[500] text-text-secondary">Tahap<span className="ml-0.5 text-priority-high">*</span></label>
          <Controller
            control={control}
            name="status"
            render={({ field }) => (
              <Select
                aria-label="Campaign stage"
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                options={campaignStatusOptions}
                triggerClassName="focus-visible:border-marketing focus-visible:ring-marketing/10"
                value={field.value}
              />
            )}
          />
          {errors.status ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{errors.status.message}</p> : null}
        </div>
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <label className="text-[13px] font-[500] text-text-secondary">Anggaran<span className="ml-0.5 text-priority-high">*</span></label>
          <Controller
            control={control}
            name="budget_amount"
            render={({ field }) => (
              <div className="rounded-[6px] transition-all focus-within:border-marketing focus-within:bg-surface focus-within:ring-4 focus-within:ring-marketing/10">
                <CurrencyInput onValueChange={field.onChange} value={field.value} />
              </div>
            )}
          />
          {errors.budget_amount ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{errors.budget_amount.message}</p> : null}
        </div>

        <div className="flex flex-col gap-1.5">
          <label className="text-[13px] font-[500] text-text-secondary">PIC</label>
          <Controller
            control={control}
            name="pic_employee_id"
            render={({ field }) => (
              <Select
                aria-label="Campaign PIC"
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                options={picOptions}
                triggerClassName="focus-visible:border-marketing focus-visible:ring-marketing/10"
                value={field.value}
              />
            )}
          />
        </div>
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-start-date">
            Tanggal mulai<span className="ml-0.5 text-priority-high">*</span>
          </label>
          <Input className="focus-visible:border-marketing focus-visible:ring-marketing/10" id="campaign-start-date" type="date" {...register("start_date")} />
          {errors.start_date ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{errors.start_date.message}</p> : null}
        </div>
        <div className="flex flex-col gap-1.5">
          <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-end-date">
            Tanggal selesai<span className="ml-0.5 text-priority-high">*</span>
          </label>
          <Input className="focus-visible:border-marketing focus-visible:ring-marketing/10" id="campaign-end-date" type="date" {...register("end_date")} />
          {errors.end_date ? <p className="mt-1 text-[12px] font-[500] text-priority-high">{errors.end_date.message}</p> : null}
        </div>
      </div>

      <div className="flex flex-col gap-1.5">
        <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-description">
          Deskripsi
        </label>
        <textarea
          className={cn(textareaClass, "min-h-[96px]")}
          id="campaign-description"
          placeholder="Main goal, positioning, target audience, and rollout context."
          {...register("description")}
        />
      </div>

      <div className="flex flex-col gap-1.5">
        <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-brief">
          Teks brief
        </label>
        <textarea
          className={cn(textareaClass, "min-h-[128px]")}
          id="campaign-brief"
          placeholder="Copy notes, asset direction, CTA, landing page references, or launch checklist."
          {...register("brief_text")}
        />
      </div>
    </FormModal>
  );
}

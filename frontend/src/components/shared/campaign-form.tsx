import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { CurrencyInput } from "@/components/ui/currency-input";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { campaignChannelOptions, campaignStatusOptions } from "@/lib/marketing";
import type { Employee } from "@/types/hris";
import type { CampaignFormValues } from "@/types/marketing";
import { cn } from "@/lib/utils";

const campaignFormSchema = z
  .object({
    name: z.string().trim().min(3, "Name must be at least 3 characters").max(180),
    description: z.string(),
    channel: z.enum(["instagram", "facebook", "google_ads", "tiktok", "youtube", "email", "other"]),
    budget_amount: z.number().min(0, "Budget must be zero or higher"),
    budget_currency: z.string().trim().min(3).max(8),
    pic_employee_id: z.string(),
    start_date: z.string().min(1, "Start date is required"),
    end_date: z.string().min(1, "End date is required"),
    brief_text: z.string(),
    status: z.enum(["ideation", "planning", "in_production", "live", "completed", "archived"]),
  })
  .refine((value) => value.end_date >= value.start_date, {
    message: "End date must be after start date",
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
    resolver: zodResolver(campaignFormSchema) as any,
    defaultValues: initialValues ?? defaultValues,
  });

  const {
    control,
    register,
    handleSubmit,
    formState: { errors },
  } = form;

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-marketing focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-marketing/10 disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <Card className="p-6">
      <div className="mb-6">
        <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-marketing mb-1">
          Campaign composer
        </p>
        <h4 className="text-[20px] font-[700] text-text-primary leading-tight">{title}</h4>
        <p className="mt-1 max-w-2xl text-[13px] text-text-secondary">{description}</p>
      </div>

      <form className="space-y-5" onSubmit={handleSubmit(onSubmit)}>
        <div className="grid gap-1.5 flex flex-col">
          <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-name">
            Campaign name
          </label>
          <Input className="focus-visible:border-marketing focus-visible:ring-marketing/10" id="campaign-name" placeholder="Q2 retargeting push" {...register("name")} />
          {errors.name ? <p className="text-[12px] text-priority-high mt-1 font-[500]">{errors.name.message}</p> : null}
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <div className="grid gap-1.5 flex flex-col">
            <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-channel">
              Channel
            </label>
            <select
              className={formControlClass}
              id="campaign-channel"
              {...register("channel")}
            >
              {campaignChannelOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>

          <div className="grid gap-1.5 flex flex-col">
            <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-status">
              Stage
            </label>
            <select
              className={formControlClass}
              id="campaign-status"
              {...register("status")}
            >
              {campaignStatusOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <div className="grid gap-1.5 flex flex-col">
            <label className="text-[13px] font-[500] text-text-secondary">Budget</label>
            <Controller
              control={control}
              name="budget_amount"
              render={({ field }) => (
                <div className="focus-within:ring-4 focus-within:ring-marketing/10 focus-within:border-marketing focus-within:bg-surface rounded-[6px] transition-all">
                  <CurrencyInput onValueChange={field.onChange} value={field.value} />
                </div>
              )}
            />
          </div>

          <div className="grid gap-1.5 flex flex-col">
            <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-pic">
              PIC
            </label>
            <select
              className={formControlClass}
              id="campaign-pic"
              {...register("pic_employee_id")}
            >
              <option value="">No PIC yet</option>
              {employees.map((employee) => (
                <option key={employee.id} value={employee.id}>
                  {employee.full_name} · {employee.position}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <div className="grid gap-1.5 flex flex-col">
            <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-start-date">
              Start date
            </label>
            <Input className="focus-visible:border-marketing focus-visible:ring-marketing/10" id="campaign-start-date" type="date" {...register("start_date")} />
          </div>
          <div className="grid gap-1.5 flex flex-col">
            <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-end-date">
              End date
            </label>
            <Input className="focus-visible:border-marketing focus-visible:ring-marketing/10" id="campaign-end-date" type="date" {...register("end_date")} />
            {errors.end_date ? <p className="text-[12px] text-priority-high mt-1 font-[500]">{errors.end_date.message}</p> : null}
          </div>
        </div>

        <div className="grid gap-1.5 flex flex-col">
          <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-description">
            Description
          </label>
          <textarea
            className={cn(formControlClass, "min-h-[96px] py-3")}
            id="campaign-description"
            placeholder="Main goal, positioning, target audience, and rollout context."
            {...register("description")}
          />
        </div>

        <div className="grid gap-1.5 flex flex-col">
          <label className="text-[13px] font-[500] text-text-secondary" htmlFor="campaign-brief">
            Brief text
          </label>
          <textarea
            className={cn(formControlClass, "min-h-[128px] py-3")}
            id="campaign-brief"
            placeholder="Copy notes, asset direction, CTA, landing page references, or launch checklist."
            {...register("brief_text")}
          />
        </div>

        <div className="flex flex-wrap gap-3 pt-2">
          <Button variant="mkt" disabled={isSubmitting} type="submit">
            {isSubmitting ? "Saving..." : submitLabel}
          </Button>
          <Button onClick={onCancel} type="button" variant="ghost">
            Cancel
          </Button>
        </div>
      </form>
    </Card>
  );
}

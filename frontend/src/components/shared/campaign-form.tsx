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
    resolver: zodResolver(campaignFormSchema),
    defaultValues: initialValues ?? defaultValues,
  });

  const {
    control,
    register,
    handleSubmit,
    formState: { errors },
  } = form;

  return (
    <Card className="p-6">
      <div className="border-b border-border/70 pb-5">
        <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Campaign composer</p>
        <h4 className="mt-2 text-2xl font-bold">{title}</h4>
        <p className="mt-2 max-w-2xl text-sm text-muted-foreground">{description}</p>
      </div>

      <form className="mt-6 space-y-5" onSubmit={handleSubmit(onSubmit)}>
        <div className="grid gap-2">
          <label className="text-sm font-medium" htmlFor="campaign-name">
            Campaign name
          </label>
          <Input id="campaign-name" placeholder="Q2 retargeting push" {...register("name")} />
          {errors.name ? <p className="text-sm text-red-700">{errors.name.message}</p> : null}
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="campaign-channel">
              Channel
            </label>
            <select
              className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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

          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="campaign-status">
              Stage
            </label>
            <select
              className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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

        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <label className="text-sm font-medium">Budget</label>
            <Controller
              control={control}
              name="budget_amount"
              render={({ field }) => (
                <CurrencyInput onValueChange={field.onChange} value={field.value} />
              )}
            />
          </div>

          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="campaign-pic">
              PIC
            </label>
            <select
              className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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

        <div className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="campaign-start-date">
              Start date
            </label>
            <Input id="campaign-start-date" type="date" {...register("start_date")} />
          </div>
          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="campaign-end-date">
              End date
            </label>
            <Input id="campaign-end-date" type="date" {...register("end_date")} />
            {errors.end_date ? <p className="text-sm text-red-700">{errors.end_date.message}</p> : null}
          </div>
        </div>

        <div className="grid gap-2">
          <label className="text-sm font-medium" htmlFor="campaign-description">
            Description
          </label>
          <textarea
            className="min-h-24 rounded-[24px] border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
            id="campaign-description"
            placeholder="Main goal, positioning, target audience, and rollout context."
            {...register("description")}
          />
        </div>

        <div className="grid gap-2">
          <label className="text-sm font-medium" htmlFor="campaign-brief">
            Brief text
          </label>
          <textarea
            className="min-h-32 rounded-[24px] border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
            id="campaign-brief"
            placeholder="Copy notes, asset direction, CTA, landing page references, or launch checklist."
            {...register("brief_text")}
          />
        </div>

        <div className="flex flex-wrap gap-3">
          <Button disabled={isSubmitting} type="submit">
            {isSubmitting ? "Saving..." : submitLabel}
          </Button>
          <Button onClick={onCancel} type="button" variant="outline">
            Cancel
          </Button>
        </div>
      </form>
    </Card>
  );
}

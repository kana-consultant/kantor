import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { ProjectFormValues } from "@/types/project";
import { cn } from "@/lib/utils";

const projectFormSchema = z.object({
  name: z.string().min(3, "Project name must contain at least 3 characters"),
  description: z.string(),
  deadline: z.string(),
  status: z.enum(["draft", "active", "on_hold", "completed", "archived"]),
  priority: z.enum(["low", "medium", "high", "critical"]),
});

interface ProjectFormProps {
  defaultValues?: ProjectFormValues;
  title: string;
  description: string;
  submitLabel: string;
  isSubmitting: boolean;
  onCancel?: () => void;
  onSubmit: (values: ProjectFormValues) => void;
}

const baseValues: ProjectFormValues = {
  name: "",
  description: "",
  deadline: "",
  status: "draft",
  priority: "medium",
};

export function ProjectForm({
  defaultValues,
  title,
  description,
  submitLabel,
  isSubmitting,
  onCancel,
  onSubmit,
}: ProjectFormProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<ProjectFormValues>({
    resolver: zodResolver(projectFormSchema),
    defaultValues: defaultValues ?? baseValues,
  });

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10 disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <Card className="p-6">
      <div className="mb-6">
        <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-ops mb-1">
          Project Form
        </p>
        <h3 className="text-[20px] font-[700] text-text-primary leading-tight">{title}</h3>
        <p className="mt-1 text-[13px] text-text-secondary">{description}</p>
      </div>

      <form className="space-y-5" onSubmit={handleSubmit(onSubmit)}>
        <Field error={errors.name?.message} label="Project name">
          <Input className="focus-visible:border-ops focus-visible:ring-ops/10" {...register("name")} placeholder="Q2 Operational Revamp" />
        </Field>

        <Field error={errors.description?.message} label="Description">
          <textarea
            className={cn(formControlClass, "min-h-[96px] py-3")}
            {...register("description")}
            placeholder="Short brief about objectives, scope, and owners."
          />
        </Field>

        <div className="grid gap-5 md:grid-cols-3">
          <Field error={errors.status?.message} label="Status">
            <select
              className={formControlClass}
              {...register("status")}
            >
              <option value="draft">Draft</option>
              <option value="active">Active</option>
              <option value="on_hold">On Hold</option>
              <option value="completed">Completed</option>
              <option value="archived">Archived</option>
            </select>
          </Field>

          <Field error={errors.priority?.message} label="Priority">
            <select
              className={formControlClass}
              {...register("priority")}
            >
              <option value="low">Low</option>
              <option value="medium">Medium</option>
              <option value="high">High</option>
              <option value="critical">Critical</option>
            </select>
          </Field>

          <Field error={errors.deadline?.message} label="Deadline">
            <Input className="focus-visible:border-ops focus-visible:ring-ops/10" {...register("deadline")} type="date" />
          </Field>
        </div>

        <div className="flex flex-wrap gap-3 pt-2">
          <Button variant="ops" disabled={isSubmitting} type="submit">
            {isSubmitting ? "Saving..." : submitLabel}
          </Button>
          {onCancel ? (
            <Button onClick={onCancel} type="button" variant="ghost">
              Cancel
            </Button>
          ) : null}
        </div>
      </form>
    </Card>
  );
}

interface FieldProps {
  label: string;
  error?: string;
  children: ReactNode;
}

function Field({ label, error, children }: FieldProps) {
  return (
    <div className="space-y-1.5 flex flex-col">
      <label className="text-[13px] font-[500] text-text-secondary">{label}</label>
      {children}
      {error ? <p className="text-[12px] text-priority-high mt-1 font-[500]">{error}</p> : null}
    </div>
  );
}

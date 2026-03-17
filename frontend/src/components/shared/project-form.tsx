import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { ProjectFormValues } from "@/types/project";

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

  return (
    <Card className="p-6">
      <div className="mb-6">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
          Project form
        </p>
        <h3 className="mt-2 text-2xl font-bold">{title}</h3>
        <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      </div>

      <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
        <Field error={errors.name?.message} label="Project name">
          <Input {...register("name")} placeholder="Q2 Operational Revamp" />
        </Field>

        <Field error={errors.description?.message} label="Description">
          <textarea
            className="min-h-28 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
            {...register("description")}
            placeholder="Short brief about objectives, scope, and owners."
          />
        </Field>

        <div className="grid gap-4 md:grid-cols-3">
          <Field error={errors.status?.message} label="Status">
            <select
              className="flex h-12 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
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
              className="flex h-12 w-full rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
              {...register("priority")}
            >
              <option value="low">Low</option>
              <option value="medium">Medium</option>
              <option value="high">High</option>
              <option value="critical">Critical</option>
            </select>
          </Field>

          <Field error={errors.deadline?.message} label="Deadline">
            <Input {...register("deadline")} type="date" />
          </Field>
        </div>

        <div className="flex flex-wrap gap-3">
          <Button disabled={isSubmitting} type="submit">
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
    <div className="space-y-2">
      <label className="text-sm font-medium">{label}</label>
      {children}
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
    </div>
  );
}

import { useEffect, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { permissions } from "@/lib/permissions";
import {
  assignmentRuleKeys,
  createAssignmentRule,
  deleteAssignmentRule,
  listAssignmentRules,
  updateAssignmentRule,
} from "@/services/operational-assignment-rules";
import type { AssignmentRule, AssignmentRuleFormValues } from "@/types/assignment";

const ruleSchema = z.object({
  rule_type: z.enum(["by_department", "by_skill", "by_workload"]),
  config_value: z.string(),
  role_in_project: z.string(),
  priority: z.coerce.number().min(1).max(1000),
  is_active: z.boolean(),
});

const emptyRuleForm: AssignmentRuleFormValues = {
  rule_type: "by_workload",
  config_value: "",
  role_in_project: "",
  priority: 1,
  is_active: true,
};

export function AssignmentRulesPanel({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [editingRule, setEditingRule] = useState<AssignmentRule | null>(null);
  const form = useForm<AssignmentRuleFormValues>({
    resolver: zodResolver(ruleSchema),
    defaultValues: emptyRuleForm,
  });

  const ruleType = form.watch("rule_type");

  const rulesQuery = useQuery({
    queryKey: assignmentRuleKeys.all(projectId),
    queryFn: () => listAssignmentRules(projectId),
  });

  const createMutation = useMutation({
    mutationFn: (values: AssignmentRuleFormValues) => createAssignmentRule(projectId, values),
    onSuccess: async () => {
      form.reset(emptyRuleForm);
      await queryClient.invalidateQueries({ queryKey: assignmentRuleKeys.all(projectId) });
    },
  });

  const updateMutation = useMutation({
    mutationFn: (values: AssignmentRuleFormValues) => {
      if (!editingRule) {
        throw new Error("Rule is not selected");
      }

      return updateAssignmentRule(projectId, editingRule.id, values);
    },
    onSuccess: async () => {
      setEditingRule(null);
      form.reset(emptyRuleForm);
      await queryClient.invalidateQueries({ queryKey: assignmentRuleKeys.all(projectId) });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (ruleId: string) => deleteAssignmentRule(projectId, ruleId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: assignmentRuleKeys.all(projectId) });
    },
  });

  useEffect(() => {
    if (!editingRule) {
      form.reset(emptyRuleForm);
      return;
    }

    form.reset(mapRuleToForm(editingRule));
  }, [editingRule, form]);

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
          Assignment settings
        </p>
        <h4 className="mt-2 text-2xl font-bold">Auto assign rules</h4>
        <p className="mt-3 max-w-3xl text-sm text-muted-foreground">
          Rule akan dievaluasi berdasarkan priority kecil ke besar. Untuk `by_workload`,
          backend memilih member project dengan workload paling rendah.
        </p>
      </Card>

      <PermissionGate
        fallback={
          <Card className="p-6 text-sm text-muted-foreground">
            You do not have permission to manage assignment rules.
          </Card>
        }
        permission={editingRule ? permissions.operationalAssignmentEdit : permissions.operationalAssignmentCreate}
      >
        <Card className="p-6">
          <form
            className="space-y-4"
            onSubmit={form.handleSubmit((values) => {
              if (editingRule) {
                updateMutation.mutate(values);
                return;
              }

              createMutation.mutate(values);
            })}
          >
            <div className="grid gap-4 md:grid-cols-2">
              <div className="grid gap-2">
                <label className="text-sm font-medium" htmlFor="rule-type">
                  Rule type
                </label>
                <select
                  className="h-12 rounded-2xl border border-input bg-card/80 px-4 py-3 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-ring"
                  id="rule-type"
                  {...form.register("rule_type")}
                >
                  <option value="by_workload">By workload</option>
                  <option value="by_department">By department</option>
                  <option value="by_skill">By skill</option>
                </select>
              </div>

              <div className="grid gap-2">
                <label className="text-sm font-medium" htmlFor="rule-priority">
                  Priority
                </label>
                <Input id="rule-priority" min={1} type="number" {...form.register("priority")} />
              </div>
            </div>

            {ruleType !== "by_workload" ? (
              <div className="grid gap-2">
                <label className="text-sm font-medium" htmlFor="rule-config-value">
                  {ruleType === "by_department" ? "Department" : "Skill"}
                </label>
                <Input
                  id="rule-config-value"
                  placeholder={ruleType === "by_department" ? "design" : "frontend"}
                  {...form.register("config_value")}
                />
              </div>
            ) : null}

            <div className="grid gap-4 md:grid-cols-[1fr_auto] md:items-end">
              <div className="grid gap-2">
                <label className="text-sm font-medium" htmlFor="rule-role">
                  Role in project
                </label>
                <Input
                  id="rule-role"
                  placeholder="Optional: designer, lead, qa"
                  {...form.register("role_in_project")}
                />
              </div>

              <label className="flex items-center gap-3 rounded-2xl border border-border/70 bg-background/70 px-4 py-3 text-sm font-medium">
                <input type="checkbox" {...form.register("is_active")} />
                Active
              </label>
            </div>

            <div className="flex gap-3">
              <Button disabled={createMutation.isPending || updateMutation.isPending} type="submit">
                {createMutation.isPending || updateMutation.isPending
                  ? "Saving..."
                  : editingRule
                    ? "Save rule"
                    : "Create rule"}
              </Button>
              {editingRule ? (
                <Button
                  onClick={() => setEditingRule(null)}
                  type="button"
                  variant="outline"
                >
                  Cancel
                </Button>
              ) : null}
            </div>
          </form>
        </Card>
      </PermissionGate>

      {rulesQuery.error instanceof Error ? (
        <Card className="p-6 text-sm text-red-700">{rulesQuery.error.message}</Card>
      ) : null}

      <div className="space-y-4">
        {rulesQuery.data?.map((rule) => (
          <Card className="p-6" key={rule.id}>
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
                  {rule.rule_type.replaceAll("_", " ")}
                </p>
                <h5 className="mt-2 text-xl font-semibold">Priority #{rule.priority}</h5>
                <p className="mt-3 text-sm text-muted-foreground">
                  {describeRule(rule)}
                </p>
              </div>

              <div className="flex flex-wrap gap-3">
                <span className="rounded-full bg-secondary px-3 py-2 text-xs font-semibold uppercase tracking-[0.18em] text-secondary-foreground">
                  {rule.is_active ? "active" : "inactive"}
                </span>
                <PermissionGate permission={permissions.operationalAssignmentEdit}>
                  <Button onClick={() => setEditingRule(rule)} size="sm" variant="outline">
                    Edit
                  </Button>
                </PermissionGate>
                <PermissionGate permission={permissions.operationalAssignmentDelete}>
                  <Button
                    disabled={deleteMutation.isPending}
                    onClick={() => {
                      if (window.confirm("Delete this assignment rule?")) {
                        deleteMutation.mutate(rule.id);
                      }
                    }}
                    size="sm"
                    variant="ghost"
                  >
                    Delete
                  </Button>
                </PermissionGate>
              </div>
            </div>
          </Card>
        ))}

        {rulesQuery.isLoading ? (
          <Card className="p-6">Loading assignment rules...</Card>
        ) : null}

        {rulesQuery.data?.length === 0 ? (
          <Card className="p-6 text-sm text-muted-foreground">
            No assignment rules yet. Create one to enable auto assign from the Kanban board.
          </Card>
        ) : null}
      </div>
    </div>
  );
}

function mapRuleToForm(rule: AssignmentRule): AssignmentRuleFormValues {
  return {
    rule_type: rule.rule_type,
    config_value:
      String(rule.rule_config.department ?? rule.rule_config.skill ?? ""),
    role_in_project: String(rule.rule_config.role_in_project ?? ""),
    priority: rule.priority,
    is_active: rule.is_active,
  };
}

function describeRule(rule: AssignmentRule) {
  const roleText = rule.rule_config.role_in_project
    ? ` for role "${String(rule.rule_config.role_in_project)}"`
    : "";

  if (rule.rule_type === "by_department") {
    return `Assign the first matching member from department "${String(rule.rule_config.department ?? "-")}"${roleText}.`;
  }

  if (rule.rule_type === "by_skill") {
    return `Assign the first matching member with skill "${String(rule.rule_config.skill ?? "-")}"${roleText}.`;
  }

  return `Assign the member with the lowest workload${roleText}.`;
}

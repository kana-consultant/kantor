import { useEffect, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Plus } from "lucide-react";

import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { EmptyState } from "@/components/shared/empty-state";
import { FormModal } from "@/components/shared/form-modal";
import { PermissionGate } from "@/components/shared/permission-gate";
import { StatusBadge } from "@/components/shared/status-badge";
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
  const [isRuleModalOpen, setIsRuleModalOpen] = useState(false);
  const [ruleToDelete, setRuleToDelete] = useState<AssignmentRule | null>(null);
  const form = useForm<AssignmentRuleFormValues>({
    resolver: zodResolver(ruleSchema) as any,
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

  const formControlClass = "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10 disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <div className="space-y-6">
      <Card className="p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-ops mb-1">
          Assignment settings
            </p>
            <h4 className="text-[20px] font-[700] text-text-primary leading-tight">Auto assign rules</h4>
            <p className="mt-2 max-w-3xl text-[13px] text-text-secondary">
              Rules are evaluated from the lowest priority number upward. Workload rules choose the member with the lightest task load.
            </p>
          </div>
          <PermissionGate permission={permissions.operationalAssignmentCreate}>
            <Button
              onClick={() => {
                setEditingRule(null);
                form.reset(emptyRuleForm);
                setIsRuleModalOpen(true);
              }}
              variant="ops"
            >
              <Plus className="h-4 w-4" />
              Add rule
            </Button>
          </PermissionGate>
        </div>
      </Card>

      <PermissionGate
        fallback={
          <Card className="p-6 text-[13px] font-[500] text-text-tertiary">
            You do not have permission to manage assignment rules.
          </Card>
        }
        permission={editingRule ? permissions.operationalAssignmentEdit : permissions.operationalAssignmentCreate}
      >
        <FormModal
          isLoading={createMutation.isPending || updateMutation.isPending}
          isOpen={isRuleModalOpen}
          onClose={() => {
            setIsRuleModalOpen(false);
            setEditingRule(null);
            form.reset(emptyRuleForm);
          }}
          onSubmit={form.handleSubmit((values) => {
            if (editingRule) {
              updateMutation.mutate(values);
              return;
            }

            createMutation.mutate(values);
          })}
          size="md"
          submitLabel={editingRule ? "Save rule" : "Create rule"}
          title={editingRule ? "Edit assignment rule" : "Create assignment rule"}
          subtitle="Choose the matching strategy, optional project role, and the order this rule should be evaluated."
        >
            <div className="grid gap-5 md:grid-cols-2">
              <div className="grid gap-1.5">
                <label className="text-[13px] font-[500] text-text-secondary" htmlFor="rule-type">
                  Rule type
                </label>
                <select
                  className={formControlClass}
                  id="rule-type"
                  {...form.register("rule_type")}
                >
                  <option value="by_workload">By workload</option>
                  <option value="by_department">By department</option>
                  <option value="by_skill">By skill</option>
                </select>
              </div>

              <div className="grid gap-1.5">
                <label className="text-[13px] font-[500] text-text-secondary" htmlFor="rule-priority">
                  Priority
                </label>
                <Input className="focus-visible:border-ops focus-visible:ring-ops/10" id="rule-priority" min={1} type="number" {...form.register("priority")} />
              </div>
            </div>

            {ruleType !== "by_workload" ? (
              <div className="grid gap-1.5">
                <label className="text-[13px] font-[500] text-text-secondary" htmlFor="rule-config-value">
                  {ruleType === "by_department" ? "Department" : "Skill"}
                </label>
                <Input
                  className="focus-visible:border-ops focus-visible:ring-ops/10"
                  id="rule-config-value"
                  placeholder={ruleType === "by_department" ? "design" : "frontend"}
                  {...form.register("config_value")}
                />
              </div>
            ) : null}

            <div className="grid gap-5 md:grid-cols-[1fr_auto] md:items-end">
              <div className="grid gap-1.5">
                <label className="text-[13px] font-[500] text-text-secondary" htmlFor="rule-role">
                  Role in project
                </label>
                <Input
                  className="focus-visible:border-ops focus-visible:ring-ops/10"
                  id="rule-role"
                  placeholder="Optional: designer, lead, qa"
                  {...form.register("role_in_project")}
                />
              </div>

              <label className="flex items-center gap-3 rounded-[6px] border border-border bg-surface-muted px-4 h-[44px] text-[13px] font-[600] text-text-primary cursor-pointer hover:bg-border/50 transition-colors">
                <input className="w-4 h-4 rounded text-ops focus:ring-ops" type="checkbox" {...form.register("is_active")} />
                Active Rule
              </label>
            </div>
        </FormModal>
      </PermissionGate>

      {rulesQuery.error instanceof Error ? (
        <Card className="p-6 text-[13px] font-[500] text-priority-high border-priority-high/20 bg-priority-high/5">{rulesQuery.error.message}</Card>
      ) : null}

      <div className="space-y-4">
        {rulesQuery.data?.map((rule) => (
          <Card className="p-5 transition hover:shadow-card" key={rule.id}>
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-text-secondary">
                  {rule.rule_type.replaceAll("_", " ")}
                </p>
                <h5 className="mt-1 text-[16px] font-[600] text-text-primary">Priority #{rule.priority}</h5>
                <p className="mt-2 text-[13px] text-text-secondary">
                  {describeRule(rule)}
                </p>
              </div>

              <div className="flex items-center gap-3">
                <StatusBadge status={rule.is_active ? "active" : "inactive"} />
                <PermissionGate permission={permissions.operationalAssignmentEdit}>
                  <Button
                    onClick={() => {
                      setEditingRule(rule);
                      setIsRuleModalOpen(true);
                    }}
                    size="sm"
                    variant="secondary"
                  >
                    Edit
                  </Button>
                </PermissionGate>
                <PermissionGate permission={permissions.operationalAssignmentDelete}>
                  <Button
                    disabled={deleteMutation.isPending}
                    onClick={() => setRuleToDelete(rule)}
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
          <Card className="p-6 text-[13px] text-text-secondary font-[500]">Loading assignment rules...</Card>
        ) : null}

        {rulesQuery.data?.length === 0 ? (
          <EmptyState
            actionLabel="Add rule"
            description="Create the first assignment rule to enable auto-assign actions from the Kanban board."
            onAction={() => {
              setEditingRule(null);
              form.reset(emptyRuleForm);
              setIsRuleModalOpen(true);
            }}
            title="No assignment rules yet"
          />
        ) : null}
      </div>

      <ConfirmDialog
        confirmLabel="Delete rule"
        description={ruleToDelete ? `Rule priority #${ruleToDelete.priority} will be removed from this project.` : ""}
        isLoading={deleteMutation.isPending}
        isOpen={Boolean(ruleToDelete)}
        onClose={() => setRuleToDelete(null)}
        onConfirm={() => {
          if (ruleToDelete) {
            deleteMutation.mutate(ruleToDelete.id);
          }
        }}
        title={ruleToDelete ? `Delete ${ruleToDelete.rule_type.replaceAll("_", " ")} rule?` : "Delete rule?"}
      />
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

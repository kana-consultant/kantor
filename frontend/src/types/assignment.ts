import type { KanbanTask } from "@/types/kanban";

export type AssignmentRuleType = "by_department" | "by_skill" | "by_workload";

export interface AssignmentRule {
  id: string;
  project_id: string;
  rule_type: AssignmentRuleType;
  rule_config: Record<string, unknown>;
  priority: number;
  is_active: boolean;
  created_by: string;
  created_at: string;
}

export interface AssignmentRuleFormValues {
  rule_type: AssignmentRuleType;
  config_value: string;
  role_in_project: string;
  priority: number;
  is_active: boolean;
}

export interface AutoAssignResult {
  task: KanbanTask;
  matched_rule: AssignmentRule;
  assigned_to: {
    user_id: string;
    full_name: string;
    email: string;
    role_in_project: string;
    workload: number;
  };
}

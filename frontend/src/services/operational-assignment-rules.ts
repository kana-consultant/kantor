import { authRequestJSON } from "@/lib/api-client";
import type { AssignmentRule, AssignmentRuleFormValues, AutoAssignResult } from "@/types/assignment";

export const assignmentRuleKeys = {
  all: (projectId: string) => ["operational", "projects", projectId, "assignment-rules"] as const,
};

export async function listAssignmentRules(projectId: string) {
  return authRequestJSON<AssignmentRule[]>(
    `/operational/projects/${projectId}/assignment-rules`,
    { method: "GET" },
  );
}

export async function createAssignmentRule(projectId: string, input: AssignmentRuleFormValues) {
  return authRequestJSON<AssignmentRule>(
    `/operational/projects/${projectId}/assignment-rules`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeRuleForm(input)),
    },
  );
}

export async function updateAssignmentRule(projectId: string, ruleId: string, input: AssignmentRuleFormValues) {
  return authRequestJSON<AssignmentRule>(
    `/operational/projects/${projectId}/assignment-rules/${ruleId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeRuleForm(input)),
    },
  );
}

export async function deleteAssignmentRule(projectId: string, ruleId: string) {
  await authRequestJSON<{ message: string }>(
    `/operational/projects/${projectId}/assignment-rules/${ruleId}`,
    {
      method: "DELETE",
    },
  );
}

export async function autoAssignTask(projectId: string, taskId: string) {
  return authRequestJSON<AutoAssignResult>(
    `/operational/projects/${projectId}/tasks/${taskId}/auto-assign`,
    {
      method: "POST",
    },
  );
}

function serializeRuleForm(input: AssignmentRuleFormValues) {
  const config: Record<string, string> = {};
  const trimmedValue = input.config_value.trim();
  const trimmedRole = input.role_in_project.trim();

  if (input.rule_type === "by_department") {
    config.department = trimmedValue;
  }

  if (input.rule_type === "by_skill") {
    config.skill = trimmedValue;
  }

  if (trimmedRole) {
    config.role_in_project = trimmedRole;
  }

  return {
    rule_type: input.rule_type,
    rule_config: config,
    priority: input.priority,
    is_active: input.is_active,
  };
}

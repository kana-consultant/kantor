import { ApiError, requestJSON } from "@/lib/api-client";
import { ensureAuthenticated } from "@/services/auth";
import type { AssignmentRule, AssignmentRuleFormValues, AutoAssignResult } from "@/types/assignment";

export const assignmentRuleKeys = {
  all: (projectId: string) => ["operational", "projects", projectId, "assignment-rules"] as const,
};

export async function listAssignmentRules(projectId: string) {
  const token = await requireAccessToken();
  return requestJSON<AssignmentRule[]>(
    `/operational/projects/${projectId}/assignment-rules`,
    { method: "GET" },
    token,
  );
}

export async function createAssignmentRule(projectId: string, input: AssignmentRuleFormValues) {
  const token = await requireAccessToken();
  return requestJSON<AssignmentRule>(
    `/operational/projects/${projectId}/assignment-rules`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeRuleForm(input)),
    },
    token,
  );
}

export async function updateAssignmentRule(projectId: string, ruleId: string, input: AssignmentRuleFormValues) {
  const token = await requireAccessToken();
  return requestJSON<AssignmentRule>(
    `/operational/projects/${projectId}/assignment-rules/${ruleId}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(serializeRuleForm(input)),
    },
    token,
  );
}

export async function deleteAssignmentRule(projectId: string, ruleId: string) {
  const token = await requireAccessToken();
  await requestJSON<{ message: string }>(
    `/operational/projects/${projectId}/assignment-rules/${ruleId}`,
    {
      method: "DELETE",
    },
    token,
  );
}

export async function autoAssignTask(projectId: string, taskId: string) {
  const token = await requireAccessToken();
  return requestJSON<AutoAssignResult>(
    `/operational/projects/${projectId}/tasks/${taskId}/auto-assign`,
    {
      method: "POST",
    },
    token,
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

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

import { ApiError, requestJSON } from "@/lib/api-client";
import { parseLooseCurrency } from "@/lib/currency";
import { ensureAuthenticated } from "@/services/auth";
import type {
  BonusRecord,
  BonusFormValues,
  SalaryFormValues,
  SalaryRecord,
} from "@/types/hris";

export const compensationKeys = {
  all: ["hris", "compensation"] as const,
  salaries: (employeeId: string) => [...compensationKeys.all, "salaries", employeeId] as const,
  currentSalary: (employeeId: string) => [...compensationKeys.all, "salaries", employeeId, "current"] as const,
  bonuses: (employeeId: string) => [...compensationKeys.all, "bonuses", employeeId] as const,
};

export async function listSalaries(employeeId: string) {
  const token = await requireAccessToken();
  return requestJSON<SalaryRecord[]>(`/hris/employees/${employeeId}/salaries`, { method: "GET" }, token);
}

export async function getCurrentSalary(employeeId: string) {
  const token = await requireAccessToken();
  return requestJSON<SalaryRecord>(`/hris/employees/${employeeId}/salaries/current`, { method: "GET" }, token);
}

export async function createSalary(employeeId: string, input: SalaryFormValues) {
  const token = await requireAccessToken();
  return requestJSON<SalaryRecord>(
    `/hris/employees/${employeeId}/salaries`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        base_salary: input.base_salary,
        allowances: parseAmountMap(input.allowances),
        deductions: parseAmountMap(input.deductions),
        effective_date: new Date(input.effective_date).toISOString(),
      }),
    },
    token,
  );
}

export async function listBonuses(employeeId: string) {
  const token = await requireAccessToken();
  return requestJSON<BonusRecord[]>(`/hris/employees/${employeeId}/bonuses`, { method: "GET" }, token);
}

export async function createBonus(employeeId: string, input: BonusFormValues) {
  const token = await requireAccessToken();
  return requestJSON<BonusRecord>(
    `/hris/employees/${employeeId}/bonuses`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        amount: input.amount,
        reason: input.reason.trim(),
        period_month: input.period_month,
        period_year: input.period_year,
      }),
    },
    token,
  );
}

export async function approveBonus(bonusId: string) {
  const token = await requireAccessToken();
  return requestJSON<BonusRecord>(`/hris/bonuses/${bonusId}/approve`, { method: "PATCH" }, token);
}

export async function rejectBonus(bonusId: string) {
  const token = await requireAccessToken();
  return requestJSON<BonusRecord>(`/hris/bonuses/${bonusId}/reject`, { method: "PATCH" }, token);
}

function parseAmountMap(raw: string) {
  const result: Record<string, number> = {};
  raw
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .forEach((line) => {
      const [label, amountRaw] = line.split(":");
      const amount = parseLooseCurrency((amountRaw ?? "").trim());
      if (label && Number.isFinite(amount)) {
        result[label.trim()] = amount;
      }
    });

  return result;
}

async function requireAccessToken() {
  const session = await ensureAuthenticated();
  if (!session?.tokens.access_token) {
    throw new ApiError(401, "Session is not available");
  }

  return session.tokens.access_token;
}

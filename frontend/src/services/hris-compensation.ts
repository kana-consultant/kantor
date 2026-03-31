import { authRequestJSON } from "@/lib/api-client";
import { parseLooseCurrency } from "@/lib/currency";
import { toDateOnlyString } from "@/lib/date";
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
  return authRequestJSON<SalaryRecord[]>(`/hris/employees/${employeeId}/salaries`, { method: "GET" });
}

export async function getCurrentSalary(employeeId: string) {
  return authRequestJSON<SalaryRecord>(`/hris/employees/${employeeId}/salaries/current`, { method: "GET" });
}

export async function createSalary(employeeId: string, input: SalaryFormValues) {
  return authRequestJSON<SalaryRecord>(
    `/hris/employees/${employeeId}/salaries`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        base_salary: input.base_salary,
        allowances: parseAmountMap(input.allowances),
        deductions: parseAmountMap(input.deductions),
        effective_date: toDateOnlyString(input.effective_date),
      }),
    },
  );
}

export async function listBonuses(employeeId: string) {
  return authRequestJSON<BonusRecord[]>(`/hris/employees/${employeeId}/bonuses`, { method: "GET" });
}

export async function createBonus(employeeId: string, input: BonusFormValues) {
  return authRequestJSON<BonusRecord>(
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
  );
}

export async function updateBonus(bonusId: string, input: BonusFormValues) {
  return authRequestJSON<BonusRecord>(
    `/hris/bonuses/${bonusId}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        amount: input.amount,
        reason: input.reason.trim(),
        period_month: input.period_month,
        period_year: input.period_year,
      }),
    },
  );
}

export async function deleteBonus(bonusId: string) {
  return authRequestJSON<{ message: string }>(`/hris/bonuses/${bonusId}`, { method: "DELETE" });
}

export async function approveBonus(bonusId: string) {
  return authRequestJSON<BonusRecord>(`/hris/bonuses/${bonusId}/approve`, { method: "PATCH" });
}

export async function rejectBonus(bonusId: string) {
  return authRequestJSON<BonusRecord>(`/hris/bonuses/${bonusId}/reject`, { method: "PATCH" });
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


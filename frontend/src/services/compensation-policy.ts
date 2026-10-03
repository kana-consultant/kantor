import {
  authGetJSON,
  authRequestEnvelope,
  authRequestJSON,
} from "@/lib/api-client";

export type CompensationPolicy = {
  monthly_base_salary: number;
  min_hours_per_day: number;
  min_hours_per_month: number;
  timezone: string;
  updated_at: string;
};

export type UpdateCompensationPolicyPayload = {
  monthly_base_salary: number;
  min_hours_per_day: number;
  min_hours_per_month: number;
  timezone: string;
};

export type SalarySafetyStatus = "safe" | "at_risk" | "no_data";

export type DailyHoursViolation = {
  date: string;
  active_hours: number;
};

export type SalarySafetyReason =
  | "no_user"
  | "future_month"
  | "target_not_applicable";

export type SalarySafetyEvaluation = {
  employee_id: string;
  user_id?: string | null;
  full_name: string;
  period_year: number;
  period_month: number;
  /** Tracked active hours in the whole month (weekends and today included). */
  monthly_active_hours: number;
  min_hours_per_month: number;
  min_hours_per_day: number;
  base_salary: number;
  /** Finished weekdays in the window with 0 < hours < min_hours_per_day. */
  daily_violations: DailyHoursViolation[];
  status: SalarySafetyStatus;
  /** Monthly target pro-rated by the weekdays elapsed so far. */
  expected_hours_to_date: number;
  /** Full-month target; 0 when the target does not apply. */
  target_hours_month: number;
  working_days_elapsed: number;
  working_days_total: number;
  /** Last finished day counted (YYYY-MM-DD); null before the first finished day. */
  evaluated_through: string | null;
  has_tracker_data: boolean;
  short_days: number;
  absent_days: number;
  reason: SalarySafetyReason | null;
  position_unset: boolean;
};

export type SalarySafetyPeriod = {
  year: number;
  month: number;
};

/** meta of GET /hris/salary-safety. */
export type SalarySafetyMeta = {
  period_year: number;
  period_month: number;
  /** Current period in the policy timezone (the latest evaluable month). */
  current_year: number;
  current_month: number;
};

export type SalarySafetyResult = {
  evaluations: SalarySafetyEvaluation[];
  meta: SalarySafetyMeta | null;
};

export const compensationPolicyKeys = {
  all: ["compensation-policy"] as const,
  policy: () => [...compensationPolicyKeys.all, "policy"] as const,
  /** A null period is the server's current period (policy timezone). */
  salarySafety: (period: SalarySafetyPeriod | null) =>
    [
      ...compensationPolicyKeys.all,
      "salary-safety",
      period ?? "current",
    ] as const,
};

export function getCompensationPolicy(): Promise<CompensationPolicy> {
  return authGetJSON<CompensationPolicy>("/hris/compensation-policy");
}

export function updateCompensationPolicy(
  payload: UpdateCompensationPolicyPayload,
): Promise<CompensationPolicy> {
  return authRequestJSON<CompensationPolicy>("/hris/compensation-policy", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

/**
 * Salary safety for a period. Without a period the server evaluates its
 * current month in the policy timezone; meta reports which period that was.
 */
export async function getSalarySafety(
  period: SalarySafetyPeriod | null,
): Promise<SalarySafetyResult> {
  const query = period ? `?year=${period.year}&month=${period.month}` : "";
  const payload = await authRequestEnvelope<SalarySafetyEvaluation[] | undefined>(
    `/hris/salary-safety${query}`,
  );
  return {
    evaluations: payload.data ?? [],
    meta: (payload.meta as SalarySafetyMeta | undefined) ?? null,
  };
}

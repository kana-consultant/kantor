import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

import {
  compensationPolicyKeys,
  getSalarySafety,
  type SalarySafetyPeriod,
} from "@/services/compensation-policy";

/**
 * Salary-safety data with a selectable period. The default period and the
 * latest selectable month come from the server (the current month in the
 * policy timezone), never from the browser clock: a browser in another
 * timezone around a month boundary would otherwise open on a month the server
 * treats as future, or be unable to pick the real current month.
 */
export function useSalarySafety() {
  const [selected, setSelected] = useState<SalarySafetyPeriod | null>(null);

  const currentQuery = useQuery({
    queryKey: compensationPolicyKeys.salarySafety(null),
    queryFn: () => getSalarySafety(null),
  });
  const selectedQuery = useQuery({
    queryKey: compensationPolicyKeys.salarySafety(selected),
    queryFn: () => getSalarySafety(selected),
    enabled: selected !== null,
  });

  const meta = currentQuery.data?.meta ?? null;
  const current: SalarySafetyPeriod | null = meta
    ? { year: meta.current_year, month: meta.current_month }
    : null;
  const defaultPeriod: SalarySafetyPeriod | null = meta
    ? { year: meta.period_year, month: meta.period_month }
    : null;
  const period = selected ?? defaultPeriod;
  const query = selected ? selectedQuery : currentQuery;

  return {
    /** Current period in the policy timezone; null until the first load. */
    current,
    /** Period on screen; null until the first load. */
    period,
    setPeriod: setSelected,
    evaluations: query.data?.evaluations ?? [],
    isLoading: query.isLoading,
    error: query.error instanceof Error ? query.error : null,
  };
}

import { authRequestJSON } from "@/lib/api-client";
import type { HRProfile, UpdateHRProfilePayload } from "@/types/hris";

// Kept outside employeesKeys on purpose: invalidating an employee must not
// refetch the HR profile, because every identity read is access-logged.
export const hrProfileKeys = {
  all: ["hris", "hr-profile"] as const,
  detail: (employeeId: string) => [...hrProfileKeys.all, employeeId] as const,
};

export function getHRProfile(employeeId: string) {
  return authRequestJSON<HRProfile>(`/hris/employees/${employeeId}/hr-profile`, { method: "GET" });
}

export function updateHRProfile(employeeId: string, payload: UpdateHRProfilePayload) {
  return authRequestJSON<HRProfile>(`/hris/employees/${employeeId}/hr-profile`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

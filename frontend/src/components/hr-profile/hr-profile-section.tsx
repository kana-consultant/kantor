import { useQuery } from "@tanstack/react-query";

import { EmploymentDataCard } from "@/components/hr-profile/employment-data-card";
import { IdentityCard } from "@/components/hr-profile/identity-card";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { getHRProfile, hrProfileKeys } from "@/services/hris-hr-profile";

/**
 * "Data Kepegawaian" (employee code, job title) and, for identity holders,
 * "Data Identitas" of one employee. Both read the same HR profile; with
 * hris:employee_identity:view every fetch is access-logged by the backend,
 * so this query does not refetch on focus.
 */
export function HRProfileSection({ employeeId }: { employeeId: string }) {
  const { hasPermission } = useRBAC();
  const canEditJobTitle = hasPermission(permissions.hrisEmployeeEdit);
  const canViewIdentity = hasPermission(permissions.hrisEmployeeIdentityView);
  const canEditIdentity = hasPermission(permissions.hrisEmployeeIdentityEdit);

  const profileQuery = useQuery({
    queryKey: hrProfileKeys.detail(employeeId),
    queryFn: () => getHRProfile(employeeId),
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  });

  const error = profileQuery.error instanceof Error ? profileQuery.error.message : null;
  const showIdentity = canViewIdentity || canEditIdentity;

  return (
    <div className={showIdentity ? "grid gap-6 xl:grid-cols-[0.8fr_1.2fr]" : "grid gap-6"}>
      <EmploymentDataCard
        canEdit={canEditJobTitle}
        employeeId={employeeId}
        error={error}
        loading={profileQuery.isLoading}
        profile={profileQuery.data}
      />
      {showIdentity ? (
        <IdentityCard
          canEdit={canEditIdentity}
          employeeId={employeeId}
          error={error}
          loading={profileQuery.isLoading}
          profile={profileQuery.data}
        />
      ) : null}
    </div>
  );
}

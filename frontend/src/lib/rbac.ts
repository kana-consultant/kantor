import { redirect } from "@tanstack/react-router";

import { permissions } from "@/lib/permissions";
import { ensureAuthenticated } from "@/services/auth";
import { useAuthStore } from "@/stores/auth-store";
import type { AuthModuleRole, AuthSession } from "@/types/auth";

interface AccessOptions {
  permission?: string;
  role?: string;
  module?: string;
}

export function hasPermission(session: AuthSession | null, permission: string) {
  if (!session) {
    return false;
  }

  if (session.is_super_admin) {
    return true;
  }

  return session.permissions.includes(permission);
}

export function hasRole(
  session: AuthSession | null,
  role: string,
  module?: string,
) {
  if (!session) {
    return false;
  }

  if (role === "super_admin") {
    return session.is_super_admin;
  }

  if (session.is_super_admin) {
    return true;
  }

  if (module) {
    return session.module_roles[module]?.role_slug === role;
  }

  return Object.values(session.module_roles).some(
    (moduleRole) => moduleRole.role_slug === role,
  );
}

export function hasModuleAccess(session: AuthSession | null, module: string) {
  if (!session) {
    return false;
  }

  return session.is_super_admin || Boolean(session.module_roles[module]);
}

export function getModuleRole(
  session: AuthSession | null,
  module: string,
): AuthModuleRole | null {
  if (!session) {
    return null;
  }

  return session.module_roles[module] ?? null;
}

export function hasAnyPermission(
  session: AuthSession | null,
  ...permissions: string[]
) {
  if (!session) {
    return false;
  }

  if (session.is_super_admin) {
    return true;
  }

  return permissions.some((permission) => session.permissions.includes(permission));
}

export function hasAllPermissions(
  session: AuthSession | null,
  ...permissions: string[]
) {
  if (!session) {
    return false;
  }

  if (session.is_super_admin) {
    return true;
  }

  return permissions.every((permission) => session.permissions.includes(permission));
}

export function canAccess(session: AuthSession | null, options: AccessOptions) {
  if (!session) {
    return false;
  }

  if (session.is_super_admin) {
    return true;
  }

  if (options.permission) {
    return hasPermission(session, options.permission);
  }

  if (options.role) {
    return hasRole(session, options.role, options.module);
  }

  if (options.module) {
    return hasModuleAccess(session, options.module);
  }

  return false;
}

const landingRouteCandidates = [
  { path: "/operational/overview", permission: permissions.operationalOverview },
  { path: "/operational/projects", permission: permissions.operationalProjectView },
  { path: "/operational/tracker", permission: permissions.operationalTrackerView },
  { path: "/operational/wa-broadcast", permission: permissions.operationalWAView },
  { path: "/hris/overview", permission: permissions.hrisOverview },
  { path: "/hris/employees", permission: permissions.hrisEmployeeView },
  { path: "/hris/departments", permission: permissions.hrisDepartmentView },
  { path: "/hris/finance", permission: permissions.hrisFinanceView },
  { path: "/hris/reimbursements", permission: permissions.hrisReimbursementView },
  { path: "/hris/subscriptions", permission: permissions.hrisSubscriptionView },
  { path: "/marketing/overview", permission: permissions.marketingOverview },
  { path: "/marketing/campaigns", permission: permissions.marketingCampaignView },
  { path: "/marketing/ads-metrics", permission: permissions.marketingAdsMetricsView },
  { path: "/marketing/leads", permission: permissions.marketingLeadsView },
  { path: "/admin/audit-logs", permission: permissions.adminAuditLogView },
  { path: "/admin/roles", permission: permissions.adminRolesView },
  { path: "/admin/users", permission: permissions.adminUsersView },
  { path: "/admin/settings", permission: permissions.adminSettingsView },
] as const;

export function getDefaultAuthorizedPath(session: AuthSession | null) {
  if (!session) {
    return "/login";
  }

  const matchedRoute = landingRouteCandidates.find((candidate) =>
    hasPermission(session, candidate.permission),
  );

  return matchedRoute?.path ?? "/profile";
}

export async function ensurePermission(permission: string) {
  const session = await ensureAuthenticated();
  if (!session) {
    throw redirect({
      to: "/login",
    });
  }

  if (!hasPermission(session, permission)) {
    throw redirect({
      to: "/forbidden",
    });
  }

  return session;
}

export async function ensureModuleAccess(module: string) {
  const session = await ensureAuthenticated();
  if (!session) {
    throw redirect({
      to: "/login",
    });
  }

  if (!hasModuleAccess(session, module)) {
    throw redirect({
      to: "/forbidden",
    });
  }

  return session;
}

export function getCurrentSession() {
  return useAuthStore.getState().session;
}

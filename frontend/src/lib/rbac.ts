import { redirect } from "@tanstack/react-router";

import { ensureAuthenticated } from "@/services/auth";
import { useAuthStore } from "@/stores/auth-store";
import type { AuthSession } from "@/types/auth";

interface AccessOptions {
  permission?: string;
  role?: string;
  module?: string;
}

export function hasPermission(session: AuthSession | null, permission: string) {
  if (!session) {
    return false;
  }

  if (session.roles.includes("super_admin")) {
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

  if (session.roles.includes("super_admin")) {
    return true;
  }

  if (module) {
    return session.roles.includes(`${role}:${module}`);
  }

  return session.roles.includes(role);
}

export function canAccess(session: AuthSession | null, options: AccessOptions) {
  if (!session) {
    return false;
  }

  if (session.roles.includes("super_admin")) {
    return true;
  }

  if (options.permission) {
    return hasPermission(session, options.permission);
  }

  if (options.role) {
    return hasRole(session, options.role, options.module);
  }

  return false;
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

export function getCurrentSession() {
  return useAuthStore.getState().session;
}

import { useAuthStore } from "@/stores/auth-store";

export function useAuth() {
  const session = useAuthStore((state) => state.session);
  const setSession = useAuthStore((state) => state.setSession);
  const clearSession = useAuthStore((state) => state.clearSession);

  return {
    session,
    user: session?.user ?? null,
    roles: session?.roles ?? [],
    permissions: session?.permissions ?? [],
    isAuthenticated: Boolean(session?.tokens.access_token),
    setSession,
    clearSession,
  };
}

import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { AuthSession } from "@/types/auth";

interface AuthState {
  session: AuthSession | null;
  setSession: (session: AuthSession) => void;
  clearSession: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      session: null,
      setSession: (session) => set({ session }),
      clearSession: () => set({ session: null }),
    }),
    {
      name: "kantor-auth",
      version: 2,
      partialize: (state) => ({
        session: state.session,
      }),
      migrate: (persistedState) => {
        const state = persistedState as { session?: Partial<AuthSession> | null } | undefined;
        if (!state?.session) {
          return { session: null };
        }

        const session = state.session;
        if (
          !("module_roles" in session) ||
          !("is_super_admin" in session) ||
          !("permissions" in session) ||
          !("tokens" in session)
        ) {
          return { session: null };
        }

        return {
          session: session as AuthSession,
        };
      },
    },
  ),
);

export function getStoredSession() {
  return useAuthStore.getState().session;
}

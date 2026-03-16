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
      partialize: (state) => ({
        session: state.session,
      }),
    },
  ),
);

export function getStoredSession() {
  return useAuthStore.getState().session;
}

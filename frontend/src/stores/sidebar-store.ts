import { create } from "zustand";
import { persist } from "zustand/middleware";

interface SidebarState {
  isDesktopCollapsed: boolean;
  isMobileOpen: boolean;
  toggleDesktopCollapsed: () => void;
  setDesktopCollapsed: (value: boolean) => void;
  toggleMobileOpen: () => void;
  setMobileOpen: (value: boolean) => void;
}

export const useSidebarStore = create<SidebarState>()(
  persist(
    (set) => ({
      isDesktopCollapsed: false,
      isMobileOpen: false,
      toggleDesktopCollapsed: () =>
        set((state) => ({
          isDesktopCollapsed: !state.isDesktopCollapsed,
        })),
      setDesktopCollapsed: (value) => set({ isDesktopCollapsed: value }),
      toggleMobileOpen: () =>
        set((state) => ({
          isMobileOpen: !state.isMobileOpen,
        })),
      setMobileOpen: (value) => set({ isMobileOpen: value }),
    }),
    {
      name: "kantor-sidebar",
      partialize: (state) => ({
        isDesktopCollapsed: state.isDesktopCollapsed,
      }),
    },
  ),
);

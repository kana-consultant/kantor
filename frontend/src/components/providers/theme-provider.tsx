import type { PropsWithChildren } from "react";
import { useEffect } from "react";

import { useThemeStore } from "@/stores/theme-store";

export function ThemeProvider({ children }: PropsWithChildren) {
  const mode = useThemeStore((state) => state.mode);

  useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle("dark", mode === "dark");
    root.style.colorScheme = mode;
  }, [mode]);

  return children;
}

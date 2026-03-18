import type { PropsWithChildren, ReactNode } from "react";

import { cn } from "@/lib/utils";

interface TooltipProps extends PropsWithChildren {
  content: ReactNode;
  side?: "left" | "right";
  className?: string;
}

export function Tooltip({ children, content, side = "right", className }: TooltipProps) {
  return (
    <div className="group/tooltip relative flex">
      {children}
      <div
        className={cn(
          "pointer-events-none absolute top-1/2 z-50 hidden -translate-y-1/2 whitespace-nowrap rounded-md border border-border bg-surface px-3 py-2 text-xs font-medium text-text-primary shadow-md group-hover/tooltip:block",
          side === "right" ? "left-full ml-3" : "right-full mr-3",
          className,
        )}
        role="tooltip"
      >
        {content}
      </div>
    </div>
  );
}

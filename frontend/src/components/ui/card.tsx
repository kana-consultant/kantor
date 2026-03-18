import { type HTMLAttributes, forwardRef } from "react";

import { cn } from "@/lib/utils";

export const Card = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={cn(
          "rounded-[12px] border border-border bg-surface shadow-card",
          className,
        )}
        {...props}
      />
    );
  }
);

Card.displayName = "Card";

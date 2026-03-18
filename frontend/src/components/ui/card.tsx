import { type HTMLAttributes, forwardRef } from "react";

import { cn } from "@/lib/utils";

export const Card = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={cn(
          "rounded-md border border-border bg-surface shadow-card transition-[box-shadow,border-color] duration-200 hover:shadow-card-hover",
          className,
        )}
        {...props}
      />
    );
  }
);

Card.displayName = "Card";

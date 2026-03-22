import { type HTMLAttributes, forwardRef } from "react";

import { cn } from "@/lib/utils";

export const Card = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={cn(
          "rounded-[20px] border border-border/80 bg-surface/95 shadow-[0_18px_42px_-28px_rgba(15,23,42,0.32)] backdrop-blur-[2px] transition-[box-shadow,border-color,transform] duration-200 hover:border-border hover:shadow-[0_24px_56px_-30px_rgba(15,23,42,0.38)]",
          className,
        )}
        {...props}
      />
    );
  }
);

Card.displayName = "Card";

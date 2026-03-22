import * as React from "react";

import { cn } from "@/lib/utils";

export type InputProps = React.ComponentPropsWithoutRef<"input">;

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type = "text", ...props }, ref) => (
    <input
      className={cn(
        "flex h-11 w-full rounded-xl border border-border/70 bg-surface-muted/90 px-3.5 py-2 text-[14px] text-text-primary shadow-none outline-none transition-all duration-150 placeholder:text-text-tertiary focus-visible:border-[#4C9AFF] focus-visible:bg-surface focus-visible:shadow-focus disabled:cursor-not-allowed disabled:text-text-tertiary",
        className,
      )}
      ref={ref}
      type={type}
      {...props}
    />
  ),
);

Input.displayName = "Input";

export { Input };

import * as React from "react";

import { cn } from "@/lib/utils";

export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type = "text", ...props }, ref) => (
    <input
      className={cn(
        "flex h-10 w-full rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-none outline-none transition-all duration-150 placeholder:text-text-tertiary focus-visible:border-[#4C9AFF] focus-visible:bg-surface focus-visible:shadow-focus disabled:cursor-not-allowed disabled:text-text-tertiary",
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

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 font-[600] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ops/50 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "bg-ops text-white shadow-sm hover:bg-ops-dark",
        ops: "bg-ops text-white shadow-sm hover:bg-ops-dark",
        hr: "bg-hr text-white shadow-sm hover:bg-hr-dark",
        mkt: "bg-mkt text-white shadow-sm hover:bg-mkt-dark",
        secondary: "bg-surface-muted text-text-primary border border-border hover:bg-border/50 font-[500]",
        outline: "border border-border bg-surface text-text-primary hover:bg-surface-muted font-[500]",
        ghost: "hover:bg-surface-muted hover:text-text-primary text-text-secondary font-[500]",
        danger: "bg-priority-high text-white shadow-sm hover:opacity-90",
      },
      size: {
        default: "h-[44px] px-4 rounded-[8px] text-[14px]",
        sm: "h-[36px] px-3 rounded-[6px] text-[13px]",
        xs: "h-[32px] px-2.5 rounded-[6px] text-[12px]",
        icon: "h-[36px] w-[36px] rounded-[6px]",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => (
    <button
      className={cn(buttonVariants({ variant, size }), className)}
      ref={ref}
      {...props}
    />
  ),
);

Button.displayName = "Button";

export { Button, buttonVariants };

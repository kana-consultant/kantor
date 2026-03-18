import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { useModuleTheme } from "@/hooks/use-module-theme";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md font-[600] transition-all duration-150 focus-visible:outline-none focus-visible:shadow-focus active:scale-[0.98] disabled:pointer-events-none disabled:opacity-40",
  {
    variants: {
      variant: {
        default: "bg-module text-white shadow-xs hover:brightness-95",
        ops: "bg-ops text-white shadow-sm hover:bg-ops-dark",
        hr: "bg-hr text-white shadow-sm hover:bg-hr-dark",
        mkt: "bg-mkt text-white shadow-sm hover:bg-mkt-dark",
        secondary: "border-[1.5px] border-module bg-transparent text-[color:var(--module-primary)] hover:bg-module-light",
        outline: "border border-border bg-surface text-text-primary hover:bg-surface-muted font-[500]",
        ghost: "hover:bg-surface-muted hover:text-text-primary text-text-secondary font-[500]",
        danger: "bg-error text-white shadow-xs hover:brightness-95",
      },
      size: {
        default: "h-10 px-5 text-[14px]",
        sm: "h-9 px-4 text-[14px]",
        xs: "h-8 px-3 text-[14px]",
        icon: "h-9 w-9 px-0",
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
  ({ className, variant, size, ...props }, ref) => {
    const { module } = useModuleTheme();

    const resolvedVariant =
      !variant || variant === "default"
        ? module.key === "hr"
          ? "hr"
          : module.key === "mkt"
            ? "mkt"
            : "ops"
        : variant;

    return (
      <button
        className={cn(buttonVariants({ variant: resolvedVariant, size }), className)}
        ref={ref}
        {...props}
      />
    );
  },
);

Button.displayName = "Button";

export { Button, buttonVariants };

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { useModuleTheme } from "@/hooks/use-module-theme";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-xl font-[600] transition-all duration-150 focus-visible:outline-none focus-visible:shadow-focus active:scale-[0.98] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-[0.72]",
  {
    variants: {
      variant: {
        default:
          "bg-module text-white shadow-xs hover:brightness-[0.97] dark:border dark:border-module/35 dark:bg-module-light dark:text-[color:var(--module-dark)] dark:hover:border-module/60 dark:hover:bg-module-light dark:hover:text-[color:var(--module-dark)] dark:hover:brightness-110",
        primary:
          "bg-module text-white shadow-xs hover:brightness-[0.97] dark:border dark:border-module/35 dark:bg-module-light dark:text-[color:var(--module-dark)] dark:hover:border-module/60 dark:hover:bg-module-light dark:hover:text-[color:var(--module-dark)] dark:hover:brightness-110",
        ops:
          "bg-ops text-white shadow-sm hover:bg-ops-dark dark:border dark:border-ops/35 dark:bg-ops-light dark:text-ops-dark dark:hover:border-ops/60 dark:hover:bg-ops-light dark:hover:text-ops-dark dark:hover:brightness-110",
        hr:
          "bg-hr text-white shadow-sm hover:bg-hr-dark dark:border dark:border-hr/35 dark:bg-hr-light dark:text-hr-dark dark:hover:border-hr/60 dark:hover:bg-hr-light dark:hover:text-hr-dark dark:hover:brightness-110",
        mkt:
          "bg-mkt text-white shadow-sm hover:bg-mkt-dark dark:border dark:border-mkt/35 dark:bg-mkt-light dark:text-mkt-dark dark:hover:border-mkt/60 dark:hover:bg-mkt-light dark:hover:text-mkt-dark dark:hover:brightness-110",
        secondary:
          "border-[1.5px] border-module bg-transparent text-[color:var(--module-primary)] hover:bg-module-light hover:text-[color:var(--module-dark)] dark:bg-surface dark:text-[color:var(--module-primary)] dark:hover:border-module/70 dark:hover:bg-module-light dark:hover:text-[color:var(--module-dark)]",
        outline:
          "border border-border bg-surface text-text-primary font-[500] hover:border-border/90 hover:bg-surface-muted hover:text-text-primary dark:hover:bg-surface-muted dark:hover:text-text-primary",
        ghost:
          "text-text-secondary font-[500] hover:bg-surface-muted hover:text-text-primary dark:hover:bg-surface-muted dark:hover:text-text-primary",
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

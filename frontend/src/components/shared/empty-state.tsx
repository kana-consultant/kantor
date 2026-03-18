import type { LucideIcon } from "lucide-react";

import { Button, type ButtonProps } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
  actionProps?: Omit<ButtonProps, "children" | "onClick">;
  className?: string;
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  actionLabel,
  onAction,
  actionProps,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center rounded-md border border-dashed border-border bg-surface px-6 py-12 text-center",
        className,
      )}
    >
      <div className="flex h-12 w-12 items-center justify-center rounded-md bg-surface-muted text-text-tertiary">
        <Icon className="h-6 w-6" strokeWidth={1.75} />
      </div>
      <h3 className="mt-4 text-base font-semibold text-text-primary">{title}</h3>
      <p className="mt-2 max-w-[360px] text-sm leading-6 text-text-secondary">{description}</p>
      {actionLabel && onAction ? (
        <Button className="mt-4" onClick={onAction} {...actionProps}>
          {actionLabel}
        </Button>
      ) : null}
    </div>
  );
}

import type { LucideIcon } from "lucide-react";
import { ArrowDownRight, ArrowUpRight } from "lucide-react";

import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

type StatTone = "ops" | "hr" | "mkt" | "success" | "warning" | "error" | "info";

interface StatCardProps {
  label: string;
  value: string;
  icon: LucideIcon;
  tone?: StatTone;
  mono?: boolean;
  helper?: string;
  trend?: {
    value: string;
    direction: "up" | "down" | "neutral";
  };
}

const toneClasses: Record<StatTone, { border: string; icon: string; iconBg: string }> = {
  ops: { border: "border-t-ops", icon: "text-ops", iconBg: "bg-ops-light" },
  hr: { border: "border-t-hr", icon: "text-hr", iconBg: "bg-hr-light" },
  mkt: { border: "border-t-mkt", icon: "text-mkt", iconBg: "bg-mkt-light" },
  success: { border: "border-t-success", icon: "text-success", iconBg: "bg-success-light" },
  warning: { border: "border-t-warning", icon: "text-warning", iconBg: "bg-warning-light" },
  error: { border: "border-t-error", icon: "text-error", iconBg: "bg-error-light" },
  info: { border: "border-t-info", icon: "text-info", iconBg: "bg-info-light" },
};

export function StatCard({
  label,
  value,
  icon: Icon,
  tone = "ops",
  mono = false,
  helper,
  trend,
}: StatCardProps) {
  const style = toneClasses[tone];

  return (
    <Card className={cn("border-t-[3px] p-4 sm:p-5", style.border)}>
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-text-secondary">
            {label}
          </p>
          <p
            className={cn(
              "mt-3 text-[24px] font-bold leading-none text-text-primary sm:text-[28px]",
              mono ? "font-mono tabular-nums" : "font-display",
            )}
          >
            {value}
          </p>
          {helper ? <p className="mt-2 max-w-[24ch] text-[13px] leading-5 text-text-secondary">{helper}</p> : null}
        </div>
        <div className={cn("flex h-11 w-11 items-center justify-center rounded-xl shadow-inner", style.iconBg)}>
          <Icon className={cn("h-5 w-5", style.icon)} />
        </div>
      </div>
      {trend ? <TrendBadge direction={trend.direction} value={trend.value} /> : null}
    </Card>
  );
}

function TrendBadge({
  direction,
  value,
}: {
  direction: "up" | "down" | "neutral";
  value: string;
}) {
  const isPositive = direction === "up";
  const isNegative = direction === "down";
  const Icon = isNegative ? ArrowDownRight : ArrowUpRight;

  return (
    <div
      className={cn(
        "mt-4 inline-flex items-center gap-1 rounded-full px-2 py-1 text-xs font-semibold",
        isPositive && "bg-success-light text-success",
        isNegative && "bg-error-light text-error",
        direction === "neutral" && "bg-surface-muted text-text-secondary",
      )}
    >
      {direction === "neutral" ? null : <Icon className="h-3.5 w-3.5" />}
      <span>{value}</span>
    </div>
  );
}

import { cn } from "@/lib/utils";

type StatusVariant =
  | "semantic"
  | "project-status"
  | "priority"
  | "employee-status"
  | "finance-status"
  | "reimbursement-status"
  | "subscription-status"
  | "renewal-alert"
  | "campaign-status"
  | "lead-status"
  | "assignment"
  | "audit-action"
  | "module";

interface StatusBadgeProps {
  status: string | null | undefined;
  variant?: StatusVariant;
  className?: string;
}

type Tone = {
  bgClassName: string;
  textClassName: string;
  dotClassName: string;
};

function tone(bgClassName: string, textClassName: string, dotClassName = textClassName): Tone {
  return {
    bgClassName,
    textClassName,
    dotClassName,
  };
}

const tones = {
  success: tone("bg-success-light", "text-success", "bg-success"),
  warning: tone("bg-warning-light", "text-warning", "bg-warning"),
  error: tone("bg-error-light", "text-error", "bg-error"),
  info: tone("bg-info-light", "text-info", "bg-info"),
  neutral: tone("bg-surface-muted", "text-text-secondary", "bg-text-tertiary"),
  ops: tone("bg-ops-light", "text-ops", "bg-ops"),
  contacted: tone("bg-pipelineLight-contacted", "text-pipeline-contacted", "bg-pipeline-contacted"),
  qualified: tone("bg-hr-light", "text-hr", "bg-hr"),
  proposal: tone("bg-pipelineLight-proposal", "text-high", "bg-high"),
  negotiation: tone("bg-warning-light", "text-warning", "bg-warning"),
  renewal30: tone("bg-warning-light", "text-warning", "bg-warning"),
  renewal7: tone("bg-pipelineLight-proposal", "text-high", "bg-high"),
  renewal1: tone("bg-error-light", "text-error", "bg-error"),
  low: tone("bg-success-light", "text-low", "bg-low"),
  medium: tone("bg-warning-light", "text-medium", "bg-medium"),
  high: tone("bg-[#FFF1E6]", "text-high", "bg-high"),
  critical: tone("bg-error-light", "text-critical", "bg-critical"),
  draft: tone("bg-ops-light", "text-ops", "bg-ops"),
  archived: tone("bg-surface-muted", "text-text-tertiary", "bg-text-tertiary"),
};

function normalizeStatusLabel(status: string) {
  return status.replace(/_/g, " ").replace(/\b\w/g, (char) => char.toUpperCase());
}

function resolveTone(status: string, variant: StatusVariant): Tone {
  const normalized = status.trim().toLowerCase();

  if (variant === "priority") {
    switch (normalized) {
      case "low":
        return tones.low;
      case "medium":
        return tones.medium;
      case "high":
        return tones.high;
      case "critical":
        return tones.critical;
      default:
        return tones.neutral;
    }
  }

  if (variant === "lead-status") {
    switch (normalized) {
      case "new":
        return tones.info;
      case "contacted":
        return tones.contacted;
      case "qualified":
        return tones.qualified;
      case "proposal":
        return tones.proposal;
      case "negotiation":
        return tones.negotiation;
      case "won":
        return tones.success;
      case "lost":
        return tones.archived;
      default:
        return tones.neutral;
    }
  }

  if (variant === "campaign-status") {
    switch (normalized) {
      case "ideation":
        return tones.info;
      case "planning":
      case "in production":
      case "in_production":
        return tones.warning;
      case "live":
      case "completed":
        return tones.success;
      case "archived":
        return tones.archived;
      default:
        return tones.neutral;
    }
  }

  if (variant === "project-status") {
    switch (normalized) {
      case "draft":
        return tones.draft;
      case "active":
      case "completed":
        return tones.success;
      case "on hold":
      case "on_hold":
        return tones.warning;
      case "archived":
        return tones.archived;
      default:
        return tones.neutral;
    }
  }

  if (variant === "finance-status") {
    switch (normalized) {
      case "approved":
        return tones.success;
      case "pending review":
      case "pending_review":
        return tones.warning;
      case "rejected":
        return tones.error;
      case "draft":
        return tones.draft;
      default:
        return tones.neutral;
    }
  }

  if (variant === "reimbursement-status") {
    switch (normalized) {
      case "approved":
        return tones.success;
      case "paid":
        return tones.info;
      case "submitted":
        return tones.warning;
      case "rejected":
        return tones.error;
      default:
        return tones.neutral;
    }
  }

  if (variant === "employee-status") {
    switch (normalized) {
      case "active":
        return tones.success;
      case "probation":
        return tones.warning;
      case "resigned":
      case "terminated":
        return tones.error;
      default:
        return tones.neutral;
    }
  }

  if (variant === "subscription-status") {
    switch (normalized) {
      case "active":
        return tones.success;
      case "cancelled":
      case "expired":
        return tones.error;
      default:
        return tones.neutral;
    }
  }

  if (variant === "renewal-alert") {
    switch (normalized) {
      case "30_days":
      case "30 days":
        return tones.renewal30;
      case "7_days":
      case "7 days":
        return tones.renewal7;
      case "1_day":
      case "1 day":
        return tones.renewal1;
      default:
        return tones.neutral;
    }
  }

  if (variant === "assignment") {
    switch (normalized) {
      case "auto":
        return tones.info;
      case "manual":
        return tones.neutral;
      default:
        return tones.neutral;
    }
  }

  if (variant === "audit-action") {
    switch (normalized) {
      case "create":
      case "register":
      case "login":
      case "submit":
      case "send":
        return tones.success;
      case "update":
      case "edit":
      case "review":
      case "move":
      case "toggle":
      case "trigger":
        return tones.info;
      case "view":
        return tones.neutral;
      case "approve":
        return tones.success;
      case "reject":
      case "delete":
      case "logout":
        return tones.error;
      default:
        return tones.neutral;
    }
  }

  if (variant === "module") {
    switch (normalized) {
      case "operational":
        return tones.ops;
      case "hris":
        return tones.qualified;
      case "marketing":
        return tones.proposal;
      case "admin":
        return tones.error;
      default:
        return tones.neutral;
    }
  }

  switch (normalized) {
    case "active":
    case "approved":
    case "won":
    case "completed":
      return tones.success;
    case "pending":
    case "submitted":
    case "on hold":
    case "on_hold":
      return tones.warning;
    case "rejected":
    case "failed":
    case "lost":
      return tones.error;
    case "draft":
    case "new":
      return tones.draft;
    case "productive":
      return tones.success;
    case "unproductive":
      return tones.error;
    case "archived":
    case "inactive":
      return tones.archived;
    default:
      return tones.neutral;
  }
}

export function StatusBadge({
  status,
  variant = "semantic",
  className,
}: StatusBadgeProps) {
  const value = (status ?? "").trim();
  if (!value) {
    return null;
  }

  const resolved = resolveTone(value, variant);

  return (
    <span
      className={cn(
        "inline-flex items-center gap-2 rounded-full px-2 py-1 text-xs font-semibold",
        resolved.bgClassName,
        resolved.textClassName,
        className,
      )}
    >
      <span className={cn("h-1.5 w-1.5 rounded-full", resolved.dotClassName)} />
      <span>{normalizeStatusLabel(value)}</span>
    </span>
  );
}

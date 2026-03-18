import { useEffect } from "react";
import { AlertCircle, AlertTriangle, CheckCircle2, Info, X } from "lucide-react";

import { cn } from "@/lib/utils";
import { useToastStore, type ToastTone } from "@/stores/toast-store";

const toneClasses: Record<
  ToastTone,
  { icon: typeof Info; panelClassName: string; textClassName: string }
> = {
  info: {
    icon: Info,
    panelClassName: "border-info/30 bg-info-light",
    textClassName: "text-info",
  },
  success: {
    icon: CheckCircle2,
    panelClassName: "border-success/30 bg-success-light",
    textClassName: "text-success",
  },
  warning: {
    icon: AlertTriangle,
    panelClassName: "border-warning/30 bg-warning-light",
    textClassName: "text-warning",
  },
  error: {
    icon: AlertCircle,
    panelClassName: "border-error/30 bg-error-light",
    textClassName: "text-error",
  },
};

export function ToastProvider() {
  const items = useToastStore((state) => state.items);
  const dismiss = useToastStore((state) => state.dismiss);

  useEffect(() => {
    if (items.length === 0) {
      return undefined;
    }

    const timers = items.map((item) =>
      window.setTimeout(() => {
        dismiss(item.id);
      }, 5_000),
    );

    return () => {
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, [items, dismiss]);

  return (
    <div className="pointer-events-none fixed right-4 top-4 z-[100] flex w-full max-w-[360px] flex-col gap-3">
      {items.map((item) => {
        const tone = toneClasses[item.tone];
        const Icon = tone.icon;

        return (
          <div
            className={cn(
              "pointer-events-auto rounded-lg border px-4 py-4 shadow-lg motion-safe:animate-in motion-safe:slide-in-from-right-3 motion-safe:fade-in motion-safe:duration-300",
              tone.panelClassName,
            )}
            key={item.id}
            role="status"
          >
            <div className="flex items-start gap-3">
              <div className={cn("mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center", tone.textClassName)}>
                <Icon className="h-4.5 w-4.5" />
              </div>
              <div className="min-w-0 flex-1">
                <p className={cn("text-sm font-semibold leading-5", tone.textClassName)}>{item.title}</p>
                {item.description ? (
                  <p className="mt-1 text-[13px] leading-5 text-text-secondary">{item.description}</p>
                ) : null}
              </div>
              <button
                aria-label="Dismiss notification"
                className="rounded-md p-1 text-text-secondary transition hover:bg-surface hover:text-text-primary"
                onClick={() => dismiss(item.id)}
                type="button"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          </div>
        );
      })}
    </div>
  );
}

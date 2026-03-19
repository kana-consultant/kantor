import {
  createContext,
  forwardRef,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type HTMLAttributes,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type DialogSize = "sm" | "md" | "lg" | "xl";

interface DialogContextValue {
  open: boolean;
  rendered: boolean;
  dismissible: boolean;
  contentId: string;
  titleId: string;
  descriptionId: string;
  onOpenChange: (open: boolean) => void;
}

const DialogContext = createContext<DialogContextValue | null>(null);

const sizeClassNames: Record<DialogSize, string> = {
  sm: "max-w-[420px]",
  md: "max-w-[560px]",
  lg: "max-w-[720px]",
  xl: "max-w-[960px]",
};

interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
  dismissible?: boolean;
}

export function Dialog({
  open,
  onOpenChange,
  children,
  dismissible = true,
}: DialogProps) {
  const contentId = useId();
  const titleId = useId();
  const descriptionId = useId();
  const [rendered, setRendered] = useState(open);

  useEffect(() => {
    if (open) {
      setRendered(true);
      return undefined;
    }

    const timeout = window.setTimeout(() => setRendered(false), 150);
    return () => window.clearTimeout(timeout);
  }, [open]);

  useEffect(() => {
    if (!open) {
      return undefined;
    }

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [open]);

  const value = useMemo<DialogContextValue>(
    () => ({
      open,
      rendered,
      dismissible,
      contentId,
      titleId,
      descriptionId,
      onOpenChange,
    }),
    [contentId, descriptionId, dismissible, onOpenChange, open, rendered, titleId],
  );

  return <DialogContext.Provider value={value}>{children}</DialogContext.Provider>;
}

interface DialogContentProps extends HTMLAttributes<HTMLDivElement> {
  size?: DialogSize;
  children: ReactNode;
}

export const DialogContent = forwardRef<HTMLDivElement, DialogContentProps>(
  ({ children, className, size = "md", ...props }, ref) => {
    const context = useDialogContext();
    const contentRef = useRef<HTMLDivElement | null>(null);

    useEffect(() => {
      if (!context.rendered) {
        return undefined;
      }

      const node = contentRef.current;
      if (!node) {
        return undefined;
      }

      const focusable = getFocusableElements(node);
      const inputElement = focusable.find(
        (el) => el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT",
      );
      const target = inputElement ?? node;
      window.setTimeout(() => {
        if (context.open) {
          target.focus();
        }
      }, 0);

      const handleKeyDown = (event: KeyboardEvent) => {
        if (event.key === "Escape" && context.dismissible) {
          event.preventDefault();
          context.onOpenChange(false);
          return;
        }

        if (event.key !== "Tab") {
          return;
        }

        const elements = getFocusableElements(node);
        if (elements.length === 0) {
          event.preventDefault();
          node.focus();
          return;
        }

        const first = elements[0];
        const last = elements[elements.length - 1];
        const active = document.activeElement;

        if (event.shiftKey) {
          if (active === first || active === node) {
            event.preventDefault();
            last.focus();
          }
          return;
        }

        if (active === last) {
          event.preventDefault();
          first.focus();
        }
      };

      document.addEventListener("keydown", handleKeyDown);
      return () => {
        document.removeEventListener("keydown", handleKeyDown);
      };
    }, [context]);

    if (!context.rendered || typeof document === "undefined") {
      return null;
    }

    const state = context.open ? "open" : "closed";

    return createPortal(
      <div className="fixed inset-0 z-[90]">
        <button
          aria-label="Close dialog"
          className={cn(
            "absolute inset-0 h-full w-full bg-[rgba(23,43,77,0.5)] backdrop-blur-[4px]",
            "motion-safe:data-[state=open]:animate-in motion-safe:data-[state=open]:fade-in",
            "motion-safe:data-[state=closed]:animate-out motion-safe:data-[state=closed]:fade-out motion-safe:data-[state=closed]:duration-150",
          )}
          data-state={state}
          onClick={() => {
            if (context.dismissible) {
              context.onOpenChange(false);
            }
          }}
          type="button"
        />
        <div className="absolute inset-0 flex items-center justify-center p-4 md:p-6">
          <div
            aria-describedby={context.descriptionId}
            aria-labelledby={context.titleId}
            aria-modal="true"
            className={cn(
              "relative flex max-h-[90vh] w-full flex-col overflow-hidden rounded-[16px] border border-border bg-surface text-text-primary shadow-xl outline-none",
              "motion-safe:data-[state=open]:animate-in motion-safe:data-[state=open]:fade-in motion-safe:data-[state=open]:zoom-in-95 motion-safe:data-[state=open]:duration-200",
              "motion-safe:data-[state=closed]:animate-out motion-safe:data-[state=closed]:fade-out motion-safe:data-[state=closed]:zoom-out-95 motion-safe:data-[state=closed]:duration-150",
              sizeClassNames[size],
              className,
            )}
            data-state={state}
            ref={(node) => {
              contentRef.current = node;
              if (typeof ref === "function") {
                ref(node);
              } else if (ref) {
                ref.current = node;
              }
            }}
            role="dialog"
            tabIndex={-1}
            {...props}
          >
            {children}
          </div>
        </div>
      </div>,
      document.body,
    );
  },
);

DialogContent.displayName = "DialogContent";

export function DialogHeader({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("border-b border-border px-6 py-6", className)}>
      {children}
    </div>
  );
}

export function DialogTitle({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  const context = useDialogContext();
  return (
    <h2
      className={cn("text-[18px] font-[700] leading-tight text-text-primary", className)}
      id={context.titleId}
    >
      {children}
    </h2>
  );
}

export function DialogDescription({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  const context = useDialogContext();
  return (
    <p
      className={cn("mt-2 text-[14px] leading-6 text-text-secondary", className)}
      id={context.descriptionId}
    >
      {children}
    </p>
  );
}

export function DialogBody({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <div className={cn("max-h-[65vh] overflow-y-auto px-6 py-6", className)}>{children}</div>;
}

export function DialogFooter({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex justify-end gap-3 border-t border-border px-6 py-4", className)}>
      {children}
    </div>
  );
}

export function DialogClose({
  className,
  ...props
}: Omit<React.ComponentProps<typeof Button>, "variant" | "size">) {
  const context = useDialogContext();

  return (
    <Button
      aria-label="Close dialog"
      className={className}
      onClick={(event) => {
        props.onClick?.(event);
        if (!event.defaultPrevented && context.dismissible) {
          context.onOpenChange(false);
        }
      }}
      size="icon"
      type="button"
      variant="ghost"
      {...props}
    >
      <X className="h-4 w-4" />
    </Button>
  );
}

function useDialogContext() {
  const context = useContext(DialogContext);
  if (!context) {
    throw new Error("Dialog components must be used within <Dialog>");
  }
  return context;
}

function getFocusableElements(container: HTMLElement) {
  return Array.from(
    container.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => !element.hasAttribute("aria-hidden"));
}

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

type DrawerSize = "md" | "lg";

interface DrawerContextValue {
  open: boolean;
  rendered: boolean;
  dismissible: boolean;
  titleId: string;
  descriptionId: string;
  onOpenChange: (open: boolean) => void;
}

const DrawerContext = createContext<DrawerContextValue | null>(null);

const sizeClassNames: Record<DrawerSize, string> = {
  md: "max-w-[480px]",
  lg: "max-w-[640px]",
};

export function Drawer({
  open,
  onOpenChange,
  children,
  dismissible = true,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
  dismissible?: boolean;
}) {
  const titleId = useId();
  const descriptionId = useId();
  const [rendered, setRendered] = useState(open);

  useEffect(() => {
    if (open) {
      setRendered(true);
      return undefined;
    }

    const timeout = window.setTimeout(() => setRendered(false), 250);
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

  const value = useMemo(
    () => ({ open, rendered, dismissible, titleId, descriptionId, onOpenChange }),
    [descriptionId, dismissible, onOpenChange, open, rendered, titleId],
  );

  return <DrawerContext.Provider value={value}>{children}</DrawerContext.Provider>;
}

export const DrawerContent = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement> & { size?: DrawerSize; children: ReactNode }
>(({ children, className, size = "md", ...props }, ref) => {
  const context = useDrawerContext();
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
    const target = focusable[0] ?? node;
    window.setTimeout(() => {
      if (context.open) {
        target.focus();
      }
    }, 0);

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && context.dismissible) {
        event.preventDefault();
        context.onOpenChange(false);
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [context]);

  if (!context.rendered || typeof document === "undefined") {
    return null;
  }

  const state = context.open ? "open" : "closed";

  return createPortal(
    <div className="fixed inset-0 z-[95]">
      <button
        aria-label="Close drawer"
        className={cn(
          "absolute inset-0 h-full w-full bg-[rgba(23,43,77,0.5)] backdrop-blur-[4px]",
          "motion-safe:data-[state=open]:animate-in motion-safe:data-[state=open]:fade-in motion-safe:data-[state=open]:duration-150",
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
      <div className="absolute inset-y-0 right-0 flex justify-end">
        <div
          aria-describedby={context.descriptionId}
          aria-labelledby={context.titleId}
          aria-modal="true"
          className={cn(
            "flex h-full w-full flex-col overflow-hidden border-l border-border bg-surface shadow-xl outline-none",
            "motion-safe:data-[state=open]:animate-in motion-safe:data-[state=open]:slide-in-from-right motion-safe:data-[state=open]:duration-200",
            "motion-safe:data-[state=closed]:animate-out motion-safe:data-[state=closed]:slide-out-to-right motion-safe:data-[state=closed]:duration-150",
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
});

DrawerContent.displayName = "DrawerContent";

export function DrawerHeader({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <div className={cn("border-b border-border px-6 py-6", className)}>{children}</div>;
}

export function DrawerTitle({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  const context = useDrawerContext();
  return (
    <h2 className={cn("text-[18px] font-[700] text-text-primary", className)} id={context.titleId}>
      {children}
    </h2>
  );
}

export function DrawerDescription({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  const context = useDrawerContext();
  return (
    <p className={cn("mt-2 text-[14px] leading-6 text-text-secondary", className)} id={context.descriptionId}>
      {children}
    </p>
  );
}

export function DrawerBody({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <div className={cn("flex-1 overflow-y-auto px-6 py-6", className)}>{children}</div>;
}

export function DrawerClose({
  className,
  ...props
}: Omit<React.ComponentProps<typeof Button>, "variant" | "size">) {
  const context = useDrawerContext();

  return (
    <Button
      aria-label="Close drawer"
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

function useDrawerContext() {
  const context = useContext(DrawerContext);
  if (!context) {
    throw new Error("Drawer components must be used within <Drawer>");
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

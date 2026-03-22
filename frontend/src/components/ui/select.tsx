import {
  createPortal,
} from "react-dom";
import {
  forwardRef,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { Check, ChevronDown } from "lucide-react";

import { cn } from "@/lib/utils";

export interface SelectOption {
  value: string;
  label: string;
  description?: string;
  disabled?: boolean;
  icon?: ReactNode;
}

interface SelectProps {
  value?: string;
  options: SelectOption[];
  onValueChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  triggerClassName?: string;
  menuClassName?: string;
  name?: string;
  onBlur?: () => void;
  align?: "left" | "right";
  "aria-label"?: string;
}

export const Select = forwardRef<HTMLButtonElement, SelectProps>(
  (
    {
      value = "",
      options,
      onValueChange,
      placeholder = "Pilih opsi",
      disabled = false,
      className,
      triggerClassName,
      menuClassName,
      name,
      onBlur,
      align = "left",
      "aria-label": ariaLabel,
    },
    ref,
  ) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const triggerRef = useRef<HTMLButtonElement | null>(null);
    const menuRef = useRef<HTMLDivElement>(null);
    const [isOpen, setIsOpen] = useState(false);
    const [menuStyle, setMenuStyle] = useState<CSSProperties | null>(null);

    const selectedOption = useMemo(
      () => options.find((option) => option.value === value),
      [options, value],
    );

    useEffect(() => {
      if (!isOpen) {
        setMenuStyle(null);
        return undefined;
      }

      const updateMenuPosition = () => {
        const trigger = triggerRef.current;
        if (!trigger) {
          return;
        }

        const rect = trigger.getBoundingClientRect();
        const margin = 12;
        const width = Math.min(
          Math.max(220, rect.width),
          window.innerWidth - margin * 2,
        );
        const left =
          align === "right"
            ? Math.min(
                Math.max(margin, rect.right - width),
                window.innerWidth - width - margin,
              )
            : Math.min(
                Math.max(margin, rect.left),
                window.innerWidth - width - margin,
              );
        const spaceBelow = window.innerHeight - rect.bottom - margin;
        const spaceAbove = rect.top - margin;
        const placeAbove = spaceBelow < 220 && spaceAbove > spaceBelow;
        const maxHeight = Math.max(
          180,
          Math.min(320, (placeAbove ? spaceAbove : spaceBelow) - 8),
        );

        setMenuStyle({
          left,
          top: placeAbove
            ? Math.max(margin, rect.top - maxHeight - 8)
            : rect.bottom + 8,
          width,
          maxHeight,
        });
      };

      const handlePointerDown = (event: MouseEvent) => {
        const target = event.target as Node;
        if (
          containerRef.current?.contains(target) ||
          menuRef.current?.contains(target)
        ) {
          return;
        }
        setIsOpen(false);
      };

      const handleEscape = (event: KeyboardEvent) => {
        if (event.key === "Escape") {
          setIsOpen(false);
          triggerRef.current?.focus();
        }
      };

      updateMenuPosition();

      document.addEventListener("mousedown", handlePointerDown);
      document.addEventListener("keydown", handleEscape);
      window.addEventListener("resize", updateMenuPosition);
      window.addEventListener("scroll", updateMenuPosition, true);

      return () => {
        document.removeEventListener("mousedown", handlePointerDown);
        document.removeEventListener("keydown", handleEscape);
        window.removeEventListener("resize", updateMenuPosition);
        window.removeEventListener("scroll", updateMenuPosition, true);
      };
    }, [align, isOpen]);

    return (
      <div className={cn("relative", className)} ref={containerRef}>
        {name ? <input name={name} type="hidden" value={value} /> : null}
        <button
          aria-expanded={isOpen}
          aria-haspopup="listbox"
          aria-label={ariaLabel}
          className={cn(
            "flex h-11 w-full items-center justify-between gap-3 rounded-xl border border-border/70 bg-surface-muted/90 px-3.5 py-2 text-left text-[14px] text-text-primary shadow-none outline-none transition-all duration-150 hover:border-border focus-visible:border-[#4C9AFF] focus-visible:bg-surface focus-visible:shadow-focus disabled:cursor-not-allowed disabled:text-text-tertiary",
            triggerClassName,
          )}
          disabled={disabled}
          onBlur={onBlur}
          onClick={() => setIsOpen((current) => !current)}
          ref={(node) => {
            triggerRef.current = node;
            if (typeof ref === "function") {
              ref(node);
            } else if (ref) {
              ref.current = node;
            }
          }}
          type="button"
        >
          <span className="min-w-0 flex-1 truncate">
            {selectedOption ? selectedOption.label : (
              <span className="text-text-tertiary">{placeholder}</span>
            )}
          </span>
          <ChevronDown
            className={cn(
              "h-4 w-4 shrink-0 text-text-secondary transition-transform duration-150",
              isOpen && "rotate-180",
            )}
          />
        </button>

        {isOpen && menuStyle && typeof document !== "undefined"
          ? createPortal(
              <div
                className={cn(
                  "fixed z-[150] overflow-hidden rounded-2xl border border-border/80 bg-surface/98 p-1.5 shadow-[0_18px_42px_-24px_rgba(15,23,42,0.42)] backdrop-blur-sm",
                  menuClassName,
                )}
                ref={menuRef}
                role="listbox"
                style={menuStyle}
              >
                <div className="max-h-[inherit] overflow-y-auto">
                  {options.map((option) => {
                    const isSelected = option.value === value;

                    return (
                      <button
                        className={cn(
                          "flex w-full items-start gap-3 rounded-xl px-3 py-2.5 text-left transition-colors",
                          option.disabled
                            ? "cursor-not-allowed opacity-55"
                            : "hover:bg-surface-muted/90",
                          isSelected && "bg-surface-muted/80",
                        )}
                        disabled={option.disabled}
                        key={`${option.value}-${option.label}`}
                        onClick={() => {
                          onValueChange(option.value);
                          setIsOpen(false);
                          triggerRef.current?.focus();
                        }}
                        role="option"
                        type="button"
                      >
                        <span className="flex min-h-5 min-w-5 items-center justify-center pt-0.5">
                          {isSelected ? (
                            <Check className="h-4 w-4 text-primary" />
                          ) : option.icon ? (
                            option.icon
                          ) : (
                            <span className="h-1.5 w-1.5 rounded-full bg-border" />
                          )}
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium text-text-primary">
                            {option.label}
                          </span>
                          {option.description ? (
                            <span className="mt-0.5 block text-xs text-text-secondary">
                              {option.description}
                            </span>
                          ) : null}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>,
              document.body,
            )
          : null}
      </div>
    );
  },
);

Select.displayName = "Select";

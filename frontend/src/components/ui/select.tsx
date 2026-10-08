import {
  createPortal,
} from "react-dom";
import {
  forwardRef,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
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
  /** Id of the trigger button, so a form can point a `<label htmlFor>` at it. */
  id?: string;
  onBlur?: () => void;
  align?: "left" | "right";
  /**
   * Only when there is no visible label. A `<label htmlFor={id}>` or
   * `aria-labelledby` names the field and keeps the selected option as its
   * value; a label right before the Select is picked up on its own.
   */
  "aria-label"?: string;
  "aria-labelledby"?: string;
  /** Id(s) of a hint or error message for the field. */
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
  /** Set while a change is being saved; the field stays focusable. */
  "aria-busy"?: boolean;
}

const MENU_GAP = 8;
const VIEWPORT_MARGIN = 12;
const MENU_MAX_HEIGHT = 320;
// How long typed letters add up to one search ("ma" finds "Marketing").
const TYPEAHEAD_RESET_MS = 500;

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
      id,
      onBlur,
      align = "left",
      "aria-label": ariaLabel,
      "aria-labelledby": ariaLabelledBy,
      "aria-describedby": ariaDescribedBy,
      "aria-invalid": ariaInvalid,
      "aria-busy": ariaBusy,
    },
    ref,
  ) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const triggerRef = useRef<HTMLButtonElement | null>(null);
    const menuRef = useRef<HTMLDivElement>(null);
    const [isOpen, setIsOpen] = useState(false);
    const [menuStyle, setMenuStyle] = useState<CSSProperties | null>(null);
    const [activeIndex, setActiveIndex] = useState(-1);
    // True while the active option was reached with the keyboard: it then
    // gets a focus ring, as focus itself stays on the trigger.
    const [isKeyboardActive, setIsKeyboardActive] = useState(false);
    const typeaheadRef = useRef({ text: "", timeout: 0 });
    const baseId = useId();
    const listboxId = `${baseId}-listbox`;
    const optionId = (index: number) => `${baseId}-option-${index}`;

    // Many forms put a plain <label> right before the Select without
    // htmlFor. Use it as the name, so the field is announced as
    // "<label>, combobox, <value>" instead of having no name. A Select with
    // no label at all (some list filters) is named by its current value, as
    // it was before it became a combobox.
    const valueId = `${baseId}-value`;
    const [fallbackLabelId, setFallbackLabelId] = useState<string | undefined>(undefined);
    useLayoutEffect(() => {
      const trigger = triggerRef.current;
      if (!trigger || ariaLabel || ariaLabelledBy || (trigger.labels && trigger.labels.length > 0)) {
        setFallbackLabelId(undefined);
        return;
      }
      const previous = containerRef.current?.previousElementSibling;
      if (previous instanceof HTMLLabelElement && !previous.htmlFor) {
        if (!previous.id) {
          previous.id = `${baseId}-label`;
        }
        setFallbackLabelId(previous.id);
      } else {
        setFallbackLabelId(valueId);
      }
    }, [ariaLabel, ariaLabelledBy, baseId, valueId]);
    const labelledBy = ariaLabelledBy ?? fallbackLabelId;

    const selectedOption = useMemo(
      () => options.find((option) => option.value === value),
      [options, value],
    );

    const enabledIndexes = useMemo(
      () =>
        options.reduce<number[]>((indexes, option, index) => {
          if (!option.disabled) {
            indexes.push(index);
          }
          return indexes;
        }, []),
      [options],
    );

    const openMenu = (fromKeyboard: boolean, startIndex?: number) => {
      const selectedIndex = options.findIndex(
        (option) => option.value === value && !option.disabled,
      );
      setActiveIndex(
        startIndex ?? (selectedIndex >= 0 ? selectedIndex : (enabledIndexes[0] ?? -1)),
      );
      setIsKeyboardActive(fromKeyboard);
      setIsOpen(true);
    };

    const closeMenu = (refocus: boolean) => {
      setIsOpen(false);
      if (refocus) {
        triggerRef.current?.focus({ preventScroll: true });
      }
    };

    const selectOption = (index: number) => {
      const option = options[index];
      if (!option || option.disabled) {
        return;
      }
      onValueChange(option.value);
      closeMenu(true);
    };

    useEffect(() => {
      if (!isOpen) {
        setMenuStyle(null);
        return undefined;
      }

      // Clipping ancestors of the trigger (a dialog's or drawer's scroll
      // area, an overflow-hidden card): the trigger can be cut off by them
      // while still inside the viewport.
      const clippingAncestors: HTMLElement[] = [];
      for (
        let node = triggerRef.current?.parentElement ?? null;
        node && node !== document.body && node !== document.documentElement;
        node = node.parentElement
      ) {
        const style = getComputedStyle(node);
        if (style.display !== "contents" && (style.overflowX !== "visible" || style.overflowY !== "visible")) {
          clippingAncestors.push(node);
        }
      }

      // The menu is portalled to <body>, outside the page's data-module
      // scope, so carry the module accent over. A trigger inside a dialog is
      // portalled too, so fall back to the page shell's module scope.
      const moduleScope =
        triggerRef.current?.closest<HTMLElement>("[data-module]") ??
        document.querySelector<HTMLElement>("[data-module]") ??
        triggerRef.current;
      const moduleAccent = moduleScope
        ? getComputedStyle(moduleScope).getPropertyValue("--module-primary").trim()
        : "";

      let lastPosition = "";
      const updateMenuPosition = () => {
        const trigger = triggerRef.current;
        if (!trigger) {
          return;
        }

        const rect = trigger.getBoundingClientRect();
        // The area the user actually sees: the visual viewport, which is
        // smaller than the window under pinch zoom or with an on-screen
        // keyboard. Its offsets are in the same (layout viewport) coordinates
        // as the trigger's rect and the menu's fixed position.
        const visual = window.visualViewport;
        const view = visual
          ? { left: visual.offsetLeft, top: visual.offsetTop, width: visual.width, height: visual.height }
          : { left: 0, top: 0, width: window.innerWidth, height: window.innerHeight };

        // A menu whose trigger is out of view (scrolled away, cut off by a
        // dialog's scroll area, below a keyboard) would float detached from
        // it, so close it instead.
        let visibleTop = Math.max(rect.top, view.top);
        let visibleBottom = Math.min(rect.bottom, view.top + view.height);
        for (const ancestor of clippingAncestors) {
          const box = ancestor.getBoundingClientRect();
          visibleTop = Math.max(visibleTop, box.top);
          visibleBottom = Math.min(visibleBottom, box.bottom);
        }
        if (visibleBottom - visibleTop < Math.min(rect.height / 2, 16)) {
          setIsOpen(false);
          return;
        }

        const width = Math.min(
          Math.max(220, rect.width),
          view.width - VIEWPORT_MARGIN * 2,
        );
        const minLeft = view.left + VIEWPORT_MARGIN;
        const maxLeft = view.left + view.width - width - VIEWPORT_MARGIN;
        const left =
          align === "right"
            ? Math.min(Math.max(minLeft, rect.right - width), maxLeft)
            : Math.min(Math.max(minLeft, rect.left), maxLeft);
        const spaceBelow = view.top + view.height - rect.bottom - VIEWPORT_MARGIN - MENU_GAP;
        const spaceAbove = rect.top - view.top - VIEWPORT_MARGIN - MENU_GAP;
        // Rough natural height (one 36px row per option plus padding), so a
        // short menu does not flip above just because a long one would not fit.
        const estimatedHeight = Math.min(MENU_MAX_HEIGHT, options.length * 38 + 10);
        const placeAbove = spaceBelow < estimatedHeight && spaceAbove > spaceBelow;
        const maxHeight = Math.max(
          0,
          Math.min(MENU_MAX_HEIGHT, placeAbove ? spaceAbove : spaceBelow),
        );
        const anchor = placeAbove
          ? { bottom: window.innerHeight - rect.top + MENU_GAP }
          : { top: rect.bottom + MENU_GAP };
        const position = JSON.stringify([left, width, maxHeight, anchor]);
        if (position === lastPosition) {
          return;
        }
        lastPosition = position;
        setMenuStyle({
          left,
          width,
          maxHeight,
          ...anchor,
          ...(moduleAccent
            ? ({ "--module-primary": moduleAccent } as CSSProperties)
            : {}),
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

      // Escape closes only this menu. The listener runs in the capture phase
      // and stops the event, so a surrounding dialog or drawer (which listens
      // on document in the bubble phase) stays open; a second Escape closes it.
      const handleEscape = (event: KeyboardEvent) => {
        if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          setIsOpen(false);
          triggerRef.current?.focus();
        }
      };

      updateMenuPosition();
      // Follow the trigger every frame while open: besides scroll and
      // resize, a dialog can re-lay out (a viewport height change, an
      // on-screen keyboard, content above the field growing) without any
      // event here. Only a changed position re-renders.
      let frame = window.requestAnimationFrame(function follow() {
        updateMenuPosition();
        frame = window.requestAnimationFrame(follow);
      });

      document.addEventListener("mousedown", handlePointerDown);
      document.addEventListener("keydown", handleEscape, true);

      return () => {
        window.cancelAnimationFrame(frame);
        document.removeEventListener("mousedown", handlePointerDown);
        document.removeEventListener("keydown", handleEscape, true);
      };
    }, [align, isOpen, options.length]);

    const activeOptionId =
      isOpen && activeIndex >= 0 ? optionId(activeIndex) : undefined;
    const isPositioned = menuStyle !== null;

    useEffect(() => {
      if (!activeOptionId || !isPositioned) {
        return;
      }
      document.getElementById(activeOptionId)?.scrollIntoView({ block: "nearest" });
    }, [activeOptionId, isPositioned]);

    useEffect(() => () => window.clearTimeout(typeaheadRef.current.timeout), []);

    // Type-ahead as in a native select: letters typed within a short time
    // form one search; the next enabled option whose label starts with it
    // becomes active (and the menu opens if it was closed).
    const typeahead = (key: string) => {
      const state = typeaheadRef.current;
      window.clearTimeout(state.timeout);
      state.text += key.toLowerCase();
      state.timeout = window.setTimeout(() => {
        state.text = "";
      }, TYPEAHEAD_RESET_MS);

      if (enabledIndexes.length === 0) {
        return;
      }
      const current = isOpen ? activeIndex : options.findIndex((option) => option.value === value);
      // The same letter typed again ("dd") searches for that one letter, so
      // repeating it cycles through the options that start with it, however
      // fast it is pressed (as a native select does).
      const first = state.text.charAt(0);
      const search = [...state.text].every((character) => character === first) ? first : state.text;
      // A single letter moves past the current option (that is the cycling).
      const startPosition = Math.max(0, enabledIndexes.indexOf(current));
      const offset = search.length === 1 ? 1 : 0;
      for (let step = 0; step < enabledIndexes.length; step += 1) {
        const index = enabledIndexes[(startPosition + offset + step) % enabledIndexes.length]!;
        const label = options[index]?.label.toLowerCase() ?? "";
        if (label.startsWith(search)) {
          if (isOpen) {
            setActiveIndex(index);
            setIsKeyboardActive(true);
          } else {
            openMenu(true, index);
          }
          return;
        }
      }
    };

    const moveActive = (direction: "next" | "previous" | "first" | "last") => {
      if (enabledIndexes.length === 0) {
        return;
      }
      const position = enabledIndexes.indexOf(activeIndex);
      let nextPosition = 0;
      if (direction === "last") {
        nextPosition = enabledIndexes.length - 1;
      } else if (direction === "next" && position >= 0) {
        nextPosition = Math.min(enabledIndexes.length - 1, position + 1);
      } else if (direction === "previous" && position >= 0) {
        nextPosition = Math.max(0, position - 1);
      }
      setActiveIndex(enabledIndexes[nextPosition] ?? -1);
      setIsKeyboardActive(true);
    };

    const handleTriggerKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>) => {
      const isPrintable =
        event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey;
      // A space while a search is being typed belongs to the search.
      if (isPrintable && (event.key !== " " || typeaheadRef.current.text !== "")) {
        event.preventDefault();
        typeahead(event.key);
        return;
      }

      if (!isOpen) {
        if (["ArrowDown", "ArrowUp", "Enter", " "].includes(event.key)) {
          event.preventDefault();
          openMenu(true);
        }
        return;
      }

      switch (event.key) {
        case "ArrowDown":
          event.preventDefault();
          moveActive("next");
          break;
        case "ArrowUp":
          event.preventDefault();
          moveActive("previous");
          break;
        case "Home":
          event.preventDefault();
          moveActive("first");
          break;
        case "End":
          event.preventDefault();
          moveActive("last");
          break;
        case "Enter":
        case " ":
          event.preventDefault();
          if (activeIndex >= 0) {
            selectOption(activeIndex);
          } else {
            closeMenu(true);
          }
          break;
        case "Tab":
          setIsOpen(false);
          break;
        default:
          break;
      }
    };

    return (
      <div className={cn("relative", className)} ref={containerRef}>
        {name ? <input name={name} type="hidden" value={value} /> : null}
        {/* Select-only combobox (WAI-ARIA APG): the label names the field and
            the button's text (the selected option) is its value. */}
        <button
          aria-activedescendant={activeOptionId}
          aria-busy={ariaBusy || undefined}
          aria-controls={isOpen ? listboxId : undefined}
          aria-describedby={ariaDescribedBy}
          aria-expanded={isOpen}
          aria-haspopup="listbox"
          aria-invalid={ariaInvalid || undefined}
          aria-label={labelledBy ? undefined : ariaLabel}
          aria-labelledby={labelledBy}
          className={cn(
            "flex h-11 w-full items-center justify-between gap-3 rounded-xl border border-border/70 bg-surface-muted/90 px-3.5 py-2 text-left text-[14px] text-text-primary shadow-none outline-none transition-all duration-150 hover:border-border focus-visible:border-[#4C9AFF] focus-visible:bg-surface focus-visible:shadow-focus disabled:cursor-not-allowed disabled:bg-surface-muted disabled:text-text-tertiary disabled:opacity-60 aria-[invalid=true]:border-error",
            triggerClassName,
          )}
          disabled={disabled}
          id={id}
          onBlur={onBlur}
          onClick={(event) => {
            if (isOpen) {
              setIsOpen(false);
            } else {
              // detail is 0 when Enter/Space "clicked" the button.
              openMenu(event.detail === 0);
            }
          }}
          onKeyDown={handleTriggerKeyDown}
          onKeyUp={(event) => {
            // Space activates a button on keyup; the keydown handler already
            // acted on it, so keep the browser from toggling the menu again.
            if (event.key === " ") {
              event.preventDefault();
            }
          }}
          ref={(node) => {
            triggerRef.current = node;
            if (typeof ref === "function") {
              ref(node);
            } else if (ref) {
              ref.current = node;
            }
          }}
          role="combobox"
          type="button"
        >
          <span className="min-w-0 flex-1 truncate" id={valueId}>
            {selectedOption ? selectedOption.label : (
              <span className="text-text-secondary">{placeholder}</span>
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
                aria-label={labelledBy ? undefined : ariaLabel}
                aria-labelledby={labelledBy === valueId ? undefined : labelledBy}
                className={cn(
                  "fixed z-[150] overflow-y-auto overscroll-contain rounded-xl border border-border bg-surface p-1 shadow-lg dark:shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)] motion-safe:animate-in motion-safe:fade-in-0 motion-safe:zoom-in-95 motion-safe:duration-150",
                  menuClassName,
                )}
                id={listboxId}
                ref={menuRef}
                role="listbox"
                style={menuStyle}
              >
                {options.map((option, index) => {
                  const isSelected = option.value === value;
                  const isActive = index === activeIndex;

                  return (
                    <button
                      aria-disabled={option.disabled || undefined}
                      aria-selected={isSelected}
                      className={cn(
                        "flex w-full items-start gap-2.5 rounded-lg px-2.5 py-2 text-left transition-colors duration-100",
                        option.disabled && "cursor-not-allowed opacity-55",
                        // The active option is the only highlighted one (the
                        // pointer makes an option active by moving onto it),
                        // so the fill always shows what Enter picks.
                        isActive && !option.disabled && "bg-surface-muted",
                        // Keyboard position: a ring in the focus colour, as a
                        // fill alone is too faint against the menu.
                        isActive && !option.disabled && isKeyboardActive && "ring-2 ring-inset ring-ring",
                      )}
                      disabled={option.disabled}
                      id={optionId(index)}
                      key={`${option.value}-${option.label}`}
                      onClick={() => selectOption(index)}
                      // Only a real pointer movement moves the active option.
                      // A menu that opens (or scrolls) under a resting pointer
                      // gets mouseenter/mouseover from the browser, and a
                      // synthetic mousemove without movement; reacting to
                      // those would undo what the keyboard just chose.
                      onMouseMove={(event) => {
                        if (option.disabled || (event.movementX === 0 && event.movementY === 0)) {
                          return;
                        }
                        if (activeIndex !== index || isKeyboardActive) {
                          setActiveIndex(index);
                          setIsKeyboardActive(false);
                        }
                      }}
                      role="option"
                      tabIndex={-1}
                      type="button"
                    >
                      <span className="flex h-5 w-5 shrink-0 items-center justify-center">
                        {isSelected ? (
                          <Check className="h-4 w-4 text-[color:var(--module-primary)]" />
                        ) : option.icon ? (
                          option.icon
                        ) : null}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span
                          className={cn(
                            "block truncate text-sm text-text-primary",
                            isSelected ? "font-semibold" : "font-medium",
                          )}
                        >
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
              </div>,
              document.body,
            )
          : null}
      </div>
    );
  },
);

Select.displayName = "Select";

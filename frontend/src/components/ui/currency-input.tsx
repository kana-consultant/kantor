import * as React from "react";

import { formatRupiahInput, parseRupiahInput } from "@/lib/currency";
import { cn } from "@/lib/utils";

interface CurrencyInputProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "type" | "value" | "onChange"> {
  value?: number;
  onValueChange?: (value: number) => void;
}

const CurrencyInput = React.forwardRef<HTMLInputElement, CurrencyInputProps>(
  ({ className, onValueChange, placeholder = "0", value = 0, ...props }, ref) => (
    <div className="relative">
      <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm font-semibold text-text-secondary">
        Rp
      </span>
      <input
        {...props}
        className={cn(
          "flex h-10 w-full rounded-sm border-[1.5px] border-transparent bg-surface-muted py-2 pl-10 pr-3 text-[14px] font-mono tabular-nums text-text-primary shadow-none outline-none transition-all duration-150 placeholder:text-text-tertiary focus-visible:border-[#4C9AFF] focus-visible:bg-surface focus-visible:shadow-focus disabled:cursor-not-allowed disabled:text-text-tertiary",
          className,
        )}
        inputMode="numeric"
        onChange={(event) => onValueChange?.(parseRupiahInput(event.target.value))}
        placeholder={placeholder}
        ref={ref}
        type="text"
        value={formatRupiahInput(value)}
      />
    </div>
  ),
);

CurrencyInput.displayName = "CurrencyInput";

export { CurrencyInput };

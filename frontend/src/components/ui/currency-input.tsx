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
      <span className="pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 text-sm font-semibold text-muted-foreground">
        Rp
      </span>
      <input
        {...props}
        className={cn(
          "flex h-12 w-full rounded-2xl border border-input bg-card/80 py-3 pl-12 pr-4 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring",
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

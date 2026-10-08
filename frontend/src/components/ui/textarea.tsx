import * as React from "react";

import { inputBaseClassName } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export type TextareaProps = React.ComponentPropsWithoutRef<"textarea">;

// A multi-line Input: same border, fill and focus style.
const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(({ className, ...props }, ref) => (
  <textarea className={cn(inputBaseClassName, "min-h-[96px] py-2.5", className)} ref={ref} {...props} />
));

Textarea.displayName = "Textarea";

export { Textarea };

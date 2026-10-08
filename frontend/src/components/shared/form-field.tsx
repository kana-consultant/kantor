import { cn } from "@/lib/utils";

// Typography shared by the Marketing forms (campaign, ads metrics), so labels,
// sub-labels and messages look the same in every dialog.
export const formLabelClassName = "text-[13px] font-[500] text-text-secondary";
export const formSubLabelClassName = "text-[12px] text-text-secondary";
export const formHintClassName = "text-[12px] text-text-secondary";

export function RequiredMark() {
  return (
    <span aria-hidden="true" className="ml-0.5 text-error">
      *
    </span>
  );
}

// A field's validation message. Give it an id and point the control's
// aria-describedby at it, so a screen reader reads it with the field.
export function FieldError({ id, message, className }: { id?: string; message?: string; className?: string }) {
  return message ? (
    <p className={cn("mt-1 text-[12px] font-[500] text-error-strong", className)} id={id}>
      {message}
    </p>
  ) : null;
}

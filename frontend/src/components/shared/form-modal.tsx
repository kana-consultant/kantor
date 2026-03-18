import type { FormEvent, ReactNode } from "react";

import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";

type FormModalSize = "sm" | "md" | "lg" | "xl";

interface FormModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  title: string;
  subtitle?: string;
  submitLabel: string;
  cancelLabel?: string;
  isLoading?: boolean;
  submitDisabled?: boolean;
  error?: string | null;
  size?: FormModalSize;
  children: ReactNode;
}

export function FormModal({
  isOpen,
  onClose,
  onSubmit,
  title,
  subtitle,
  submitLabel,
  cancelLabel = "Batal",
  isLoading = false,
  submitDisabled = false,
  error,
  size = "lg",
  children,
}: FormModalProps) {
  return (
    <Dialog dismissible={!isLoading} onOpenChange={(open) => (!open ? onClose() : undefined)} open={isOpen}>
      <DialogContent size={size}>
        <form onSubmit={onSubmit}>
          <DialogHeader className="flex items-start justify-between gap-4">
            <div>
              <DialogTitle>{title}</DialogTitle>
              {subtitle ? <DialogDescription>{subtitle}</DialogDescription> : null}
            </div>
            <DialogClose disabled={isLoading} />
          </DialogHeader>
          <DialogBody>
            {error ? (
              <div className="mb-4 rounded-md border border-error/20 bg-error-light px-4 py-3 text-sm text-error">
                {error}
              </div>
            ) : null}
            <div className="space-y-4">{children}</div>
          </DialogBody>
          <DialogFooter>
            <Button disabled={isLoading} onClick={onClose} type="button" variant="ghost">
              {cancelLabel}
            </Button>
            <Button disabled={isLoading || submitDisabled} type="submit">
              {isLoading ? "Menyimpan..." : submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

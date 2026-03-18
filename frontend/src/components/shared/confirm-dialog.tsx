import { useEffect, useState } from "react";
import { AlertTriangle } from "lucide-react";

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
import { cn } from "@/lib/utils";

interface ConfirmDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (note?: string) => void;
  title: string;
  description: string;
  confirmLabel: string;
  isLoading?: boolean;
  noteLabel?: string;
  notePlaceholder?: string;
  noteRequired?: boolean;
  tone?: "danger" | "warning";
}

export function ConfirmDialog({
  isOpen,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel,
  isLoading = false,
  noteLabel,
  notePlaceholder,
  noteRequired = false,
  tone = "danger",
}: ConfirmDialogProps) {
  const [note, setNote] = useState("");

  useEffect(() => {
    if (!isOpen) {
      setNote("");
    }
  }, [isOpen]);

  return (
    <Dialog dismissible={!isLoading} onOpenChange={(open) => (!open ? onClose() : undefined)} open={isOpen}>
      <DialogContent size="sm">
        <DialogHeader className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-4">
            <div
              className={cn(
                "flex h-10 w-10 shrink-0 items-center justify-center rounded-full",
                tone === "danger" ? "bg-error-light text-error" : "bg-warning-light text-warning",
              )}
            >
              <AlertTriangle className="h-5 w-5" />
            </div>
            <div>
              <DialogTitle>{title}</DialogTitle>
              <DialogDescription>{description}</DialogDescription>
            </div>
          </div>
          <DialogClose disabled={isLoading} />
        </DialogHeader>
        <DialogBody>
          {noteLabel ? (
            <div className="space-y-2">
              <label className="text-[13px] font-[600] text-text-primary" htmlFor="confirm-dialog-note">
                {noteLabel}
              </label>
              <textarea
                className="min-h-28 w-full rounded-[6px] border-[1.5px] border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary outline-none transition-all duration-150 placeholder:text-text-tertiary focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
                id="confirm-dialog-note"
                onChange={(event) => setNote(event.target.value)}
                placeholder={notePlaceholder}
                value={note}
              />
            </div>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button disabled={isLoading} onClick={onClose} type="button" variant="ghost">
            Batal
          </Button>
          <Button
            disabled={isLoading || (noteRequired && note.trim().length === 0)}
            onClick={() => onConfirm(note.trim())}
            type="button"
            variant={tone === "danger" ? "danger" : "outline"}
          >
            {isLoading ? "Memproses..." : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

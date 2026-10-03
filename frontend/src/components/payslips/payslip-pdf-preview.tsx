import { useEffect, useState } from "react";
import { ExternalLink, FileWarning, LoaderCircle } from "lucide-react";

import { Button } from "@/components/ui/button";
import { downloadPayslipPDF } from "@/services/hris-payslips";

type PreviewState =
  | { kind: "loading" }
  | { kind: "ready"; url: string; filename: string | undefined }
  | { kind: "error"; message: string };

/**
 * Inline PDF preview of one slip. The PDF is fetched with the bearer token
 * (the server logs the access before streaming) and shown through an object
 * URL that is revoked when the version changes or the component unmounts,
 * i.e. when the drawer closes. `version` (the PDF sha256) re-fetches after a
 * re-render.
 */
export function PayslipPdfPreview({ payslipId, version }: { payslipId: string; version: string }) {
  const [state, setState] = useState<PreviewState>({ kind: "loading" });

  useEffect(() => {
    let active = true;
    let objectUrl: string | null = null;
    setState({ kind: "loading" });

    downloadPayslipPDF(payslipId, "inline")
      .then((result) => {
        const blob = result.blob.type === "application/pdf"
          ? result.blob
          : new Blob([result.blob], { type: "application/pdf" });
        const url = URL.createObjectURL(blob);
        if (!active) {
          URL.revokeObjectURL(url);
          return;
        }
        objectUrl = url;
        setState({ kind: "ready", url, filename: result.filename });
      })
      .catch((error: unknown) => {
        if (active) {
          setState({ kind: "error", message: error instanceof Error ? error.message : "PDF tidak dapat dimuat" });
        }
      });

    return () => {
      active = false;
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [payslipId, version]);

  if (state.kind === "loading") {
    return (
      <div className="flex h-[520px] items-center justify-center rounded-2xl border border-border/70 bg-surface-muted/60 text-sm text-text-secondary">
        <LoaderCircle className="mr-2 h-4 w-4 animate-spin" />
        Memuat pratinjau PDF...
      </div>
    );
  }

  if (state.kind === "error") {
    return (
      <div className="flex h-[200px] flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-border bg-surface-muted/60 px-6 text-center text-sm text-text-secondary">
        <FileWarning className="h-5 w-5 text-error" />
        {state.message}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <iframe
        className="h-[620px] w-full rounded-2xl border border-border/70 bg-surface-muted"
        data-testid="payslip-pdf-frame"
        // Chrome's viewer: hide the thumbnail pane and fit the page width.
        src={`${state.url}#navpanes=0&view=FitH`}
        title={state.filename ?? "Pratinjau slip gaji"}
      />
      <div className="flex justify-end">
        <Button
          onClick={() => window.open(state.url, "_blank", "noopener")}
          size="sm"
          type="button"
          variant="ghost"
        >
          <ExternalLink className="h-4 w-4" />
          Buka di tab baru
        </Button>
      </div>
    </div>
  );
}

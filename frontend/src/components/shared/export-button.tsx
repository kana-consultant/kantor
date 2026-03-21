import { useEffect, useMemo, useRef, useState } from "react";
import {
  Download,
  FileDown,
  FileSpreadsheet,
  FileText,
  LoaderCircle,
} from "lucide-react";

import { authDownload, ApiError } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import { toast } from "@/stores/toast-store";
import { Button, type ButtonProps } from "@/components/ui/button";

type ExportFormat = "csv" | "xlsx" | "pdf";

export interface ExportButtonProps {
  endpoint: string;
  filters?: Record<string, unknown>;
  formats: ExportFormat[];
  filename?: string;
  className?: string;
  align?: "left" | "right";
  size?: ButtonProps["size"];
  variant?: ButtonProps["variant"];
}

const formatMeta: Record<
  ExportFormat,
  { icon: typeof FileText; label: string }
> = {
  pdf: {
    icon: FileText,
    label: "Export PDF",
  },
  xlsx: {
    icon: FileSpreadsheet,
    label: "Export Excel",
  },
  csv: {
    icon: FileDown,
    label: "Export CSV",
  },
};

export function ExportButton({
  endpoint,
  filters,
  formats,
  filename,
  className,
  align = "right",
  size = "default",
  variant = "outline",
}: ExportButtonProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [loadingFormat, setLoadingFormat] = useState<ExportFormat | null>(null);

  const normalizedFormats = useMemo(
    () => Array.from(new Set(formats)),
    [formats],
  );

  useEffect(() => {
    if (!isOpen) {
      return undefined;
    }

    const handlePointerDown = (event: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(event.target as Node)
      ) {
        setIsOpen(false);
      }
    };

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setIsOpen(false);
      }
    };

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleEscape);

    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleEscape);
    };
  }, [isOpen]);

  const downloadReport = async (format: ExportFormat) => {
    setIsOpen(false);
    setLoadingFormat(format);

    try {
      const search = new URLSearchParams();
      search.set("format", format);

      for (const [key, value] of Object.entries(filters ?? {})) {
        appendFilter(search, key, value);
      }

      const query = search.toString();
      const result = await authDownload(
        `${endpoint}${query ? `?${query}` : ""}`,
      );
      const resolvedFilename =
        result.filename ?? `${filename ?? defaultFilename(endpoint)}.${format}`;

      triggerDownload(result.blob, resolvedFilename);
    } catch (error) {
      const description =
        error instanceof ApiError
          ? error.message
          : "Coba lagi beberapa saat lagi.";
      toast.error("Export gagal", description);
    } finally {
      setLoadingFormat(null);
    }
  };

  return (
    <div className={cn("relative", className)} ref={containerRef}>
      <Button
        disabled={Boolean(loadingFormat)}
        onClick={() => setIsOpen((current) => !current)}
        size={size}
        type="button"
        variant={variant}
      >
        {loadingFormat ? (
          <LoaderCircle className="h-4 w-4 animate-spin" />
        ) : (
          <Download className="h-4 w-4" />
        )}
        Export
      </Button>

      {isOpen ? (
        <div
          className={cn(
            "absolute top-full z-20 mt-2 min-w-[190px] rounded-lg border border-border bg-surface p-2 shadow-lg",
            align === "right" ? "right-0" : "left-0",
          )}
          role="menu"
        >
          <div className="flex flex-col gap-1">
            {normalizedFormats.map((format) => {
              const meta = formatMeta[format];
              const Icon = meta.icon;

              return (
                <button
                  className="flex items-center gap-3 rounded-md px-3 py-2 text-left text-sm font-medium text-text-primary transition hover:bg-surface-muted disabled:cursor-not-allowed disabled:opacity-50"
                  disabled={Boolean(loadingFormat)}
                  key={format}
                  onClick={() => void downloadReport(format)}
                  role="menuitem"
                  type="button"
                >
                  <Icon className="h-4 w-4 text-text-secondary" />
                  <span>{meta.label}</span>
                </button>
              );
            })}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function appendFilter(
  search: URLSearchParams,
  key: string,
  value: unknown,
) {
  if (value === undefined || value === null || value === "") {
    return;
  }

  if (Array.isArray(value)) {
    value.forEach((item) => appendFilter(search, key, item));
    return;
  }

  if (value instanceof Date) {
    search.append(key, value.toISOString());
    return;
  }

  search.append(key, String(value));
}

function defaultFilename(endpoint: string) {
  const parts = endpoint.split("/").filter(Boolean);
  const meaningful = parts.at(-1) === "export" ? parts.at(-2) : parts.at(-1);
  return meaningful ?? "report";
}

function triggerDownload(blob: Blob, filename: string) {
  const objectUrl = window.URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = filename;
  anchor.click();
  window.URL.revokeObjectURL(objectUrl);
}

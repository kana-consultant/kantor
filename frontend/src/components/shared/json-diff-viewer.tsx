import { useMemo } from "react";

import { cn } from "@/lib/utils";

interface JsonDiffViewerProps {
  oldValue: unknown | null;
  newValue: unknown | null;
}

type DiffStatus = "added" | "removed" | "changed" | "unchanged";

interface DiffRow {
  path: string;
  oldValue: unknown;
  newValue: unknown;
  status: DiffStatus;
}

export function JsonDiffViewer({ oldValue, newValue }: JsonDiffViewerProps) {
  const rows = useMemo(() => buildDiffRows(oldValue, newValue), [oldValue, newValue]);

  return (
    <div className="overflow-hidden rounded-md border border-border bg-surface">
      <div className="grid grid-cols-[minmax(180px,220px)_1fr_1fr] border-b border-border bg-surface-muted text-[11px] font-semibold uppercase tracking-[0.08em] text-text-secondary">
        <div className="px-4 py-3">Field</div>
        <div className="border-l border-border px-4 py-3">Old Value</div>
        <div className="border-l border-border px-4 py-3">New Value</div>
      </div>

      {rows.length === 0 ? (
        <div className="px-4 py-5 font-mono text-sm text-text-secondary">
          Tidak ada perubahan JSON yang bisa ditampilkan.
        </div>
      ) : (
        <div className="divide-y divide-border">
          {rows.map((row) => (
            <div
              className={cn(
                "grid grid-cols-[minmax(180px,220px)_1fr_1fr]",
                row.status === "added" && "bg-success-light/50",
                row.status === "removed" && "bg-error-light/50",
                row.status === "changed" && "bg-warning-light/40",
              )}
              key={row.path}
            >
              <div className="px-4 py-3 font-mono text-[13px] text-text-primary">{row.path}</div>
              <div className="border-l border-border px-4 py-3">
                <pre className="whitespace-pre-wrap break-all font-mono text-[12px] leading-6 text-text-secondary">
                  {formatJsonValue(row.oldValue)}
                </pre>
              </div>
              <div className="border-l border-border px-4 py-3">
                <pre className="whitespace-pre-wrap break-all font-mono text-[12px] leading-6 text-text-primary">
                  {formatJsonValue(row.newValue)}
                </pre>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function buildDiffRows(oldValue: unknown | null, newValue: unknown | null): DiffRow[] {
  const oldFlat = flattenValue(oldValue);
  const newFlat = flattenValue(newValue);
  const paths = Array.from(new Set([...oldFlat.keys(), ...newFlat.keys()])).sort();

  return paths.map((path) => {
    const oldEntry = oldFlat.get(path);
    const newEntry = newFlat.get(path);

    if (oldEntry === undefined) {
      return { path, oldValue: null, newValue: newEntry, status: "added" };
    }
    if (newEntry === undefined) {
      return { path, oldValue: oldEntry, newValue: null, status: "removed" };
    }

    const status = isSameValue(oldEntry, newEntry) ? "unchanged" : "changed";
    return { path, oldValue: oldEntry, newValue: newEntry, status };
  });
}

function flattenValue(input: unknown, parentPath = ""): Map<string, unknown> {
  const output = new Map<string, unknown>();

  if (input === null || input === undefined) {
    if (parentPath) {
      output.set(parentPath, input);
    }
    return output;
  }

  if (Array.isArray(input)) {
    if (input.length === 0) {
      output.set(parentPath || "$", []);
      return output;
    }

    input.forEach((item, index) => {
      const nextPath = parentPath ? `${parentPath}[${index}]` : `$[${index}]`;
      const nested = flattenValue(item, nextPath);
      if (nested.size === 0) {
        output.set(nextPath, item);
        return;
      }
      nested.forEach((value, key) => output.set(key, value));
    });
    return output;
  }

  if (isPlainObject(input)) {
    const entries = Object.entries(input);
    if (entries.length === 0) {
      output.set(parentPath || "$", {});
      return output;
    }

    entries.forEach(([key, value]) => {
      const nextPath = parentPath ? `${parentPath}.${key}` : key;
      const nested = flattenValue(value, nextPath);
      if (nested.size === 0) {
        output.set(nextPath, value);
        return;
      }
      nested.forEach((nestedValue, nestedKey) => output.set(nestedKey, nestedValue));
    });
    return output;
  }

  output.set(parentPath || "$", input);
  return output;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isSameValue(left: unknown, right: unknown) {
  return JSON.stringify(left) === JSON.stringify(right);
}

function formatJsonValue(value: unknown) {
  if (value === undefined || value === null) {
    return "-";
  }

  if (typeof value === "string") {
    return value;
  }

  return JSON.stringify(value, null, 2);
}

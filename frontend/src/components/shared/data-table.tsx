import { Fragment } from "react";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { ChevronDown, ChevronUp, Inbox } from "lucide-react";

import { EmptyState } from "@/components/shared/empty-state";
import { DataTablePageSkeleton } from "@/components/shared/skeletons";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export interface DataTableColumn<TData> {
  id: string;
  header: string;
  accessor?: keyof TData;
  sortable?: boolean;
  numeric?: boolean;
  align?: "left" | "center" | "right";
  widthClassName?: string;
  cell?: (row: TData) => ReactNode;
}

interface DataTableProps<TData> {
  columns: Array<DataTableColumn<TData>>;
  data: TData[];
  loading?: boolean;
  loadingRows?: number;
  getRowId: (row: TData) => string;
  getRowClassName?: (row: TData) => string | undefined;
  selectedRowId?: string | null;
  onRowClick?: (row: TData) => void;
  renderExpandedRow?: (row: TData) => ReactNode;
  emptyTitle: string;
  emptyDescription: string;
  emptyActionLabel?: string;
  onEmptyAction?: () => void;
  pagination?: {
    page: number;
    perPage: number;
    total: number;
    onPageChange: (page: number) => void;
  };
}

type SortState = {
  columnId: string;
  direction: "asc" | "desc";
} | null;

export function DataTable<TData>({
  columns,
  data,
  loading = false,
  loadingRows = 5,
  getRowId,
  getRowClassName,
  selectedRowId,
  onRowClick,
  renderExpandedRow,
  emptyTitle,
  emptyDescription,
  emptyActionLabel,
  onEmptyAction,
  pagination,
}: DataTableProps<TData>) {
  const [sortState, setSortState] = useState<SortState>(null);

  const sortedRows = useMemo(() => {
    if (!sortState) {
      return data;
    }

    const column = columns.find((item) => item.id === sortState.columnId);
    if (!column) {
      return data;
    }

    const next = [...data];
    next.sort((left, right) => {
      const leftValue = readValue(left, column);
      const rightValue = readValue(right, column);

      if (leftValue === rightValue) {
        return 0;
      }
      if (leftValue == null) {
        return 1;
      }
      if (rightValue == null) {
        return -1;
      }

      if (typeof leftValue === "number" && typeof rightValue === "number") {
        return sortState.direction === "asc" ? leftValue - rightValue : rightValue - leftValue;
      }

      const comparison = String(leftValue).localeCompare(String(rightValue), "id");
      return sortState.direction === "asc" ? comparison : -comparison;
    });

    return next;
  }, [columns, data, sortState]);

  const totalPages = pagination ? Math.max(1, Math.ceil(pagination.total / pagination.perPage)) : 1;

  if (loading) {
    return <DataTablePageSkeleton columns={columns.length} rows={loadingRows} />;
  }

  if (sortedRows.length === 0) {
    return (
      <EmptyState
        actionLabel={emptyActionLabel}
        description={emptyDescription}
        icon={Inbox}
        onAction={onEmptyAction}
        title={emptyTitle}
      />
    );
  }

  return (
    <div className="table-shell">
      <div className="overflow-x-auto">
        <table className="min-w-full border-collapse">
          <thead className="bg-surface-muted">
            <tr>
              {columns.map((column) => (
                <th
                  className={cn(
                    "px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.08em] text-text-secondary",
                    column.widthClassName,
                    column.align === "right" && "text-right",
                    column.align === "center" && "text-center",
                  )}
                  key={column.id}
                >
                  {column.sortable ? (
                    <button
                      className="inline-flex items-center gap-2 transition hover:text-text-primary"
                      onClick={() => setSortState(toggleSort(sortState, column.id))}
                      type="button"
                    >
                      <span>{column.header}</span>
                      <SortIndicator active={sortState?.columnId === column.id} direction={sortState?.direction} />
                    </button>
                  ) : (
                    column.header
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {sortedRows.map((row) => {
              const rowId = getRowId(row);
              const selected = rowId === selectedRowId;

              return (
                <Fragment key={rowId}>
                  <tr
                    className={cn(
                      "border-b border-border transition last:border-b-0 hover:bg-surface-muted",
                      selected && "bg-module-light shadow-[inset_3px_0_0_0_var(--module-primary)]",
                      getRowClassName?.(row),
                      onRowClick && "cursor-pointer",
                    )}
                    onClick={() => onRowClick?.(row)}
                  >
                    {columns.map((column) => (
                      <td
                        className={cn(
                          "px-4 py-4 align-top text-sm text-text-primary",
                          column.numeric && "font-mono tabular-nums",
                          column.align === "right" && "text-right",
                          column.align === "center" && "text-center",
                        )}
                        key={column.id}
                      >
                        {column.cell ? column.cell(row) : formatValue(readValue(row, column))}
                      </td>
                    ))}
                  </tr>
                  {selected && renderExpandedRow ? (
                    <tr className="border-b border-border bg-surface-muted/40 last:border-b-0">
                      <td className="px-4 py-4" colSpan={columns.length}>
                        {renderExpandedRow(row)}
                      </td>
                    </tr>
                  ) : null}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>

      {pagination ? (
        <div className="flex flex-col gap-3 border-t border-border px-4 py-4 md:flex-row md:items-center md:justify-between">
          <p className="text-sm text-text-secondary">
            Page {pagination.page} of {totalPages} | Total {pagination.total}
          </p>
          <div className="flex items-center gap-2">
            <Button
              disabled={pagination.page <= 1}
              onClick={() => pagination.onPageChange(pagination.page - 1)}
              size="sm"
              variant="outline"
            >
              Previous
            </Button>
            <Button
              disabled={pagination.page >= totalPages}
              onClick={() => pagination.onPageChange(pagination.page + 1)}
              size="sm"
            >
              Next
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function readValue<TData>(row: TData, column: DataTableColumn<TData>) {
  if (!column.accessor) {
    return undefined;
  }

  return row[column.accessor];
}

function formatValue(value: unknown) {
  if (value == null || value === "") {
    return "-";
  }

  return String(value);
}

function toggleSort(current: SortState, columnId: string): SortState {
  if (!current || current.columnId !== columnId) {
    return { columnId, direction: "asc" };
  }

  if (current.direction === "asc") {
    return { columnId, direction: "desc" };
  }

  return null;
}

function SortIndicator({
  active,
  direction,
}: {
  active: boolean;
  direction?: "asc" | "desc";
}) {
  if (!active || !direction) {
    return <ChevronDown className="h-3.5 w-3.5 text-text-tertiary" />;
  }

  return direction === "asc" ? (
    <ChevronUp className="h-3.5 w-3.5 text-text-primary" />
  ) : (
    <ChevronDown className="h-3.5 w-3.5 text-text-primary" />
  );
}

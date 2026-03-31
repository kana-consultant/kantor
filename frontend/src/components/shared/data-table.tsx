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
  mobilePrimary?: boolean;
  hideOnMobile?: boolean;
}

export type SortState = {
  columnId: string;
  direction: "asc" | "desc";
} | null;

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
  manualSorting?: boolean;
  sortState?: SortState;
  onSortChange?: (next: SortState) => void;
  pagination?: {
    page: number;
    perPage: number;
    total: number;
    onPageChange: (page: number) => void;
  };
}

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
  manualSorting = false,
  sortState: controlledSortState,
  onSortChange,
  pagination,
}: DataTableProps<TData>) {
  const [internalSortState, setInternalSortState] = useState<SortState>(null);
  const activeSortState = manualSorting ? controlledSortState ?? null : internalSortState;

  const sortedRows = useMemo(() => {
    if (manualSorting || !activeSortState) {
      return data;
    }

    const column = columns.find((item) => item.id === activeSortState.columnId);
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
        return activeSortState.direction === "asc" ? leftValue - rightValue : rightValue - leftValue;
      }

      const comparison = String(leftValue).localeCompare(String(rightValue), "id");
      return activeSortState.direction === "asc" ? comparison : -comparison;
    });

    return next;
  }, [activeSortState, columns, data, manualSorting]);

  const totalPages = pagination ? Math.max(1, Math.ceil(pagination.total / pagination.perPage)) : 1;
  const mobileColumns = columns.filter((column) => !column.hideOnMobile);
  const mobilePrimaryColumn = mobileColumns.find((column) => column.mobilePrimary) ?? mobileColumns[0] ?? null;

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

  const handleSort = (columnId: string) => {
    const nextSortState = toggleSort(activeSortState, columnId);
    if (manualSorting) {
      onSortChange?.(nextSortState);
      return;
    }
    setInternalSortState(nextSortState);
  };

  return (
    <div className="table-shell">
      <div className="space-y-3 p-3 md:hidden">
        {sortedRows.map((row) => {
          const rowId = getRowId(row);
          const selected = rowId === selectedRowId;

          return (
            <Fragment key={rowId}>
              <div
                className={cn(
                  "block w-full space-y-4 rounded-[18px] border border-border/70 bg-surface px-4 py-4 text-left transition hover:border-border hover:bg-surface-muted/50",
                  selected && "border-module/20 bg-module-light shadow-[0_12px_30px_-24px_var(--module-primary)]",
                  getRowClassName?.(row),
                  !onRowClick && "cursor-default hover:bg-transparent",
                  onRowClick && "cursor-pointer",
                )}
                onClick={() => onRowClick?.(row)}
                onKeyDown={(event) => {
                  if (onRowClick && (event.key === "Enter" || event.key === " ")) {
                    event.preventDefault();
                    onRowClick(row);
                  }
                }}
                role={onRowClick ? "button" : undefined}
                tabIndex={onRowClick ? 0 : undefined}
              >
                {mobilePrimaryColumn ? (
                  <div className="space-y-1">
                    <p className="text-[11px] font-semibold uppercase tracking-[0.08em] text-text-tertiary">
                      {mobilePrimaryColumn.header}
                    </p>
                    <div className="text-sm text-text-primary">
                      {mobilePrimaryColumn.cell
                        ? mobilePrimaryColumn.cell(row)
                        : formatValue(readValue(row, mobilePrimaryColumn))}
                    </div>
                  </div>
                ) : null}

                <div className="grid gap-2.5">
                  {mobileColumns
                    .filter((column) => column.id !== mobilePrimaryColumn?.id)
                    .map((column) => (
                      <div
                        className="grid grid-cols-[minmax(0,92px)_minmax(0,1fr)] items-start gap-3 border-t border-border/60 pt-2.5 first:border-t-0 first:pt-0"
                        key={column.id}
                      >
                        <p className="text-[11px] font-semibold uppercase tracking-[0.08em] text-text-tertiary">
                          {column.header}
                        </p>
                        <div
                          className={cn(
                            "min-w-0 text-right text-sm text-text-primary",
                            column.numeric && "font-mono tabular-nums",
                            column.align === "left" && "text-left",
                            column.align === "center" && "text-center",
                          )}
                        >
                          {column.cell ? column.cell(row) : formatValue(readValue(row, column))}
                        </div>
                      </div>
                    ))}
                </div>
              </div>
              {selected && renderExpandedRow ? (
                <div className="rounded-[18px] border border-border/70 bg-surface-muted/40 px-4 py-4">
                  {renderExpandedRow(row)}
                </div>
              ) : null}
            </Fragment>
          );
        })}
      </div>

      <div className="hidden overflow-x-auto md:block">
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
                      onClick={() => handleSort(column.id)}
                      type="button"
                    >
                      <span>{column.header}</span>
                      <SortIndicator active={activeSortState?.columnId === column.id} direction={activeSortState?.direction} />
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
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
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

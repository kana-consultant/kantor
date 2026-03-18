import { Skeleton } from "@/components/ui/skeleton";

export function DataTablePageSkeleton({
  columns = 5,
  rows = 5,
}: {
  columns?: number;
  rows?: number;
}) {
  return (
    <div className="overflow-hidden rounded-md border border-border bg-surface">
      <div
        className="grid gap-0 border-b border-border bg-surface-muted px-4 py-3"
        style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}
      >
        {Array.from({ length: columns }).map((_, index) => (
          <Skeleton className="h-3 w-3/4" key={index} />
        ))}
      </div>
      <div>
        {Array.from({ length: rows }).map((_, rowIndex) => (
          <div
            className="grid gap-0 border-b border-border px-4 py-4 last:border-b-0"
            key={rowIndex}
            style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}
          >
            {Array.from({ length: columns }).map((__, columnIndex) => (
              <Skeleton className="h-4 w-3/4" key={columnIndex} />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

export function OverviewSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => (
          <div className="rounded-md border border-border bg-surface p-5" key={index}>
            <Skeleton className="h-3 w-24" />
            <Skeleton className="mt-4 h-8 w-32" />
            <Skeleton className="mt-3 h-4 w-20" />
          </div>
        ))}
      </div>
      <div className="grid gap-6 xl:grid-cols-[2fr,1fr]">
        <div className="rounded-md border border-border bg-surface p-6">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="mt-6 h-[320px] w-full" />
        </div>
        <div className="rounded-md border border-border bg-surface p-6">
          <Skeleton className="h-5 w-40" />
          <div className="mt-6 space-y-4">
            {Array.from({ length: 5 }).map((_, index) => (
              <Skeleton className="h-14 w-full" key={index} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

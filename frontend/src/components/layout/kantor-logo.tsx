import { cn } from "@/lib/utils";

interface KantorLogoProps {
  compact?: boolean;
  className?: string;
}

export function KantorLogo({ compact = false, className }: KantorLogoProps) {
  return (
    <div
      aria-label="KANTOR"
      className={cn(
        "flex items-start font-display text-[20px] font-[800] leading-none tracking-[-0.03em] text-text-primary",
        compact && "text-[18px]",
        className,
      )}
    >
      K
      <span className="relative">
        A
        <span className="absolute -top-1 left-1/2 h-1.5 w-1.5 -translate-x-1/2 rounded-full bg-module" />
      </span>
      {!compact ? "NTOR" : null}
    </div>
  );
}

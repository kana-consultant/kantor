import { cn } from "@/lib/utils";

interface KantorLogoProps {
  compact?: boolean;
  className?: string;
}

export function KantorLogo({ compact = false, className }: KantorLogoProps) {
  return (
    <div
      aria-label="KANTOR"
      className={cn("flex items-center gap-2.5", className)}
    >
      <img
        src="/logo-dark.png"
        alt=""
        className={cn(
          "shrink-0 rounded-lg object-contain",
          compact ? "h-8 w-8" : "h-9 w-9",
        )}
      />
      {!compact && (
        <span className="font-display text-[18px] font-[800] leading-none tracking-[-0.03em] text-text-primary">
          KANTOR
        </span>
      )}
    </div>
  );
}

import { adsPlatformMeta, channelMeta, platformFitsChannel, platformMismatchNote } from "@/lib/marketing";
import { cn } from "@/lib/utils";

interface ChannelBadgeProps {
  channel: string;
  /** "channel" for a campaign's channel, "platform" for an ads-metric platform. */
  kind?: "channel" | "platform";
  className?: string;
}

// Neutral pill with a small coloured dot: the platform colour is an identity
// mark only, the label stays dark text for contrast in both themes.
export function ChannelBadge({ channel, kind = "channel", className }: ChannelBadgeProps) {
  const meta = kind === "platform" ? adsPlatformMeta(channel) : channelMeta(channel);

  return (
    <span
      className={cn(
        "inline-flex max-w-full items-center gap-1.5 whitespace-nowrap rounded-full border border-border bg-surface-muted px-2 py-0.5 text-[12px] font-medium text-text-primary",
        className,
      )}
    >
      <span aria-hidden="true" className={cn("h-2 w-2 shrink-0 rounded-full", meta.dotClassName)} />
      <span className="truncate">{meta.label}</span>
    </span>
  );
}

// Small note under an ads metric's platform when it does not fit the
// campaign's channel (never blocking, see platformFitsChannel). Short on
// screen; the tooltip and screen readers get both names.
export function PlatformMismatchNote({ channel, platform }: { channel?: string | null; platform: string }) {
  if (!channel || platformFitsChannel(channel, platform)) {
    return null;
  }
  const message = `Platform ${adsPlatformMeta(platform).label} beda dari kanal campaign (${channelMeta(channel).label})`;
  return (
    <p className="relative mt-1 text-[12px] text-text-secondary" title={message}>
      {/* Phone tables are narrow: the short form keeps the note on one line. */}
      <span aria-hidden="true" className="sm:hidden">Beda kanal</span>
      <span aria-hidden="true" className="hidden sm:inline">{platformMismatchNote}</span>
      <span className="sr-only">{message}</span>
    </p>
  );
}

import { useEffect, useState } from "react";
import { UserRound } from "lucide-react";

import { cn } from "@/lib/utils";
import { fetchProtectedFileBlob, parseProtectedFilePath } from "@/services/files";

const avatarObjectUrlCache = new Map<string, string>();

interface ProtectedAvatarProps {
  name?: string | null;
  avatarUrl?: string | null;
  srcOverride?: string | null;
  className?: string;
  fallbackClassName?: string;
  iconClassName?: string;
  alt?: string;
}

export function ProtectedAvatar({
  name,
  avatarUrl,
  srcOverride,
  className,
  fallbackClassName,
  iconClassName,
  alt,
}: ProtectedAvatarProps) {
  const [resolvedSrc, setResolvedSrc] = useState<string | null>(srcOverride ?? null);

  useEffect(() => {
    let active = true;

    if (srcOverride) {
      setResolvedSrc(srcOverride);
      return () => {
        active = false;
      };
    }

    const rawValue = avatarUrl?.trim() ?? "";
    if (!rawValue) {
      setResolvedSrc(null);
      return () => {
        active = false;
      };
    }

    if (
      rawValue.startsWith("http://") ||
      rawValue.startsWith("https://") ||
      rawValue.startsWith("blob:") ||
      rawValue.startsWith("data:") ||
      rawValue.startsWith("/")
    ) {
      setResolvedSrc(rawValue);
      return () => {
        active = false;
      };
    }

    const cached = avatarObjectUrlCache.get(rawValue);
    if (cached) {
      setResolvedSrc(cached);
      return () => {
        active = false;
      };
    }

    const reference = parseProtectedFilePath(rawValue);
    if (!reference) {
      setResolvedSrc(rawValue);
      return () => {
        active = false;
      };
    }

    setResolvedSrc(null);
    void fetchProtectedFileBlob(reference.type, reference.resourceId, reference.filename)
      .then((blob) => {
        if (!active) {
          return;
        }

        const objectUrl = URL.createObjectURL(blob);
        avatarObjectUrlCache.set(rawValue, objectUrl);
        setResolvedSrc(objectUrl);
      })
      .catch(() => {
        if (active) {
          setResolvedSrc(null);
        }
      });

    return () => {
      active = false;
    };
  }, [avatarUrl, srcOverride]);

  if (resolvedSrc) {
    return (
      <img
        alt={alt ?? name ?? "Avatar"}
        className={cn("rounded-full object-cover", className)}
        src={resolvedSrc}
      />
    );
  }

  return (
    <div
      aria-hidden="true"
      className={cn(
        "flex items-center justify-center rounded-full bg-surface-muted text-text-secondary",
        className,
        fallbackClassName,
      )}
    >
      <UserRound className={cn("h-1/2 w-1/2", iconClassName)} />
    </div>
  );
}

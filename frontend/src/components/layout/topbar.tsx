import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import { Bell, ChevronDown, LogOut, Moon, PanelLeft, PanelLeftClose, Phone, Sun, User, X } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";

import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/hooks/use-auth";
import { useModuleTheme } from "@/hooks/use-module-theme";
import { cn } from "@/lib/utils";
import {
  connectNotificationsStream,
  getUnreadNotificationsCount,
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  notificationsKeys,
} from "@/services/notifications";
import { logout } from "@/services/auth";
import { getUserPhone, updateUserPhone, waKeys } from "@/services/wa-broadcast";
import { useSidebarStore } from "@/stores/sidebar-store";
import { toast } from "@/stores/toast-store";
import { useThemeStore } from "@/stores/theme-store";
import type { NotificationItem } from "@/types/notification";

function readBrowserNotificationPermission(): NotificationPermission {
  if (typeof window === "undefined" || !("Notification" in window)) {
    return "default";
  }
  return window.Notification.permission;
}

export function Topbar() {
  const queryClient = useQueryClient();
  const { breadcrumb } = useModuleTheme();
  const { isAuthenticated, user, roleLabels, roleSummary } = useAuth();
  const mode = useThemeStore((state) => state.mode);
  const toggleTheme = useThemeStore((state) => state.toggleMode);
  const { isDesktopCollapsed, toggleDesktopCollapsed, toggleMobileOpen } = useSidebarStore();
  const [isNotificationsOpen, setIsNotificationsOpen] = useState(false);
  const [isProfileOpen, setIsProfileOpen] = useState(false);
  const [phoneInput, setPhoneInput] = useState("");
  const [isEditingPhone, setIsEditingPhone] = useState(false);
  const [browserNotificationPermission, setBrowserNotificationPermission] = useState<NotificationPermission>(readBrowserNotificationPermission);
  const notificationsRef = useRef<HTMLDivElement | null>(null);
  const profileRef = useRef<HTMLDivElement | null>(null);
  const seenNotificationIdsRef = useRef<Set<string>>(new Set());

  const notificationsQuery = useQuery({
    queryKey: notificationsKeys.list({ page: 1, perPage: 8 }),
    queryFn: () => listNotifications({ page: 1, perPage: 8 }),
    refetchInterval: 30_000,
  });

  const unreadCountQuery = useQuery({
    queryKey: notificationsKeys.unreadCount(),
    queryFn: getUnreadNotificationsCount,
    refetchInterval: 30_000,
  });

  const markReadMutation = useMutation({
    mutationFn: markNotificationRead,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: notificationsKeys.all });
    },
  });

  const markAllMutation = useMutation({
    mutationFn: markAllNotificationsRead,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: notificationsKeys.all });
    },
  });

  const phoneQuery = useQuery({
    queryKey: waKeys.phone(),
    queryFn: getUserPhone,
  });

  const phoneUpdateMutation = useMutation({
    mutationFn: (phone: string | null) => updateUserPhone(phone),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: waKeys.phone() });
      setIsEditingPhone(false);
    },
  });

  const unreadCount = unreadCountQuery.data?.unread_count ?? 0;
  const unreadLabel = useMemo(() => (unreadCount > 99 ? "99+" : String(unreadCount)), [unreadCount]);
  const notificationItems = useMemo(() => notificationsQuery.data?.items ?? [], [notificationsQuery.data?.items]);
  const browserNotificationsSupported = typeof window !== "undefined" && "Notification" in window;
  const BreadcrumbIcon = breadcrumb.icon;
  const notificationsPanelClasses =
    "z-30 overflow-hidden rounded-[20px] border border-border/90 bg-surface shadow-[0_24px_56px_-28px_rgba(15,23,42,0.45)] motion-safe:animate-in motion-safe:slide-in-from-top-2 motion-safe:fade-in motion-safe:duration-200 sm:absolute sm:right-0 sm:top-12 sm:w-[320px]";
  const profilePanelClasses =
    "z-30 overflow-hidden rounded-[20px] border border-border/90 bg-surface shadow-[0_24px_56px_-28px_rgba(15,23,42,0.45)] motion-safe:animate-in motion-safe:slide-in-from-top-2 motion-safe:fade-in motion-safe:duration-200 sm:absolute sm:right-0 sm:top-12 sm:w-[280px]";

  const handleLogout = () => {
    void logout().finally(() => {
      window.location.href = "/login";
    });
  };

  const handleNotificationClick = useEffectEvent((item: NotificationItem) => {
    if (!item.is_read) {
      markReadMutation.mutate(item.id);
    }

    setIsNotificationsOpen(false);

    const href = resolveNotificationHref(item);
    if (href) {
      window.location.assign(href);
    }
  });

  useEffect(() => {
    if (!browserNotificationsSupported) {
      return;
    }

    const syncPermission = () => {
      setBrowserNotificationPermission(readBrowserNotificationPermission());
    };

    syncPermission();
    window.addEventListener("focus", syncPermission);
    document.addEventListener("visibilitychange", syncPermission);

    return () => {
      window.removeEventListener("focus", syncPermission);
      document.removeEventListener("visibilitychange", syncPermission);
    };
  }, [browserNotificationsSupported]);

  useEffect(() => {
    if (!isAuthenticated) {
      return;
    }

    let cancelled = false;
    let reconnectTimer: number | null = null;
    let reconnectAttempt = 0;
    let controller: AbortController | null = null;

    const connect = () => {
      if (cancelled) {
        return;
      }

      controller = new AbortController();

      void connectNotificationsStream({
        signal: controller.signal,
        onEvent: () => {
          reconnectAttempt = 0;
          void queryClient.invalidateQueries({ queryKey: notificationsKeys.all });
        },
      })
        .then(() => {
          if (cancelled || controller?.signal.aborted) {
            return;
          }

          reconnectTimer = window.setTimeout(connect, 1_000);
        })
        .catch((error) => {
          if (cancelled || controller?.signal.aborted) {
            return;
          }

          console.warn("Notifications stream disconnected", error);
          const delay = Math.min(30_000, 1_000 * 2 ** Math.min(reconnectAttempt, 5));
          reconnectAttempt += 1;
          reconnectTimer = window.setTimeout(connect, delay);
        });
    };

    connect();

    return () => {
      cancelled = true;
      controller?.abort();
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
      }
    };
  }, [isAuthenticated, queryClient]);

  useEffect(() => {
    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target as Node;
      if (notificationsRef.current && !notificationsRef.current.contains(target)) {
        setIsNotificationsOpen(false);
      }
      if (profileRef.current && !profileRef.current.contains(target)) {
        setIsProfileOpen(false);
        setIsEditingPhone(false);
      }
    };

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setIsNotificationsOpen(false);
        setIsProfileOpen(false);
        setIsEditingPhone(false);
      }
    };

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, []);

  useEffect(() => {
    const seen = seenNotificationIdsRef.current;
    if (notificationItems.length === 0) {
      return;
    }
    if (seen.size === 0) {
      notificationItems.forEach((item) => seen.add(item.id));
      return;
    }

    const freshUnread = notificationItems.filter((item) => !item.is_read && !seen.has(item.id));
    notificationItems.forEach((item) => seen.add(item.id));

    if (
      freshUnread.length === 0 ||
      typeof window === "undefined" ||
      !("Notification" in window) ||
      browserNotificationPermission !== "granted" ||
      document.visibilityState !== "hidden"
    ) {
      return;
    }

    freshUnread.slice().reverse().forEach((item) => {
      const notification = new window.Notification(item.title, {
        body: item.message,
        tag: item.id,
      });

      notification.onclick = () => {
        window.focus();
        handleNotificationClick(item);
        notification.close();
      };
    });
  }, [browserNotificationPermission, handleNotificationClick, notificationItems]);

  const requestBrowserNotificationPermission = async () => {
    if (!browserNotificationsSupported) {
      toast.warning("Browser notification tidak didukung");
      return;
    }

    const previousPermission = readBrowserNotificationPermission();
    const permission = await window.Notification.requestPermission();
    setBrowserNotificationPermission(permission);

    if (permission === "granted") {
      toast.success("Browser notification aktif", "Notifikasi baru akan muncul saat tab sedang tidak aktif.");
      return;
    }

    if (permission === "denied") {
      toast.warning(
        previousPermission === "denied" ? "Izin browser masih ditolak" : "Izin browser ditolak",
        "Aktifkan lagi dari site settings browser, lalu tekan tombol Cek ulang di panel notifikasi.",
      );
      return;
    }

    toast.info("Izin browser belum diaktifkan");
  };

  const refreshBrowserNotificationPermission = () => {
    setBrowserNotificationPermission(readBrowserNotificationPermission());
  };

  return (
    <header className="sticky top-1 z-20 flex min-w-0 items-center justify-between gap-2 rounded-[20px] border border-border/70 bg-surface px-2 py-2 shadow-[0_18px_42px_-28px_rgba(15,23,42,0.3)] sm:top-1.5 sm:gap-4 sm:px-3 lg:top-2">
      <div className="flex min-w-0 flex-1 items-center gap-2 sm:gap-3">
        <Button
          aria-label="Open sidebar"
          className="lg:hidden"
          onClick={toggleMobileOpen}
          size="icon"
          variant="ghost"
        >
          <PanelLeft className="h-5 w-5" />
        </Button>
        <Button
          aria-label="Toggle sidebar"
          className="hidden lg:inline-flex"
          onClick={toggleDesktopCollapsed}
          size="icon"
          variant="ghost"
        >
          {isDesktopCollapsed ? <PanelLeft className="h-5 w-5" /> : <PanelLeftClose className="h-5 w-5" />}
        </Button>

        <div className="flex min-w-0 items-center gap-2 sm:gap-3">
          <div className={cn("flex h-10 w-10 items-center justify-center rounded-xl shadow-inner", breadcrumb.module.lightClassName)}>
            <BreadcrumbIcon className={cn("h-5 w-5", breadcrumb.module.accentClassName)} />
          </div>
          <div className="flex min-w-0 items-center gap-2 text-sm">
            <span className={cn("hidden font-semibold uppercase tracking-[0.08em] sm:inline", breadcrumb.module.accentClassName)}>
              {breadcrumb.module.label}
            </span>
            <span className="hidden text-text-tertiary sm:inline">/</span>
            <span className="truncate font-semibold text-text-primary">{breadcrumb.title}</span>
          </div>
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-1 sm:gap-2">
        <Button
          aria-label="Toggle theme"
          onClick={toggleTheme}
          size="icon"
          variant="ghost"
        >
          {mode === "dark" ? <Sun className="h-4.5 w-4.5" /> : <Moon className="h-4.5 w-4.5" />}
        </Button>

        <div className="relative" ref={notificationsRef}>
          <Button
            aria-label="Open notifications"
            onClick={() => {
              setIsNotificationsOpen((value) => !value);
              setIsProfileOpen(false);
            }}
            size="icon"
            variant="ghost"
          >
            <Bell className="h-4.5 w-4.5" />
            {unreadCount > 0 ? (
              <span className="absolute right-0 top-0 inline-flex min-h-[16px] min-w-[16px] items-center justify-center rounded-full bg-error px-1 text-[10px] font-bold leading-none text-white">
                {unreadLabel}
              </span>
          ) : null}
        </Button>

          {isNotificationsOpen ? (
            <div className={cn(notificationsPanelClasses, "fixed inset-x-2 top-[4.75rem] max-h-[min(32rem,calc(100dvh-5.5rem))] sm:left-auto sm:right-0 sm:top-12 sm:max-h-[420px]")}>
              <div className="flex items-center justify-between border-b border-border px-4 py-3">
                <h3 className="text-sm font-semibold text-text-primary">Notifikasi</h3>
                <div className="flex items-center gap-2">
                  {browserNotificationsSupported && browserNotificationPermission !== "granted" ? (
                    <button
                      className="text-xs font-semibold text-module transition hover:opacity-80"
                      onClick={() => {
                        void requestBrowserNotificationPermission();
                      }}
                      type="button"
                    >
                      {browserNotificationPermission === "denied" ? "Request permission lagi" : "Aktifkan browser"}
                    </button>
                  ) : null}
                  <button
                    className="text-xs font-semibold text-module transition hover:opacity-80 disabled:opacity-50"
                    disabled={markAllMutation.isPending || unreadCount === 0}
                    onClick={() => markAllMutation.mutate()}
                    type="button"
                  >
                    Tandai semua
                  </button>
                  <button
                    aria-label="Tutup notifikasi"
                    className="inline-flex h-8 w-8 items-center justify-center rounded-full text-text-secondary transition hover:bg-surface-muted hover:text-text-primary"
                    onClick={() => setIsNotificationsOpen(false)}
                    type="button"
                  >
                    <X className="h-4 w-4" />
                  </button>
                </div>
              </div>
              <div className="max-h-[calc(100dvh-9.5rem)] overflow-y-auto sm:max-h-[320px]">
                {browserNotificationsSupported && browserNotificationPermission !== "granted" ? (
                  <div className="border-b border-border bg-surface-muted/50 px-4 py-3">
                    <p className="text-[11px] font-semibold uppercase tracking-[0.08em] text-text-tertiary">
                      Browser Notification
                    </p>
                    <p className="mt-1 text-sm text-text-secondary">
                      {browserNotificationPermission === "denied"
                        ? "Izin browser sedang ditolak. Coba request lagi. Jika browser tetap menolak prompt, aktifkan dari site settings lalu tekan Cek ulang."
                        : "Aktifkan browser notification agar notifikasi tetap muncul saat tab sedang tidak aktif."}
                    </p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button
                        className="h-8"
                        onClick={() => {
                          void requestBrowserNotificationPermission();
                        }}
                        size="sm"
                        type="button"
                        variant="outline"
                      >
                        {browserNotificationPermission === "denied" ? "Request permission lagi" : "Aktifkan browser"}
                      </Button>
                      {browserNotificationPermission === "denied" ? (
                        <Button
                          className="h-8"
                          onClick={refreshBrowserNotificationPermission}
                          size="sm"
                          type="button"
                          variant="ghost"
                        >
                          Cek ulang
                        </Button>
                      ) : null}
                    </div>
                  </div>
                ) : null}
                {notificationItems.length === 0 ? (
                  <div className="px-4 py-8 text-center text-sm text-text-secondary">
                    Belum ada notifikasi.
                  </div>
                ) : (
                  <div className="divide-y divide-border">
                    {notificationItems.map((item) => (
                      <button
                        className={cn(
                          "flex w-full items-start gap-3 px-4 py-3 text-left transition hover:bg-surface-muted",
                          !item.is_read && "bg-module-light",
                        )}
                        key={item.id}
                        onClick={() => handleNotificationClick(item)}
                        type="button"
                      >
                        <span
                          className={cn(
                            "mt-1.5 h-2 w-2 shrink-0 rounded-full",
                            !item.is_read ? "bg-module" : "bg-text-tertiary",
                          )}
                        />
                        <div className="min-w-0">
                          <p className="text-sm font-semibold text-text-primary">{item.title}</p>
                          <p className="mt-1 text-[13px] leading-5 text-text-secondary">{item.message}</p>
                          <p className="mt-2 text-xs text-text-tertiary">
                            {new Date(item.created_at).toLocaleString("id-ID")}
                          </p>
                        </div>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>
          ) : null}
        </div>

        <div className="relative" ref={profileRef}>
          <button
            className="flex items-center gap-2 rounded-full border border-border/70 bg-surface/92 px-2 py-1.5 shadow-sm transition hover:border-border hover:shadow-md sm:gap-3"
            onClick={() => {
              setIsProfileOpen((value) => !value);
              setIsNotificationsOpen(false);
            }}
            type="button"
          >
            <ProtectedAvatar
              alt={user?.full_name ?? "Pengguna"}
              avatarUrl={user?.avatar_url}
              className="h-8 w-8 border border-border/70"
              fallbackClassName="bg-module text-white"
              iconClassName="h-4 w-4"
            />
            <div className="hidden text-left md:block">
              <p className="text-sm font-semibold text-text-primary">{user?.full_name ?? "Pengguna"}</p>
              <p className="text-xs text-text-secondary">{roleSummary}</p>
            </div>
            <ChevronDown className="hidden h-4 w-4 text-text-secondary md:block" />
          </button>

          {isProfileOpen ? (
            <div className={cn(profilePanelClasses, "fixed inset-x-2 top-[4.75rem] max-h-[min(34rem,calc(100dvh-5.5rem))] overflow-y-auto sm:left-auto sm:right-0 sm:top-12 sm:max-h-none")}>
              <div className="border-b border-border px-4 py-4">
                <p className="text-sm font-semibold text-text-primary">{user?.full_name ?? "Pengguna"}</p>
                <p className="mt-1 text-xs text-text-secondary">{user?.email ?? "-"}</p>
                {roleLabels.length > 0 ? (
                  <div className="mt-3 flex flex-wrap gap-2">
                    {roleLabels.map((label) => (
                      <span
                        className="inline-flex rounded-full bg-surface-muted px-2 py-0.5 text-[11px] font-semibold text-text-secondary"
                        key={label}
                      >
                        {label}
                      </span>
                    ))}
                  </div>
                ) : null}
              </div>
              <div className="border-b border-border px-4 py-3">
                <div className="flex items-center gap-2 text-xs text-text-secondary mb-2">
                  <Phone className="h-3.5 w-3.5" />
                  <span>Nomor WhatsApp</span>
                </div>
                {isEditingPhone ? (
                  <div className="flex gap-2">
                    <Input
                      className="h-8 text-sm"
                      placeholder="08xxxxxxxxxx"
                      value={phoneInput}
                      onChange={(e) => setPhoneInput(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") {
                          phoneUpdateMutation.mutate(phoneInput.trim() || null);
                        }
                        if (e.key === "Escape") {
                          setIsEditingPhone(false);
                        }
                      }}
                    />
                    <Button
                      className="h-8 px-2 text-xs"
                      disabled={phoneUpdateMutation.isPending}
                      onClick={() => phoneUpdateMutation.mutate(phoneInput.trim() || null)}
                      size="sm"
                    >
                      {phoneUpdateMutation.isPending ? "..." : "Simpan"}
                    </Button>
                  </div>
                ) : (
                  <button
                    className="text-sm text-text-primary hover:text-module transition"
                    onClick={() => {
                      setPhoneInput(phoneQuery.data?.phone ?? "");
                      setIsEditingPhone(true);
                    }}
                    type="button"
                  >
                    {phoneQuery.data?.phone ? formatPhone(phoneQuery.data.phone) : "Atur nomor"}
                  </button>
                )}
              </div>
              <div className="border-b border-border p-2">
                <Link
                  to="/profile"
                  onClick={() => setIsProfileOpen(false)}
                  className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-text-primary hover:bg-surface-muted transition-colors"
                >
                  <User className="h-4 w-4" />
                  Profil Saya
                </Link>
              </div>
              <div className="p-2">
                <Button
                  className="w-full justify-start text-error hover:bg-error-light hover:text-error"
                  onClick={handleLogout}
                  variant="ghost"
                >
                  <LogOut className="h-4 w-4" />
                  Keluar
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </header>
  );
}

function formatPhone(phone: string) {
  return phone.startsWith("+") ? phone : `+${phone}`;
}

function resolveNotificationHref(item: NotificationItem) {
  if (!item.reference_id || !item.reference_type) {
    return null;
  }

  switch (item.reference_type) {
    case "reimbursement":
      return `/hris/reimbursements/${item.reference_id}`;
    case "project":
      return `/operational/projects/${item.reference_id}`;
    case "campaign":
      return `/marketing/campaigns#campaign:${encodeURIComponent(item.reference_id)}`;
    case "lead":
      return `/marketing/leads#lead:${encodeURIComponent(item.reference_id)}`;
    default:
      return null;
  }
}


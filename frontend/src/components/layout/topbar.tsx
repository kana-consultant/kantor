import { useMemo, useState } from "react";
import { Bell, ChevronDown, LogOut, Moon, PanelLeft, PanelLeftClose, Phone, Sun, User } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/hooks/use-auth";
import { useModuleTheme } from "@/hooks/use-module-theme";
import { cn } from "@/lib/utils";
import {
  getUnreadNotificationsCount,
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  notificationsKeys,
} from "@/services/notifications";
import { logout } from "@/services/auth";
import { getUserPhone, updateUserPhone, waKeys } from "@/services/wa-broadcast";
import { useSidebarStore } from "@/stores/sidebar-store";
import { useThemeStore } from "@/stores/theme-store";

export function Topbar() {
  const queryClient = useQueryClient();
  const { breadcrumb } = useModuleTheme();
  const { user, roles } = useAuth();
  const mode = useThemeStore((state) => state.mode);
  const toggleTheme = useThemeStore((state) => state.toggleMode);
  const { isDesktopCollapsed, toggleDesktopCollapsed, toggleMobileOpen } = useSidebarStore();
  const [isNotificationsOpen, setIsNotificationsOpen] = useState(false);
  const [isProfileOpen, setIsProfileOpen] = useState(false);
  const [phoneInput, setPhoneInput] = useState("");
  const [isEditingPhone, setIsEditingPhone] = useState(false);

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
  const notificationItems = notificationsQuery.data?.items ?? [];
  const BreadcrumbIcon = breadcrumb.icon;

  const handleLogout = () => {
    void logout().finally(() => {
      window.location.href = "/login";
    });
  };

  return (
    <header className="sticky top-0 z-20 flex h-[56px] items-center justify-between gap-4 bg-transparent px-1">
      <div className="flex items-center gap-3">
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

        <div className="flex items-center gap-3">
          <div className={cn("flex h-9 w-9 items-center justify-center rounded-md", breadcrumb.module.lightClassName)}>
            <BreadcrumbIcon className={cn("h-5 w-5", breadcrumb.module.accentClassName)} />
          </div>
          <div className="flex items-center gap-3 text-sm">
            <span className={cn("font-semibold uppercase tracking-[0.08em]", breadcrumb.module.accentClassName)}>
              {breadcrumb.module.label}
            </span>
            <span className="text-text-tertiary">/</span>
            <span className="font-semibold text-text-primary">{breadcrumb.title}</span>
          </div>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Button
          aria-label="Toggle theme"
          onClick={toggleTheme}
          size="icon"
          variant="ghost"
        >
          {mode === "dark" ? <Sun className="h-4.5 w-4.5" /> : <Moon className="h-4.5 w-4.5" />}
        </Button>

        <div className="relative">
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
            <div className="absolute right-0 top-12 w-[320px] overflow-hidden rounded-lg border border-border bg-surface shadow-lg motion-safe:animate-in motion-safe:slide-in-from-top-2 motion-safe:fade-in motion-safe:duration-200">
              <div className="flex items-center justify-between border-b border-border px-4 py-3">
                <h3 className="text-sm font-semibold text-text-primary">Notifications</h3>
                <button
                  className="text-xs font-semibold text-module transition hover:opacity-80 disabled:opacity-50"
                  disabled={markAllMutation.isPending || unreadCount === 0}
                  onClick={() => markAllMutation.mutate()}
                  type="button"
                >
                  Mark all as read
                </button>
              </div>
              <div className="max-h-[320px] overflow-y-auto">
                {notificationItems.length === 0 ? (
                  <div className="px-4 py-8 text-center text-sm text-text-secondary">
                    No notifications yet.
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
                        onClick={() => {
                          if (!item.is_read) {
                            markReadMutation.mutate(item.id);
                          }
                        }}
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

        <div className="relative">
          <button
            className="flex items-center gap-3 rounded-full bg-surface px-2 py-1.5 shadow-xs transition hover:shadow-sm"
            onClick={() => {
              setIsProfileOpen((value) => !value);
              setIsNotificationsOpen(false);
            }}
            type="button"
          >
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-module text-[13px] font-semibold text-white">
              {initials(user?.full_name ?? "Guest")}
            </div>
            <div className="hidden text-left md:block">
              <p className="text-sm font-semibold text-text-primary">{user?.full_name ?? "Guest"}</p>
              <p className="text-xs text-text-secondary">{roles[0] ?? "viewer"}</p>
            </div>
            <ChevronDown className="hidden h-4 w-4 text-text-secondary md:block" />
          </button>

          {isProfileOpen ? (
            <div className="absolute right-0 top-12 w-[280px] overflow-hidden rounded-lg border border-border bg-surface shadow-lg motion-safe:animate-in motion-safe:slide-in-from-top-2 motion-safe:fade-in motion-safe:duration-200">
              <div className="border-b border-border px-4 py-4">
                <p className="text-sm font-semibold text-text-primary">{user?.full_name ?? "Guest"}</p>
                <p className="mt-1 text-xs text-text-secondary">{user?.email ?? "guest@kantor.local"}</p>
              </div>
              <div className="border-b border-border px-4 py-3">
                <div className="flex items-center gap-2 text-xs text-text-secondary mb-2">
                  <Phone className="h-3.5 w-3.5" />
                  <span>WhatsApp Number</span>
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
                      {phoneUpdateMutation.isPending ? "..." : "Save"}
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
                    {phoneQuery.data?.phone ?? "Set phone number"}
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
                  Logout
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </header>
  );
}

function initials(value: string) {
  return value
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

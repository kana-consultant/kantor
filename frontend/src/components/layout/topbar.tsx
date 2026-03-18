import { useMemo, useState } from "react";

import { Bell, ChevronDown, LogOut, PanelLeft, PanelLeftClose } from "lucide-react";
import { useRouterState } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useAuth } from "@/hooks/use-auth";
import { cn } from "@/lib/utils";
import { useSidebarStore } from "@/stores/sidebar-store";
import { logout } from "@/services/auth";
import { listNotifications, markAllNotificationsRead, markNotificationRead, notificationsKeys } from "@/services/notifications";

const pageMetadata = [
  {
    match: (pathname: string) => pathname.startsWith("/operational/projects"),
    title: "Projects",
    module: "OPERASIONAL"
  },
  {
    match: (pathname: string) => pathname.startsWith("/operational/automation"),
    title: "Automation",
    module: "OPERASIONAL"
  },
  {
    match: (pathname: string) => pathname.startsWith("/operational"),
    title: "Overview",
    module: "OPERASIONAL"
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/employees"),
    title: "Employees",
    module: "HRIS"
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/departments"),
    title: "Departments",
    module: "HRIS"
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/reimbursements"),
    title: "Reimbursements",
    module: "HRIS"
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/finance"),
    title: "Finance",
    module: "HRIS"
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/subscriptions"),
    title: "Subscriptions",
    module: "HRIS"
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris"),
    title: "Overview",
    module: "HRIS"
  },
  {
    match: (pathname: string) => pathname.startsWith("/marketing/campaigns"),
    title: "Campaigns",
    module: "MARKETING"
  },
  {
    match: (pathname: string) => pathname.startsWith("/marketing/leads"),
    title: "Leads",
    module: "MARKETING"
  },
  {
    match: (pathname: string) => pathname.startsWith("/marketing/ads-metrics"),
    title: "Ads Metrics",
    module: "MARKETING"
  },
  {
    match: (pathname: string) => pathname.startsWith("/marketing"),
    title: "Overview",
    module: "MARKETING"
  },
];

export function Topbar() {
  const queryClient = useQueryClient();
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const { user, roles } = useAuth();
  const { isDesktopCollapsed, toggleDesktopCollapsed, toggleMobileOpen } = useSidebarStore();
  const [isNotificationsOpen, setIsNotificationsOpen] = useState(false);
  const [isProfileOpen, setIsProfileOpen] = useState(false);

  const notificationsQuery = useQuery({
    queryKey: notificationsKeys.list({ page: 1, perPage: 8 }),
    queryFn: () => listNotifications({ page: 1, perPage: 8 }),
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

  const page = pageMetadata.find((item) => item.match(pathname)) ?? {
    title: "Internal Platform",
    module: "KANTOR Workspace"
  };

  const handleLogout = () => {
    logout();
    window.location.href = "/login";
  };

  const notificationItems = notificationsQuery.data?.items ?? [];
  const unreadCount = useMemo(
    () => notificationItems.filter((item) => !item.is_read).length,
    [notificationItems],
  );

  return (
    <header className="flex h-[64px] items-center justify-between px-6 bg-surface border-b border-border z-10 sticky top-0">
      <div className="flex items-center gap-4">
        {/* Mobile Toggle */}
        <button
          className="lg:hidden flex items-center justify-center w-8 h-8 rounded-md hover:bg-surface-muted text-text-secondary transition-colors"
          onClick={toggleMobileOpen}
          aria-label="Open sidebar"
        >
          <PanelLeft className="w-5 h-5" />
        </button>

        {/* Desktop Toggle */}
        <button
          className="hidden lg:flex items-center justify-center w-8 h-8 rounded-md hover:bg-surface-muted text-text-secondary transition-colors"
          onClick={toggleDesktopCollapsed}
          aria-label="Toggle sidebar"
        >
          {isDesktopCollapsed ? <PanelLeft className="w-5 h-5" /> : <PanelLeftClose className="w-5 h-5" />}
        </button>

        {/* Breadcrumb / Title */}
        <div className="flex items-center gap-2 text-[14px]">
          <span className="text-text-secondary font-[500] uppercase tracking-wider text-[12px]">{page.module}</span>
          <span className="text-border mx-1">/</span>
          <span className="text-text-primary font-[600] text-[16px]">{page.title}</span>
        </div>
      </div>

      <div className="flex items-center gap-2">
        {/* Notifications */}
        <div className="relative">
          <button
            className="relative flex items-center justify-center w-9 h-9 rounded-full hover:bg-surface-muted text-text-secondary transition-colors"
            onClick={() => {
              setIsNotificationsOpen((value) => !value);
              setIsProfileOpen(false);
            }}
            aria-label="Toggle notifications"
          >
            <Bell className="w-5 h-5" strokeWidth={1.5} />
            {unreadCount > 0 ? (
              <span className="absolute top-1 right-1 w-2.5 h-2.5 bg-priority-high rounded-full border-2 border-surface"></span>
            ) : null}
          </button>

          {isNotificationsOpen ? (
            <div className="absolute right-0 top-12 w-80 rounded-[12px] border border-border bg-surface shadow-lg overflow-hidden animate-in fade-in slide-in-from-top-2">
              <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-surface-muted">
                <h3 className="font-[600] text-[14px] text-text-primary">Notifications</h3>
                <button
                  disabled={markAllMutation.isPending || unreadCount === 0}
                  onClick={() => markAllMutation.mutate()}
                  className="text-[12px] font-[500] text-ops hover:text-ops-hover disabled:opacity-50 disabled:hover:text-ops transition-colors"
                >
                  Mark all as read
                </button>
              </div>
              <div className="max-h-[300px] overflow-y-auto">
                {notificationItems.length === 0 ? (
                  <div className="px-4 py-8 text-center text-sm text-text-secondary">
                    No new notifications.
                  </div>
                ) : (
                  <div className="divide-y divide-border">
                    {notificationItems.map((item) => (
                      <button
                        key={item.id}
                        className={cn(
                          "w-full text-left px-4 py-3 hover:bg-surface-muted transition-colors flex items-start gap-3",
                          !item.is_read ? "bg-ops-light/30" : ""
                        )}
                        onClick={() => {
                          if (!item.is_read) {
                            markReadMutation.mutate(item.id);
                          }
                        }}
                      >
                        {!item.is_read && <span className="mt-1.5 shrink-0 w-2 h-2 rounded-full bg-ops" />}
                        <div>
                          <p className={cn("text-[13px] text-text-primary", !item.is_read ? "font-[600]" : "font-[500]")}>{item.title}</p>
                          <p className="mt-1 text-[12px] text-text-secondary line-clamp-2">{item.message}</p>
                          <p className="mt-2 text-[11px] text-text-tertiary">
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

        {/* Profile Dropdown */}
        <div className="relative ml-2">
          <button 
            className="flex items-center gap-2 hover:bg-surface-muted py-1.5 pl-1.5 pr-2.5 rounded-full transition-colors"
            onClick={() => {
              setIsProfileOpen(!isProfileOpen);
              setIsNotificationsOpen(false);
            }}
          >
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-ops text-[13px] font-[600] text-white">
              {initials(user?.full_name ?? "Guest")}
            </div>
            <div className="hidden md:block text-left">
              <p className="text-[13px] font-[600] text-text-primary leading-none">{user?.full_name ?? "Guest"}</p>
            </div>
            <ChevronDown className="w-4 h-4 text-text-secondary ml-1 hidden md:block" strokeWidth={2} />
          </button>

          {isProfileOpen && (
             <div className="absolute right-0 top-12 w-56 rounded-[12px] border border-border bg-surface shadow-lg overflow-hidden animate-in fade-in slide-in-from-top-2">
                <div className="px-4 py-3 border-b border-border bg-surface-muted">
                   <p className="text-[14px] font-[600] text-text-primary truncate">{user?.full_name ?? "Guest"}</p>
                   <p className="text-[12px] text-text-secondary mt-0.5 truncate">{user?.email ?? "guest@kantor.local"}</p>
                   <div className="mt-2 inline-flex items-center rounded-full bg-black/5 px-2 py-0.5 text-[10px] font-[600] uppercase tracking-wider text-text-secondary">
                     {roles[0] ?? "Viewer"}
                   </div>
                </div>
                <div className="p-1">
                   <button 
                     onClick={handleLogout}
                     className="w-full flex items-center gap-2 px-3 py-2 text-[13px] font-[500] text-priority-high hover:bg-priority-high/10 rounded-[6px] transition-colors"
                   >
                     <LogOut className="w-4 h-4" />
                     Logout
                   </button>
                </div>
             </div>
          )}
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

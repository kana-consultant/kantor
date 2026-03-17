import { useMemo, useState } from "react";

import { ArrowRight, Bell, FolderKanban, LogOut, PanelLeft, PanelLeftClose, Sparkles } from "lucide-react";
import { Link, useNavigate, useRouterState } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { Button } from "@/components/ui/button";
import { useAuth } from "@/hooks/use-auth";
import { cn } from "@/lib/utils";
import { useSidebarStore } from "@/stores/sidebar-store";
import { logout } from "@/services/auth";
import { listNotifications, markAllNotificationsRead, markNotificationRead, notificationsKeys } from "@/services/notifications";

const pageMetadata = [
  {
    match: (pathname: string) => pathname.startsWith("/operational/projects"),
    eyebrow: "Operational Boards",
    title: "Project delivery workspace",
    summary: "Kelola board, anggota project, dan automation tanpa pindah URL manual.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/operational/automation"),
    eyebrow: "Operational Automation",
    title: "Assignment rules",
    summary: "Atur auto assign rules per project dari satu jalur navigasi yang jelas.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/operational"),
    eyebrow: "Operational",
    title: "Workflow hub",
    summary: "Pilih board, automation, dan jalur kerja utama tim operasional.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/employees"),
    eyebrow: "HRIS Employees",
    title: "People directory",
    summary: "Kelola data karyawan, profil, dan fondasi kompensasi dari satu alur kerja.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/departments"),
    eyebrow: "HRIS Departments",
    title: "Department structure",
    summary: "Atur department, deskripsi fungsi, dan penanggung jawab team.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/reimbursements"),
    eyebrow: "HRIS Reimbursements",
    title: "Reimbursement workflow",
    summary: "Submit claim, review approval timeline, dan cek payout status dari satu alur kerja.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/finance"),
    eyebrow: "HRIS Finance",
    title: "Finance operations",
    summary: "Monitor income, outcome, approval flow, dan tren bulanan dari satu workspace.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris/subscriptions"),
    eyebrow: "HRIS Subscriptions",
    title: "Subscription tracker",
    summary: "Pantau biaya tool, renewal date, dan alert langganan aktif perusahaan.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/hris"),
    eyebrow: "HRIS",
    title: "People and finance",
    summary: "Ruang kerja HR, compensation, dan finance operations.",
  },
  {
    match: (pathname: string) => pathname.startsWith("/marketing"),
    eyebrow: "Marketing",
    title: "Campaign control room",
    summary: "Pantau pipeline campaign, leads, dan metrik performance.",
  },
];

export function Topbar() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const { user, roles } = useAuth();
  const { isDesktopCollapsed, toggleDesktopCollapsed, toggleMobileOpen } = useSidebarStore();
  const [isNotificationsOpen, setIsNotificationsOpen] = useState(false);

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
    eyebrow: "Workspace",
    title: "Internal platform",
    summary: "Workspace terpadu untuk modul operasional, HRIS, dan marketing.",
  };

  const handleLogout = () => {
    logout();
    void navigate({ to: "/login" });
  };

  const notificationItems = notificationsQuery.data?.items ?? [];
  const unreadCount = useMemo(
    () => notificationItems.filter((item) => !item.is_read).length,
    [notificationItems],
  );

  return (
    <header className="rounded-[30px] border border-border/80 bg-card/80 p-5 shadow-panel backdrop-blur">
      <div className="flex flex-col gap-5 xl:flex-row xl:items-center xl:justify-between">
        <div>
          <div className="mb-4 flex items-center gap-2">
            <Button
              className="lg:hidden"
              onClick={toggleMobileOpen}
              size="icon"
              type="button"
              variant="outline"
            >
              <PanelLeft className="h-4 w-4" />
            </Button>
            <Button
              className="hidden lg:inline-flex"
              onClick={toggleDesktopCollapsed}
              size="icon"
              type="button"
              variant="outline"
            >
              {isDesktopCollapsed ? <PanelLeft className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </Button>
          </div>
          <div className="flex flex-wrap items-center gap-2 text-xs uppercase tracking-[0.28em] text-muted-foreground">
            <span>{page.eyebrow}</span>
            <span>/</span>
            <span>{pathname.replace(/^\//, "") || "home"}</span>
          </div>
          <h2 className="mt-3 text-3xl font-bold">{page.title}</h2>
          <p className="mt-2 max-w-3xl text-sm text-muted-foreground">{page.summary}</p>
        </div>

        <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
          <Link
            className="inline-flex h-11 items-center justify-center gap-2 rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted"
            to="/operational/projects"
          >
            <FolderKanban className="h-4 w-4" />
            Open projects
          </Link>
          <div className="flex items-center gap-3 rounded-full border border-border bg-background/80 px-4 py-2.5">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/15 text-sm font-semibold text-primary">
              {initials(user?.full_name ?? "Guest")}
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold">{user?.full_name ?? "Guest"}</p>
              <p className="truncate text-xs text-muted-foreground">{roles[0] ?? "no-role"}</p>
            </div>
            <Sparkles className="h-4 w-4 text-primary" />
          </div>
          <div className="relative">
            <Button
              className="relative"
              onClick={() => setIsNotificationsOpen((value) => !value)}
              size="icon"
              type="button"
              variant="outline"
            >
              <Bell className="h-4 w-4" />
              {unreadCount > 0 ? (
                <span className="absolute -right-1 -top-1 inline-flex min-h-5 min-w-5 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-semibold text-primary-foreground">
                  {unreadCount}
                </span>
              ) : null}
            </Button>

            {isNotificationsOpen ? (
              <div className="absolute right-0 top-14 z-50 w-[22rem] rounded-[28px] border border-border bg-card p-4 shadow-panel">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Notifications</p>
                    <h3 className="mt-2 text-lg font-semibold">Recent updates</h3>
                  </div>
                  <Button
                    disabled={markAllMutation.isPending || unreadCount === 0}
                    onClick={() => markAllMutation.mutate()}
                    size="sm"
                    variant="ghost"
                  >
                    Read all
                  </Button>
                </div>
                <div className="mt-4 space-y-3">
                  {notificationItems.length === 0 ? (
                    <div className="rounded-[22px] border border-dashed border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
                      Belum ada notifikasi.
                    </div>
                  ) : (
                    notificationItems.map((item) => (
                      <button
                        className={cn(
                          "block w-full rounded-[22px] border p-4 text-left transition",
                          item.is_read ? "border-border/60 bg-background/60" : "border-primary/30 bg-primary/5",
                        )}
                        key={item.id}
                        onClick={() => {
                          if (!item.is_read) {
                            markReadMutation.mutate(item.id);
                          }
                        }}
                        type="button"
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <p className="text-sm font-semibold">{item.title}</p>
                            <p className="mt-1 text-sm text-muted-foreground">{item.message}</p>
                          </div>
                          {!item.is_read ? <span className="mt-1 h-2.5 w-2.5 rounded-full bg-primary" /> : null}
                        </div>
                        <p className="mt-3 text-xs text-muted-foreground">
                          {new Date(item.created_at).toLocaleString("id-ID")}
                        </p>
                      </button>
                    ))
                  )}
                </div>
              </div>
            ) : null}
          </div>
          <Button onClick={handleLogout} size="sm" variant="ghost">
            <LogOut className="h-4 w-4" />
            Logout
          </Button>
        </div>
      </div>

      {pathname === "/operational" || pathname === "/operational/" ? (
        <div className="mt-5 flex flex-wrap gap-3">
          <Link
            className="inline-flex h-10 items-center justify-center gap-2 rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition hover:opacity-95"
            to="/operational/projects"
          >
            Go to Boards
            <ArrowRight className="h-4 w-4" />
          </Link>
          <Link
            className="inline-flex h-10 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted"
            to="/operational/automation"
          >
            Review automation
          </Link>
        </div>
      ) : null}
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

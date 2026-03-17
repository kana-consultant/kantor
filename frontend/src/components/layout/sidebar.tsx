import {
  Bot,
  Building2,
  ChevronRight,
  ChevronsLeft,
  FolderKanban,
  LayoutDashboard,
  Megaphone,
  ShieldCheck,
  Users,
} from "lucide-react";
import { Link, useRouterState } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";

interface NavItem {
  to:
    | "/operational"
    | "/operational/projects"
    | "/operational/automation"
    | "/hris"
    | "/hris/employees"
    | "/hris/departments"
    | "/hris/finance"
    | "/hris/reimbursements"
    | "/hris/subscriptions"
    | "/marketing";
  label: string;
  caption: string;
  icon: typeof LayoutDashboard;
  permission: string;
}

interface NavSection {
  label: string;
  items: NavItem[];
}

const sections: NavSection[] = [
  {
    label: "Operational Workspace",
    items: [
      {
        to: "/operational",
        label: "Overview",
        caption: "Home and quick actions",
        icon: LayoutDashboard,
        permission: permissions.operationalOverview,
      },
      {
        to: "/operational/projects",
        label: "Projects",
        caption: "Boards and delivery flow",
        icon: FolderKanban,
        permission: permissions.operationalProjectView,
      },
      {
        to: "/operational/automation",
        label: "Automation",
        caption: "Auto assign rules",
        icon: Bot,
        permission: permissions.operationalAssignmentView,
      },
    ],
  },
  {
    label: "Other Modules",
    items: [
      {
        to: "/hris",
        label: "HRIS Hub",
        caption: "Overview and quick actions",
        icon: Users,
        permission: permissions.hrisOverview,
      },
      {
        to: "/hris/employees",
        label: "Employees",
        caption: "Directory and profiles",
        icon: Users,
        permission: permissions.hrisEmployeeView,
      },
      {
        to: "/hris/departments",
        label: "Departments",
        caption: "Structure and headcount",
        icon: Building2,
        permission: permissions.hrisDepartmentView,
      },
      {
        to: "/hris/subscriptions",
        label: "Subscriptions",
        caption: "Tools, renewal, and cost",
        icon: Bot,
        permission: permissions.hrisSubscriptionView,
      },
      {
        to: "/hris/finance",
        label: "Finance",
        caption: "Income, outcome, and approvals",
        icon: LayoutDashboard,
        permission: permissions.hrisFinanceView,
      },
      {
        to: "/hris/reimbursements",
        label: "Reimbursements",
        caption: "Claims, approval, and payout",
        icon: Building2,
        permission: permissions.hrisReimbursementView,
      },
      {
        to: "/marketing",
        label: "Marketing",
        caption: "Campaign and leads",
        icon: Megaphone,
        permission: permissions.marketingOverview,
      },
    ],
  },
];

interface SidebarProps {
  collapsed?: boolean;
  mobile?: boolean;
  onNavigate?: () => void;
  onToggleCollapse?: () => void;
}

export function Sidebar({ collapsed = false, mobile = false, onNavigate, onToggleCollapse }: SidebarProps) {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const { hasPermission } = useRBAC();

  const visibleSections = sections
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => hasPermission(item.permission)),
    }))
    .filter((section) => section.items.length > 0)

  const pinnedItems = visibleSections
    .flatMap((section) => section.items)
    .filter((item) =>
      item.to === "/operational/projects" ||
      item.to === "/hris/employees" ||
      item.to === "/hris/finance" ||
      item.to === "/hris/reimbursements",
    )
    .slice(0, 3)

  return (
    <aside
      className={cn(
        "relative flex h-full flex-col overflow-hidden rounded-[34px] border border-border/80 bg-card/90 p-4 shadow-panel backdrop-blur",
        collapsed ? "items-center" : "",
      )}
    >
      <div className="pointer-events-none absolute inset-x-0 top-0 h-40 bg-[radial-gradient(circle_at_top,_rgba(249,115,22,0.18),_transparent_68%)]" />

      <div
        className={cn(
          "relative mb-5 w-full rounded-[28px] border border-white/10 bg-gradient-to-br from-primary via-orange-400 to-amber-300 text-primary-foreground shadow-panel",
          collapsed ? "p-3" : "p-5",
        )}
      >
        <div className={cn("flex items-start justify-between gap-3", collapsed ? "justify-center" : "")}>
          <div className={cn("flex gap-4", collapsed ? "items-center justify-center" : "items-start")}>
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-black/10 ring-1 ring-white/15">
              <ShieldCheck className="h-6 w-6" />
            </div>
            {!collapsed ? (
              <div>
                <p className="text-[11px] uppercase tracking-[0.32em] text-primary-foreground/75">
                  Workspace Navigation
                </p>
                <h1 className="mt-1 text-xl font-bold">KANTOR</h1>
                <p className="mt-2 max-w-[13rem] text-sm text-primary-foreground/80">
                  Jalur kerja utama untuk operasional, HRIS, dan marketing.
                </p>
              </div>
            ) : null}
          </div>

          {!mobile && onToggleCollapse && !collapsed ? (
            <Button
              className="shrink-0 bg-black/10 text-primary-foreground hover:bg-black/20"
              onClick={onToggleCollapse}
              size="icon"
              type="button"
              variant="ghost"
            >
              <ChevronsLeft className="h-4 w-4" />
            </Button>
          ) : null}
        </div>

        {!mobile && onToggleCollapse && collapsed ? (
          <Button
            className="mt-3 w-full bg-black/10 px-0 text-primary-foreground hover:bg-black/20"
            onClick={onToggleCollapse}
            size="sm"
            type="button"
            variant="ghost"
          >
            <ChevronsLeft className="h-4 w-4 rotate-180" />
          </Button>
        ) : null}
      </div>

      <div className="w-full flex-1 space-y-4 overflow-y-auto pr-1">
        {visibleSections.map((section) => (
          <section
            className={cn(
              "w-full rounded-[26px] border border-border/60 bg-background/70",
              collapsed ? "p-2" : "p-3",
            )}
            key={section.label}
          >
            {!collapsed ? (
              <div className="mb-3 flex items-center justify-between gap-2 px-1">
                <p className="text-[11px] uppercase tracking-[0.28em] text-muted-foreground">
                  {section.label}
                </p>
                <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] font-semibold text-muted-foreground">
                  {section.items.length}
                </span>
              </div>
            ) : null}
            <div className="space-y-2">
              {section.items.map((item) => {
                const Icon = item.icon
                const active = item.to === "/operational"
                  ? pathname === "/operational" || pathname === "/operational/"
                  : pathname.startsWith(item.to)

                return (
                  <Link
                    activeOptions={{ exact: item.to === "/operational" }}
                    className={cn(
                      "group relative rounded-[22px] border transition",
                      collapsed ? "flex justify-center px-2 py-2.5" : "flex items-start gap-3 px-3 py-3",
                      active
                        ? "border-primary/25 bg-primary/10 text-foreground"
                        : "border-transparent text-muted-foreground hover:border-border/70 hover:bg-muted/70 hover:text-foreground",
                    )}
                    key={item.to}
                    onClick={onNavigate}
                    title={collapsed ? `${item.label} - ${item.caption}` : undefined}
                    to={item.to}
                  >
                    {!collapsed && active ? (
                      <span className="absolute inset-y-3 left-0 w-1 rounded-r-full bg-primary" />
                    ) : null}
                    <div
                      className={cn(
                        "flex shrink-0 items-center justify-center rounded-2xl border transition",
                        collapsed ? "h-11 w-11" : "mt-0.5 h-10 w-10",
                        active
                          ? "border-primary/20 bg-primary text-primary-foreground"
                          : "border-border/70 bg-card/80 group-hover:border-border",
                      )}
                    >
                      <Icon className="h-4 w-4" />
                    </div>
                    {!collapsed ? (
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center justify-between gap-3">
                          <p className="truncate text-sm font-semibold">{item.label}</p>
                          {active ? (
                            <span className="rounded-full bg-primary/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.18em] text-primary">
                              Live
                            </span>
                          ) : (
                            <ChevronRight className="h-4 w-4 opacity-0 transition group-hover:opacity-100" />
                          )}
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">{item.caption}</p>
                      </div>
                    ) : active ? (
                      <span className="absolute right-2 top-2 h-2.5 w-2.5 rounded-full bg-primary" />
                    ) : null}
                  </Link>
                )
              })}
            </div>
          </section>
        ))}
      </div>

      {!collapsed ? (
        <div className="mt-5 w-full rounded-[26px] border border-border/70 bg-background/75 p-4">
          <div className="flex items-start gap-3">
            <div className="mt-1 flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-secondary text-secondary-foreground">
              <Building2 className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <p className="text-sm font-semibold">Pinned Access</p>
              <p className="mt-1 text-xs text-muted-foreground">
                Jalur tercepat ke halaman yang paling sering dipakai tim.
              </p>
            </div>
          </div>
          <div className="mt-4 grid gap-2">
            {pinnedItems.map((item) => {
              const Icon = item.icon

              return (
                <Link
                  className="flex items-center gap-3 rounded-[18px] border border-border/70 bg-card/70 px-3 py-3 text-sm font-medium transition hover:bg-muted"
                  key={item.to}
                  onClick={onNavigate}
                  to={item.to}
                >
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl bg-muted">
                    <Icon className="h-4 w-4" />
                  </div>
                  <div className="min-w-0">
                    <p className="truncate">{item.label}</p>
                    <p className="truncate text-xs text-muted-foreground">{item.caption}</p>
                  </div>
                </Link>
              )
            })}
          </div>
        </div>
      ) : (
        <div className="mt-5 flex w-full flex-col items-center gap-2">
          {pinnedItems.map((item) => {
            const Icon = item.icon
            const active = item.to === "/operational"
              ? pathname === "/operational" || pathname === "/operational/"
              : pathname.startsWith(item.to)

            return (
              <Link
                className={cn(
                  "relative flex h-11 w-11 items-center justify-center rounded-2xl border transition",
                  active
                    ? "border-primary/20 bg-primary text-primary-foreground"
                    : "border-border/70 bg-background/75 text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
                key={item.to}
                onClick={onNavigate}
                title={item.label}
                to={item.to}
              >
                <Icon className="h-4 w-4" />
              </Link>
            )
          })}
        </div>
      )}
    </aside>
  )
}

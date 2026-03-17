import {
  Bot,
  Building2,
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
  to: "/operational" | "/operational/projects" | "/operational/automation" | "/hris" | "/marketing";
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
        label: "HRIS",
        caption: "People and finance",
        icon: Users,
        permission: permissions.hrisOverview,
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

  return (
    <aside
      className={cn(
        "flex h-full flex-col rounded-[34px] border border-border/80 bg-card/85 p-4 shadow-panel backdrop-blur",
        collapsed ? "items-center" : "",
      )}
    >
      <div
        className={cn(
          "mb-6 w-full rounded-[28px] bg-gradient-to-br from-primary to-orange-300 text-primary-foreground shadow-panel",
          collapsed ? "p-3" : "p-5",
        )}
      >
        <div className={cn("flex gap-4", collapsed ? "items-center justify-center" : "items-start")}>
          <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-black/10">
            <ShieldCheck className="h-6 w-6" />
          </div>
          {!collapsed ? (
            <div>
              <p className="text-xs uppercase tracking-[0.28em] text-primary-foreground/80">
                Internal Platform
              </p>
              <h1 className="mt-1 text-xl font-bold">KANTOR</h1>
              <p className="mt-2 max-w-[13rem] text-sm text-primary-foreground/80">
                Boards, workflow, dan automation untuk tim internal.
              </p>
            </div>
          ) : null}
        </div>

        {!mobile && onToggleCollapse ? (
          <Button
            className={cn("mt-4 w-full bg-black/10 text-primary-foreground hover:bg-black/20", collapsed ? "px-0" : "")}
            onClick={onToggleCollapse}
            size="sm"
            type="button"
            variant="ghost"
          >
            <ChevronsLeft className={cn("h-4 w-4 transition", collapsed && "rotate-180")} />
            {!collapsed ? "Collapse sidebar" : null}
          </Button>
        ) : null}
      </div>

      <div className="w-full space-y-6">
        {sections.map((section) => {
          const visibleItems = section.items.filter((item) => hasPermission(item.permission));
          if (visibleItems.length === 0) {
            return null;
          }

          return (
            <section className="w-full" key={section.label}>
              {!collapsed ? (
                <p className="mb-3 px-2 text-xs uppercase tracking-[0.28em] text-muted-foreground">
                  {section.label}
                </p>
              ) : null}
              <div className="space-y-2">
                {visibleItems.map((item) => {
                  const Icon = item.icon;
                  const active = item.to === "/operational"
                    ? pathname === "/operational" || pathname === "/operational/"
                    : pathname.startsWith(item.to);

                  return (
                    <Link
                      activeOptions={{ exact: item.to === "/operational" }}
                      className={cn(
                        "rounded-[24px] transition",
                        collapsed ? "flex justify-center px-2 py-3" : "flex items-start gap-3 px-4 py-3",
                        active
                          ? "bg-primary text-primary-foreground shadow-panel"
                          : "text-muted-foreground hover:bg-muted hover:text-foreground",
                      )}
                      key={item.to}
                      onClick={onNavigate}
                      title={collapsed ? item.label : undefined}
                      to={item.to}
                    >
                      <div
                        className={cn(
                          "flex shrink-0 items-center justify-center rounded-2xl border",
                          collapsed ? "h-11 w-11" : "mt-0.5 h-10 w-10",
                          active
                            ? "border-white/20 bg-black/10"
                            : "border-border/70 bg-background/80",
                        )}
                      >
                        <Icon className="h-4 w-4" />
                      </div>
                      {!collapsed ? (
                        <div className="min-w-0">
                          <p className="text-sm font-semibold">{item.label}</p>
                          <p
                            className={cn(
                              "mt-1 text-xs",
                              active ? "text-primary-foreground/80" : "text-muted-foreground",
                            )}
                          >
                            {item.caption}
                          </p>
                        </div>
                      ) : null}
                    </Link>
                  );
                })}
              </div>
            </section>
          );
        })}
      </div>

      <div
        className={cn(
          "mt-auto w-full rounded-[24px] border border-border/70 bg-background/75",
          collapsed ? "p-3" : "p-4",
        )}
      >
        <div className={cn("flex gap-3", collapsed ? "justify-center" : "items-center")}>
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-secondary text-secondary-foreground">
            <Building2 className="h-5 w-5" />
          </div>
          {!collapsed ? (
            <div>
              <p className="text-sm font-semibold">Operational boards</p>
              <p className="text-xs text-muted-foreground">
                Buka `Projects` untuk masuk ke board utama tim.
              </p>
            </div>
          ) : null}
        </div>
      </div>
    </aside>
  );
}

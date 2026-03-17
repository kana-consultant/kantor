import {
  Bot,
  Building2,
  FolderKanban,
  LayoutDashboard,
  Megaphone,
  ShieldCheck,
  Users,
} from "lucide-react";
import { Link, useRouterState } from "@tanstack/react-router";

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

export function Sidebar() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const { hasPermission } = useRBAC();

  return (
    <aside className="flex h-full flex-col rounded-[34px] border border-border/80 bg-card/80 p-5 shadow-panel backdrop-blur">
      <div className="mb-8 rounded-[28px] bg-gradient-to-br from-primary to-orange-300 p-5 text-primary-foreground shadow-panel">
        <div className="flex items-start gap-4">
          <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-black/10">
            <ShieldCheck className="h-6 w-6" />
          </div>
          <div>
            <p className="text-xs uppercase tracking-[0.28em] text-primary-foreground/80">
              Internal Platform
            </p>
            <h1 className="mt-1 text-xl font-bold">KANTOR</h1>
            <p className="mt-2 max-w-[13rem] text-sm text-primary-foreground/80">
              Boards, workflow, dan automation untuk tim internal.
            </p>
          </div>
        </div>
      </div>

      <div className="space-y-7">
        {sections.map((section) => {
          const visibleItems = section.items.filter((item) => hasPermission(item.permission));
          if (visibleItems.length === 0) {
            return null;
          }

          return (
            <section key={section.label}>
              <p className="mb-3 px-2 text-xs uppercase tracking-[0.28em] text-muted-foreground">
                {section.label}
              </p>
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
                        "flex items-start gap-3 rounded-[24px] px-4 py-3 transition",
                        active
                          ? "bg-primary text-primary-foreground shadow-panel"
                          : "text-muted-foreground hover:bg-muted hover:text-foreground",
                      )}
                      key={item.to}
                      to={item.to}
                    >
                      <div
                        className={cn(
                          "mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl border",
                          active
                            ? "border-white/20 bg-black/10"
                            : "border-border/70 bg-background/80",
                        )}
                      >
                        <Icon className="h-4 w-4" />
                      </div>
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
                    </Link>
                  );
                })}
              </div>
            </section>
          );
        })}
      </div>

      <div className="mt-auto rounded-[24px] border border-border/70 bg-background/75 p-4">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-secondary text-secondary-foreground">
            <Building2 className="h-5 w-5" />
          </div>
          <div>
            <p className="text-sm font-semibold">Operational boards</p>
            <p className="text-xs text-muted-foreground">
              Buka `Projects` untuk masuk ke board seperti Trello.
            </p>
          </div>
        </div>
      </div>
    </aside>
  );
}

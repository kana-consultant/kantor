import { Building2, Megaphone, ShieldCheck, Users } from "lucide-react";
import { Link, useRouterState } from "@tanstack/react-router";

import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";

const items = [
  {
    to: "/operational",
    label: "Operational",
    icon: Building2,
    permission: permissions.operationalOverview,
  },
  {
    to: "/hris",
    label: "HRIS",
    icon: Users,
    permission: permissions.hrisOverview,
  },
  {
    to: "/marketing",
    label: "Marketing",
    icon: Megaphone,
    permission: permissions.marketingOverview,
  },
];

export function Sidebar() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const { hasPermission } = useRBAC();

  return (
    <aside className="flex h-full flex-col rounded-[32px] border border-border/80 bg-card/75 p-5 shadow-panel backdrop-blur">
      <div className="mb-10 flex items-center gap-3">
        <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
          <ShieldCheck className="h-6 w-6" />
        </div>
        <div>
          <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Internal Platform
          </p>
          <h1 className="text-lg font-bold">KANTOR</h1>
        </div>
      </div>

      <nav className="space-y-2">
        {items
          .filter((item) => hasPermission(item.permission))
          .map((item) => {
          const Icon = item.icon;
          const active = pathname.startsWith(item.to);

          return (
            <Link
              activeOptions={{ exact: item.to === "/operational" }}
              className={cn(
                "flex items-center gap-3 rounded-2xl px-4 py-3 text-sm font-medium transition",
                active
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground",
              )}
              key={item.to}
              to={item.to}
            >
              <Icon className="h-4 w-4" />
              <span>{item.label}</span>
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}

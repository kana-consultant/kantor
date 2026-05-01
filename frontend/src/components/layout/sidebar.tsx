import {
  Activity,
  ChevronsLeft,
  X,
  FolderKanban,
  LayoutDashboard,
  Megaphone,
  MessageCircle,
  Globe,
  Server,
  Users,
  Building2,
  Wallet,
  Receipt,
  CreditCard,
  BarChart3,
  UserPlus,
  Shield,
  ShieldCheck,
  ScrollText,
  Settings2,
} from "lucide-react";
import { Link, useRouterState } from "@tanstack/react-router";

import { KantorLogo } from "@/components/layout/kantor-logo";
import { Tooltip } from "@/components/ui/tooltip";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";

interface NavItem {
  to: string;
  label: string;
  icon: typeof LayoutDashboard;
  permission: string;
}

interface NavSection {
  id: 'ops' | 'hr' | 'mkt' | 'admin';
  label: string;
  items: NavItem[];
}

const sections: NavSection[] = [
  {
    id: 'ops',
    label: "OPERASIONAL",
    items: [
      {
        to: "/operational/overview",
        label: "Overview",
        icon: LayoutDashboard,
        permission: permissions.operationalOverview,
      },
      {
        to: "/operational/projects",
        label: "Projects",
        icon: FolderKanban,
        permission: permissions.operationalProjectView,
      },
      {
        to: "/operational/tracker",
        label: "Activity Tracker",
        icon: Activity,
        permission: permissions.operationalTrackerView,
      },
      {
        to: "/operational/wa-broadcast",
        label: "WA Broadcast",
        icon: MessageCircle,
        permission: permissions.operationalWAView,
      },
      {
        to: "/operational/vps",
        label: "VPS Monitor",
        icon: Server,
        permission: permissions.operationalVPSView,
      },
      {
        to: "/operational/domains",
        label: "Domains",
        icon: Globe,
        permission: permissions.operationalDomainView,
      },
    ],
  },
  {
    id: 'hr',
    label: "HRIS",
    items: [
      {
        to: "/hris/overview",
        label: "Overview",
        icon: LayoutDashboard,
        permission: permissions.hrisOverview,
      },
      {
        to: "/hris/employees",
        label: "Employees",
        icon: Users,
        permission: permissions.hrisEmployeeView,
      },
      {
        to: "/hris/departments",
        label: "Departments",
        icon: Building2,
        permission: permissions.hrisDepartmentView,
      },
      {
        to: "/hris/finance",
        label: "Finance",
        icon: Wallet,
        permission: permissions.hrisFinanceView,
      },
      {
        to: "/hris/reimbursements",
        label: "Reimbursements",
        icon: Receipt,
        permission: permissions.hrisReimbursementView,
      },
      {
        to: "/hris/subscriptions",
        label: "Subscriptions",
        icon: CreditCard,
        permission: permissions.hrisSubscriptionView,
      },
    ],
  },
  {
    id: 'mkt',
    label: "MARKETING",
    items: [
      {
        to: "/marketing/overview",
        label: "Overview",
        icon: LayoutDashboard,
        permission: permissions.marketingOverview,
      },
      {
        to: "/marketing/campaigns",
        label: "Campaigns",
        icon: Megaphone,
        permission: permissions.marketingCampaignView,
      },
      {
        to: "/marketing/ads-metrics",
        label: "Ads Metrics",
        icon: BarChart3,
        permission: permissions.marketingAdsMetricsView,
      },
      {
        to: "/marketing/leads",
        label: "Leads",
        icon: UserPlus,
        permission: permissions.marketingLeadsView,
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
  const { hasModuleAccess, hasPermission, isSuperAdmin } = useRBAC();

  const visibleSections: NavSection[] = sections
    .filter((section) => {
      if (section.id === "ops") return hasModuleAccess("operational");
      if (section.id === "hr") return hasModuleAccess("hris");
      if (section.id === "mkt") return hasModuleAccess("marketing");
      return true;
    })
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => hasPermission(item.permission)),
    }))
    .filter((section) => section.items.length > 0);

  if (isSuperAdmin || hasModuleAccess("admin")) {
    const adminItems = [
      {
        to: "/admin/audit-logs",
        label: "Audit Logs",
        icon: ScrollText,
        permission: permissions.adminAuditLogView,
      },
      {
        to: "/admin/roles",
        label: "Roles",
        icon: Shield,
        permission: permissions.adminRolesView,
      },
      {
        to: "/admin/users",
        label: "Users",
        icon: Users,
        permission: permissions.adminUsersView,
      },
      {
        to: "/admin/settings",
        label: "Settings",
        icon: Settings2,
        permission: permissions.adminSettingsView,
      },
    ].filter((item) => hasPermission(item.permission));

    if (isSuperAdmin) {
      adminItems.push({
        to: "/admin/registration",
        label: "Registrasi",
        icon: ShieldCheck,
        permission: "__super_admin__",
      });
    }

    if (adminItems.length > 0) {
      visibleSections.push({
        id: "admin",
        label: "ADMIN",
        items: adminItems,
      });
    }
  }

  return (
    <aside
      className={cn(
        "relative flex h-full shrink-0 flex-col transition-all duration-200 ease-in-out",
        mobile
          ? "h-full w-full rounded-[28px] border border-border/80 bg-surface shadow-[0_28px_64px_-28px_rgba(15,23,42,0.55)]"
          : "rounded-[24px] border border-border/70 bg-surface/92 shadow-[0_24px_56px_-32px_rgba(15,23,42,0.38)] backdrop-blur-xl",
        !mobile && (collapsed ? "w-[68px]" : "w-[256px]")
      )}
    >
      <div className={cn("flex shrink-0 items-center", mobile ? "h-[78px] px-5" : "h-[72px] px-5")}>
        <div className={cn("flex w-full items-center", collapsed ? "justify-center" : "justify-between")}>
          <KantorLogo compact={collapsed} />
          {mobile ? (
            <button
              aria-label="Close sidebar"
              className="inline-flex h-10 w-10 items-center justify-center rounded-full border border-border/70 bg-surface text-text-secondary transition hover:bg-surface-muted hover:text-text-primary"
              onClick={onNavigate}
              type="button"
            >
              <X className="h-4.5 w-4.5" />
            </button>
          ) : null}
        </div>
      </div>

      <div className={cn("flex-1 space-y-5 overflow-y-auto overflow-x-hidden", mobile ? "px-4 pb-6 pt-2" : "px-3 py-2")}>
        {visibleSections.map((section) => (
          <div key={section.id}>
             {!collapsed ? (
                <div className={cn(
                  mobile ? "mb-2 mt-5 px-3 text-[10px] font-[800] uppercase tracking-[0.18em]" : "mb-2 mt-6 px-3 text-[11px] font-[700] uppercase tracking-[0.12em]",
                  section.id === 'ops' ? 'text-ops' : section.id === 'hr' ? 'text-hr' : section.id === 'mkt' ? 'text-mkt' : 'text-error'
                )}>
                 {section.label}
                </div>
             ) : (
                <div className="mt-6 mb-2 flex justify-center">
                  <Tooltip content={section.label}>
                    <div className={cn(
                      "w-2 h-2 rounded-full",
                      section.id === 'ops' ? 'bg-ops' : section.id === 'hr' ? 'bg-hr' : section.id === 'mkt' ? 'bg-mkt' : 'bg-error'
                    )} />
                  </Tooltip>
                </div>
             )}

             <div className="space-y-1">
               {section.items.map((item) => {
                  const Icon = item.icon;
                  // For the overview pages we want to match exact, or startswith for nested pages
                  const active = pathname === item.to || pathname.startsWith(item.to + '/');
                  
                  let activeColors = "";
                  if (active) {
                     if (section.id === 'ops') activeColors = "border border-ops/15 bg-ops-light text-ops font-[700] shadow-[inset_0_1px_0_rgba(255,255,255,0.25)]";
                     if (section.id === 'hr') activeColors = "border border-hr/15 bg-hr-light text-hr font-[700] shadow-[inset_0_1px_0_rgba(255,255,255,0.25)]";
                     if (section.id === 'mkt') activeColors = "border border-mkt/15 bg-mkt-light text-mkt font-[700] shadow-[inset_0_1px_0_rgba(255,255,255,0.25)]";
                     if (section.id === 'admin') activeColors = "border border-error/15 bg-error-light text-error font-[700] shadow-[inset_0_1px_0_rgba(255,255,255,0.25)]";
                  }

                  return (
                    <Tooltip content={item.label} key={item.to}>
                      <Link
                        to={item.to}
                        onClick={onNavigate}
                        className={cn(
                          mobile ? "flex h-11 items-center rounded-2xl px-3.5 text-[15px] transition-all" : "flex h-10 items-center rounded-xl px-3 text-[14px] transition-all",
                          collapsed ? "justify-center px-0 w-11 mx-auto" : "",
                          !active && (mobile
                            ? "font-[600] text-text-primary hover:bg-surface-muted hover:text-text-primary"
                            : "font-[500] text-text-secondary hover:bg-surface-muted/80 hover:text-text-primary"),
                          activeColors
                        )}
                      >
                        <Icon className={cn("shrink-0", collapsed ? "w-5 h-5 mx-auto" : mobile ? "mr-3 h-5 w-5" : "w-5 h-5 mr-3")} strokeWidth={1.5} />
                        {!collapsed && <span>{item.label}</span>}
                      </Link>
                    </Tooltip>
                  )
               })}
             </div>
          </div>
        ))}
      </div>

      {!mobile && onToggleCollapse && (
        <div className="mt-auto shrink-0 border-t border-border/50 p-3">
           <button
             onClick={onToggleCollapse}
             className={cn(
               "flex h-10 w-full items-center rounded-xl text-text-secondary transition-colors hover:bg-surface-muted hover:text-text-primary",
               collapsed ? "justify-center" : "px-3"
             )}
             aria-label="Toggle sidebar"
           >
             <ChevronsLeft className={cn("w-5 h-5 shrink-0 transition-transform", collapsed ? "rotate-180" : "mr-3")} strokeWidth={1.5} />
             {!collapsed && <span className="text-[14px] font-[500]">Collapse</span>}
           </button>
        </div>
      )}
    </aside>
  );
}

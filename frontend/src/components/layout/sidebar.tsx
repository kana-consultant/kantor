import {
  Activity,
  ChevronsLeft,
  FolderKanban,
  LayoutDashboard,
  Megaphone,
  MessageCircle,
  Users,
  Building2,
  Wallet,
  Receipt,
  CreditCard,
  BarChart3,
  UserPlus,
  Shield,
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
        "relative flex h-full flex-col rounded-lg bg-surface transition-all duration-200 ease-in-out shrink-0",
        collapsed ? "w-[68px]" : "w-[256px]"
      )}
    >
      <div className="flex h-[64px] items-center px-5 shrink-0">
        <div className={cn("flex w-full items-center", collapsed ? "justify-center" : "justify-between")}>
          <KantorLogo compact={collapsed} />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto overflow-x-hidden px-3 py-2 space-y-6">
        {visibleSections.map((section) => (
          <div key={section.id}>
             {!collapsed ? (
                <div className={cn(
                  "mt-6 mb-2 px-3 text-[11px] font-[700] uppercase tracking-[0.08em]",
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
                     if (section.id === 'ops') activeColors = "bg-ops-light text-ops font-[600]";
                     if (section.id === 'hr') activeColors = "bg-hr-light text-hr font-[600]";
                     if (section.id === 'mkt') activeColors = "bg-mkt-light text-mkt font-[600]";
                     if (section.id === 'admin') activeColors = "bg-error-light text-error font-[600]";
                  }

                  return (
                    <Tooltip content={item.label} key={item.to}>
                      <Link
                        to={item.to}
                        onClick={onNavigate}
                        className={cn(
                          "flex items-center h-[36px] px-3 rounded-sm text-[14px] transition-colors",
                          collapsed ? "justify-center px-0 w-11 mx-auto" : "",
                          !active && "text-text-secondary font-[500] hover:bg-surface-muted hover:text-text-primary",
                          active && "shadow-[inset_3px_0_0_0_currentColor]",
                          activeColors
                        )}
                      >
                        <Icon className={cn("shrink-0", collapsed ? "w-5 h-5 mx-auto" : "w-5 h-5 mr-3")} strokeWidth={1.5} />
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
        <div className="p-3 mt-auto shrink-0 border-t border-border/50">
           <button
             onClick={onToggleCollapse}
             className={cn(
               "flex items-center text-text-secondary hover:bg-surface-muted hover:text-text-primary h-[36px] rounded-[6px] transition-colors w-full",
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

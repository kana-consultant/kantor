import {
  ChevronsLeft,
  FolderKanban,
  LayoutDashboard,
  Megaphone,
  Users,
  Building2,
  Wallet,
  Receipt,
  CreditCard,
  Zap,
  BarChart3,
  UserPlus
} from "lucide-react";
import { Link, useRouterState } from "@tanstack/react-router";

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
  id: 'ops' | 'hr' | 'mkt';
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
        to: "/operational/automation",
        label: "Automation",
        icon: Zap,
        permission: permissions.operationalAssignmentView,
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
  const { hasPermission } = useRBAC();

  const visibleSections = sections
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => hasPermission(item.permission)),
    }))
    .filter((section) => section.items.length > 0);

  return (
    <aside
      className={cn(
        "relative flex h-full flex-col bg-surface border-r border-border transition-all duration-200 ease-in-out shrink-0",
        collapsed ? "w-[68px]" : "w-[256px]"
      )}
    >
      {/* Logo Area */}
      <div className="flex h-[64px] items-center p-[20px] shrink-0">
        <div className={cn("flex w-full items-center", collapsed ? "justify-center" : "justify-between")}>
          {!collapsed ? (
             <div className="text-[20px] font-[800] leading-none tracking-tight font-display text-text-primary flex items-start">
               K<span className="relative">A<span className="absolute -top-1 left-1/2 -translate-x-1/2 w-1.5 h-1.5 rounded-full bg-ops"></span></span>NTOR
             </div>
          ) : (
             <div className="text-[20px] font-[800] leading-none tracking-tight font-display text-text-primary flex items-start">
               K<span className="relative">A<span className="absolute -top-1 left-1/2 -translate-x-1/2 w-1.5 h-1.5 rounded-full bg-ops"></span></span>
             </div>
          )}
        </div>
      </div>

      {/* Navigation Area */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden px-3 py-2 space-y-6">
        {visibleSections.map((section) => (
          <div key={section.id}>
             {!collapsed ? (
                <div className={cn(
                  "mt-6 mb-2 px-3 text-[11px] font-[700] uppercase tracking-[0.08em]",
                  section.id === 'ops' ? 'text-ops' : section.id === 'hr' ? 'text-hr' : 'text-mkt'
                )}>
                  {section.label}
                </div>
             ) : (
                <div className="mt-6 mb-2 flex justify-center">
                  <div className={cn(
                    "w-2 h-2 rounded-full",
                    section.id === 'ops' ? 'bg-ops' : section.id === 'hr' ? 'bg-hr' : 'bg-mkt'
                  )} title={section.label} />
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
                  }

                  return (
                    <Link
                      key={item.to}
                      to={item.to}
                      onClick={onNavigate}
                      title={collapsed ? item.label : undefined}
                      className={cn(
                        "flex items-center h-[36px] px-3 rounded-[6px] text-[14px] transition-colors",
                        collapsed ? "justify-center px-0 w-11 mx-auto" : "",
                        !active && "text-text-secondary font-[500] hover:bg-surface-muted hover:text-text-primary",
                        activeColors
                      )}
                    >
                      <Icon className={cn("shrink-0", collapsed ? "w-5 h-5 mx-auto" : "w-5 h-5 mr-3")} strokeWidth={1.5} />
                      {!collapsed && <span>{item.label}</span>}
                    </Link>
                  )
               })}
             </div>
          </div>
        ))}
      </div>

      {/* Collapse Toggle */}
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

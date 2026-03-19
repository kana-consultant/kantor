import {
  BarChart3,
  Building2,
  CreditCard,
  FolderKanban,
  LayoutDashboard,
  Megaphone,
  Receipt,
  UserPlus,
  Users,
  Wallet,
  type LucideIcon,
} from "lucide-react";

export type ModuleThemeKey = "ops" | "hr" | "mkt" | "base";

export interface ModuleThemeMeta {
  key: ModuleThemeKey;
  label: string;
  accentClassName: string;
  lightClassName: string;
  darkClassName: string;
}

export interface BreadcrumbMeta {
  module: ModuleThemeMeta;
  title: string;
  icon: LucideIcon;
}

const moduleThemes: Record<Exclude<ModuleThemeKey, "base">, ModuleThemeMeta> = {
  ops: {
    key: "ops",
    label: "Operasional",
    accentClassName: "text-ops",
    lightClassName: "bg-ops-light",
    darkClassName: "text-ops-dark",
  },
  hr: {
    key: "hr",
    label: "HRIS",
    accentClassName: "text-hr",
    lightClassName: "bg-hr-light",
    darkClassName: "text-hr-dark",
  },
  mkt: {
    key: "mkt",
    label: "Marketing",
    accentClassName: "text-mkt",
    lightClassName: "bg-mkt-light",
    darkClassName: "text-mkt-dark",
  },
};

const baseModule: ModuleThemeMeta = {
  key: "base",
  label: "KANTOR",
  accentClassName: "text-text-secondary",
  lightClassName: "bg-surface-muted",
  darkClassName: "text-text-primary",
};

const breadcrumbRules: Array<{
  match: (pathname: string) => boolean;
  meta: BreadcrumbMeta;
}> = [
  {
    match: (pathname) => pathname.startsWith("/operational/projects"),
    meta: { module: moduleThemes.ops, title: "Projects", icon: FolderKanban },
  },
  {
    match: (pathname) => pathname.startsWith("/operational/overview") || pathname === "/operational",
    meta: { module: moduleThemes.ops, title: "Overview", icon: LayoutDashboard },
  },
  {
    match: (pathname) => pathname.startsWith("/hris/employees"),
    meta: { module: moduleThemes.hr, title: "Employees", icon: Users },
  },
  {
    match: (pathname) => pathname.startsWith("/hris/departments"),
    meta: { module: moduleThemes.hr, title: "Departments", icon: Building2 },
  },
  {
    match: (pathname) => pathname.startsWith("/hris/finance"),
    meta: { module: moduleThemes.hr, title: "Finance", icon: Wallet },
  },
  {
    match: (pathname) => pathname.startsWith("/hris/reimbursements"),
    meta: { module: moduleThemes.hr, title: "Reimbursements", icon: Receipt },
  },
  {
    match: (pathname) => pathname.startsWith("/hris/subscriptions"),
    meta: { module: moduleThemes.hr, title: "Subscriptions", icon: CreditCard },
  },
  {
    match: (pathname) => pathname.startsWith("/hris/overview") || pathname === "/hris",
    meta: { module: moduleThemes.hr, title: "Overview", icon: LayoutDashboard },
  },
  {
    match: (pathname) => pathname.startsWith("/marketing/campaigns"),
    meta: { module: moduleThemes.mkt, title: "Campaigns", icon: Megaphone },
  },
  {
    match: (pathname) => pathname.startsWith("/marketing/ads-metrics"),
    meta: { module: moduleThemes.mkt, title: "Ads Metrics", icon: BarChart3 },
  },
  {
    match: (pathname) => pathname.startsWith("/marketing/leads"),
    meta: { module: moduleThemes.mkt, title: "Leads", icon: UserPlus },
  },
  {
    match: (pathname) => pathname.startsWith("/marketing/overview") || pathname === "/marketing",
    meta: { module: moduleThemes.mkt, title: "Overview", icon: LayoutDashboard },
  },
];

export function resolveModuleTheme(pathname: string): ModuleThemeMeta {
  if (pathname.startsWith("/operational")) {
    return moduleThemes.ops;
  }

  if (pathname.startsWith("/hris")) {
    return moduleThemes.hr;
  }

  if (pathname.startsWith("/marketing")) {
    return moduleThemes.mkt;
  }

  return baseModule;
}

export function resolveBreadcrumb(pathname: string): BreadcrumbMeta {
  return (
    breadcrumbRules.find((rule) => rule.match(pathname))?.meta ?? {
      module: baseModule,
      title: "Workspace",
      icon: LayoutDashboard,
    }
  );
}

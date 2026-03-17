import { ArrowRight, FolderKanban, LogOut, Sparkles } from "lucide-react";
import { Link, useNavigate, useRouterState } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { useAuth } from "@/hooks/use-auth";
import { logout } from "@/services/auth";

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
  const navigate = useNavigate();
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const { user, roles } = useAuth();

  const page = pageMetadata.find((item) => item.match(pathname)) ?? {
    eyebrow: "Workspace",
    title: "Internal platform",
    summary: "Workspace terpadu untuk modul operasional, HRIS, dan marketing.",
  };

  const handleLogout = () => {
    logout();
    void navigate({ to: "/login" });
  };

  return (
    <header className="rounded-[30px] border border-border/80 bg-card/80 p-5 shadow-panel backdrop-blur">
      <div className="flex flex-col gap-5 xl:flex-row xl:items-center xl:justify-between">
        <div>
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

import { LogOut, Search, ShieldCheck } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { useAuth } from "@/hooks/use-auth";
import { logout } from "@/services/auth";

export function Topbar() {
  const navigate = useNavigate();
  const { user, roles } = useAuth();

  const handleLogout = () => {
    logout();
    void navigate({ to: "/login" });
  };

  return (
    <header className="flex flex-col gap-4 rounded-[28px] border border-border/80 bg-card/75 p-5 shadow-panel backdrop-blur lg:flex-row lg:items-center lg:justify-between">
      <div>
        <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Foundation Phase
        </p>
        <h2 className="text-2xl font-bold">Workspace Verification</h2>
      </div>

      <div className="flex items-center gap-3">
        <div className="hidden items-center gap-2 rounded-full border border-border bg-background/70 px-4 py-2 text-sm text-muted-foreground md:flex">
          <Search className="h-4 w-4" />
          {user ? `${user.full_name} · ${roles[0] ?? "no-role"}` : "Guest"}
        </div>
        <Button size="sm" variant="outline">
          <ShieldCheck className="h-4 w-4" />
          Active session
        </Button>
        <Button onClick={handleLogout} size="sm" variant="ghost">
          <LogOut className="h-4 w-4" />
          Logout
        </Button>
      </div>
    </header>
  );
}

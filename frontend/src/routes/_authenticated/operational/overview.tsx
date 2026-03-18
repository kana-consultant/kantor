import { createFileRoute } from "@tanstack/react-router";
import { CopyPlus, TrendingUp, FolderKanban, CheckCircle2 } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

export const Route = createFileRoute("/_authenticated/operational/overview")({
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold font-display tracking-tight text-foreground">Operational Overview</h1>
          <p className="text-muted-foreground mt-1 text-sm">Monitor project health and team productivity.</p>
        </div>
        <Button variant="ops">
          <CopyPlus className="mr-2 h-4 w-4" />
          New Project
        </Button>
      </div>

      <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-ops/10 text-ops">
              <FolderKanban className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Active Projects</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5">12</h3>
            </div>
          </div>
        </Card>
        
        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-success/10 text-success">
              <CheckCircle2 className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Completed (Week)</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5">8</h3>
            </div>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-warning/10 text-warning">
              <TrendingUp className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">At Risk Projects</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5">2</h3>
            </div>
          </div>
        </Card>
      </div>
      
      <Card className="border-dashed border-2 bg-transparent shadow-none p-12 text-center rounded-2xl">
        <FolderKanban className="mx-auto h-12 w-12 text-muted-foreground/30 mb-4" />
        <h3 className="text-lg font-semibold font-display text-foreground">Dashboard Analytics Coming Soon</h3>
        <p className="mt-2 text-sm text-muted-foreground">We are working on integrating more detailed project analytics and utilization metrics.</p>
      </Card>
    </div>
  );
}

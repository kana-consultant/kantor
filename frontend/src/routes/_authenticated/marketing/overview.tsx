import { createFileRoute } from "@tanstack/react-router";
import { CopyPlus, Megaphone, TrendingUp, Presentation } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

export const Route = createFileRoute("/_authenticated/marketing/overview")({
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold font-display tracking-tight text-foreground">Marketing Overview</h1>
          <p className="text-muted-foreground mt-1 text-sm">Monitor campaign performance and lead generation.</p>
        </div>
        <Button variant="mkt">
          <CopyPlus className="mr-2 h-4 w-4" />
          New Campaign
        </Button>
      </div>

      <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-mkt/10 text-mkt">
              <Megaphone className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Active Campaigns</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5">8</h3>
            </div>
          </div>
        </Card>
        
        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-success/10 text-success">
              <TrendingUp className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">New Leads (This Week)</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5">124</h3>
            </div>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-mkt-light text-mkt">
              <Presentation className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Avg. Conversion Rate</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5 text-mkt">4.2%</h3>
            </div>
          </div>
        </Card>
      </div>
      
      <Card className="border-dashed border-2 bg-transparent shadow-none p-12 text-center rounded-2xl">
        <Megaphone className="mx-auto h-12 w-12 text-muted-foreground/30 mb-4" />
        <h3 className="text-lg font-semibold font-display text-foreground">Marketing Analytics Coming Soon</h3>
        <p className="mt-2 text-sm text-muted-foreground">We are working on bringing more detailed funnel analysis and ad spend tracking.</p>
      </Card>
    </div>
  );
}

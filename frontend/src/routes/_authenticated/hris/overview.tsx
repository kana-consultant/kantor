import { createFileRoute } from "@tanstack/react-router";
import { CopyPlus, Users, Banknote, Building2 } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

export const Route = createFileRoute("/_authenticated/hris/overview")({
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold font-display tracking-tight text-foreground">HRIS Overview</h1>
          <p className="text-muted-foreground mt-1 text-sm">Manage employees, departments, and financial data.</p>
        </div>
        <Button variant="hr">
          <CopyPlus className="mr-2 h-4 w-4" />
          Add Employee
        </Button>
      </div>

      <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-hr/10 text-hr">
              <Users className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Total Employees</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5">48</h3>
            </div>
          </div>
        </Card>
        
        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-hr-light text-hr">
              <Building2 className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Departments</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5">6</h3>
            </div>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-success/10 text-success">
              <Banknote className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">This Month Payroll</p>
              <h3 className="text-2xl font-bold font-display tracking-tight mt-0.5 text-success">Rp 148M</h3>
            </div>
          </div>
        </Card>
      </div>
      
      <Card className="border-dashed border-2 bg-transparent shadow-none p-12 text-center rounded-2xl">
        <Users className="mx-auto h-12 w-12 text-muted-foreground/30 mb-4" />
        <h3 className="text-lg font-semibold font-display text-foreground">HR Dashboard Analytics Coming Soon</h3>
        <p className="mt-2 text-sm text-muted-foreground">We are working on bringing full organization charts and attendance tracking.</p>
      </Card>
    </div>
  );
}

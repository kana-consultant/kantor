import { createFileRoute } from "@tanstack/react-router";

import { Card } from "@/components/ui/card";

export const Route = createFileRoute("/_authenticated/forbidden")({
  component: ForbiddenPage,
});

function ForbiddenPage() {
  return (
    <Card className="p-8">
      <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">
        Access denied
      </p>
      <h3 className="mt-3 text-3xl font-bold">You do not have permission</h3>
      <p className="mt-3 max-w-2xl text-muted-foreground">
        Akun Anda sudah login, tetapi permission untuk halaman ini tidak ada di
        JWT claims saat ini.
      </p>
    </Card>
  );
}

import { useEffect, useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";

import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { formatIDR } from "@/lib/currency";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensurePermission } from "@/lib/rbac";
import { fetchProtectedFileBlob, getProtectedFileName, openProtectedFile } from "@/services/files";
import {
  getReimbursement,
  markReimbursementPaid,
  reimbursementsKeys,
  reviewReimbursement,
} from "@/services/hris-reimbursements";

export const Route = createFileRoute("/_authenticated/hris/reimbursements/$reimbursementId")({
  beforeLoad: async () => {
    await ensurePermission(permissions.hrisReimbursementView);
  },
  component: ReimbursementDetailPage,
});

function ReimbursementDetailPage() {
  const { reimbursementId } = Route.useParams();
  const queryClient = useQueryClient();
  const { hasPermission, hasRole } = useRBAC();
  const [notes, setNotes] = useState("");

  const reimbursementQuery = useQuery({
    queryKey: reimbursementsKeys.detail(reimbursementId),
    queryFn: () => getReimbursement(reimbursementId),
  });

  const reviewMutation = useMutation({
    mutationFn: (decision: "approved" | "rejected") =>
      reviewReimbursement(reimbursementId, decision, notes),
    onSuccess: async () => {
      setNotes("");
      await queryClient.invalidateQueries({ queryKey: reimbursementsKeys.all });
    },
  });

  const paidMutation = useMutation({
    mutationFn: () => markReimbursementPaid(reimbursementId, notes),
    onSuccess: async () => {
      setNotes("");
      await queryClient.invalidateQueries({ queryKey: reimbursementsKeys.all });
    },
  });

  const item = reimbursementQuery.data;
  if (!item) {
    return (
      <Card className="p-8 text-sm text-muted-foreground">
        {reimbursementQuery.isLoading ? "Memuat reimbursement..." : "Reimbursement tidak ditemukan."}
      </Card>
    );
  }

  const canReview = hasPermission(permissions.hrisReimbursementApprove);
  const canMarkPaid = hasRole("manager", "hris") || hasRole("admin", "hris") || hasRole("super_admin");

  return (
    <div className="space-y-6">
      <Card className="p-8">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <p className="text-sm uppercase tracking-[0.28em] text-muted-foreground">{item.employee_name}</p>
            <h3 className="mt-2 text-3xl font-bold">{item.title}</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              {item.category} - {new Date(item.transaction_date).toLocaleDateString("id-ID")}
            </p>
          </div>
          <div className="text-right">
            <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Amount</p>
            <p className="mt-2 text-3xl font-bold">{formatIDR(item.amount)}</p>
            <div className="mt-3">
              <StatusBadge status={item.status} variant="reimbursement-status" />
            </div>
          </div>
        </div>
        <p className="mt-5 text-sm text-muted-foreground">{item.description}</p>
        <div className="mt-5">
          <Link className="inline-flex h-10 items-center justify-center rounded-full border border-border bg-background/80 px-5 text-sm font-medium transition hover:bg-muted" to="/hris/reimbursements">
            Back to reimbursements
          </Link>
        </div>
      </Card>

      <div className="grid gap-6 xl:grid-cols-[2fr,1fr]">
        <div className="space-y-6">
          <Card className="p-6">
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Attachments</p>
            <h4 className="mt-2 text-2xl font-bold">Proof files</h4>
            <div className="mt-5 grid gap-4">
              {item.attachments.length === 0 ? (
                <div className="rounded-[22px] border border-dashed border-border/70 bg-background/70 p-6 text-sm text-muted-foreground">
                  Belum ada attachment.
                </div>
              ) : (
                item.attachments.map((attachment) => (
                  <AttachmentPreview attachment={attachment} key={attachment} reimbursementId={item.id} />
                ))
              )}
            </div>
          </Card>

          <Card className="p-6">
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Approval timeline</p>
            <h4 className="mt-2 text-2xl font-bold">Status journey</h4>
            <div className="mt-5 space-y-3">
              <TimelineRow label="Submitted" value={new Date(item.created_at).toLocaleString("id-ID")} />
              <TimelineRow label="Reviewed" value={item.manager_action_at ? new Date(item.manager_action_at).toLocaleString("id-ID") : "-"} notes={item.manager_notes ?? undefined} />
              <TimelineRow label="Paid" value={item.paid_at ? new Date(item.paid_at).toLocaleString("id-ID") : "-"} />
            </div>
          </Card>
        </div>

        <div className="space-y-6">
          <Card className="p-6">
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Actions</p>
            <h4 className="mt-2 text-2xl font-bold">Claim actions</h4>
            <Input className="mt-4" onChange={(event) => setNotes(event.target.value)} placeholder="Notes for this action" value={notes} />
            <div className="mt-5 flex flex-col gap-3">
              {canReview && item.status === "submitted" ? (
                <>
                  <Button disabled={reviewMutation.isPending} onClick={() => reviewMutation.mutate("approved")}>
                    Approve reimbursement
                  </Button>
                  <Button disabled={reviewMutation.isPending} onClick={() => reviewMutation.mutate("rejected")} variant="ghost">
                    Reject reimbursement
                  </Button>
                </>
              ) : null}
              {canMarkPaid && item.status === "approved" ? (
                <Button disabled={paidMutation.isPending} onClick={() => paidMutation.mutate()}>
                  Mark paid
                </Button>
              ) : null}
              {!canReview && !canMarkPaid ? (
                <div className="rounded-[22px] border border-dashed border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
                  Anda hanya punya akses lihat untuk reimbursement ini.
                </div>
              ) : null}
            </div>
          </Card>

          <Card className="p-6">
            <p className="text-sm uppercase tracking-[0.24em] text-muted-foreground">Summary</p>
            <h4 className="mt-2 text-2xl font-bold">Claim details</h4>
            <div className="mt-5 space-y-3">
              <InfoRow label="Employee" value={item.employee_name} />
              <InfoRow label="Category" value={item.category} />
              <InfoRow label="Amount" value={formatIDR(item.amount)} />
              <InfoRow label="Attachments" value={String(item.attachments.length)} />
              <InfoRow label="Last update" value={new Date(item.updated_at).toLocaleString("id-ID")} />
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}

function AttachmentPreview({
  attachment,
  reimbursementId,
}: {
  attachment: string;
  reimbursementId: string;
}) {
  const isPDF = attachment.toLowerCase().endsWith(".pdf");
  const fileName = getProtectedFileName(attachment);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);

  useEffect(() => {
    if (isPDF) {
      return undefined;
    }

    let active = true;
    let objectURL: string | null = null;

    void fetchProtectedFileBlob("reimbursements", reimbursementId, fileName)
      .then((blob) => {
        if (!active) {
          return;
        }
        objectURL = URL.createObjectURL(blob);
        setPreviewUrl(objectURL);
      })
      .catch(() => {
        if (!active) {
          return;
        }
        setPreviewError("Preview attachment tidak tersedia.");
      });

    return () => {
      active = false;
      if (objectURL) {
        URL.revokeObjectURL(objectURL);
      }
    };
  }, [fileName, isPDF, reimbursementId]);

  return (
    <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
      <div className="mb-4 flex items-center justify-between gap-3">
        <p className="text-sm font-semibold">{fileName}</p>
        <button
          className="text-sm font-medium text-primary"
          onClick={() => void openProtectedFile("reimbursements", reimbursementId, fileName)}
          type="button"
        >
          Open
        </button>
      </div>
      {isPDF ? (
        <div className="flex min-h-[220px] flex-col items-center justify-center rounded-2xl border border-dashed border-border/60 bg-card/70 px-6 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-3xl bg-primary/10 text-lg font-semibold text-primary">
            PDF
          </div>
          <p className="mt-4 text-sm font-semibold">Preview PDF dibuka di tab baru</p>
          <p className="mt-2 text-sm text-muted-foreground">
            Browser sering tidak merender PDF inline dengan konsisten. Gunakan tombol open untuk melihat dokumen penuh.
          </p>
        </div>
      ) : previewError ? (
        <div className="flex min-h-[220px] items-center justify-center rounded-2xl border border-dashed border-border/60 bg-card/70 px-6 text-center text-sm text-muted-foreground">
          {previewError}
        </div>
      ) : previewUrl ? (
        <img alt={attachment} className="max-h-[420px] w-full rounded-2xl object-cover" src={previewUrl} />
      ) : (
        <div className="h-[220px] animate-pulse rounded-2xl bg-muted" />
      )}
    </div>
  );
}

function TimelineRow({ label, value, notes }: { label: string; value: string; notes?: string }) {
  return (
    <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
      <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{label}</p>
      <p className="mt-2 text-sm font-semibold">{value}</p>
      {notes ? <p className="mt-2 text-sm text-muted-foreground">{notes}</p> : null}
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[18px] border border-border/70 bg-background/70 px-4 py-3">
      <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{label}</p>
      <p className="mt-2 text-sm font-semibold">{value}</p>
    </div>
  );
}


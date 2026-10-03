import { useEffect, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { BriefcaseBusiness, Pencil, Save } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { hrProfileKeys, updateHRProfile } from "@/services/hris-hr-profile";
import { toast } from "@/stores/toast-store";
import type { HRProfile } from "@/types/hris";

const jobTitleSchema = z.object({
  job_title: z.string().trim().max(150, "Maksimal 150 karakter"),
});

type JobTitleFormValues = z.infer<typeof jobTitleSchema>;

export function EmploymentDataCard({
  canEdit,
  employeeId,
  error,
  loading,
  profile,
}: {
  canEdit: boolean;
  employeeId: string;
  error: string | null;
  loading: boolean;
  profile: HRProfile | undefined;
}) {
  const queryClient = useQueryClient();
  const [isEditing, setIsEditing] = useState(false);

  const {
    formState: { errors },
    handleSubmit,
    register,
    reset,
  } = useForm<JobTitleFormValues>({
    resolver: zodResolver(jobTitleSchema),
    defaultValues: { job_title: "" },
  });

  useEffect(() => {
    if (!isEditing) {
      reset({ job_title: profile?.job_title ?? "" });
    }
  }, [isEditing, profile?.job_title, reset]);

  const saveMutation = useMutation({
    mutationFn: (values: JobTitleFormValues) => updateHRProfile(employeeId, { job_title: values.job_title }),
    onSuccess: (saved) => {
      toast.success("Jabatan berhasil diperbarui");
      queryClient.setQueryData(hrProfileKeys.detail(employeeId), saved);
      setIsEditing(false);
    },
    onError: (mutationError) => {
      toast.error(
        "Gagal memperbarui jabatan",
        mutationError instanceof Error ? mutationError.message : undefined,
      );
    },
  });

  const onSubmit = handleSubmit((values) => saveMutation.mutate(values));

  return (
    <Card className="space-y-5 p-6">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <div className="rounded-md bg-hr-light p-3 text-hr">
            <BriefcaseBusiness className="h-5 w-5" />
          </div>
          <div>
            <h3 className="font-display text-[18px] font-[700] text-text-primary">Data Kepegawaian</h3>
            <p className="mt-1 text-sm text-text-secondary">Kode karyawan dan jabatan untuk slip gaji dan kontrak.</p>
          </div>
        </div>
        {canEdit && !isEditing && profile ? (
          <Button onClick={() => setIsEditing(true)} size="sm" type="button" variant="outline">
            <Pencil className="h-4 w-4" />
            Ubah jabatan
          </Button>
        ) : null}
      </div>

      {error ? <p className="text-sm text-error">{error}</p> : null}

      {loading ? (
        <div className="space-y-3">
          <Skeleton className="h-16 rounded-lg" />
          <Skeleton className="h-16 rounded-lg" />
        </div>
      ) : profile ? (
        <div className="grid gap-4">
          <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
            <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Kode karyawan</p>
            {profile.employee_code ? (
              <p className="mt-2 font-mono text-sm font-semibold">{profile.employee_code}</p>
            ) : (
              <p className="mt-2 text-sm font-semibold text-text-secondary">Belum ada</p>
            )}
            <p className="mt-1 text-xs text-muted-foreground">
              Dibuat otomatis saat slip gaji atau kontrak pertama dibuat, dan tidak bisa diubah.
            </p>
          </div>

          {isEditing ? (
            <form className="space-y-3 rounded-[22px] border border-border/70 bg-background/70 p-4" noValidate onSubmit={onSubmit}>
              <label className="text-xs uppercase tracking-[0.18em] text-muted-foreground" htmlFor="hr-job-title">
                Jabatan
              </label>
              <Input
                autoFocus
                className="focus-visible:border-hr focus-visible:ring-hr/10"
                disabled={saveMutation.isPending}
                id="hr-job-title"
                maxLength={150}
                placeholder="mis. Staf Pemasaran Digital"
                {...register("job_title")}
              />
              {errors.job_title?.message ? <p className="text-xs text-error">{errors.job_title.message}</p> : null}
              <p className="text-xs text-muted-foreground">
                Dicetak di kontrak dan slip gaji. Kosongkan untuk menghapus.
              </p>
              <div className="flex justify-end gap-2">
                <Button
                  disabled={saveMutation.isPending}
                  onClick={() => setIsEditing(false)}
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  Batal
                </Button>
                <Button disabled={saveMutation.isPending} size="sm" type="submit">
                  <Save className="h-4 w-4" />
                  {saveMutation.isPending ? "Menyimpan..." : "Simpan"}
                </Button>
              </div>
            </form>
          ) : (
            <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">Jabatan</p>
              <p className="mt-2 text-sm font-semibold">{profile.job_title || "-"}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                Terpisah dari tipe kepegawaian (Full Time, Part Time, Internship, dan seterusnya).
              </p>
            </div>
          )}
        </div>
      ) : null}
    </Card>
  );
}

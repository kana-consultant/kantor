import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save, Wallet } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import {
  compensationPolicyKeys,
  getCompensationPolicy,
  updateCompensationPolicy,
} from "@/services/compensation-policy";
import { toast } from "@/stores/toast-store";

const timezoneOptions = [
  "Asia/Jakarta",
  "Asia/Makassar",
  "Asia/Jayapura",
  "UTC",
];

export function PolicySettingsCard() {
  const queryClient = useQueryClient();
  const { hasPermission } = useRBAC();
  const canManage = hasPermission(permissions.hrisCompensationPolicyManage);

  const [monthlyBaseSalary, setMonthlyBaseSalary] = useState(30000000);
  const [minHoursPerDay, setMinHoursPerDay] = useState(4);
  const [minHoursPerMonth, setMinHoursPerMonth] = useState(160);
  const [timezone, setTimezone] = useState("Asia/Jakarta");

  const policyQuery = useQuery({
    queryKey: compensationPolicyKeys.policy(),
    queryFn: getCompensationPolicy,
  });

  useEffect(() => {
    if (!policyQuery.data) {
      return;
    }
    setMonthlyBaseSalary(policyQuery.data.monthly_base_salary);
    setMinHoursPerDay(policyQuery.data.min_hours_per_day);
    setMinHoursPerMonth(policyQuery.data.min_hours_per_month);
    setTimezone(policyQuery.data.timezone);
  }, [policyQuery.data]);

  const updateMutation = useMutation({
    mutationFn: updateCompensationPolicy,
    onSuccess: async () => {
      toast.success("Aturan kompensasi berhasil diperbarui");
      await queryClient.invalidateQueries({
        queryKey: compensationPolicyKeys.all,
      });
    },
    onError: (error) => {
      toast.error(
        "Gagal memperbarui aturan kompensasi",
        error instanceof Error ? error.message : undefined,
      );
    },
  });

  return (
    <Card className="space-y-5 p-6 xl:col-span-2">
      <div className="flex items-start gap-3">
        <div className="rounded-md bg-error-light p-3 text-error">
          <Wallet className="h-5 w-5" />
        </div>
        <div>
          <h2 className="font-display text-[18px] font-[700] text-text-primary">
            Aturan Kompensasi & Jam Kerja
          </h2>
          <p className="mt-1 text-sm text-text-secondary">
            Gaji aman jika jam aktif bulan ini sudah mencapai target berjalan:
            target jam per bulan dibagi rata ke hari kerja Senin–Jumat, dihitung
            sejak awal bulan (atau tanggal bergabung) sampai kemarin; bulan lalu
            memakai target penuh. Jam di akhir pekan dan hari ini tetap dihitung.
            Part Time, Internship, Project Based, dan Outsourcing tidak
            dievaluasi.
          </p>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-1.5">
          <label
            className="text-[13px] font-[600] text-text-primary"
            htmlFor="monthly-base-salary"
          >
            Base Salary Bulanan Default (Rp)
          </label>
          <Input
            className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
            disabled={!canManage}
            id="monthly-base-salary"
            min={0}
            onChange={(event) =>
              setMonthlyBaseSalary(Number(event.target.value) || 0)
            }
            type="number"
            value={monthlyBaseSalary}
          />
          <p className="text-xs text-text-secondary">
            Dipakai hanya jika karyawan belum punya data gaji. Gaji per karyawan
            diambil dari data gaji masing-masing.
          </p>
        </div>

        <div className="space-y-1.5">
          <label
            className="text-[13px] font-[600] text-text-primary"
            htmlFor="policy-timezone"
          >
            Timezone
          </label>
          <select
            className="h-10 w-full rounded-sm border-[1.5px] border-transparent bg-surface-muted px-3 text-[14px] text-text-primary outline-none transition-all focus:border-[#4C9AFF] focus:bg-surface focus:shadow-focus"
            disabled={!canManage}
            id="policy-timezone"
            onChange={(event) => setTimezone(event.target.value)}
            value={timezone}
          >
            {timezoneOptions.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </div>

        <div className="space-y-1.5">
          <label
            className="text-[13px] font-[600] text-text-primary"
            htmlFor="min-hours-per-day"
          >
            Batas Hari Kerja Pendek (jam)
          </label>
          <Input
            className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
            disabled={!canManage}
            id="min-hours-per-day"
            max={24}
            min={0}
            onChange={(event) =>
              setMinHoursPerDay(Number(event.target.value) || 0)
            }
            step={0.5}
            type="number"
            value={minHoursPerDay}
          />
          <p className="text-xs text-text-secondary">
            Hari kerja dengan jam aktif di bawah angka ini ditandai sebagai
            peringatan. Tidak mengubah status aman/berisiko.
          </p>
        </div>

        <div className="space-y-1.5">
          <label
            className="text-[13px] font-[600] text-text-primary"
            htmlFor="min-hours-per-month"
          >
            Target Jam per Bulan
          </label>
          <Input
            className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
            disabled={!canManage}
            id="min-hours-per-month"
            min={0}
            onChange={(event) =>
              setMinHoursPerMonth(Number(event.target.value) || 0)
            }
            step={0.5}
            type="number"
            value={minHoursPerMonth}
          />
          <p className="text-xs text-text-secondary">
            Target sebulan penuh untuk karyawan Full Time. Untuk bulan berjalan
            dan karyawan baru, target dihitung proporsional per hari kerja.
          </p>
        </div>
      </div>

      <div className="flex justify-end">
        <Button
          disabled={
            !canManage || updateMutation.isPending || policyQuery.isLoading
          }
          onClick={() =>
            updateMutation.mutate({
              monthly_base_salary: monthlyBaseSalary,
              min_hours_per_day: minHoursPerDay,
              min_hours_per_month: minHoursPerMonth,
              timezone,
            })
          }
          type="button"
        >
          <Save className="h-4 w-4" />
          Simpan Aturan
        </Button>
      </div>
    </Card>
  );
}

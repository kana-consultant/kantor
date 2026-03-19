import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Building2,
  Calendar,
  Mail,
  MapPin,
  Pencil,
  Phone,
  Save,
  Shield,
  User,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/hooks/use-auth";
import { cn } from "@/lib/utils";
import { ensureAuthenticated } from "@/services/auth";
import { getProfile, profileKeys, updateProfile } from "@/services/profile";

export const Route = createFileRoute("/_authenticated/profile")({
  beforeLoad: async () => {
    const session = await ensureAuthenticated();
    if (!session) {
      throw new Error("Not authenticated");
    }
  },
  component: ProfilePage,
});

function ProfilePage() {
  const queryClient = useQueryClient();
  const { user, roles } = useAuth();
  const [isEditing, setIsEditing] = useState(false);

  const profileQuery = useQuery({
    queryKey: profileKeys.me,
    queryFn: getProfile,
  });

  const [form, setForm] = useState({
    full_name: "",
    phone: "",
    address: "",
    emergency_contact: "",
    avatar_url: "",
  });

  const mutation = useMutation({
    mutationFn: (values: typeof form) =>
      updateProfile({
        full_name: values.full_name,
        phone: values.phone || null,
        address: values.address || null,
        emergency_contact: values.emergency_contact || null,
        avatar_url: values.avatar_url || null,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: profileKeys.me });
      setIsEditing(false);
    },
  });

  const employee = profileQuery.data;

  function startEditing() {
    if (!employee) return;
    setForm({
      full_name: employee.full_name,
      phone: employee.phone ?? "",
      address: employee.address ?? "",
      emergency_contact: employee.emergency_contact ?? "",
      avatar_url: employee.avatar_url ?? "",
    });
    setIsEditing(true);
  }

  const statusLabel: Record<string, string> = {
    active: "Aktif",
    probation: "Probation",
    resigned: "Resign",
    terminated: "Terminated",
  };

  const statusColor: Record<string, string> = {
    active: "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400",
    probation: "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400",
    resigned: "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400",
    terminated: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  };

  if (profileQuery.isLoading) {
    return (
      <div className="flex items-center justify-center p-20">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-border border-t-primary" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Profil Saya</h1>
          <p className="text-sm text-text-tertiary">Kelola informasi pribadi Anda</p>
        </div>
        {!isEditing ? (
          <Button variant="outline" size="sm" onClick={startEditing}>
            <Pencil className="mr-2 h-4 w-4" />
            Edit Profil
          </Button>
        ) : (
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => setIsEditing(false)}>
              <X className="mr-2 h-4 w-4" />
              Batal
            </Button>
            <Button size="sm" onClick={() => mutation.mutate(form)} disabled={mutation.isPending}>
              <Save className="mr-2 h-4 w-4" />
              Simpan
            </Button>
          </div>
        )}
      </div>

      {mutation.isError && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
          Gagal menyimpan perubahan. Silakan coba lagi.
        </div>
      )}

      {/* Header card */}
      <div className="rounded-xl border bg-card p-6">
        <div className="flex items-center gap-4">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-primary/10 text-2xl font-bold text-primary">
            {(employee?.full_name ?? user?.full_name ?? "?")[0]?.toUpperCase()}
          </div>
          <div className="flex-1">
            {isEditing ? (
              <Input
                value={form.full_name}
                onChange={(e) => setForm((f) => ({ ...f, full_name: e.target.value }))}
                className="text-lg font-semibold"
                placeholder="Nama lengkap"
              />
            ) : (
              <h2 className="text-lg font-semibold text-text-primary">{employee?.full_name ?? user?.full_name}</h2>
            )}
            <p className="text-sm text-text-tertiary">{employee?.position ?? "Belum Ditentukan"}</p>
          </div>
          {employee && (
            <span className={cn("rounded-full px-3 py-1 text-xs font-medium", statusColor[employee.employment_status] ?? statusColor.active)}>
              {statusLabel[employee.employment_status] ?? employee.employment_status}
            </span>
          )}
        </div>
      </div>

      {/* Info grid */}
      <div className="grid gap-4 md:grid-cols-2">
        <InfoCard icon={Mail} label="Email" value={user?.email ?? "-"} />
        <InfoCard icon={Shield} label="Role" value={roles.join(", ") || "-"} />
        <InfoCard
          icon={Phone}
          label="Telepon"
          value={employee?.phone ?? "-"}
          editable={isEditing}
          editValue={form.phone}
          onEdit={(v) => setForm((f) => ({ ...f, phone: v }))}
          placeholder="+62..."
        />
        <InfoCard icon={Building2} label="Department" value={employee?.department ?? "-"} />
        <InfoCard
          icon={Calendar}
          label="Tanggal Bergabung"
          value={employee?.date_joined ? new Date(employee.date_joined).toLocaleDateString("id-ID", { year: "numeric", month: "long", day: "numeric" }) : "-"}
        />
        <InfoCard
          icon={User}
          label="Emergency Contact"
          value={employee?.emergency_contact ?? "-"}
          editable={isEditing}
          editValue={form.emergency_contact}
          onEdit={(v) => setForm((f) => ({ ...f, emergency_contact: v }))}
          placeholder="Nama - Nomor telepon"
        />
      </div>

      {/* Address */}
      <div className="rounded-xl border bg-card p-5">
        <div className="flex items-center gap-2 text-text-secondary">
          <MapPin className="h-4 w-4" />
          <span className="text-sm font-medium">Alamat</span>
        </div>
        {isEditing ? (
          <textarea
            className="mt-2 w-full rounded-lg border bg-surface-muted px-3 py-2 text-sm text-text-primary outline-none focus:border-primary focus:ring-2 focus:ring-primary/10"
            rows={3}
            value={form.address}
            onChange={(e) => setForm((f) => ({ ...f, address: e.target.value }))}
            placeholder="Alamat tempat tinggal"
          />
        ) : (
          <p className="mt-1 text-sm text-text-primary">{employee?.address || "-"}</p>
        )}
      </div>
    </div>
  );
}

function InfoCard({
  icon: Icon,
  label,
  value,
  editable,
  editValue,
  onEdit,
  placeholder,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  editable?: boolean;
  editValue?: string;
  onEdit?: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <div className="rounded-xl border bg-card p-4">
      <div className="flex items-center gap-2 text-text-secondary">
        <Icon className="h-4 w-4" />
        <span className="text-xs font-medium uppercase tracking-wide">{label}</span>
      </div>
      {editable && onEdit ? (
        <Input
          className="mt-2 text-sm"
          value={editValue ?? ""}
          onChange={(e) => onEdit(e.target.value)}
          placeholder={placeholder}
        />
      ) : (
        <p className="mt-1 text-sm font-medium text-text-primary">{value}</p>
      )}
    </div>
  );
}

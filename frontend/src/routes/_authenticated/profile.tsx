import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import {
  Building2,
  Calendar,
  KeyRound,
  Mail,
  MapPin,
  Pencil,
  Phone,
  Save,
  Shield,
  User,
  X,
} from "lucide-react";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import { changePassword, ensureAuthenticated, logout } from "@/services/auth";
import { getProfile, profileKeys, updateProfile } from "@/services/profile";
import { toast } from "@/stores/toast-store";

const changePasswordSchema = z
  .object({
    current_password: z.string().min(1, "Password saat ini wajib diisi"),
    new_password: z.string().min(8, "Password baru minimal 8 karakter"),
    confirm_password: z.string().min(1, "Konfirmasi password wajib diisi"),
  })
  .refine((values) => values.new_password === values.confirm_password, {
    message: "Konfirmasi password harus sama dengan password baru",
    path: ["confirm_password"],
  });

type ChangePasswordFormValues = z.infer<typeof changePasswordSchema>;

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
  const navigate = useNavigate();
  const { user, roleLabels, roleSummary } = useAuth();
  const [isEditing, setIsEditing] = useState(false);
  const [isPasswordModalOpen, setIsPasswordModalOpen] = useState(false);

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

  const {
    register: registerPasswordField,
    handleSubmit: submitPasswordForm,
    reset: resetPasswordForm,
    formState: { errors: passwordErrors },
  } = useForm<ChangePasswordFormValues>({
    resolver: zodResolver(changePasswordSchema),
    defaultValues: {
      current_password: "",
      new_password: "",
      confirm_password: "",
    },
  });

  const changePasswordMutation = useMutation({
    mutationFn: (values: ChangePasswordFormValues) =>
      changePassword({
        current_password: values.current_password,
        new_password: values.new_password,
      }),
    onSuccess: async () => {
      setIsPasswordModalOpen(false);
      resetPasswordForm();
      toast.success(
        "Password berhasil diubah",
        "Silakan login ulang. Semua sesi aktif sudah dicabut.",
      );
      await logout();
      await navigate({ to: "/login" });
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

  function closePasswordModal() {
    if (changePasswordMutation.isPending) {
      return;
    }

    setIsPasswordModalOpen(false);
    changePasswordMutation.reset();
    resetPasswordForm();
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
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => setIsPasswordModalOpen(true)}>
              <KeyRound className="mr-2 h-4 w-4" />
              Ganti Password
            </Button>
            <Button variant="outline" size="sm" onClick={startEditing}>
              <Pencil className="mr-2 h-4 w-4" />
              Edit Profil
            </Button>
          </div>
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
        <InfoCard icon={Shield} label="Role" value={roleLabels.join(", ") || roleSummary || "-"} />
        <InfoCard
          icon={Phone}
          label="Telepon"
          value={employee?.phone ? formatPhone(employee.phone) : "-"}
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

      <div className="rounded-xl border bg-card p-5">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <div className="flex items-center gap-2 text-text-secondary">
              <Shield className="h-4 w-4" />
              <span className="text-sm font-medium">Keamanan Akun</span>
            </div>
            <p className="mt-1 text-sm text-text-primary">
              Ganti password akun Anda. Setelah berhasil, Anda akan diminta login ulang.
            </p>
          </div>
          <Button onClick={() => setIsPasswordModalOpen(true)} type="button">
            <KeyRound className="mr-2 h-4 w-4" />
            Ganti Password
          </Button>
        </div>
      </div>

      <FormModal
        error={changePasswordMutation.error instanceof ApiError ? changePasswordMutation.error.message : null}
        isLoading={changePasswordMutation.isPending}
        isOpen={isPasswordModalOpen}
        onClose={closePasswordModal}
        onSubmit={submitPasswordForm((values) => changePasswordMutation.mutate(values))}
        size="md"
        submitLabel="Simpan Password Baru"
        subtitle="Gunakan password baru yang kuat. Sistem akan mencabut semua sesi aktif setelah perubahan berhasil."
        title="Ganti Password"
      >
        <div className="space-y-1.5">
          <label className="text-[13px] font-[600] text-text-primary" htmlFor="current_password">
            Password Saat Ini
          </label>
          <Input
            id="current_password"
            placeholder="Masukkan password saat ini"
            type="password"
            {...registerPasswordField("current_password")}
          />
          {passwordErrors.current_password ? (
            <p className="text-[12px] text-error">{passwordErrors.current_password.message}</p>
          ) : null}
        </div>

        <div className="space-y-1.5">
          <label className="text-[13px] font-[600] text-text-primary" htmlFor="new_password">
            Password Baru
          </label>
          <Input
            id="new_password"
            placeholder="Minimal 8 karakter"
            type="password"
            {...registerPasswordField("new_password")}
          />
          {passwordErrors.new_password ? (
            <p className="text-[12px] text-error">{passwordErrors.new_password.message}</p>
          ) : null}
        </div>

        <div className="space-y-1.5">
          <label className="text-[13px] font-[600] text-text-primary" htmlFor="confirm_password">
            Konfirmasi Password Baru
          </label>
          <Input
            id="confirm_password"
            placeholder="Ulangi password baru"
            type="password"
            {...registerPasswordField("confirm_password")}
          />
          {passwordErrors.confirm_password ? (
            <p className="text-[12px] text-error">{passwordErrors.confirm_password.message}</p>
          ) : null}
        </div>
      </FormModal>
    </div>
  );
}

function formatPhone(phone: string) {
  return phone.startsWith("+") ? phone : `+${phone}`;
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

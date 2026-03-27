import { useRef, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import {
  Building2,
  Calendar,
  Camera,
  CreditCard,
  KeyRound,
  Link2,
  Mail,
  MapPin,
  Pencil,
  Phone,
  Save,
  Shield,
  TerminalSquare,
  User,
  X,
} from "lucide-react";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import { changePassword, ensureAuthenticated, logout } from "@/services/auth";
import { changeEmail, getProfile, profileKeys, updateProfile, uploadProfileAvatar } from "@/services/profile";
import { toast } from "@/stores/toast-store";

const changePasswordSchema = z
  .object({
    current_password: z.string().min(1, "Kata sandi saat ini wajib diisi"),
    new_password: z.string().min(8, "Kata sandi baru minimal 8 karakter"),
    confirm_password: z.string().min(1, "Konfirmasi kata sandi wajib diisi"),
  })
  .refine((values) => values.new_password === values.confirm_password, {
    message: "Konfirmasi kata sandi harus sama dengan kata sandi baru",
    path: ["confirm_password"],
  });

type ChangePasswordFormValues = z.infer<typeof changePasswordSchema>;

const changeEmailSchema = z.object({
  email: z.string().email("Email tidak valid").min(1, "Email wajib diisi"),
  password: z.string().min(1, "Password wajib diisi untuk konfirmasi"),
});

type ChangeEmailFormValues = z.infer<typeof changeEmailSchema>;

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
  const [isEmailModalOpen, setIsEmailModalOpen] = useState(false);
  const avatarInputRef = useRef<HTMLInputElement>(null);

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
    bank_account_number: "",
    bank_name: "",
    linkedin_profile: "",
    ssh_keys: "",
  });

  const mutation = useMutation({
    mutationFn: (values: typeof form) =>
      updateProfile({
        full_name: values.full_name,
        phone: values.phone || null,
        address: values.address || null,
        emergency_contact: values.emergency_contact || null,
        avatar_url: values.avatar_url || null,
        bank_account_number: values.bank_account_number || null,
        bank_name: values.bank_name || null,
        linkedin_profile: values.linkedin_profile || null,
        ssh_keys: values.ssh_keys || null,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: profileKeys.me });
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

  const {
    register: registerEmailField,
    handleSubmit: submitEmailForm,
    reset: resetEmailForm,
    formState: { errors: emailErrors },
  } = useForm<ChangeEmailFormValues>({
    resolver: zodResolver(changeEmailSchema),
    defaultValues: { email: "", password: "" },
  });

  const changeEmailMutation = useMutation({
    mutationFn: (values: ChangeEmailFormValues) => changeEmail(values),
    onSuccess: async () => {
      setIsEmailModalOpen(false);
      resetEmailForm();
      toast.success("Email berhasil diubah");
      await queryClient.invalidateQueries({ queryKey: profileKeys.me });
      await queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
  });

  const avatarMutation = useMutation({
    mutationFn: (file: File) => uploadProfileAvatar(file),
    onSuccess: async () => {
      toast.success("Foto profil berhasil diubah");
      await queryClient.invalidateQueries({ queryKey: profileKeys.me });
      await queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "Gagal mengupload foto");
    },
  });

  function handleAvatarChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (file) {
      avatarMutation.mutate(file);
    }
    e.target.value = "";
  }

  function closeEmailModal() {
    if (changeEmailMutation.isPending) return;
    setIsEmailModalOpen(false);
    changeEmailMutation.reset();
    resetEmailForm();
  }

  const employee = profileQuery.data;

  function startEditing() {
    if (!employee) return;
    setForm({
      full_name: employee.full_name,
      phone: employee.phone ?? "",
      address: employee.address ?? "",
      emergency_contact: employee.emergency_contact ?? "",
      avatar_url: employee.avatar_url ?? "",
      bank_account_number: employee.bank_account_number ?? "",
      bank_name: employee.bank_name ?? "",
      linkedin_profile: employee.linkedin_profile ?? "",
      ssh_keys: employee.ssh_keys ?? "",
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
    <div className="mx-auto max-w-3xl space-y-6 px-4 py-4 sm:p-6">
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-text-primary">Profil Saya</h1>
          <p className="text-sm text-text-tertiary">Kelola informasi pribadi Anda</p>
        </div>
        {!isEditing ? (
          <div className="flex w-full md:w-auto">
            <Button className="w-full md:w-auto" variant="outline" size="sm" onClick={startEditing}>
              <Pencil className="mr-2 h-4 w-4" />
              Edit Profil
            </Button>
          </div>
        ) : (
          <div className="flex w-full flex-col gap-2 sm:flex-row md:w-auto">
            <Button className="w-full sm:w-auto" variant="outline" size="sm" onClick={() => setIsEditing(false)}>
              <X className="mr-2 h-4 w-4" />
              Batal
            </Button>
            <Button className="w-full sm:w-auto" size="sm" onClick={() => mutation.mutate(form)} disabled={mutation.isPending}>
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
          <div className="group relative">
            <ProtectedAvatar
              alt={employee?.full_name ?? user?.full_name ?? "Profil"}
              avatarUrl={employee?.avatar_url ?? user?.avatar_url}
              className="h-16 w-16 border border-border/70"
            />
            <button
              type="button"
              className="absolute inset-0 flex items-center justify-center rounded-full bg-black/50 opacity-0 transition-opacity group-hover:opacity-100"
              onClick={() => avatarInputRef.current?.click()}
              disabled={avatarMutation.isPending}
              title="Ganti foto profil"
            >
              <Camera className="h-5 w-5 text-white" />
            </button>
            <input
              ref={avatarInputRef}
              type="file"
              accept="image/*"
              className="hidden"
              onChange={handleAvatarChange}
            />
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
        <div className="cursor-pointer" onClick={() => setIsEmailModalOpen(true)} title="Klik untuk ganti email">
          <InfoCard icon={Mail} label="Email" value={user?.email ?? "-"} />
        </div>
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
          icon={CreditCard}
          label="Nomor Rekening"
          value={employee?.bank_account_number ?? "-"}
          editable={isEditing}
          editValue={form.bank_account_number}
          onEdit={(v) => setForm((f) => ({ ...f, bank_account_number: v }))}
          placeholder="Nomor rekening atau akun e-wallet"
        />
        <InfoCard
          icon={Building2}
          label="Bank / E-Wallet"
          value={employee?.bank_name ?? "-"}
          editable={isEditing}
          editValue={form.bank_name}
          onEdit={(v) => setForm((f) => ({ ...f, bank_name: v }))}
          placeholder="BCA, BRI, OVO, GoPay, DANA"
        />
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
        <InfoCard
          icon={Link2}
          label="LinkedIn Profile"
          value={employee?.linkedin_profile ?? "-"}
          editable={isEditing}
          editValue={form.linkedin_profile}
          onEdit={(v) => setForm((f) => ({ ...f, linkedin_profile: v }))}
          placeholder="https://linkedin.com/in/username"
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
        <div className="flex items-center gap-2 text-text-secondary">
          <TerminalSquare className="h-4 w-4" />
          <span className="text-sm font-medium">SSH Keys</span>
        </div>
        {isEditing ? (
          <textarea
            className="mt-2 w-full rounded-lg border bg-surface-muted px-3 py-2 font-mono text-sm text-text-primary outline-none focus:border-primary focus:ring-2 focus:ring-primary/10"
            rows={6}
            value={form.ssh_keys}
            onChange={(e) => setForm((f) => ({ ...f, ssh_keys: e.target.value }))}
            placeholder="Tempel public key SSH. Jika lebih dari satu, pisahkan per baris."
          />
        ) : (
          <pre className="mt-2 whitespace-pre-wrap break-all rounded-lg bg-surface-muted px-3 py-2 font-mono text-xs text-text-primary">
            {employee?.ssh_keys || "-"}
          </pre>
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
          <Button className="w-full md:w-auto" onClick={() => setIsPasswordModalOpen(true)} type="button">
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
            Password Saat Ini<span className="ml-0.5 text-priority-high">*</span>
          </label>
          <Input
            id="current_password"
            autoComplete="current-password"
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
            Password Baru<span className="ml-0.5 text-priority-high">*</span>
          </label>
          <Input
            id="new_password"
            autoComplete="new-password"
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
            Konfirmasi Password Baru<span className="ml-0.5 text-priority-high">*</span>
          </label>
          <Input
            id="confirm_password"
            autoComplete="new-password"
            placeholder="Ulangi password baru"
            type="password"
            {...registerPasswordField("confirm_password")}
          />
          {passwordErrors.confirm_password ? (
            <p className="text-[12px] text-error">{passwordErrors.confirm_password.message}</p>
          ) : null}
        </div>
      </FormModal>

      <FormModal
        error={changeEmailMutation.error instanceof ApiError ? changeEmailMutation.error.message : null}
        isLoading={changeEmailMutation.isPending}
        isOpen={isEmailModalOpen}
        onClose={closeEmailModal}
        onSubmit={submitEmailForm((values) => changeEmailMutation.mutate(values))}
        size="md"
        submitLabel="Simpan Email Baru"
        subtitle="Masukkan email baru dan konfirmasi dengan password Anda."
        title="Ganti Email"
      >
        <div className="space-y-1.5">
          <label className="text-[13px] font-[600] text-text-primary" htmlFor="change_email">
            Email Baru<span className="ml-0.5 text-priority-high">*</span>
          </label>
          <Input
            id="change_email"
            autoComplete="email"
            placeholder="email@contoh.com"
            type="email"
            {...registerEmailField("email")}
          />
          {emailErrors.email ? (
            <p className="text-[12px] text-error">{emailErrors.email.message}</p>
          ) : null}
        </div>

        <div className="space-y-1.5">
          <label className="text-[13px] font-[600] text-text-primary" htmlFor="email_confirm_password">
            Password<span className="ml-0.5 text-priority-high">*</span>
          </label>
          <Input
            id="email_confirm_password"
            autoComplete="current-password"
            placeholder="Masukkan password untuk konfirmasi"
            type="password"
            {...registerEmailField("password")}
          />
          {emailErrors.password ? (
            <p className="text-[12px] text-error">{emailErrors.password.message}</p>
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

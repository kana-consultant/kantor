import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api-client";
import { getAuthPublicOptions, resetPassword, validateResetPasswordToken } from "@/services/auth";
import { toast } from "@/stores/toast-store";

const searchSchema = z.object({
  token: z.string().trim().min(1, "Token reset wajib ada"),
});

const resetPasswordSchema = z
  .object({
    new_password: z.string().min(8, "Kata sandi minimal 8 karakter"),
    confirmPassword: z.string().min(8, "Konfirmasi kata sandi wajib diisi"),
  })
  .refine((value) => value.new_password === value.confirmPassword, {
    message: "Konfirmasi kata sandi tidak sama",
    path: ["confirmPassword"],
  });

type ResetPasswordFormValues = z.infer<typeof resetPasswordSchema>;

export const Route = createFileRoute("/reset-password")({
  validateSearch: searchSchema,
  component: ResetPasswordPage,
});

function ResetPasswordPage() {
  const navigate = useNavigate();
  const { token } = Route.useSearch();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<ResetPasswordFormValues>({
    resolver: zodResolver(resetPasswordSchema),
    defaultValues: {
      new_password: "",
      confirmPassword: "",
    },
  });

  const tokenValidationQuery = useQuery({
    queryKey: ["auth", "reset-password", "validate", token],
    queryFn: () => validateResetPasswordToken(token),
    retry: false,
  });
  const authPublicOptionsQuery = useQuery({
    queryKey: ["auth", "public-options"],
    queryFn: getAuthPublicOptions,
    retry: false,
    staleTime: 60_000,
  });

  const resetPasswordMutation = useMutation({
    mutationFn: (values: ResetPasswordFormValues) =>
      resetPassword({
        token,
        new_password: values.new_password,
      }),
    onSuccess: (result) => {
      toast.success("Kata sandi berhasil diatur ulang", result.message);
      void navigate({ to: "/login", replace: true });
    },
  });

  const submitForm = handleSubmit((values) => {
    resetPasswordMutation.mutate(values);
  });

  const invalidTokenError = tokenValidationQuery.error instanceof ApiError
    ? tokenValidationQuery.error.message
    : resetPasswordMutation.error instanceof ApiError
      ? resetPasswordMutation.error.message
      : null;

  return (
    <div className="relative flex min-h-screen w-full items-center justify-center overflow-hidden bg-background p-4">
      <div className="pointer-events-none absolute inset-0 opacity-40">
        <div className="absolute left-0 top-0 h-1/2 w-1/2 bg-[radial-gradient(ellipse_at_top_left,_#DEEBFF,_transparent_70%)]"></div>
        <div className="absolute bottom-0 right-0 h-1/2 w-1/2 bg-[radial-gradient(ellipse_at_bottom_right,_#FFEBE6,_transparent_70%)]"></div>
        <div className="absolute bottom-0 left-1/4 h-1/2 w-1/2 bg-[radial-gradient(ellipse_at_bottom_center,_#EAE6FF,_transparent_70%)]"></div>
      </div>

      <Card className="relative z-10 w-full max-w-[440px] rounded-[16px] border-border bg-surface p-10 shadow-xl">
        <div className="mb-8 space-y-2 text-center">
          <div className="flex flex-col items-center justify-center">
            <div className="flex items-start font-display text-[24px] font-[800] leading-none tracking-tight text-text-primary">
              K<span className="relative">A<span className="absolute -top-1 left-1/2 h-1.5 w-1.5 -translate-x-1/2 rounded-full bg-ops"></span></span>NTOR
            </div>
          </div>
          <h1 className="text-[24px] font-[700] text-text-primary">Reset kata sandi</h1>
          <p className="text-[14px] leading-relaxed text-text-secondary">
            Gunakan link ini untuk membuat kata sandi baru. Link reset hanya berlaku sekali pakai.
          </p>
        </div>

        {tokenValidationQuery.isLoading ? (
          <div className="rounded-[12px] border border-border bg-surface-muted px-4 py-8 text-center text-[14px] text-text-secondary">
            Memverifikasi link reset...
          </div>
        ) : invalidTokenError ? (
          <div className="space-y-4">
            <div className="rounded-[12px] border border-error/20 bg-error-light px-4 py-4 text-[14px] text-error">
              {invalidTokenError}
            </div>
            <div className="flex flex-col gap-3 sm:flex-row">
              <Button asChild className="h-[44px] flex-1 rounded-[8px] bg-ops text-[14px] font-[600] text-white hover:bg-ops-dark">
                <Link to={authPublicOptionsQuery.data?.forgot_password_enabled ? "/forgot-password" : "/login"}>
                  {authPublicOptionsQuery.data?.forgot_password_enabled ? "Minta link baru" : "Kembali ke login"}
                </Link>
              </Button>
              <Button asChild variant="outline" className="h-[44px] flex-1 rounded-[8px]">
                <Link to="/login">Kembali ke login</Link>
              </Button>
            </div>
          </div>
        ) : (
          <form className="space-y-4" onSubmit={submitForm}>
            <div className="space-y-1.5">
              <label className="text-[13px] font-[600] text-text-primary" htmlFor="new_password">
                Kata sandi baru<span className="ml-0.5 text-priority-high">*</span>
              </label>
              <Input
                id="new_password"
                autoComplete="new-password"
                placeholder="Masukkan kata sandi baru"
                type="password"
                className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
                {...register("new_password")}
              />
              {errors.new_password ? (
                <p className="mt-1 text-[12px] text-error">{errors.new_password.message}</p>
              ) : null}
            </div>

            <div className="space-y-1.5">
              <label className="text-[13px] font-[600] text-text-primary" htmlFor="confirm_password">
                Konfirmasi kata sandi<span className="ml-0.5 text-priority-high">*</span>
              </label>
              <Input
                id="confirm_password"
                autoComplete="new-password"
                placeholder="Ulangi kata sandi baru"
                type="password"
                className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
                {...register("confirmPassword")}
              />
              {errors.confirmPassword ? (
                <p className="mt-1 text-[12px] text-error">{errors.confirmPassword.message}</p>
              ) : null}
            </div>

            {resetPasswordMutation.error instanceof ApiError ? (
              <div className="rounded-[8px] border border-error/20 bg-error-light px-4 py-3 text-[14px] text-error">
                {resetPasswordMutation.error.message}
              </div>
            ) : null}

            <div className="pt-2">
              <Button
                className="h-[44px] w-full rounded-[8px] bg-ops text-[14px] font-[600] text-white transition-transform hover:bg-ops-dark active:scale-[0.98]"
                disabled={resetPasswordMutation.isPending}
                type="submit"
              >
                {resetPasswordMutation.isPending ? "Memproses..." : "Simpan kata sandi baru"}
              </Button>
            </div>
          </form>
        )}

        <div className="mt-6 text-center">
          <p className="text-[14px] text-text-secondary">
            Kembali ke{" "}
            <Link className="font-[600] text-ops hover:underline" to="/login">
              halaman login
            </Link>
          </p>
        </div>
      </Card>
    </div>
  );
}

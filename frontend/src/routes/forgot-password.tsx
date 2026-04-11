import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, redirect } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api-client";
import { getDefaultAuthorizedPath } from "@/lib/rbac";
import { forgotPassword, getAuthPublicOptions, getValidStoredSession } from "@/services/auth";

const forgotPasswordSchema = z.object({
  email: z.email("Email wajib valid"),
});

type ForgotPasswordFormValues = z.infer<typeof forgotPasswordSchema>;

export const Route = createFileRoute("/forgot-password")({
  beforeLoad: async () => {
    const session = getValidStoredSession();
    if (session?.tokens.access_token) {
      throw redirect({
        to: getDefaultAuthorizedPath(session),
      });
    }
  },
  component: ForgotPasswordPage,
});

function ForgotPasswordPage() {
  const authPublicOptionsQuery = useQuery({
    queryKey: ["auth", "public-options"],
    queryFn: getAuthPublicOptions,
    retry: false,
    staleTime: 60_000,
  });
  const {
    register,
    handleSubmit,
    formState: { errors },
    watch,
  } = useForm<ForgotPasswordFormValues>({
    resolver: zodResolver(forgotPasswordSchema),
    defaultValues: {
      email: "",
    },
  });

  const forgotPasswordMutation = useMutation({
    mutationFn: forgotPassword,
  });

  const submitForm = handleSubmit((values) => {
    forgotPasswordMutation.mutate(values);
  });

  const submitted = forgotPasswordMutation.isSuccess;
  const emailValue = watch("email");
  const isForgotPasswordEnabled = authPublicOptionsQuery.data?.forgot_password_enabled ?? false;

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
          <h1 className="text-[24px] font-[700] text-text-primary">Lupa kata sandi</h1>
          <p className="text-[14px] leading-relaxed text-text-secondary">
            Masukkan email akun Anda. Jika email ditemukan pada tenant ini, kami akan mengirim link reset kata sandi.
          </p>
        </div>

        {authPublicOptionsQuery.isLoading ? (
          <div className="rounded-[12px] border border-border bg-surface-muted px-4 py-8 text-center text-[14px] text-text-secondary">
            Memuat konfigurasi tenant...
          </div>
        ) : !isForgotPasswordEnabled ? (
          <div className="space-y-4">
            <div className="rounded-[12px] border border-warning/30 bg-warning-light px-4 py-4 text-[14px] text-text-primary">
              Tenant ini belum mengaktifkan reset kata sandi via email.
            </div>
            <Button asChild className="h-[44px] w-full rounded-[8px] bg-ops text-[14px] font-[600] text-white hover:bg-ops-dark">
              <Link to="/login">Kembali ke login</Link>
            </Button>
          </div>
        ) : submitted ? (
          <div className="space-y-4">
            <div className="rounded-[12px] border border-success/20 bg-success/10 px-4 py-4 text-[14px] text-text-primary">
              Link reset telah diproses untuk <span className="font-semibold">{emailValue}</span> jika akun tersebut ada pada tenant ini.
            </div>
            <div className="flex flex-col gap-3 sm:flex-row">
              <Button asChild className="h-[44px] flex-1 rounded-[8px] bg-ops text-[14px] font-[600] text-white hover:bg-ops-dark">
                <Link to="/login">Kembali ke login</Link>
              </Button>
              <Button
                variant="outline"
                className="h-[44px] flex-1 rounded-[8px]"
                onClick={() => forgotPasswordMutation.reset()}
                type="button"
              >
                Kirim ulang
              </Button>
            </div>
          </div>
        ) : (
          <form className="space-y-4" onSubmit={submitForm}>
            <div className="space-y-1.5">
              <label className="text-[13px] font-[600] text-text-primary" htmlFor="email">
                Email<span className="ml-0.5 text-priority-high">*</span>
              </label>
              <Input
                id="email"
                autoComplete="email"
                placeholder="nama@company.com"
                type="email"
                className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
                {...register("email")}
              />
              {errors.email ? (
                <p className="mt-1 text-[12px] text-error">{errors.email.message}</p>
              ) : null}
            </div>

            {forgotPasswordMutation.error instanceof ApiError ? (
              <div className="rounded-[8px] border border-error/20 bg-error-light px-4 py-3 text-[14px] text-error">
                {forgotPasswordMutation.error.message}
              </div>
            ) : null}

            <div className="pt-2">
              <Button
                className="h-[44px] w-full rounded-[8px] bg-ops text-[14px] font-[600] text-white transition-transform hover:bg-ops-dark active:scale-[0.98]"
                disabled={forgotPasswordMutation.isPending}
                type="submit"
              >
                {forgotPasswordMutation.isPending ? "Memproses..." : "Kirim link reset"}
              </Button>
            </div>
          </form>
        )}

        <div className="mt-6 text-center">
          <p className="text-[14px] text-text-secondary">
            Ingat kata sandi Anda?{" "}
            <Link className="font-[600] text-ops hover:underline" to="/login">
              Masuk
            </Link>
          </p>
        </div>
      </Card>
    </div>
  );
}

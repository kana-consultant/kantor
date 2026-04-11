import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, redirect } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api-client";
import { getDefaultAuthorizedPath } from "@/lib/rbac";
import { getAuthPublicOptions, getValidStoredSession, login } from "@/services/auth";
import { useAuthStore } from "@/stores/auth-store";

const loginSchema = z.object({
  email: z.email("Email wajib valid"),
  password: z.string().min(8, "Kata sandi minimal 8 karakter"),
});

type LoginFormValues = z.infer<typeof loginSchema>;

export const Route = createFileRoute("/login")({
  beforeLoad: async () => {
    const session = getValidStoredSession();
    if (session?.tokens.access_token) {
      throw redirect({
        to: getDefaultAuthorizedPath(session),
      });
    }
  },
  component: LoginPage,
});

function LoginPage() {
  const setSession = useAuthStore((state) => state.setSession);
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
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      email: "",
      password: "",
    },
  });

  const loginMutation = useMutation({
    mutationFn: login,
    onSuccess: (session) => {
      setSession(session);
      window.location.replace(getDefaultAuthorizedPath(session));
    },
  });

  const submitForm = handleSubmit((values) => {
    loginMutation.mutate(values);
  });

  return (
    <div className="relative flex min-h-screen w-full items-center justify-center overflow-hidden bg-background p-4">
      <div className="pointer-events-none absolute inset-0 opacity-40">
        <div className="absolute left-0 top-0 h-1/2 w-1/2 bg-[radial-gradient(ellipse_at_top_left,_#DEEBFF,_transparent_70%)]"></div>
        <div className="absolute bottom-0 right-0 h-1/2 w-1/2 bg-[radial-gradient(ellipse_at_bottom_right,_#FFEBE6,_transparent_70%)]"></div>
        <div className="absolute bottom-0 left-1/4 h-1/2 w-1/2 bg-[radial-gradient(ellipse_at_bottom_center,_#EAE6FF,_transparent_70%)]"></div>
      </div>

      <Card className="relative z-10 w-full max-w-[420px] rounded-[16px] border-border bg-surface p-10 shadow-xl">
        <div className="mb-8 space-y-2 text-center">
          <div className="flex flex-col items-center justify-center">
            <div className="flex items-start font-display text-[24px] font-[800] leading-none tracking-tight text-text-primary">
              K<span className="relative">A<span className="absolute -top-1 left-1/2 h-1.5 w-1.5 -translate-x-1/2 rounded-full bg-ops"></span></span>NTOR
            </div>
          </div>
          <p className="text-[12px] font-[500] leading-tight text-text-secondary">
            KanA Intelligence Operational Dashboard
          </p>
        </div>

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

          <div className="space-y-1.5">
            <label className="text-[13px] font-[600] text-text-primary" htmlFor="password">
              Kata Sandi<span className="ml-0.5 text-priority-high">*</span>
            </label>
            <Input
              id="password"
              autoComplete="current-password"
              placeholder="Masukkan kata sandi"
              type="password"
              className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
              {...register("password")}
            />
            {errors.password ? (
              <p className="mt-1 text-[12px] text-error">{errors.password.message}</p>
            ) : null}
            {authPublicOptionsQuery.data?.forgot_password_enabled ? (
              <div className="flex justify-end pt-1">
                <Link className="text-[12px] font-[600] text-ops hover:underline" to="/forgot-password">
                  Lupa kata sandi?
                </Link>
              </div>
            ) : null}
          </div>

          {loginMutation.error instanceof ApiError ? (
            <div className="rounded-[8px] border border-error/20 bg-error-light px-4 py-3 text-[14px] text-error">
              {loginMutation.error.message}
            </div>
          ) : null}

          <div className="pt-2">
            <Button
              className="h-[44px] w-full rounded-[8px] bg-ops text-[14px] font-[600] text-white transition-transform hover:bg-ops-dark active:scale-[0.98]"
              disabled={loginMutation.isPending}
              type="submit"
            >
              {loginMutation.isPending ? "Memproses..." : "Masuk"}
            </Button>
          </div>
        </form>

        <div className="mt-6 text-center">
          <p className="text-[14px] text-text-secondary">
            Belum punya akun?{" "}
            <Link className="font-[600] text-ops hover:underline" to="/register">
              Daftar
            </Link>
          </p>
        </div>
      </Card>
    </div>
  );
}

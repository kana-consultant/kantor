import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { Link, createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { LockKeyhole, ShieldCheck } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api-client";
import { ensureAuthenticated, login } from "@/services/auth";
import { useAuthStore } from "@/stores/auth-store";

const loginSchema = z.object({
  email: z.email("Email must be valid"),
  password: z.string().min(8, "Password must contain at least 8 characters"),
});

type LoginFormValues = z.infer<typeof loginSchema>;

export const Route = createFileRoute("/login")({
  beforeLoad: async () => {
    const session = await ensureAuthenticated();
    if (session?.tokens.access_token) {
      throw redirect({
        to: "/operational",
      });
    }
  },
  component: LoginPage,
});

function LoginPage() {
  const navigate = useNavigate();
  const setSession = useAuthStore((state) => state.setSession);
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
      void navigate({ to: "/operational" });
    },
  });

  const submitForm = handleSubmit((values) => {
    loginMutation.mutate(values);
  });

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-6xl items-center px-4 py-10 lg:px-6">
      <div className="grid w-full gap-6 lg:grid-cols-[1.15fr_0.85fr]">
        <Card className="overflow-hidden p-8 lg:p-10">
          <div className="space-y-6">
            <div className="inline-flex items-center gap-3 rounded-full border border-border bg-background/70 px-4 py-2">
              <ShieldCheck className="h-4 w-4 text-primary" />
              <span className="text-sm font-medium">Internal Company Platform</span>
            </div>

            <div className="space-y-4">
              <p className="text-sm uppercase tracking-[0.32em] text-muted-foreground">
                Unified operations cockpit
              </p>
              <h1 className="max-w-xl text-4xl font-bold leading-tight lg:text-5xl">
                Operasional, HRIS, dan marketing berada dalam satu workspace.
              </h1>
              <p className="max-w-2xl text-base text-muted-foreground">
                Login dengan akun superadmin yang disuntik saat startup backend,
                lalu gunakan dashboard verification untuk mengetes health,
                protected route, refresh token, dan RBAC langsung dari UI.
              </p>
            </div>

            <div className="grid gap-4 md:grid-cols-3">
              {[
                "Akun superadmin dev di-seed otomatis saat startup",
                "Ada halaman register untuk uji role default user berikutnya",
                "Dashboard verification menjalankan health dan endpoint terlindungi",
              ].map((item) => (
                <div
                  className="rounded-3xl border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground"
                  key={item}
                >
                  {item}
                </div>
              ))}
            </div>
          </div>
        </Card>

        <Card className="p-8 lg:p-10">
          <div className="mb-8 flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
              <LockKeyhole className="h-5 w-5" />
            </div>
            <div>
              <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Secure Access
              </p>
              <h2 className="text-2xl font-bold">Login</h2>
            </div>
          </div>

          <form className="space-y-4" onSubmit={submitForm}>
            <div className="space-y-2">
              <label className="text-sm font-medium" htmlFor="email">
                Email
              </label>
              <Input
                id="email"
                placeholder="you@company.com"
                type="email"
                {...register("email")}
              />
              {errors.email ? (
                <p className="text-sm text-red-600">{errors.email.message}</p>
              ) : null}
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium" htmlFor="password">
                Password
              </label>
              <Input
                id="password"
                placeholder="Enter password"
                type="password"
                {...register("password")}
              />
              {errors.password ? (
                <p className="text-sm text-red-600">{errors.password.message}</p>
              ) : null}
            </div>
            {loginMutation.error instanceof ApiError ? (
              <div className="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                {loginMutation.error.message}
              </div>
            ) : null}
            <Button className="w-full" disabled={loginMutation.isPending} type="submit">
              {loginMutation.isPending ? "Signing in..." : "Continue to dashboard"}
            </Button>
          </form>
          <div className="mt-6 space-y-3 text-sm text-muted-foreground">
            <p>Gunakan kredensial seed superadmin yang didefinisikan di file `.env`.</p>
            <p>
              Perlu user tambahan untuk uji role bawaan?{" "}
              <Link className="font-semibold text-foreground underline-offset-4 hover:underline" to="/register">
                Buka register
              </Link>
            </p>
          </div>
        </Card>
      </div>
    </div>
  );
}

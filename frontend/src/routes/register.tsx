import type { ReactNode } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { Link, createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { ShieldPlus } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api-client";
import { ensureAuthenticated, register } from "@/services/auth";
import { useAuthStore } from "@/stores/auth-store";

const registerSchema = z
  .object({
    full_name: z.string().min(3, "Full name must contain at least 3 characters"),
    email: z.email("Email must be valid"),
    password: z.string().min(8, "Password must contain at least 8 characters"),
    confirmPassword: z.string().min(8, "Please confirm the password"),
  })
  .refine((value) => value.password === value.confirmPassword, {
    message: "Password confirmation does not match",
    path: ["confirmPassword"],
  });

type RegisterFormValues = z.infer<typeof registerSchema>;

export const Route = createFileRoute("/register")({
  beforeLoad: async () => {
    const session = await ensureAuthenticated();
    if (session?.tokens.access_token) {
      throw redirect({
        to: "/operational",
      });
    }
  },
  component: RegisterPage,
});

function RegisterPage() {
  const navigate = useNavigate();
  const setSession = useAuthStore((state) => state.setSession);
  const {
    register: registerField,
    handleSubmit,
    formState: { errors },
  } = useForm<RegisterFormValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: {
      full_name: "",
      email: "",
      password: "",
      confirmPassword: "",
    },
  });

  const registerMutation = useMutation({
    mutationFn: async (values: RegisterFormValues) =>
      register({
        email: values.email,
        password: values.password,
        full_name: values.full_name,
      }),
    onSuccess: (session) => {
      setSession(session);
      void navigate({ to: "/operational" });
    },
  });

  const submitForm = handleSubmit((values) => {
    registerMutation.mutate(values);
  });

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-6xl items-center px-4 py-10 lg:px-6">
      <div className="grid w-full gap-6 lg:grid-cols-[1fr_0.9fr]">
        <Card className="overflow-hidden p-8 lg:p-10">
          <div className="space-y-5">
            <p className="text-sm uppercase tracking-[0.32em] text-muted-foreground">
              Self-service testing
            </p>
            <h1 className="max-w-xl text-4xl font-bold leading-tight lg:text-5xl">
              Register user tambahan langsung dari UI untuk cek role default.
            </h1>
            <p className="max-w-2xl text-base text-muted-foreground">
              Akun pertama sudah disiapkan lewat seed sebagai superadmin. Halaman
              ini dipakai untuk menguji flow register user berikutnya tanpa
              Postman.
            </p>
            <div className="rounded-3xl border border-border/70 bg-background/70 p-5 text-sm text-muted-foreground">
              Setelah submit berhasil, user baru otomatis login dan diarahkan ke
              dashboard verification.
            </div>
          </div>
        </Card>

        <Card className="p-8 lg:p-10">
          <div className="mb-8 flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
              <ShieldPlus className="h-5 w-5" />
            </div>
            <div>
              <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Create account
              </p>
              <h2 className="text-2xl font-bold">Register</h2>
            </div>
          </div>

          <form className="space-y-4" onSubmit={submitForm}>
            <Field label="Full name" error={errors.full_name?.message}>
              <Input {...registerField("full_name")} placeholder="Jane Doe" />
            </Field>
            <Field label="Email" error={errors.email?.message}>
              <Input {...registerField("email")} placeholder="jane@company.com" type="email" />
            </Field>
            <Field label="Password" error={errors.password?.message}>
              <Input {...registerField("password")} placeholder="Create password" type="password" />
            </Field>
            <Field label="Confirm password" error={errors.confirmPassword?.message}>
              <Input
                {...registerField("confirmPassword")}
                placeholder="Repeat password"
                type="password"
              />
            </Field>
            {registerMutation.error instanceof ApiError ? (
              <div className="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
                {registerMutation.error.message}
              </div>
            ) : null}
            <Button className="w-full" disabled={registerMutation.isPending} type="submit">
              {registerMutation.isPending ? "Creating account..." : "Register and continue"}
            </Button>
          </form>

          <p className="mt-6 text-sm text-muted-foreground">
            Sudah punya akun seed?{" "}
            <Link className="font-semibold text-foreground underline-offset-4 hover:underline" to="/login">
              Login di sini
            </Link>
          </p>
        </Card>
      </div>
    </div>
  );
}

interface FieldProps {
  label: string;
  error?: string;
  children: ReactNode;
}

function Field({ label, error, children }: FieldProps) {
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">{label}</label>
      {children}
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
    </div>
  );
}

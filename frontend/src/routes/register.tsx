import type { ReactNode } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { Link, createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api-client";
import { getValidStoredSession, register } from "@/services/auth";
import { toast } from "@/stores/toast-store";

const registerSchema = z
  .object({
    full_name: z.string().min(3, "Nama lengkap minimal 3 karakter"),
    email: z.email("Email wajib valid"),
    password: z.string().min(8, "Kata sandi minimal 8 karakter"),
    confirmPassword: z.string().min(8, "Konfirmasi kata sandi wajib diisi"),
  })
  .refine((value) => value.password === value.confirmPassword, {
    message: "Konfirmasi kata sandi tidak sama",
    path: ["confirmPassword"],
  });

type RegisterFormValues = z.infer<typeof registerSchema>;

export const Route = createFileRoute("/register")({
  beforeLoad: async () => {
    const session = getValidStoredSession();
    if (session?.tokens.access_token) {
      throw redirect({
        to: "/operational/overview",
      });
    }
  },
  component: RegisterPage,
});

function RegisterPage() {
  const navigate = useNavigate();
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
    onSuccess: () => {
      toast.success("Akun berhasil dibuat", "Silakan masuk dengan akun yang baru Anda daftarkan.");
      void navigate({ to: "/login", replace: true });
    },
  });

  const submitForm = handleSubmit((values) => {
    registerMutation.mutate(values);
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
          <Field label="Nama Lengkap" error={errors.full_name?.message} required>
            <Input
              {...registerField("full_name")}
              autoComplete="name"
              placeholder="Nama lengkap Anda"
              className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
            />
          </Field>
          <Field label="Email" error={errors.email?.message} required>
            <Input
              {...registerField("email")}
              autoComplete="email"
              placeholder="nama@company.com"
              type="email"
              className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
            />
          </Field>
          <Field label="Kata Sandi" error={errors.password?.message} required>
            <Input
              {...registerField("password")}
              autoComplete="new-password"
              placeholder="Buat kata sandi"
              type="password"
              className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
            />
          </Field>
          <Field label="Konfirmasi Kata Sandi" error={errors.confirmPassword?.message} required>
            <Input
              {...registerField("confirmPassword")}
              autoComplete="new-password"
              placeholder="Ulangi kata sandi"
              type="password"
              className="h-10 rounded-[6px] border-transparent bg-surface-muted px-3 text-[14px] focus:border-ops focus:bg-surface focus:ring-2 focus:ring-ops/20"
            />
          </Field>

          {registerMutation.error instanceof ApiError ? (
            <div className="rounded-[8px] border border-error/20 bg-error-light px-4 py-3 text-[14px] text-error">
              {registerMutation.error.message}
            </div>
          ) : null}

          <div className="pt-2">
            <Button
              className="h-[44px] w-full rounded-[8px] bg-ops text-[14px] font-[600] text-white transition-transform hover:bg-ops-dark active:scale-[0.98]"
              disabled={registerMutation.isPending}
              type="submit"
            >
              {registerMutation.isPending ? "Memproses..." : "Buat Akun"}
            </Button>
          </div>
        </form>

        <div className="mt-6 text-center">
          <p className="text-[14px] text-text-secondary">
            Sudah punya akun?{" "}
            <Link className="font-[600] text-ops hover:underline" to="/login">
              Masuk
            </Link>
          </p>
        </div>
      </Card>
    </div>
  );
}

interface FieldProps {
  label: string;
  error?: string;
  required?: boolean;
  children: ReactNode;
}

function Field({ label, error, required, children }: FieldProps) {
  return (
    <div className="space-y-1.5">
      <label className="text-[13px] font-[600] text-text-primary">
        {label}
        {required ? <span className="ml-0.5 text-priority-high">*</span> : null}
      </label>
      {children}
      {error ? <p className="mt-1 text-[12px] text-error">{error}</p> : null}
    </div>
  );
}

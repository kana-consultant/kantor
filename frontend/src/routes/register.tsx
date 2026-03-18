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
    <div className="relative flex min-h-screen w-full items-center justify-center p-4 overflow-hidden bg-background">
      {/* Background Gradient Mesh */}
      <div className="absolute inset-0 pointer-events-none opacity-40">
        <div className="absolute top-0 left-0 w-1/2 h-1/2 bg-[radial-gradient(ellipse_at_top_left,_#DEEBFF,_transparent_70%)]"></div>
        <div className="absolute bottom-0 right-0 w-1/2 h-1/2 bg-[radial-gradient(ellipse_at_bottom_right,_#FFEBE6,_transparent_70%)]"></div>
        <div className="absolute bottom-0 left-1/4 w-1/2 h-1/2 bg-[radial-gradient(ellipse_at_bottom_center,_#EAE6FF,_transparent_70%)]"></div>
      </div>

      <Card className="relative z-10 w-full max-w-[420px] rounded-[16px] p-10 shadow-xl bg-surface border-border">
        <div className="mb-8 text-center space-y-2">
          {/* Logo KANTOR with dot indicator styling */}
          <div className="flex justify-center flex-col items-center">
             <div className="text-[24px] font-[800] leading-none tracking-tight font-display text-text-primary flex items-start">
               K<span className="relative">A<span className="absolute -top-1 left-1/2 -translate-x-1/2 w-1.5 h-1.5 rounded-full bg-ops"></span></span>NTOR
             </div>
          </div>
          <p className="text-[12px] font-[500] text-text-secondary leading-tight">
            KanA Intelligence Operational dashboaRd
          </p>
        </div>

        <form className="space-y-4" onSubmit={submitForm}>
          <Field label="Full name" error={errors.full_name?.message}>
            <Input 
              {...registerField("full_name")} 
              placeholder="Jane Doe" 
              className="h-10 px-3 bg-surface-muted border-transparent focus:bg-surface focus:border-ops focus:ring-2 focus:ring-ops/20 rounded-[6px] text-[14px]"
            />
          </Field>
          <Field label="Email" error={errors.email?.message}>
            <Input 
              {...registerField("email")} 
              placeholder="jane@company.com" 
              type="email" 
              className="h-10 px-3 bg-surface-muted border-transparent focus:bg-surface focus:border-ops focus:ring-2 focus:ring-ops/20 rounded-[6px] text-[14px]"
            />
          </Field>
          <Field label="Password" error={errors.password?.message}>
            <Input 
              {...registerField("password")} 
              placeholder="••••••••" 
              type="password" 
              className="h-10 px-3 bg-surface-muted border-transparent focus:bg-surface focus:border-ops focus:ring-2 focus:ring-ops/20 rounded-[6px] text-[14px]"
            />
          </Field>
          <Field label="Confirm password" error={errors.confirmPassword?.message}>
            <Input
              {...registerField("confirmPassword")}
              placeholder="••••••••"
              type="password"
              className="h-10 px-3 bg-surface-muted border-transparent focus:bg-surface focus:border-ops focus:ring-2 focus:ring-ops/20 rounded-[6px] text-[14px]"
            />
          </Field>
          
          {registerMutation.error instanceof ApiError ? (
            <div className="rounded-[8px] border border-error/20 bg-error-light px-4 py-3 text-[14px] text-error">
              {registerMutation.error.message}
            </div>
          ) : null}

          <div className="pt-2">
            <Button 
              className="w-full h-[44px] bg-ops hover:bg-ops-dark active:scale-[0.98] transition-transform text-white rounded-[8px] font-[600] text-[14px]" 
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
  children: ReactNode;
}

function Field({ label, error, children }: FieldProps) {
  return (
    <div className="space-y-1.5">
      <label className="text-[13px] font-[600] text-text-primary">{label}</label>
      {children}
      {error ? <p className="text-[12px] mt-1 text-error">{error}</p> : null}
    </div>
  );
}

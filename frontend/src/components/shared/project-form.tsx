import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { X } from "lucide-react";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { listAvailableUsers, projectsKeys } from "@/services/operational-projects";
import type { ProjectFormValues } from "@/types/project";
import { cn } from "@/lib/utils";

const projectFormSchema = z.object({
  name: z.string().min(3, "Nama project minimal 3 karakter"),
  description: z.string(),
  deadline: z.string().min(1, "Deadline wajib diisi"),
  status: z.enum(["draft", "active", "on_hold", "completed", "archived"]),
  priority: z.enum(["low", "medium", "high", "critical"]),
  members: z
    .array(
      z.object({
        user_id: z.string().optional(),
        email: z.string().email(),
        full_name: z.string().optional(),
        avatar_url: z.string().nullable().optional(),
        role_in_project: z.string().min(2, "Role wajib dipilih"),
      }),
    )
    .optional(),
  member_emails: z.array(z.string()).optional(),
});

const projectFormSchemaWithMembers = projectFormSchema.extend({
  members: z
    .array(
      z.object({
        user_id: z.string().optional(),
        email: z.string().email(),
        full_name: z.string().optional(),
        avatar_url: z.string().nullable().optional(),
        role_in_project: z.string().min(2, "Role wajib dipilih"),
      }),
    )
    .min(1, "Pilih minimal 1 member"),
});

interface ProjectFormProps {
  isOpen: boolean;
  defaultValues?: ProjectFormValues;
  title: string;
  description: string;
  submitLabel: string;
  isSubmitting: boolean;
  showMemberPicker?: boolean;
  onCancel?: () => void;
  onSubmit: (values: ProjectFormValues) => void;
}

const baseValues: ProjectFormValues = {
  name: "",
  description: "",
  deadline: "",
  status: "draft",
  priority: "medium",
  members: [],
  member_emails: [],
};

export function ProjectForm({
  isOpen,
  defaultValues,
  title,
  description,
  submitLabel,
  isSubmitting,
  showMemberPicker = false,
  onCancel,
  onSubmit,
}: ProjectFormProps) {
  const schema = showMemberPicker ? projectFormSchemaWithMembers : projectFormSchema;

  const {
    control,
    register,
    handleSubmit,
    reset,
    setValue,
    watch,
    formState: { errors },
  } = useForm<ProjectFormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaultValues ?? baseValues,
  });

  const selectedMembers = watch("members") ?? [];
  const [memberSearch, setMemberSearch] = useState("");
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const usersQuery = useQuery({
    queryKey: [...projectsKeys.all, "available-users"],
    queryFn: listAvailableUsers,
    enabled: isOpen && showMemberPicker,
  });

  const availableUsers = usersQuery.data ?? [];
  const filteredUsers = availableUsers.filter(
    (u) =>
      !selectedMembers.some((member) => member.email === u.email) &&
      (memberSearch === "" ||
        u.full_name.toLowerCase().includes(memberSearch.toLowerCase()) ||
        u.email.toLowerCase().includes(memberSearch.toLowerCase())),
  );

  const addMember = useCallback(
    (user: (typeof availableUsers)[number]) => {
      const current = selectedMembers;
      if (!current.some((member) => member.email === user.email)) {
        setValue(
          "members",
          [
            ...current,
            {
              user_id: user.id,
              email: user.email,
              full_name: user.full_name,
              avatar_url: user.avatar_url ?? null,
              role_in_project: "member",
            },
          ],
          { shouldValidate: true },
        );
      }
      setMemberSearch("");
    },
    [selectedMembers, setValue],
  );

  const removeMember = useCallback(
    (email: string) => {
      setValue(
        "members",
        selectedMembers.filter((member) => member.email !== email),
        { shouldValidate: true },
      );
    },
    [selectedMembers, setValue],
  );

  const updateMemberRole = useCallback(
    (email: string, roleInProject: string) => {
      setValue(
        "members",
        selectedMembers.map((member) =>
          member.email === email
            ? {
                ...member,
                role_in_project: roleInProject,
              }
            : member,
        ),
        { shouldValidate: true },
      );
    },
    [selectedMembers, setValue],
  );

  // Close dropdown on click outside
  useEffect(() => {
    function handleClick(event: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setDropdownOpen(false);
      }
    }
    if (dropdownOpen) {
      document.addEventListener("mousedown", handleClick);
      return () => document.removeEventListener("mousedown", handleClick);
    }
  }, [dropdownOpen]);

  // Reset when modal closes
  useEffect(() => {
    if (!isOpen) {
      setMemberSearch("");
      setDropdownOpen(false);
    }
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    reset({
      ...baseValues,
      ...defaultValues,
      members: defaultValues?.members ?? [],
      member_emails: defaultValues?.member_emails ?? [],
    });
  }, [defaultValues, isOpen, reset]);

  const formControlClass =
    "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10 disabled:cursor-not-allowed disabled:opacity-50";
  const richTextareaClass = cn(formControlClass, "h-auto min-h-[96px] py-3");
  const statusOptions = [
    { value: "draft", label: "Draft" },
    { value: "active", label: "Aktif" },
    { value: "on_hold", label: "Ditunda" },
    { value: "completed", label: "Selesai" },
    { value: "archived", label: "Arsip" },
  ];
  const priorityOptions = [
    { value: "low", label: "Rendah" },
    { value: "medium", label: "Sedang" },
    { value: "high", label: "Tinggi" },
    { value: "critical", label: "Kritis" },
  ];
  const memberRoleOptions = [
    { value: "lead", label: "Lead" },
    { value: "developer", label: "Developer" },
    { value: "designer", label: "Designer" },
    { value: "qa", label: "QA" },
    { value: "member", label: "Member" },
  ];

  const memberError = showMemberPicker
    ? (errors.members as { message?: string } | undefined)?.message
    : undefined;

  return (
    <FormModal
      isLoading={isSubmitting}
      isOpen={isOpen}
      onClose={onCancel ?? (() => undefined)}
      onSubmit={handleSubmit(onSubmit)}
      size="lg"
      submitLabel={submitLabel}
      title={title}
      subtitle={description}
    >
      <Field error={errors.name?.message} label="Nama project" required>
        <Input
          className="focus-visible:border-ops focus-visible:ring-ops/10"
          {...register("name")}
          placeholder="Q2 Operational Revamp"
        />
      </Field>

        <Field error={errors.description?.message} label="Deskripsi">
          <textarea
          className={richTextareaClass}
          {...register("description")}
          placeholder="Ringkasan singkat tujuan, scope, dan pemilik project."
        />
      </Field>

      <div className="grid gap-5 md:grid-cols-3">
        <Field error={errors.status?.message} label="Status" required>
          <Controller
            control={control}
            name="status"
            render={({ field }) => (
              <Select
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                options={statusOptions}
                triggerClassName="focus-visible:border-ops focus-visible:ring-ops/10"
                value={field.value}
              />
            )}
          />
        </Field>

        <Field error={errors.priority?.message} label="Prioritas" required>
          <Controller
            control={control}
            name="priority"
            render={({ field }) => (
              <Select
                onBlur={field.onBlur}
                onValueChange={field.onChange}
                options={priorityOptions}
                triggerClassName="focus-visible:border-ops focus-visible:ring-ops/10"
                value={field.value}
              />
            )}
          />
        </Field>

        <Field error={errors.deadline?.message} label="Deadline" required>
          <Input
            className={cn(
              "focus-visible:border-ops focus-visible:ring-ops/10",
              errors.deadline && "border-priority-high",
            )}
            {...register("deadline")}
            type="date"
          />
        </Field>
      </div>

      {showMemberPicker ? (
        <Field error={memberError} label="Anggota project" required>
          <div className="relative" ref={dropdownRef}>
            <div
              className={cn(
                "flex min-h-[48px] items-center rounded-[6px] border bg-surface-muted px-3 py-2 transition-all focus-within:border-ops focus-within:bg-surface focus-within:ring-4 focus-within:ring-ops/10",
                memberError ? "border-priority-high" : "border-transparent",
              )}
            >
              <input
                className="min-w-[120px] flex-1 border-none bg-transparent py-1 text-[14px] text-text-primary outline-none placeholder:text-text-tertiary"
                onChange={(e) => {
                  setMemberSearch(e.target.value);
                  setDropdownOpen(true);
                }}
                onFocus={() => setDropdownOpen(true)}
                onKeyDown={(e) => {
                  if (e.key === "Backspace" && memberSearch === "" && selectedMembers.length > 0) {
                    removeMember(selectedMembers[selectedMembers.length - 1].email);
                  }
                  if (e.key === "Escape") {
                    setDropdownOpen(false);
                  }
                }}
                placeholder={selectedMembers.length > 0 ? "Tambah anggota lain..." : "Cari employee atau user..."}
                value={memberSearch}
              />
            </div>

            {dropdownOpen && usersQuery.isLoading && (
              <div className="absolute left-0 right-0 top-full z-[130] mt-1 rounded-lg border border-border bg-surface px-3 py-3 text-center text-sm text-text-tertiary shadow-lg">
                Memuat daftar user...
              </div>
            )}

            {dropdownOpen && usersQuery.isError && (
              <div className="absolute left-0 right-0 top-full z-[130] mt-1 rounded-lg border border-error/30 bg-error-light px-3 py-3 text-center text-sm text-error shadow-lg">
                Gagal memuat daftar user. Coba tutup lalu buka lagi form project.
              </div>
            )}

            {dropdownOpen && filteredUsers.length > 0 && (
              <div className="absolute left-0 right-0 top-full z-[130] mt-1 max-h-52 overflow-y-auto rounded-lg border border-border bg-surface shadow-lg">
                {filteredUsers.slice(0, 10).map((user) => (
                  <button
                    className="flex w-full items-center gap-3 px-3 py-2.5 text-left text-sm transition-colors hover:bg-surface-muted"
                    key={user.id}
                    onMouseDown={(e) => {
                      e.preventDefault(); // prevent blur from closing dropdown
                      addMember(user);
                      setDropdownOpen(false);
                    }}
                    type="button"
                  >
                    <ProtectedAvatar
                      alt={user.full_name}
                      avatarUrl={user.avatar_url}
                      className="h-8 w-8 shrink-0 border border-border/70"
                    />
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-medium text-text-primary">
                        {user.full_name}
                      </p>
                      <p className="truncate text-xs text-text-tertiary">{user.email}</p>
                    </div>
                  </button>
                ))}
              </div>
            )}

            {dropdownOpen && !usersQuery.isLoading && !usersQuery.isError && filteredUsers.length === 0 && availableUsers.length > 0 && (
              <div className="absolute left-0 right-0 top-full z-[130] mt-1 rounded-lg border border-border bg-surface px-3 py-3 text-center text-sm text-text-tertiary shadow-lg">
                {selectedMembers.length === availableUsers.length
                  ? "Semua user sudah dipilih"
                  : "Tidak ada hasil"}
              </div>
            )}

            {dropdownOpen && !usersQuery.isLoading && !usersQuery.isError && filteredUsers.length === 0 && availableUsers.length === 0 && (
              <div className="absolute left-0 right-0 top-full z-[130] mt-1 rounded-lg border border-border bg-surface px-3 py-3 text-center text-sm text-text-tertiary shadow-lg">
                Belum ada user aktif yang bisa dipilih.
              </div>
            )}
          </div>

          {selectedMembers.length > 0 ? (
            <div className="mt-3 space-y-3">
              {selectedMembers.map((member) => (
                <div
                  className="grid gap-3 rounded-[14px] border border-border/80 bg-background/70 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_170px_auto]"
                  key={member.email}
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <ProtectedAvatar
                      alt={member.full_name ?? member.email}
                      avatarUrl={member.avatar_url}
                      className="h-9 w-9 shrink-0 border border-border/70"
                    />
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-text-primary">
                        {member.full_name ?? member.email}
                      </p>
                      <p className="truncate text-xs text-text-tertiary">{member.email}</p>
                    </div>
                  </div>

                  <Select
                    onValueChange={(nextRole) => updateMemberRole(member.email, nextRole)}
                    options={memberRoleOptions}
                    triggerClassName="focus-visible:border-ops focus-visible:ring-ops/10"
                    value={member.role_in_project}
                  />

                  <button
                    className="inline-flex h-11 items-center justify-center rounded-[10px] border border-border bg-surface px-3 text-sm font-semibold text-text-secondary transition hover:bg-surface-muted"
                    onClick={() => removeMember(member.email)}
                    type="button"
                  >
                    Hapus
                  </button>
                </div>
              ))}
            </div>
          ) : null}
        </Field>
      ) : null}
    </FormModal>
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
    <div className="flex flex-col space-y-1.5">
      <label className="text-[13px] font-[500] text-text-secondary">
        {label}
        {required ? <span className="ml-0.5 text-priority-high">*</span> : null}
      </label>
      {children}
      {error ? (
        <p className="mt-1 text-[12px] font-[500] text-priority-high">{error}</p>
      ) : null}
    </div>
  );
}

import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { X } from "lucide-react";
import { z } from "zod";

import { FormModal } from "@/components/shared/form-modal";
import { Input } from "@/components/ui/input";
import { listAvailableUsers, projectsKeys } from "@/services/operational-projects";
import type { ProjectFormValues } from "@/types/project";
import { cn } from "@/lib/utils";

const projectFormSchema = z.object({
  name: z.string().min(3, "Nama project minimal 3 karakter"),
  description: z.string(),
  deadline: z.string().min(1, "Deadline wajib diisi"),
  status: z.enum(["draft", "active", "on_hold", "completed", "archived"]),
  priority: z.enum(["low", "medium", "high", "critical"]),
  member_emails: z.array(z.string()).optional(),
});

const projectFormSchemaWithMembers = projectFormSchema.extend({
  member_emails: z.array(z.string()).min(1, "Pilih minimal 1 member"),
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
    register,
    handleSubmit,
    setValue,
    watch,
    formState: { errors },
  } = useForm<ProjectFormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaultValues ?? baseValues,
  });

  const selectedEmails = watch("member_emails") ?? [];
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
      !selectedEmails.includes(u.email) &&
      (memberSearch === "" ||
        u.full_name.toLowerCase().includes(memberSearch.toLowerCase()) ||
        u.email.toLowerCase().includes(memberSearch.toLowerCase())),
  );

  const addMember = useCallback(
    (email: string) => {
      const current = selectedEmails;
      if (!current.includes(email)) {
        setValue("member_emails", [...current, email], { shouldValidate: true });
      }
      setMemberSearch("");
    },
    [selectedEmails, setValue],
  );

  const removeMember = useCallback(
    (email: string) => {
      setValue(
        "member_emails",
        selectedEmails.filter((e) => e !== email),
        { shouldValidate: true },
      );
    },
    [selectedEmails, setValue],
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

  const formControlClass =
    "flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10 disabled:cursor-not-allowed disabled:opacity-50";

  const memberError = showMemberPicker
    ? (errors.member_emails as { message?: string } | undefined)?.message
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
      <Field error={errors.name?.message} label="Project name" required>
        <Input
          className="focus-visible:border-ops focus-visible:ring-ops/10"
          {...register("name")}
          placeholder="Q2 Operational Revamp"
        />
      </Field>

      <Field error={errors.description?.message} label="Description">
        <textarea
          className={cn(formControlClass, "min-h-[96px] py-3")}
          {...register("description")}
          placeholder="Short brief about objectives, scope, and owners."
        />
      </Field>

      <div className="grid gap-5 md:grid-cols-3">
        <Field error={errors.status?.message} label="Status" required>
          <select className={formControlClass} {...register("status")}>
            <option value="draft">Draft</option>
            <option value="active">Active</option>
            <option value="on_hold">On Hold</option>
            <option value="completed">Completed</option>
            <option value="archived">Archived</option>
          </select>
        </Field>

        <Field error={errors.priority?.message} label="Priority" required>
          <select className={formControlClass} {...register("priority")}>
            <option value="low">Low</option>
            <option value="medium">Medium</option>
            <option value="high">High</option>
            <option value="critical">Critical</option>
          </select>
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
        <Field error={memberError} label="Members" required>
          <div className="relative" ref={dropdownRef}>
            <div
              className={cn(
                "flex min-h-[44px] flex-wrap items-center gap-1.5 rounded-[6px] border bg-surface-muted px-2 py-1.5 transition-all focus-within:border-ops focus-within:bg-surface focus-within:ring-4 focus-within:ring-ops/10",
                memberError ? "border-priority-high" : "border-transparent",
              )}
            >
              {selectedEmails.map((email) => {
                const user = availableUsers.find((u) => u.email === email);
                return (
                  <span
                    className="inline-flex items-center gap-1 rounded-full bg-ops/10 px-2 py-0.5 text-xs font-medium text-ops"
                    key={email}
                  >
                    {user?.full_name ?? email}
                    <button
                      className="rounded-full p-0.5 hover:bg-ops/20"
                      onClick={() => removeMember(email)}
                      type="button"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                );
              })}
              <input
                className="min-w-[120px] flex-1 border-none bg-transparent py-1 text-[14px] text-text-primary outline-none placeholder:text-text-tertiary"
                onChange={(e) => {
                  setMemberSearch(e.target.value);
                  setDropdownOpen(true);
                }}
                onFocus={() => setDropdownOpen(true)}
                onKeyDown={(e) => {
                  if (e.key === "Backspace" && memberSearch === "" && selectedEmails.length > 0) {
                    removeMember(selectedEmails[selectedEmails.length - 1]);
                  }
                  if (e.key === "Escape") {
                    setDropdownOpen(false);
                  }
                }}
                placeholder={selectedEmails.length > 0 ? "Tambah member lagi..." : "Cari employee atau user..."}
                value={memberSearch}
              />
            </div>

            {dropdownOpen && usersQuery.isLoading && (
              <div className="absolute left-0 right-0 top-full z-20 mt-1 rounded-lg border border-border bg-surface px-3 py-3 text-center text-sm text-text-tertiary shadow-lg">
                Memuat daftar user...
              </div>
            )}

            {dropdownOpen && usersQuery.isError && (
              <div className="absolute left-0 right-0 top-full z-20 mt-1 rounded-lg border border-error/30 bg-error-light px-3 py-3 text-center text-sm text-error shadow-lg">
                Gagal memuat daftar user. Coba tutup lalu buka lagi form project.
              </div>
            )}

            {dropdownOpen && filteredUsers.length > 0 && (
              <div className="absolute left-0 right-0 top-full z-20 mt-1 max-h-52 overflow-y-auto rounded-lg border border-border bg-surface shadow-lg">
                {filteredUsers.slice(0, 10).map((user) => (
                  <button
                    className="flex w-full items-center gap-3 px-3 py-2.5 text-left text-sm transition-colors hover:bg-surface-muted"
                    key={user.id}
                    onMouseDown={(e) => {
                      e.preventDefault(); // prevent blur from closing dropdown
                      addMember(user.email);
                    }}
                    type="button"
                  >
                    <UserAvatar name={user.full_name} />
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
              <div className="absolute left-0 right-0 top-full z-20 mt-1 rounded-lg border border-border bg-surface px-3 py-3 text-center text-sm text-text-tertiary shadow-lg">
                {selectedEmails.length === availableUsers.length
                  ? "Semua user sudah dipilih"
                  : "Tidak ada hasil"}
              </div>
            )}

            {dropdownOpen && !usersQuery.isLoading && !usersQuery.isError && filteredUsers.length === 0 && availableUsers.length === 0 && (
              <div className="absolute left-0 right-0 top-full z-20 mt-1 rounded-lg border border-border bg-surface px-3 py-3 text-center text-sm text-text-tertiary shadow-lg">
                Belum ada user aktif yang bisa dipilih.
              </div>
            )}
          </div>
        </Field>
      ) : null}
    </FormModal>
  );
}

function UserAvatar({ name }: { name: string }) {
  const initials = name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0])
    .join("")
    .toUpperCase();

  return (
    <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-ops/15 text-[10px] font-semibold text-ops">
      {initials}
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

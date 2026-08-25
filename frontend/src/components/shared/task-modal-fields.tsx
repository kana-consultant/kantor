import type { UseFormReturn } from "react-hook-form";

import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { TaskFormValues } from "@/types/kanban";
import type { ProjectMember } from "@/types/project";

const selectClassName =
	"flex h-[44px] w-full rounded-[6px] border border-border bg-surface px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10";

export function TaskTitleField({
	form,
	className,
}: {
	form: UseFormReturn<TaskFormValues>;
	className?: string;
}) {
	const {
		register,
		formState: { errors },
	} = form;

	return (
		<div className={cn("grid gap-2", className)}>
			<label
				className="text-[13px] font-[600] text-text-primary"
				htmlFor="task-title">
				Judul<span className="ml-0.5 text-priority-high">*</span>
			</label>
			<Input
				className="focus-visible:border-ops focus-visible:ring-ops/10"
				id="task-title"
				{...register("title")}
			/>
			{errors.title ? (
				<p className="text-[13px] font-[500] text-priority-high">
					{errors.title.message}
				</p>
			) : null}
		</div>
	);
}

export function TaskMetaFields({
	form,
	members,
	className,
	idPrefix = "task",
}: {
	form: UseFormReturn<TaskFormValues>;
	members: ProjectMember[];
	className?: string;
	idPrefix?: string;
}) {
	const { register } = form;

	return (
		<div className={cn("grid gap-5 md:grid-cols-2", className)}>
			<div className="grid gap-2">
				<label
					className="text-[13px] font-[600] text-text-primary"
					htmlFor={`${idPrefix}-assignee`}>
					PIC
				</label>
				<select
					className={selectClassName}
					id={`${idPrefix}-assignee`}
					{...register("assignee_id")}>
					<option value="">Belum ditentukan</option>
					{members.map((member) => (
						<option
							key={member.user_id}
							value={member.user_id}>
							{member.full_name || member.user_email || member.user_id}
						</option>
					))}
				</select>
			</div>

			<div className="grid gap-2">
				<label
					className="text-[13px] font-[600] text-text-primary"
					htmlFor={`${idPrefix}-due-date`}>
					Deadline
				</label>
				<Input
					className="focus-visible:border-ops focus-visible:ring-ops/10"
					id={`${idPrefix}-due-date`}
					type="date"
					{...register("due_date")}
				/>
			</div>

			<div className="grid gap-2">
				<label
					className="text-[13px] font-[600] text-text-primary"
					htmlFor={`${idPrefix}-priority`}>
					Prioritas
				</label>
				<select
					className={selectClassName}
					id={`${idPrefix}-priority`}
					{...register("priority")}>
					<option value="low">Rendah</option>
					<option value="medium">Medium</option>
					<option value="high">Tinggi</option>
					<option value="critical">Kritis</option>
				</select>
			</div>

			<div className="grid gap-2">
				<label
					className="text-[13px] font-[600] text-text-primary"
					htmlFor={`${idPrefix}-label`}>
					Label
				</label>
				<Input
					className="focus-visible:border-ops focus-visible:ring-ops/10"
					id={`${idPrefix}-label`}
					placeholder="Bug, desain, backend"
					{...register("label")}
				/>
			</div>
		</div>
	);
}

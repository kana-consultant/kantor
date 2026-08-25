import type { UseFormReturn } from "react-hook-form";

import {
	Drawer,
	DrawerClose,
	DrawerContent,
	DrawerDescription,
	DrawerHeader,
	DrawerTitle,
} from "@/components/shared/drawer";
import { PermissionGate } from "@/components/shared/permission-gate";
import {
	TaskMetaFields,
	TaskTitleField,
} from "@/components/shared/task-modal-fields";
import { Button } from "@/components/ui/button";
import { permissions } from "@/lib/permissions";
import type { TaskFormValues } from "@/types/kanban";
import type { ProjectMember } from "@/types/project";

export function TaskModal({
	mode,
	form,
	members,
	isSubmitting,
	isDeleting,
	onSubmit,
	onDelete,
	onClose,
}: {
	mode: "create" | "edit";
	form: UseFormReturn<TaskFormValues>;
	members: ProjectMember[];
	isSubmitting: boolean;
	isDeleting: boolean;
	onSubmit: (values: TaskFormValues) => void;
	onDelete: () => void;
	onClose: () => void;
}) {
	const { register, handleSubmit } = form;

	return (
		<Drawer
			onOpenChange={(open) => (!open ? onClose() : undefined)}
			open>
			<DrawerContent size="lg">
				<form
					className="flex h-full min-h-0 flex-col"
					onSubmit={handleSubmit(onSubmit)}>
					<DrawerHeader className="shrink-0">
						<div className="flex items-start justify-between gap-4">
							<div>
								<p className="text-[11px] font-[700] uppercase tracking-[0.08em] text-ops mb-1">
									{mode === "create" ? "Buat task" : "Detail task"}
								</p>
								<DrawerTitle>
									{mode === "create" ? "Task baru" : "Edit task"}
								</DrawerTitle>
								<DrawerDescription>
									{mode === "create"
										? "Isi ringkasan task, PIC, dan due date tanpa keluar dari board."
										: "Tinjau detail task, edit informasi pengerjaan, atau hapus task dari board."}
								</DrawerDescription>
							</div>
							<DrawerClose />
						</div>
					</DrawerHeader>

					<div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-6 pb-4 pt-6 [scrollbar-gutter:stable]">
						<TaskTitleField form={form} />

						<div className="grid gap-2">
							<label
								className="text-[13px] font-[600] text-text-primary"
								htmlFor="task-description">
								Deskripsi
							</label>
							<textarea
								className="min-h-32 w-full rounded-[6px] border border-border bg-surface px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10 disabled:cursor-not-allowed disabled:opacity-50"
								id="task-description"
								{...register("description")}
							/>
						</div>

						<TaskMetaFields
							form={form}
							members={members}
						/>
					</div>

					<div className="shrink-0 border-t border-border bg-surface px-6 pb-5 pt-5">
						<div className="flex flex-wrap gap-3">
							<Button
								variant="ops"
								disabled={isSubmitting}
								type="submit">
								{isSubmitting
									? "Menyimpan..."
									: mode === "create"
										? "Buat task"
										: "Simpan perubahan"}
							</Button>
							{mode === "edit" ? (
								<PermissionGate permission={permissions.operationalTaskDelete}>
									<Button
										disabled={isDeleting}
										onClick={onDelete}
										type="button"
										variant="ghost">
										{isDeleting ? "Menghapus..." : "Hapus task"}
									</Button>
								</PermissionGate>
							) : null}
						</div>
					</div>
				</form>
			</DrawerContent>
		</Drawer>
	);
}

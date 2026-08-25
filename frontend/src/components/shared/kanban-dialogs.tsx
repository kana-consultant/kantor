import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { FormModal } from "@/components/shared/form-modal";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { KanbanColumn, KanbanTaskListItem } from "@/types/kanban";

export type ColumnModalState =
	| { mode: "create" }
	| { mode: "edit"; columnId: string };

export interface ColumnFormValues {
	name: string;
	color: string;
}

interface KanbanDialogsProps {
	columnModal: ColumnModalState | null;
	columnForm: ColumnFormValues;
	columnColorOptions: string[];
	isColumnSaving: boolean;
	isColumnDeleting: boolean;
	isTaskDeleting: boolean;
	columnToDelete: KanbanColumn | null;
	taskToDelete: KanbanTaskListItem | null;
	onColumnFormChange: (
		updater: (current: ColumnFormValues) => ColumnFormValues,
	) => void;
	onColumnModalClose: () => void;
	onColumnSubmit: () => void;
	onColumnDeleteClose: () => void;
	onColumnDeleteConfirm: () => void;
	onTaskDeleteClose: () => void;
	onTaskDeleteConfirm: () => void;
}

export function KanbanDialogs({
	columnModal,
	columnForm,
	columnColorOptions,
	isColumnSaving,
	isColumnDeleting,
	isTaskDeleting,
	columnToDelete,
	taskToDelete,
	onColumnFormChange,
	onColumnModalClose,
	onColumnSubmit,
	onColumnDeleteClose,
	onColumnDeleteConfirm,
	onTaskDeleteClose,
	onTaskDeleteConfirm,
}: KanbanDialogsProps) {
	return (
		<>
		<FormModal
			isLoading={isColumnSaving}
			isOpen={Boolean(columnModal)}
			onClose={onColumnModalClose}
			onSubmit={(event) => {
				event.preventDefault();
				onColumnSubmit();
			}}
			size="sm"
			submitLabel={
				columnModal?.mode === "edit" ? "Simpan kolom" : "Buat kolom"
			}
			title={columnModal?.mode === "edit" ? "Edit kolom" : "Buat kolom"}
			subtitle="Gunakan nama singkat dan warna aksen yang jelas agar kolom mudah dipindai di board.">
			<div className="space-y-2">
				<label className="text-[13px] font-[600] text-text-primary">
					Nama kolom
				</label>
				<Input
					className="focus-visible:border-ops focus-visible:ring-ops/10"
					onChange={(event) =>
						onColumnFormChange((current) => ({
							...current,
							name: event.target.value,
						}))
					}
					placeholder="Review"
					value={columnForm.name}
				/>
			</div>
			<div className="space-y-2">
				<p className="text-[13px] font-[600] text-text-primary">
					Warna aksen
				</p>
				<div className="flex flex-wrap gap-2">
					{columnColorOptions.map((color) => (
						<button
							aria-label={`Choose list color ${color}`}
							className={cn(
								"h-9 w-9 rounded-full border-2 transition hover:scale-105",
								columnForm.color === color
									? "border-foreground shadow-sm"
									: "border-transparent",
							)}
							key={color}
							onClick={() =>
								onColumnFormChange((current) => ({ ...current, color }))
							}
							style={{ backgroundColor: color }}
							type="button"
						/>
					))}
				</div>
			</div>
		</FormModal>

		<ConfirmDialog
			confirmLabel="Hapus kolom"
			description={
				columnToDelete
					? `Semua task di dalam "${columnToDelete.name}" akan ikut terhapus bersama kolom ini.`
					: ""
			}
			isLoading={isColumnDeleting}
			isOpen={Boolean(columnToDelete)}
			onClose={onColumnDeleteClose}
			onConfirm={onColumnDeleteConfirm}
			title={
				columnToDelete ? `Hapus ${columnToDelete.name}?` : "Hapus kolom?"
			}
		/>

		<ConfirmDialog
			confirmLabel="Hapus task"
			description={
				taskToDelete
					? `Task "${taskToDelete.title}" akan dihapus dari board ini.`
					: ""
			}
			isLoading={isTaskDeleting}
			isOpen={Boolean(taskToDelete)}
			onClose={onTaskDeleteClose}
			onConfirm={onTaskDeleteConfirm}
			title={taskToDelete ? `Hapus ${taskToDelete.title}?` : "Hapus task?"}
		/>
		</>
	);
}

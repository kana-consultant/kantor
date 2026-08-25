import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { ChevronDown, ChevronRight, GripVertical, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { TaskFieldDraft } from "@/types/kanban";

interface TaskFieldRowProps {
	id: string;
	field: TaskFieldDraft;
	isExpanded: boolean;
	onToggle: () => void;
	onRemove: () => void;
	onValueChange: (value: string) => void;
}

export function TaskFieldRow({
	id,
	field,
	isExpanded,
	onToggle,
	onRemove,
	onValueChange,
}: TaskFieldRowProps) {
	const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
		useSortable({ id });

	return (
		<div
			className={cn(
				"rounded-[8px] border border-border bg-surface",
				isDragging && "z-10 opacity-80 shadow-card",
			)}
			ref={setNodeRef}
			style={{
				transform: CSS.Transform.toString(transform),
				transition,
			}}>
			<div className="flex items-center gap-1 px-2 py-2">
				<button
					aria-label={`Geser kolom ${field.name}`}
					className="shrink-0 cursor-grab touch-none rounded-[6px] p-1.5 text-text-tertiary transition-colors hover:bg-surface-muted hover:text-text-secondary active:cursor-grabbing"
					type="button"
					{...attributes}
					{...listeners}>
					<GripVertical className="h-4 w-4" />
				</button>

				<button
					aria-expanded={isExpanded}
					className="flex min-w-0 flex-1 items-center gap-2 rounded-[6px] px-1 py-1 text-left transition-colors hover:bg-surface-muted"
					onClick={onToggle}
					type="button">
					{isExpanded ? (
						<ChevronDown className="h-4 w-4 shrink-0 text-text-tertiary" />
					) : (
						<ChevronRight className="h-4 w-4 shrink-0 text-text-tertiary" />
					)}
					<span className="truncate text-[13px] font-[600] text-text-primary">
						{field.name}
					</span>
				</button>

				<Button
					aria-label={`Hapus kolom ${field.name}`}
					className="h-7 w-7 shrink-0 px-0 text-text-tertiary hover:text-priority-high"
					onClick={onRemove}
					size="icon"
					type="button"
					variant="ghost">
					<Trash2 className="h-4 w-4" />
				</Button>
			</div>

			{isExpanded ? (
				<div className="border-t border-border px-3 py-3">
					<textarea
						className="min-h-24 w-full rounded-[6px] border border-border bg-surface px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:ring-4 focus-visible:ring-ops/10"
						onChange={(event) => onValueChange(event.target.value)}
						placeholder={`Isi ${field.name}`}
						value={field.value}
					/>
				</div>
			) : null}
		</div>
	);
}

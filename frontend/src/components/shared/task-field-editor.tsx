import { useState } from "react";
import {
	DndContext,
	KeyboardSensor,
	PointerSensor,
	closestCenter,
	useSensor,
	useSensors,
	type DragEndEvent,
} from "@dnd-kit/core";
import {
	SortableContext,
	arrayMove,
	sortableKeyboardCoordinates,
	verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { Plus } from "lucide-react";

import { TaskFieldRow } from "@/components/shared/task-field-row";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { TaskFieldDraft } from "@/types/kanban";

const suggestedFieldNames = ["Problem", "Solution", "Outcome", "Reference"];

const fieldKey = (field: TaskFieldDraft) => field.name.trim().toLowerCase();

interface TaskFieldEditorProps {
	fields: TaskFieldDraft[];
	onChange: (fields: TaskFieldDraft[]) => void;
}

export function TaskFieldEditor({ fields, onChange }: TaskFieldEditorProps) {
	const [expanded, setExpanded] = useState<Record<string, boolean>>({});
	const [draftName, setDraftName] = useState("");
	const sensors = useSensors(
		useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
		useSensor(KeyboardSensor, {
			coordinateGetter: sortableKeyboardCoordinates,
		}),
	);

	const usedNames = new Set(fields.map(fieldKey));
	const availableSuggestions = suggestedFieldNames.filter(
		(name) => !usedNames.has(name.toLowerCase()),
	);
	const canAddDraft =
		Boolean(draftName.trim()) && !usedNames.has(draftName.trim().toLowerCase());

	function addField(name: string) {
		const trimmed = name.trim();
		if (!trimmed || usedNames.has(trimmed.toLowerCase())) {
			return;
		}

		onChange([...fields, { name: trimmed, value: "" }]);
		setExpanded((current) => ({ ...current, [trimmed.toLowerCase()]: true }));
	}

	function handleDragEnd(event: DragEndEvent) {
		const { active, over } = event;
		if (!over || active.id === over.id) {
			return;
		}

		const from = fields.findIndex((field) => fieldKey(field) === active.id);
		const to = fields.findIndex((field) => fieldKey(field) === over.id);
		if (from === -1 || to === -1) {
			return;
		}

		onChange(arrayMove(fields, from, to));
	}

	return (
		<div className="space-y-3">
			{availableSuggestions.length > 0 ? (
				<div className="hidden flex-wrap gap-2 md:flex">
					{availableSuggestions.map((name) => (
						<Button
							className="h-8 shrink-0 px-3 text-[13px]"
							key={name}
							onClick={() => addField(name)}
							size="sm"
							type="button"
							variant="outline">
							Add {name}
							<Plus className="h-3.5 w-3.5" />
						</Button>
					))}
				</div>
			) : null}

			<div className="flex gap-2">
				<Input
					className="h-9 focus-visible:border-ops focus-visible:ring-ops/10"
					onChange={(event) => setDraftName(event.target.value)}
					onKeyDown={(event) => {
						if (event.key === "Enter") {
							event.preventDefault();
							addField(draftName);
							setDraftName("");
						}
					}}
					placeholder="Add column"
					value={draftName}
				/>
				<Button
					aria-label="Tambah kolom baru"
					className="h-9 shrink-0 px-3"
					disabled={!canAddDraft}
					onClick={() => {
						addField(draftName);
						setDraftName("");
					}}
					size="sm"
					type="button"
					variant="outline">
					<Plus className="h-4 w-4" />
				</Button>
			</div>

			<DndContext
				collisionDetection={closestCenter}
				onDragEnd={handleDragEnd}
				sensors={sensors}>
				<SortableContext
					items={fields.map(fieldKey)}
					strategy={verticalListSortingStrategy}>
					<div className="space-y-2">
						{fields.map((field, index) => {
							const key = fieldKey(field);

							return (
								<TaskFieldRow
									field={field}
									id={key}
									isExpanded={expanded[key] ?? false}
									key={key}
									onRemove={() =>
										onChange(
											fields.filter((_, position) => position !== index),
										)
									}
									onToggle={() =>
										setExpanded((current) => ({
											...current,
											[key]: !(current[key] ?? false),
										}))
									}
									onValueChange={(value) =>
										onChange(
											fields.map((item, position) =>
												position === index ? { ...item, value } : item,
											),
										)
									}
								/>
							);
						})}
					</div>
				</SortableContext>
			</DndContext>
		</div>
	);
}

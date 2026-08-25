import { useDroppable } from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical } from "lucide-react";

import { PermissionGate } from "@/components/shared/permission-gate";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { formatCalendarDate } from "@/lib/date";
import { permissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import type { DragColumnData, DragTaskData } from "@/components/shared/kanban-dnd";
import type {
	KanbanColumn,
	KanbanTaskListItem,
} from "@/types/kanban";
import type { ProjectPriority } from "@/types/project";

export interface KanbanColumnCardProps {
	column: KanbanColumn;
	tasks: KanbanTaskListItem[];
	totalTasks: number;
	hasMore: boolean;
	isLoadingTasks: boolean;
	isFetchingMore: boolean;
	onLoadMore: () => void;
	quickDraft: string;
	onQuickDraftChange: (value: string) => void;
	onQuickAdd: () => void;
	onTaskClick: (task: KanbanTaskListItem) => void;
	onTaskCreate: () => void;
	onEditColumn: () => void;
	onDeleteColumn: () => void;
}

export function KanbanColumnCard(props: KanbanColumnCardProps) {
	const sortable = useSortable({
		id: props.column.id,
		data: { type: "column", column: props.column } satisfies DragColumnData,
	});
	const droppable = useDroppable({
		id: `column-drop-${props.column.id}`,
		data: { type: "column", column: props.column } satisfies DragColumnData,
	});
	const style = {
		transform: CSS.Transform.toString(sortable.transform),
		transition: sortable.transition,
	};

	return (
		<div
			className="w-[min(82vw,320px)] shrink-0 md:w-[320px]"
			ref={sortable.setNodeRef}
			style={style}>
			<Card
				className={cn(
					"flex h-full min-h-[420px] flex-col border-border bg-surface-muted p-3 shadow-sm transition-all md:min-h-[500px] md:p-4",
					sortable.isDragging && "opacity-70",
					droppable.isOver && "border-ops/40 bg-ops/5 shadow-card",
				)}
				ref={droppable.setNodeRef}>
				<div className="flex items-start justify-between gap-3">
					<div className="flex items-center gap-3">
						<button
							{...sortable.attributes}
							{...sortable.listeners}
							className="rounded-[6px] border border-border bg-background px-2 py-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-text-secondary hover:bg-surface-muted transition-colors cursor-grab"
							type="button">
							Geser
						</button>
						<div>
							<div className="flex items-center gap-2">
								<span
									className="h-2.5 w-2.5 rounded-full"
									style={{ backgroundColor: props.column.color ?? "#94A3B8" }}
								/>
								<h5 className="font-[600] text-[14px] text-text-primary truncate max-w-[120px]">
									{props.column.name}
								</h5>
							</div>
							<p className="text-[12px] font-[500] text-text-tertiary">
								{props.totalTasks} task
							</p>
						</div>
					</div>

					<PermissionGate permission={permissions.operationalColumnManage}>
						<div className="flex gap-2">
							<Button
								onClick={props.onEditColumn}
								size="sm"
								variant="secondary"
								className="h-7 px-2 text-[11px]">
								Edit
							</Button>
							<PermissionGate permission={permissions.operationalColumnManage}>
								<Button
									onClick={props.onDeleteColumn}
									size="sm"
									variant="ghost"
									className="h-7 px-2 text-[11px]">
									Hapus
								</Button>
							</PermissionGate>
						</div>
					</PermissionGate>
				</div>

				<div
					className="mt-4 flex-1 space-y-3 overflow-y-auto pr-1 max-h-[460px]"
					onScroll={(event) => {
						const el = event.currentTarget;
						if (
							props.hasMore &&
							!props.isFetchingMore &&
							el.scrollHeight - el.scrollTop - el.clientHeight < 120
						) {
							props.onLoadMore();
						}
					}}>
					<SortableContext
						items={props.tasks.map((task) => task.id)}
						strategy={verticalListSortingStrategy}>
						{props.tasks.map((task) => (
							<KanbanTaskCard
								key={task.id}
								onClick={() => props.onTaskClick(task)}
								task={task}
							/>
						))}
					</SortableContext>

					{props.tasks.length === 0 && !props.isLoadingTasks ? (
						<div className="rounded-[12px] border border-dashed border-border bg-background/50 px-4 py-8 text-center text-[13px] font-[500] text-text-tertiary">
							Belum ada task di kolom ini.
						</div>
					) : null}

					{props.isLoadingTasks || props.isFetchingMore ? (
						<p className="py-3 text-center text-[12px] font-[500] text-text-tertiary">
							Memuat task...
						</p>
					) : null}
				</div>

				<PermissionGate permission={permissions.operationalTaskCreate}>
					<div className="mt-4 rounded-[12px] border border-border bg-background p-3 space-y-3">
						<Input
							className="focus-visible:border-ops focus-visible:ring-ops/10"
							onChange={(event) => props.onQuickDraftChange(event.target.value)}
							placeholder="Tambahkan task cepat"
							value={props.quickDraft}
						/>
						<div className="flex gap-3">
							<Button
								variant="ops"
								onClick={props.onQuickAdd}
								size="sm">
								Tambah cepat
							</Button>
							<Button
								onClick={props.onTaskCreate}
								size="sm"
								variant="ghost">
								Form lengkap
							</Button>
						</div>
					</div>
				</PermissionGate>
			</Card>
		</div>
	);
}

export function KanbanTaskCard({
	task,
	onClick,
}: {
	task: KanbanTaskListItem;
	onClick: () => void;
}) {
	const sortable = useSortable({
		id: task.id,
		data: { type: "task", task } satisfies DragTaskData,
	});
	const style = {
		transform: CSS.Transform.toString(sortable.transform),
		transition: sortable.transition,
	};

	return (
		<div
			ref={sortable.setNodeRef}
			style={style}>
			<Card
				className={cn(
					"cursor-pointer p-4 shadow-sm transition-all hover:border-ops/30 hover:shadow-card group",
					sortable.isDragging && "opacity-60 ring-2 ring-ops",
				)}
				onClick={onClick}>
				<div className="flex items-start justify-between gap-3">
					<div>
						<div className="flex flex-wrap items-center gap-2 mb-2">
							<PriorityBadge priority={task.priority} />
							<AssignmentBadge assignedVia={task.assigned_via} />
							{task.label ? (
								<span className="rounded-full bg-surface-muted border border-border px-2 py-0.5 text-[11px] font-[600] uppercase tracking-wider text-text-secondary">
									{task.label}
								</span>
							) : null}
						</div>
						<h6 className="text-[14px] font-[600] text-text-primary leading-tight">
							{task.title}
						</h6>
					</div>
					<button
						{...sortable.attributes}
						{...sortable.listeners}
						aria-label={`Pindahkan task ${task.title}`}
						className="rounded-[8px] border border-border bg-surface-muted p-2 text-text-tertiary transition hover:border-ops/40 hover:bg-ops/10 hover:text-ops active:cursor-grabbing"
						onClick={(event) => event.stopPropagation()}
						type="button">
						<GripVertical className="h-4 w-4" />
					</button>
				</div>

				{task.description ? (
					<p className="mt-2 line-clamp-2 text-[12px] text-text-secondary leading-relaxed">
						{task.description}
					</p>
				) : null}

				<div className="mt-4 flex items-center justify-between gap-3 border-t border-border pt-4">
					<div className="flex items-center gap-2">
						<AvatarBadge
							avatarUrl={task.avatar_url}
							name={task.assignee_name}
						/>
						<span className="text-[12px] font-[500] text-text-secondary truncate max-w-[120px]">
							{task.assignee_name ?? "Belum ada PIC"}
						</span>
					</div>
					<span className="text-[11px] font-[600] text-text-tertiary uppercase tracking-wider">
						{task.due_date ? formatDate(task.due_date) : "Tanpa deadline"}
					</span>
				</div>
			</Card>
		</div>
	);
}

export function TaskOverlay({ task }: { task: KanbanTaskListItem }) {
	return (
		<div className="w-[min(82vw,320px)] md:w-[320px]">
			<Card className="border-ops shadow-2xl p-4 rotate-2">
				<PriorityBadge priority={task.priority} />
				<p className="mt-2 text-[14px] font-[600] text-text-primary leading-tight">
					{task.title}
				</p>
			</Card>
		</div>
	);
}

export function ColumnOverlay({
	column,
	taskCount,
}: {
	column: KanbanColumn;
	taskCount: number;
}) {
	return (
		<div className="w-[min(82vw,320px)] md:w-[320px]">
			<Card className="border-ops shadow-2xl p-4 rotate-2">
				<div className="flex items-center gap-2">
					<span
						className="h-2 w-2 rounded-full"
						style={{ backgroundColor: column.color ?? "#94A3B8" }}
					/>
					<p className="text-[14px] font-[600] text-text-primary">
						{column.name}
					</p>
				</div>
				<p className="mt-1 text-[12px] font-[500] text-text-tertiary">
					{taskCount} tasks
				</p>
			</Card>
		</div>
	);
}

function PriorityBadge({ priority }: { priority: ProjectPriority }) {
	const tone =
		priority === "critical"
			? "bg-priority-critical-bg text-priority-critical"
			: priority === "high"
				? "bg-priority-high-bg text-priority-high"
				: priority === "medium"
					? "bg-priority-medium-bg text-priority-medium"
					: "bg-surface-muted text-text-secondary border border-border";
	const label =
		priority === "critical"
			? "kritis"
			: priority === "high"
				? "tinggi"
				: priority === "medium"
					? "medium"
					: "rendah";

	return (
		<span
			className={cn(
				"rounded-[6px] px-2 py-0.5 text-[11px] font-[700] uppercase tracking-[0.08em]",
				tone,
			)}>
			{label}
		</span>
	);
}

function AssignmentBadge({ assignedVia }: { assignedVia: "manual" | "auto" }) {
	const tone =
		assignedVia === "auto"
			? "bg-ops/10 text-ops"
			: "bg-surface-muted text-text-secondary border border-border";

	return (
		<span
			className={cn(
				"rounded-[6px] px-2 py-0.5 text-[11px] font-[700] uppercase tracking-[0.08em]",
				tone,
			)}>
			{assignedVia === "auto" ? "otomatis" : "manual"}
		</span>
	);
}

function AvatarBadge({
	name,
	avatarUrl,
}: {
	name?: string | null;
	avatarUrl?: string | null;
}) {
	return (
		<ProtectedAvatar
			alt={name ?? "Unassigned"}
			avatarUrl={avatarUrl}
			className="h-8 w-8 shadow-sm ring-2 ring-background"
			fallbackClassName="bg-ops text-white"
			iconClassName="h-4 w-4"
		/>
	);
}

function formatDate(value: string) {
	return formatCalendarDate(value);
}


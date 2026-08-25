import { Plus } from "lucide-react";

import { PermissionGate } from "@/components/shared/permission-gate";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { permissions } from "@/lib/permissions";
import type { KanbanFilters } from "@/types/kanban";
import type { ProjectMember } from "@/types/project";

const emptyFilters: KanbanFilters = {
	assignee: "",
	priority: "",
	label: "",
	dueDate: "",
};

interface KanbanToolbarProps {
	members: ProjectMember[];
	filters: KanbanFilters;
	onFiltersChange: (
		updater: (current: KanbanFilters) => KanbanFilters,
	) => void;
	onColumnCreate: () => void;
}

export function KanbanToolbar({
	members,
	filters,
	onFiltersChange,
	onColumnCreate,
}: KanbanToolbarProps) {
	const activeFilterCount = [
		filters.assignee,
		filters.priority,
		filters.label,
		filters.dueDate,
	].filter(Boolean).length;

	return (
		<Card className="p-6">
			<div className="flex flex-col gap-5">
				<div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
					<div>
						<p className="mb-1 text-[11px] font-[700] uppercase tracking-[0.08em] text-ops">
							Board proyek
						</p>
						<h4 className="text-[20px] font-[700] text-text-primary leading-tight">
							Ruang kerja task proyek
						</h4>
						<p className="mt-1 max-w-3xl text-[13px] text-text-secondary">
							Kelola task proyek dalam satu board, pindahkan kartu antar tahap
							kerja, dan gunakan filter tanpa keluar dari halaman ini.
						</p>
					</div>

					<PermissionGate permission={permissions.operationalColumnManage}>
						<div className="flex w-full flex-col items-stretch gap-3 xl:w-auto xl:items-end">
							<Button
								variant="ops"
								className="xl:self-end"
								onClick={onColumnCreate}
								type="button">
								<Plus className="h-4 w-4" />
								Kolom baru
							</Button>
						</div>
					</PermissionGate>
				</div>

				<div className="grid gap-4 xl:grid-cols-[repeat(4,minmax(0,1fr))_auto]">
					<select
						className="flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10"
						onChange={(event) =>
							onFiltersChange((current) => ({
								...current,
								assignee: event.target.value,
							}))
						}
						value={filters.assignee}>
						<option value="">Semua PIC</option>
						{members.map((member) => (
							<option
								key={member.user_id}
								value={member.user_id}>
								{member.full_name || member.user_email || member.user_id}
							</option>
						))}
					</select>
					<select
						className="flex h-[44px] w-full rounded-[6px] border border-transparent bg-surface-muted px-3 py-2 text-[14px] text-text-primary shadow-sm outline-none transition-all placeholder:text-text-tertiary focus-visible:border-ops focus-visible:bg-surface focus-visible:ring-4 focus-visible:ring-ops/10"
						onChange={(event) =>
							onFiltersChange((current) => ({
								...current,
								priority: event.target.value,
							}))
						}
						value={filters.priority}>
						<option value="">Semua prioritas</option>
						<option value="low">Rendah</option>
						<option value="medium">Medium</option>
						<option value="high">Tinggi</option>
						<option value="critical">Kritis</option>
					</select>
					<Input
						className="h-[44px] focus-visible:border-ops focus-visible:ring-ops/10"
						onChange={(event) =>
							onFiltersChange((current) => ({
								...current,
								label: event.target.value,
							}))
						}
						placeholder="Filter label"
						value={filters.label}
					/>
					<Input
						className="h-[44px] focus-visible:border-ops focus-visible:ring-ops/10"
						onChange={(event) =>
							onFiltersChange((current) => ({
								...current,
								dueDate: event.target.value,
							}))
						}
						type="date"
						value={filters.dueDate}
					/>
					<Button
						onClick={() => onFiltersChange(() => emptyFilters)}
						variant="secondary"
						className="h-[44px]">
						Reset {activeFilterCount > 0 ? `(${activeFilterCount})` : ""}
					</Button>
				</div>
			</div>
		</Card>
	);
}

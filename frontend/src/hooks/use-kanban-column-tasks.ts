import { useInfiniteQuery } from "@tanstack/react-query";

import {
	kanbanKeys,
	kanbanPageSize,
	listKanbanTasks,
	type KanbanTaskQuery,
} from "@/services/operational-kanban";
import type { KanbanTask } from "@/types/kanban";

export interface ColumnTasksResult {
	tasks: KanbanTask[];
	total: number;
	hasMore: boolean;
	isLoading: boolean;
	isFetchingMore: boolean;
	error: Error | null;
	fetchMore: () => void;
}

export function useKanbanColumnTasks(
	projectId: string,
	columnId: string,
	filters: KanbanTaskQuery,
): ColumnTasksResult {
	const query = useInfiniteQuery({
		queryKey: kanbanKeys.columnTasks(projectId, columnId, filters),
		initialPageParam: 0,
		queryFn: ({ pageParam }) =>
			listKanbanTasks(projectId, {
				...filters,
				columnId,
				limit: kanbanPageSize,
				offset: pageParam,
			}),
		getNextPageParam: (lastPage) =>
			lastPage.has_more ? lastPage.offset + lastPage.items.length : undefined,
	});

	const pages = query.data?.pages ?? [];

	return {
		tasks: pages.flatMap((page) => page.items),
		total: pages[0]?.total ?? 0,
		hasMore: Boolean(query.hasNextPage),
		isLoading: query.isLoading,
		isFetchingMore: query.isFetchingNextPage,
		error: query.error as Error | null,
		fetchMore: () => {
			if (query.hasNextPage && !query.isFetchingNextPage) {
				void query.fetchNextPage();
			}
		},
	};
}

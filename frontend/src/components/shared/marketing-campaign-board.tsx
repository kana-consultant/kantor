import { useEffect, useState } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  closestCorners,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { CalendarDays, Paperclip } from "lucide-react";

import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { Card } from "@/components/ui/card";
import { formatIDR } from "@/lib/currency";
import { formatCalendarDate } from "@/lib/date";
import { channelMeta } from "@/lib/marketing";
import { cn } from "@/lib/utils";
import type { Campaign, CampaignColumn } from "@/types/marketing";

interface MarketingCampaignBoardProps {
  columns: CampaignColumn[];
  onCampaignOpen: (campaign: Campaign) => void;
  onMoveCampaign: (campaignId: string, columnId: string, position: number) => Promise<void>;
}

type CampaignDragData = { type: "campaign"; campaign: Campaign };
type ColumnDropData = { type: "column"; column: CampaignColumn };

export function MarketingCampaignBoard({
  columns,
  onCampaignOpen,
  onMoveCampaign,
}: MarketingCampaignBoardProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));
  const [boardColumns, setBoardColumns] = useState(columns);
  const [activeCampaign, setActiveCampaign] = useState<Campaign | null>(null);
  const [snapshot, setSnapshot] = useState<CampaignColumn[] | null>(null);

  useEffect(() => {
    setBoardColumns(columns);
  }, [columns]);

  function handleDragStart(event: DragStartEvent) {
    const data = event.active.data.current;
    if (!isCampaignDragData(data)) {
      return;
    }

    setActiveCampaign(data.campaign);
    setSnapshot(boardColumns);
  }

  function handleDragEnd(event: DragEndEvent) {
    const data = event.active.data.current;
    if (!event.over || !isCampaignDragData(data)) {
      if (snapshot) {
        setBoardColumns(snapshot);
      }
      setActiveCampaign(null);
      setSnapshot(null);
      return;
    }

    const nextColumns = moveCampaignInMemory(
      snapshot ?? boardColumns,
      data.campaign.id,
      event.over.id.toString(),
      event.over.data.current,
    );
    const nextLocation = nextColumns ? locateCampaign(nextColumns, data.campaign.id) : null;
    const previousLocation = snapshot ? locateCampaign(snapshot, data.campaign.id) : null;

    setActiveCampaign(null);

    if (!nextColumns || !nextLocation || !previousLocation) {
      if (snapshot) {
        setBoardColumns(snapshot);
      }
      setSnapshot(null);
      return;
    }

    if (
      nextLocation.columnId === previousLocation.columnId &&
      nextLocation.position === previousLocation.position
    ) {
      setSnapshot(null);
      return;
    }

    setBoardColumns(nextColumns);
    void onMoveCampaign(data.campaign.id, nextLocation.columnId, nextLocation.position).catch(() => {
      if (snapshot) {
        setBoardColumns(snapshot);
      }
    });

    setSnapshot(null);
  }

  return (
    <DndContext
      collisionDetection={closestCorners}
      onDragEnd={handleDragEnd}
      onDragStart={handleDragStart}
      sensors={sensors}
    >
      <div className="-mx-1 overflow-x-auto px-1 pb-3">
        <div className="flex min-w-max gap-3 md:gap-5">
          {boardColumns.map((column) => (
            <CampaignLane column={column} key={column.id} onCampaignOpen={onCampaignOpen} />
          ))}
        </div>
      </div>

      <DragOverlay>{activeCampaign ? <CampaignOverlay campaign={activeCampaign} /> : null}</DragOverlay>
    </DndContext>
  );
}

function CampaignLane({
  column,
  onCampaignOpen,
}: {
  column: CampaignColumn;
  onCampaignOpen: (campaign: Campaign) => void;
}) {
  const droppable = useDroppable({
    id: column.id,
    data: { type: "column", column } satisfies ColumnDropData,
  });

  return (
    <div className="w-[min(82vw,320px)] shrink-0 md:w-[320px]">
      <Card
        className={cn(
          "flex min-h-[420px] flex-col border border-border bg-surface-muted p-3 shadow-sm transition-all md:min-h-[500px] md:p-4",
          droppable.isOver && "border-mkt/50 shadow-card bg-mkt/5",
        )}
        ref={droppable.setNodeRef}
      >
        <div className="flex items-center justify-between gap-3 border-b border-border pb-3">
          <div className="flex items-center gap-3">
            <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: column.color ?? "#94A3B8" }} />
            <div>
              <h4 className="font-[600] text-[14px] text-text-primary">{column.name}</h4>
              <p className="text-[12px] font-[500] text-text-tertiary">{column.campaigns?.length ?? 0} campaigns</p>
            </div>
          </div>
        </div>

        <div className="mt-4 flex-1 space-y-3 overflow-y-auto pr-1">
          <SortableContext items={(column.campaigns ?? []).map((campaign) => campaign.id)} strategy={verticalListSortingStrategy}>
            {(column.campaigns ?? []).map((campaign) => (
              <CampaignCard campaign={campaign} key={campaign.id} onClick={() => onCampaignOpen(campaign)} />
            ))}
          </SortableContext>

          {(column.campaigns ?? []).length === 0 ? (
            <div className="rounded-[12px] border border-dashed border-border bg-background/50 px-4 py-10 text-center text-[13px] font-[500] text-text-tertiary">
              Drop campaign here.
            </div>
          ) : null}
        </div>
      </Card>
    </div>
  );
}

function CampaignCard({
  campaign,
  onClick,
}: {
  campaign: Campaign;
  onClick: () => void;
}) {
  const sortable = useSortable({
    id: campaign.id,
    data: { type: "campaign", campaign } satisfies CampaignDragData,
  });
  const style = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  };

  const channel = channelMeta(campaign.channel);
  const ChannelIcon = channel.icon;

  return (
    <div ref={sortable.setNodeRef} style={style}>
      <Card
        {...sortable.attributes}
        {...sortable.listeners}
        className={cn(
          "cursor-grab border border-border bg-background p-4 shadow-sm transition-all hover:border-mkt/30 hover:shadow-card active:cursor-grabbing group",
          sortable.isDragging && "opacity-60 ring-2 ring-mkt",
        )}
        onClick={onClick}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className={cn("inline-flex items-center gap-1.5 rounded-[6px] border px-2 py-0.5 text-[11px] font-[700] uppercase tracking-wider", channel.badgeClassName)}>
              <ChannelIcon className="h-3.5 w-3.5" />
              <span>{channel.label}</span>
            </div>
            <h5 className="mt-2 text-[14px] font-[600] text-text-primary leading-tight">{campaign.name}</h5>
          </div>
          <span className="opacity-0 group-hover:opacity-100 transition-opacity rounded-[6px] border border-border bg-surface-muted px-2 py-1 text-[10px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
            Move
          </span>
        </div>

        {campaign.description ? <p className="mt-2 line-clamp-2 text-[12px] text-text-secondary leading-relaxed">{campaign.description}</p> : null}

        <div className="mt-4 grid gap-2 text-[13px] border-t border-border pt-3">
          <div className="flex items-center justify-between gap-3">
            <span className="text-text-secondary">Budget</span>
            <span className="font-[600] text-text-primary">{formatIDR(campaign.budget_amount)}</span>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-text-secondary">PIC</span>
            <div className="flex items-center gap-2">
              <ProtectedAvatar
                alt={campaign.pic_employee_name ?? "Campaign PIC"}
                avatarUrl={campaign.pic_avatar_url}
                className="h-6 w-6 shadow-sm ring-2 ring-background"
                fallbackClassName="bg-mkt text-white"
                iconClassName="h-3 w-3"
              />
              <span className="max-w-[9rem] truncate text-[12px] font-[500] text-text-primary">{campaign.pic_employee_name ?? "Unassigned"}</span>
            </div>
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-3 text-[11px] font-[600] text-text-tertiary uppercase tracking-wider">
          <span className="inline-flex items-center gap-1.5">
            <CalendarDays className="h-3.5 w-3.5" />
            {formatCalendarDate(campaign.start_date)} - {formatCalendarDate(campaign.end_date)}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <Paperclip className="h-3.5 w-3.5" />
            {campaign.attachment_count}
          </span>
        </div>
      </Card>
    </div>
  );
}

function CampaignOverlay({ campaign }: { campaign: Campaign }) {
  const channel = channelMeta(campaign.channel);
  const ChannelIcon = channel.icon;

  return (
    <div className="w-[min(82vw,320px)] md:w-[320px]">
      <Card className="border-mkt shadow-2xl p-4 rotate-2 rounded-[12px]">
        <div className={cn("inline-flex items-center gap-1.5 rounded-[6px] border px-2 py-0.5 text-[11px] font-[700] uppercase tracking-wider", channel.badgeClassName)}>
          <ChannelIcon className="h-3.5 w-3.5" />
          <span>{channel.label}</span>
        </div>
        <p className="mt-2 text-[14px] font-[600] text-text-primary leading-tight">{campaign.name}</p>
      </Card>
    </div>
  );
}

function isCampaignDragData(value: unknown): value is CampaignDragData {
  return typeof value === "object" && value !== null && "type" in value && value.type === "campaign";
}

function isColumnDropData(value: unknown): value is ColumnDropData {
  return typeof value === "object" && value !== null && "type" in value && value.type === "column";
}

function locateCampaign(columns: CampaignColumn[], campaignId: string) {
  for (const column of columns) {
    const index = (column.campaigns ?? []).findIndex((campaign) => campaign.id === campaignId);
    if (index >= 0) {
      return { columnId: column.id, position: index + 1 };
    }
  }
  return null;
}

function moveCampaignInMemory(columns: CampaignColumn[], campaignId: string, overId: string, overData: unknown) {
  const nextColumns = columns.map((column) => ({
    ...column,
    campaigns: [...(column.campaigns ?? [])],
  }));

  let sourceColumnIndex = -1;
  let sourceCampaignIndex = -1;
  let movingCampaign: Campaign | undefined;

  nextColumns.forEach((column, columnIndex) => {
    const itemIndex = (column.campaigns ?? []).findIndex((campaign) => campaign.id === campaignId);
    if (itemIndex >= 0) {
      sourceColumnIndex = columnIndex;
      sourceCampaignIndex = itemIndex;
      movingCampaign = column.campaigns?.[itemIndex];
    }
  });

  if (sourceColumnIndex < 0 || sourceCampaignIndex < 0 || !movingCampaign) {
    return null;
  }

  nextColumns[sourceColumnIndex]!.campaigns?.splice(sourceCampaignIndex, 1);

  let destinationColumnIndex = sourceColumnIndex;
  let destinationIndex = nextColumns[sourceColumnIndex]!.campaigns?.length ?? 0;

  if (isCampaignDragData(overData)) {
    destinationColumnIndex = nextColumns.findIndex((column) => (column.campaigns ?? []).some((campaign) => campaign.id === overData.campaign.id));
    const destinationCampaigns = nextColumns[destinationColumnIndex]?.campaigns ?? [];
    const overIndex = destinationCampaigns.findIndex((campaign) => campaign.id === overData.campaign.id);
    destinationIndex = overIndex >= 0 ? overIndex : destinationCampaigns.length;
  } else if (isColumnDropData(overData)) {
    destinationColumnIndex = nextColumns.findIndex((column) => column.id === overData.column.id);
    destinationIndex = nextColumns[destinationColumnIndex]?.campaigns?.length ?? 0;
  } else {
    destinationColumnIndex = nextColumns.findIndex((column) => column.id === overId);
    destinationIndex = nextColumns[destinationColumnIndex]?.campaigns?.length ?? 0;
  }

  if (destinationColumnIndex < 0) {
    return columns;
  }

  nextColumns[destinationColumnIndex]!.campaigns?.splice(destinationIndex, 0, {
    ...movingCampaign,
    column_id: nextColumns[destinationColumnIndex]!.id,
    column_name: nextColumns[destinationColumnIndex]!.name,
    column_color: nextColumns[destinationColumnIndex]!.color,
  });

  return nextColumns;
}

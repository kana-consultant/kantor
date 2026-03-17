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
  type DragOverEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { CalendarDays, Paperclip } from "lucide-react";

import { Card } from "@/components/ui/card";
import { formatIDR } from "@/lib/currency";
import { channelMeta, initials } from "@/lib/marketing";
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

  function handleDragOver(event: DragOverEvent) {
    if (!event.over) {
      return;
    }

    const data = event.active.data.current;
    if (!isCampaignDragData(data)) {
      return;
    }

    const nextColumns = moveCampaignInMemory(
      boardColumns,
      data.campaign.id,
      event.over.id.toString(),
      event.over.data.current,
    );

    if (nextColumns) {
      setBoardColumns(nextColumns);
    }
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

    const nextLocation = locateCampaign(boardColumns, data.campaign.id);
    const previousLocation = snapshot ? locateCampaign(snapshot, data.campaign.id) : null;

    setActiveCampaign(null);

    if (!nextLocation || !previousLocation) {
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
      onDragOver={handleDragOver}
      onDragStart={handleDragStart}
      sensors={sensors}
    >
      <div className="overflow-x-auto pb-3">
        <div className="flex min-w-max gap-5">
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
    <div className="w-[23rem] shrink-0">
      <Card
        className={cn(
          "flex min-h-[36rem] flex-col rounded-[28px] border-border/70 bg-muted/50 p-4 transition",
          droppable.isOver && "border-primary/30 shadow-panel",
        )}
        ref={droppable.setNodeRef}
      >
        <div className="flex items-center justify-between gap-3 border-b border-border/70 pb-3">
          <div className="flex items-center gap-3">
            <span className="h-3 w-3 rounded-full" style={{ backgroundColor: column.color ?? "#94A3B8" }} />
            <div>
              <h4 className="font-semibold">{column.name}</h4>
              <p className="text-xs text-muted-foreground">{column.campaigns?.length ?? 0} campaigns</p>
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
            <div className="rounded-[22px] border border-dashed border-border/70 bg-background/70 px-4 py-10 text-center text-sm text-muted-foreground">
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
          "cursor-grab rounded-[24px] border-border/60 bg-background/95 p-4 transition hover:border-primary/25 hover:shadow-panel active:cursor-grabbing",
          sortable.isDragging && "opacity-60",
        )}
        onClick={onClick}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className={cn("inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold", channel.badgeClassName)}>
              <ChannelIcon className="h-3.5 w-3.5" />
              <span>{channel.label}</span>
            </div>
            <h5 className="mt-3 text-base font-semibold">{campaign.name}</h5>
          </div>
          <span className="rounded-full border border-border/70 px-3 py-1 text-[11px] uppercase tracking-[0.2em] text-muted-foreground">
            Move
          </span>
        </div>

        {campaign.description ? <p className="mt-3 line-clamp-2 text-sm text-muted-foreground">{campaign.description}</p> : null}

        <div className="mt-4 grid gap-2 text-sm">
          <div className="flex items-center justify-between gap-3">
            <span className="text-muted-foreground">Budget</span>
            <span className="font-semibold">{formatIDR(campaign.budget_amount)}</span>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-muted-foreground">PIC</span>
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/15 text-[11px] font-semibold uppercase text-primary">
                {initials(campaign.pic_employee_name)}
              </div>
              <span className="max-w-[9rem] truncate text-xs font-medium">{campaign.pic_employee_name ?? "Unassigned"}</span>
            </div>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1.5">
            <CalendarDays className="h-3.5 w-3.5" />
            {new Date(campaign.start_date).toLocaleDateString()} - {new Date(campaign.end_date).toLocaleDateString()}
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
    <div className="w-[20rem]">
      <Card className="border-primary/30 bg-card p-4 shadow-2xl">
        <div className={cn("inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold", channel.badgeClassName)}>
          <ChannelIcon className="h-3.5 w-3.5" />
          <span>{channel.label}</span>
        </div>
        <p className="mt-3 font-semibold">{campaign.name}</p>
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

  nextColumns[sourceColumnIndex].campaigns?.splice(sourceCampaignIndex, 1);

  let destinationColumnIndex = sourceColumnIndex;
  let destinationIndex = nextColumns[sourceColumnIndex].campaigns?.length ?? 0;

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

  nextColumns[destinationColumnIndex].campaigns?.splice(destinationIndex, 0, {
    ...movingCampaign,
    column_id: nextColumns[destinationColumnIndex].id,
    column_name: nextColumns[destinationColumnIndex].name,
    column_color: nextColumns[destinationColumnIndex].color,
  });

  return nextColumns;
}

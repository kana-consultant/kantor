import { useEffect, useId, useState } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  closestCorners,
  useDroppable,
  useSensor,
  useSensors,
  type Announcements,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Paperclip, Plus } from "lucide-react";

import { ChannelBadge } from "@/components/shared/channel-badge";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { Button } from "@/components/ui/button";
import { formatIDR } from "@/lib/currency";
import { channelMeta, formatShortPeriod, initials } from "@/lib/marketing";
import { cn } from "@/lib/utils";
import type { Campaign, CampaignColumn, CampaignStatus } from "@/types/marketing";

interface MarketingCampaignBoardProps {
  columns: CampaignColumn[];
  // False for users without marketing:campaign:edit: cards cannot be dragged.
  canEdit: boolean;
  // Set while a filter hides some cards: lane id -> a position past the last
  // card of that lane on the unfiltered board. Positions inside the filtered
  // lanes are not the real positions, so a move then goes to the end of the
  // target lane and reordering within a lane is disabled.
  laneEndPositions?: Record<string, number> | null;
  onCampaignOpen: (campaign: Campaign) => void;
  onMoveCampaign: (campaignId: string, columnId: string, position: number) => Promise<void>;
  // Set for users who may create campaigns: the lanes where work starts get a
  // "+" that opens the create form with that stage. Completed, Archived and
  // custom lanes (no stage) never get it.
  onCreateInLane?: (stage: CampaignStatus) => void;
}

const laneCreateStages: readonly CampaignStatus[] = ["ideation", "planning", "in_production", "live"];

// Cards are moved with the pointer only (keyboard users change the stage in
// the drawer or the edit form), so there are no keyboard drag instructions;
// the live region still says, in Indonesian, what a drag did.
const dragAnnouncements: Announcements = {
  onDragStart: ({ active }) => `Memindahkan ${dragName(active.data.current)}`,
  onDragOver: ({ active, over }) =>
    over ? `${dragName(active.data.current)} di atas ${dropName(over.data.current)}` : `${dragName(active.data.current)} tidak di atas kolom`,
  onDragEnd: ({ active, over }) =>
    over ? `${dragName(active.data.current)} dilepas di ${dropName(over.data.current)}` : `${dragName(active.data.current)} dikembalikan`,
  onDragCancel: ({ active }) => `Batal memindahkan ${dragName(active.data.current)}`,
};

function dragName(data: unknown) {
  return isCampaignDragData(data) ? data.campaign.name : "campaign";
}

function dropName(data: unknown) {
  if (isColumnDropData(data)) {
    return `kolom ${data.column.name}`;
  }
  if (isCampaignDragData(data)) {
    return data.campaign.column_name ? `kolom ${data.campaign.column_name}` : data.campaign.name;
  }
  return "kolom";
}

type CampaignDragData = { type: "campaign"; campaign: Campaign };
type ColumnDropData = { type: "column"; column: CampaignColumn };

export function MarketingCampaignBoard({
  columns,
  canEdit,
  laneEndPositions,
  onCampaignOpen,
  onMoveCampaign,
  onCreateInLane,
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
      (laneEndPositions || nextLocation.position === previousLocation.position)
    ) {
      // Same lane while filtering: the visible order is not the real order,
      // so reordering is not offered; put the card back.
      if (snapshot) {
        setBoardColumns(snapshot);
      }
      setSnapshot(null);
      return;
    }

    const position = laneEndPositions ? (laneEndPositions[nextLocation.columnId] ?? 1) : nextLocation.position;

    setBoardColumns(laneEndPositions ? moveCampaignToEnd(nextColumns, data.campaign.id, nextLocation.columnId) : nextColumns);
    void onMoveCampaign(data.campaign.id, nextLocation.columnId, position).catch(() => {
      if (snapshot) {
        setBoardColumns(snapshot);
      }
    });

    setSnapshot(null);
  }

  return (
    <DndContext
      accessibility={{ announcements: dragAnnouncements, screenReaderInstructions: { draggable: "" } }}
      collisionDetection={closestCorners}
      onDragEnd={handleDragEnd}
      onDragStart={handleDragStart}
      sensors={sensors}
    >
      <div className="-mx-1 overflow-x-auto px-1 pb-3">
        <div className="flex min-w-max gap-3">
          {boardColumns.map((column) => (
            <CampaignLane
              canEdit={canEdit}
              column={column}
              isFiltering={Boolean(laneEndPositions)}
              key={column.id}
              onCampaignOpen={onCampaignOpen}
              onCreateInLane={onCreateInLane}
            />
          ))}
        </div>
      </div>

      <DragOverlay>{activeCampaign ? <CampaignOverlay campaign={activeCampaign} /> : null}</DragOverlay>
    </DndContext>
  );
}

function CampaignLane({
  canEdit,
  column,
  isFiltering,
  onCampaignOpen,
  onCreateInLane,
}: {
  canEdit: boolean;
  column: CampaignColumn;
  isFiltering: boolean;
  onCampaignOpen: (campaign: Campaign) => void;
  onCreateInLane?: (stage: CampaignStatus) => void;
}) {
  const droppable = useDroppable({
    id: column.id,
    data: { type: "column", column } satisfies ColumnDropData,
  });
  const campaigns = column.campaigns ?? [];
  const stage = column.stage ?? null;
  const createHere = stage && onCreateInLane && laneCreateStages.includes(stage) ? () => onCreateInLane(stage) : null;

  return (
    <section
      aria-label={`${column.name}, ${campaigns.length} campaign`}
      className={cn(
        "flex min-h-[280px] w-[min(82vw,288px)] shrink-0 flex-col rounded-xl border border-transparent bg-surface-muted p-3 transition-colors duration-150 dark:bg-surface-muted/40 md:min-h-[360px] md:w-[288px]",
        droppable.isOver && "border-mkt/40 bg-mkt/5",
      )}
      ref={droppable.setNodeRef}
    >
      <div className="flex min-h-8 items-center justify-between gap-2 px-1">
        <div className="flex min-w-0 items-center gap-2">
          {/* The ring keeps a dark or light lane colour visible in both themes. */}
          <span
            aria-hidden="true"
            className={cn("h-2 w-2 shrink-0 rounded-full ring-1 ring-text-tertiary", !column.color && "bg-text-tertiary")}
            style={column.color ? { backgroundColor: column.color } : undefined}
          />
          <h4 className="truncate font-sans text-sm font-semibold tracking-normal text-text-primary">{column.name}</h4>
          <span className="text-sm tabular-nums text-text-secondary">{campaigns.length}</span>
        </div>
        {createHere ? (
          <Button
            aria-label={`Tambah campaign di ${column.name}`}
            className="h-8 w-8 shrink-0 rounded-lg"
            onClick={createHere}
            size="icon"
            title={`Tambah campaign di ${column.name}`}
            type="button"
            variant="ghost"
          >
            <Plus className="h-4 w-4" />
          </Button>
        ) : null}
      </div>

      <div className="mt-2 flex flex-1 flex-col gap-2">
        <SortableContext items={campaigns.map((campaign) => campaign.id)} strategy={verticalListSortingStrategy}>
          {campaigns.map((campaign) => (
            <CampaignCard campaign={campaign} canEdit={canEdit} key={campaign.id} onOpen={() => onCampaignOpen(campaign)} />
          ))}
        </SortableContext>

        {campaigns.length === 0 ? (
          <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-border px-3 py-6 text-center">
            <p className="text-[13px] text-text-secondary">{isFiltering ? "Tidak ada yang cocok" : "Belum ada campaign"}</p>
            {createHere && !isFiltering ? (
              <Button onClick={createHere} size="xs" type="button" variant="ghost">
                <Plus className="h-4 w-4" />
                Campaign
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
    </section>
  );
}

function CampaignCard({
  campaign,
  canEdit,
  onOpen,
}: {
  campaign: Campaign;
  canEdit: boolean;
  onOpen: () => void;
}) {
  const sortable = useSortable({
    id: campaign.id,
    data: { type: "campaign", campaign } satisfies CampaignDragData,
    disabled: !canEdit,
  });
  const style = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  };
  const titleId = useId();
  const summaryId = useId();

  return (
    <div ref={sortable.setNodeRef} style={style}>
      <div
        {...sortable.attributes}
        {...sortable.listeners}
        // A plain button that opens the details: named by the title,
        // described by the facts on the card. dnd-kit's "sortable" role
        // description and keyboard-drag instructions do not apply (cards are
        // dragged with the pointer only), and the card always opens on
        // Enter/click, even when it cannot be dragged.
        aria-describedby={summaryId}
        aria-disabled={undefined}
        aria-labelledby={titleId}
        aria-roledescription={undefined}
        // A stable id, so focus can return to the card after the drawer
        // closes even if the card was re-rendered in another lane.
        id={`campaign-card-${campaign.id}`}
        className={cn(
          // relative: the sr-only summary is absolutely positioned and must
          // stay inside the board's scroll box, not widen <main>.
          "relative rounded-xl border border-border bg-surface p-3 outline-none transition-[border-color,box-shadow] duration-150 hover:border-mkt/30 hover:shadow-card focus-visible:shadow-focus",
          canEdit ? "cursor-grab active:cursor-grabbing" : "cursor-pointer",
          sortable.isDragging && "opacity-50",
        )}
        onClick={onOpen}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onOpen();
          }
        }}
      >
        <CampaignCardContent campaign={campaign} titleId={titleId} />
        <span className="sr-only" id={summaryId}>
          {cardSummary(campaign)}
        </span>
      </div>
    </div>
  );
}

function cardSummary(campaign: Campaign) {
  return [
    channelMeta(campaign.channel).label,
    formatIDR(campaign.budget_amount),
    formatShortPeriod(campaign.start_date, campaign.end_date),
    campaign.pic_employee_name ? `PIC ${campaign.pic_employee_name}` : "Belum ada PIC",
    campaign.attachment_count > 0 ? `${campaign.attachment_count} lampiran` : null,
  ]
    .filter(Boolean)
    .join(", ");
}

function CampaignCardContent({ campaign, titleId }: { campaign: Campaign; titleId?: string }) {
  return (
    <>
      <div className="flex items-start justify-between gap-2">
        <ChannelBadge channel={campaign.channel} />
        <PicAvatar avatarUrl={campaign.pic_avatar_url} name={campaign.pic_employee_name} />
      </div>
      <h5
        className="mt-2 line-clamp-2 font-sans text-sm font-semibold leading-snug tracking-normal text-text-primary [overflow-wrap:anywhere]"
        id={titleId}
        title={campaign.name}
      >
        {campaign.name}
      </h5>
      <p className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[12px] text-text-secondary">
        <span className="font-mono tabular-nums">{formatIDR(campaign.budget_amount)}</span>
        <span className="whitespace-nowrap">{formatShortPeriod(campaign.start_date, campaign.end_date)}</span>
        {campaign.attachment_count > 0 ? (
          <span className="inline-flex items-center gap-0.5" title={`${campaign.attachment_count} lampiran`}>
            <Paperclip aria-hidden="true" className="h-3 w-3" />
            <span className="sr-only">Lampiran:</span>
            {campaign.attachment_count}
          </span>
        ) : null}
      </p>
    </>
  );
}

// The PIC as a small avatar. Nobody assigned reads as an empty dashed circle,
// so it cannot be mistaken for a person.
function PicAvatar({ name, avatarUrl }: { name?: string | null; avatarUrl?: string | null }) {
  if (!name) {
    return (
      <span className="h-6 w-6 shrink-0 rounded-full border border-dashed border-text-secondary" title="Belum ada PIC">
        <span className="sr-only">Belum ada PIC</span>
      </span>
    );
  }

  return (
    <span
      aria-label={`PIC: ${name}`}
      className="inline-flex h-6 w-6 shrink-0 items-center justify-center overflow-hidden rounded-full bg-surface-muted text-[12px] font-semibold leading-none text-text-primary ring-1 ring-border"
      role="img"
      title={`PIC: ${name}`}
    >
      {avatarUrl ? (
        <ProtectedAvatar alt="" avatarUrl={avatarUrl} className="h-6 w-6" iconClassName="h-3 w-3" />
      ) : (
        initials(name).slice(0, 2)
      )}
    </span>
  );
}

function CampaignOverlay({ campaign }: { campaign: Campaign }) {
  return (
    <div className="w-[min(82vw,264px)] md:w-[264px]">
      <div className="rotate-1 rounded-xl border border-mkt/40 bg-surface p-3 shadow-xl">
        <CampaignCardContent campaign={campaign} />
      </div>
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

function moveCampaignToEnd(columns: CampaignColumn[], campaignId: string, columnId: string) {
  return columns.map((column) => {
    if (column.id !== columnId) {
      return column;
    }
    const campaigns = column.campaigns ?? [];
    const moving = campaigns.find((campaign) => campaign.id === campaignId);
    if (!moving) {
      return column;
    }
    return { ...column, campaigns: [...campaigns.filter((campaign) => campaign.id !== campaignId), moving] };
  });
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

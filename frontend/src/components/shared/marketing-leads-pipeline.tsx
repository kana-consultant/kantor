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
import { Clock3 } from "lucide-react";

import { Card } from "@/components/ui/card";
import { formatIDR } from "@/lib/currency";
import { formatLeadStatus, initials, leadSourceMeta } from "@/lib/marketing";
import { cn } from "@/lib/utils";
import type { Lead, LeadPipelineColumn } from "@/types/marketing";

interface MarketingLeadsPipelineProps {
  columns: LeadPipelineColumn[];
  onLeadOpen: (lead: Lead) => void;
  onMoveLead: (leadId: string, pipelineStatus: string) => Promise<void>;
}

type LeadDragData = { type: "lead"; lead: Lead };
type LeadColumnData = { type: "column"; column: LeadPipelineColumn };

export function MarketingLeadsPipeline({
  columns,
  onLeadOpen,
  onMoveLead,
}: MarketingLeadsPipelineProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));
  const [boardColumns, setBoardColumns] = useState(columns);
  const [activeLead, setActiveLead] = useState<Lead | null>(null);
  const [snapshot, setSnapshot] = useState<LeadPipelineColumn[] | null>(null);

  useEffect(() => {
    setBoardColumns(columns);
  }, [columns]);

  function handleDragStart(event: DragStartEvent) {
    const data = event.active.data.current;
    if (!isLeadDragData(data)) {
      return;
    }
    setActiveLead(data.lead);
    setSnapshot(boardColumns);
  }

  function handleDragOver(event: DragOverEvent) {
    if (!event.over) {
      return;
    }

    const data = event.active.data.current;
    if (!isLeadDragData(data)) {
      return;
    }

    const nextColumns = moveLeadInMemory(
      boardColumns,
      data.lead.id,
      event.over.id.toString(),
      event.over.data.current,
    );
    if (nextColumns) {
      setBoardColumns(nextColumns);
    }
  }

  function handleDragEnd(event: DragEndEvent) {
    const data = event.active.data.current;
    if (!event.over || !isLeadDragData(data)) {
      if (snapshot) {
        setBoardColumns(snapshot);
      }
      setActiveLead(null);
      setSnapshot(null);
      return;
    }

    const nextLocation = locateLead(boardColumns, data.lead.id);
    const previousLocation = snapshot ? locateLead(snapshot, data.lead.id) : null;
    setActiveLead(null);

    if (!nextLocation || !previousLocation) {
      if (snapshot) {
        setBoardColumns(snapshot);
      }
      setSnapshot(null);
      return;
    }

    if (nextLocation.status === previousLocation.status) {
      setSnapshot(null);
      return;
    }

    void onMoveLead(data.lead.id, nextLocation.status).catch(() => {
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
            <LeadLane column={column} key={column.status} onLeadOpen={onLeadOpen} />
          ))}
        </div>
      </div>

      <DragOverlay>{activeLead ? <LeadOverlay lead={activeLead} /> : null}</DragOverlay>
    </DndContext>
  );
}

function LeadLane({
  column,
  onLeadOpen,
}: {
  column: LeadPipelineColumn;
  onLeadOpen: (lead: Lead) => void;
}) {
  const droppable = useDroppable({
    id: column.status,
    data: { type: "column", column } satisfies LeadColumnData,
  });

  return (
    <div className="w-[21rem] shrink-0">
      <Card
        className={cn(
          "flex min-h-[34rem] flex-col rounded-[28px] border-border/70 bg-muted/50 p-4 transition",
          droppable.isOver && "border-primary/30 shadow-panel",
        )}
        ref={droppable.setNodeRef}
      >
        <div className="flex items-center justify-between gap-3 border-b border-border/70 pb-3">
          <div>
            <h4 className="font-semibold">{column.label}</h4>
            <p className="text-xs text-muted-foreground">{column.leads.length} leads</p>
          </div>
        </div>

        <div className="mt-4 flex-1 space-y-3 overflow-y-auto pr-1">
          <SortableContext items={column.leads.map((lead) => lead.id)} strategy={verticalListSortingStrategy}>
            {column.leads.map((lead) => (
              <LeadCard key={lead.id} lead={lead} onClick={() => onLeadOpen(lead)} />
            ))}
          </SortableContext>

          {column.leads.length === 0 ? (
            <div className="rounded-[22px] border border-dashed border-border/70 bg-background/70 px-4 py-10 text-center text-sm text-muted-foreground">
              Drop lead here.
            </div>
          ) : null}
        </div>
      </Card>
    </div>
  );
}

function LeadCard({ lead, onClick }: { lead: Lead; onClick: () => void }) {
  const sortable = useSortable({
    id: lead.id,
    data: { type: "lead", lead } satisfies LeadDragData,
  });
  const style = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  };
  const source = leadSourceMeta(lead.source_channel);
  const SourceIcon = source.icon;

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
            <div className={cn("inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold", source.badgeClassName)}>
              <SourceIcon className="h-3.5 w-3.5" />
              <span>{source.label}</span>
            </div>
            <h5 className="mt-3 text-base font-semibold">{lead.name}</h5>
          </div>
          <span className="rounded-full border border-border/70 px-3 py-1 text-[11px] uppercase tracking-[0.2em] text-muted-foreground">
            Move
          </span>
        </div>

        <div className="mt-3 space-y-1 text-sm text-muted-foreground">
          {lead.phone ? <p>{lead.phone}</p> : null}
          {lead.email ? <p>{lead.email}</p> : null}
          {!lead.phone && !lead.email ? <p>No contact info</p> : null}
        </div>

        <div className="mt-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/15 text-[11px] font-semibold uppercase text-primary">
              {initials(lead.assigned_to_name)}
            </div>
            <span className="max-w-[8rem] truncate text-xs font-medium">{lead.assigned_to_name ?? "Unassigned"}</span>
          </div>
          <span className="text-xs font-semibold">{formatIDR(lead.estimated_value)}</span>
        </div>

        <div className="mt-4 flex items-center gap-2 text-xs text-muted-foreground">
          <Clock3 className="h-3.5 w-3.5" />
          <span>{new Date(lead.updated_at).toLocaleString("id-ID")}</span>
        </div>
      </Card>
    </div>
  );
}

function LeadOverlay({ lead }: { lead: Lead }) {
  return (
    <div className="w-[20rem]">
      <Card className="border-primary/30 bg-card p-4 shadow-2xl">
        <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{formatLeadStatus(lead.pipeline_status)}</p>
        <p className="mt-3 font-semibold">{lead.name}</p>
      </Card>
    </div>
  );
}

function isLeadDragData(value: unknown): value is LeadDragData {
  return typeof value === "object" && value !== null && "type" in value && value.type === "lead";
}

function isLeadColumnData(value: unknown): value is LeadColumnData {
  return typeof value === "object" && value !== null && "type" in value && value.type === "column";
}

function locateLead(columns: LeadPipelineColumn[], leadId: string) {
  for (const column of columns) {
    const found = column.leads.find((lead) => lead.id === leadId);
    if (found) {
      return { status: column.status };
    }
  }
  return null;
}

function moveLeadInMemory(columns: LeadPipelineColumn[], leadId: string, overId: string, overData: unknown) {
  const nextColumns = columns.map((column) => ({
    ...column,
    leads: [...column.leads],
  }));

  let sourceColumnIndex = -1;
  let sourceLeadIndex = -1;
  let movingLead: Lead | undefined;

  nextColumns.forEach((column, columnIndex) => {
    const itemIndex = column.leads.findIndex((lead) => lead.id === leadId);
    if (itemIndex >= 0) {
      sourceColumnIndex = columnIndex;
      sourceLeadIndex = itemIndex;
      movingLead = column.leads[itemIndex];
    }
  });

  if (sourceColumnIndex < 0 || sourceLeadIndex < 0 || !movingLead) {
    return null;
  }

  nextColumns[sourceColumnIndex].leads.splice(sourceLeadIndex, 1);

  let destinationColumnIndex = sourceColumnIndex;
  let destinationIndex = nextColumns[sourceColumnIndex].leads.length;

  if (isLeadDragData(overData)) {
    destinationColumnIndex = nextColumns.findIndex((column) => column.leads.some((lead) => lead.id === overData.lead.id));
    const destinationLeads = nextColumns[destinationColumnIndex]?.leads ?? [];
    const overIndex = destinationLeads.findIndex((lead) => lead.id === overData.lead.id);
    destinationIndex = overIndex >= 0 ? overIndex : destinationLeads.length;
  } else if (isLeadColumnData(overData)) {
    destinationColumnIndex = nextColumns.findIndex((column) => column.status === overData.column.status);
    destinationIndex = nextColumns[destinationColumnIndex]?.leads.length ?? 0;
  } else {
    destinationColumnIndex = nextColumns.findIndex((column) => column.status === overId);
  }

  if (destinationColumnIndex < 0) {
    return columns;
  }

  nextColumns[destinationColumnIndex].leads.splice(destinationIndex, 0, {
    ...movingLead,
    pipeline_status: nextColumns[destinationColumnIndex].status,
  });

  return nextColumns;
}

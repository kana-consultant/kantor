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
import { Clock3 } from "lucide-react";

import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { StatusBadge } from "@/components/shared/status-badge";
import { Card } from "@/components/ui/card";
import { formatIDR } from "@/lib/currency";
import { leadSourceMeta } from "@/lib/marketing";
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

    const nextColumns = moveLeadInMemory(
      snapshot ?? boardColumns,
      data.lead.id,
      event.over.id.toString(),
      event.over.data.current,
    );
    const nextLocation = nextColumns ? locateLead(nextColumns, data.lead.id) : null;
    const previousLocation = snapshot ? locateLead(snapshot, data.lead.id) : null;
    setActiveLead(null);

    if (!nextColumns || !nextLocation || !previousLocation) {
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

    setBoardColumns(nextColumns);
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
      onDragStart={handleDragStart}
      sensors={sensors}
    >
      <div className="-mx-1 overflow-x-auto px-1 pb-3">
        <div className="flex min-w-max gap-3 md:gap-5">
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
    <div className="w-[min(82vw,320px)] shrink-0 md:w-[320px]">
      <Card
        className={cn(
          "flex min-h-[420px] flex-col border border-border bg-surface-muted p-3 shadow-sm transition-all md:min-h-[500px] md:p-4",
          droppable.isOver && "border-mkt/50 shadow-card bg-mkt/5",
        )}
        ref={droppable.setNodeRef}
      >
        <div className="flex items-center justify-between gap-3 border-b border-border pb-3">
          <div>
            <h4 className="font-[600] text-[14px] text-text-primary">{column.label}</h4>
            <p className="mt-1 text-[12px] font-[500] text-text-tertiary">{column.leads.length} leads</p>
          </div>
        </div>

        <div className="mt-4 flex-1 space-y-3 overflow-y-auto pr-1">
          <SortableContext items={column.leads.map((lead) => lead.id)} strategy={verticalListSortingStrategy}>
            {column.leads.map((lead) => (
              <LeadCard key={lead.id} lead={lead} onClick={() => onLeadOpen(lead)} />
            ))}
          </SortableContext>

          {column.leads.length === 0 ? (
            <div className="rounded-[12px] border border-dashed border-border bg-background/50 px-4 py-10 text-center text-[13px] font-[500] text-text-tertiary">
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
          "cursor-grab border border-border bg-background p-4 shadow-sm transition-all hover:border-mkt/30 hover:shadow-card active:cursor-grabbing group",
          sortable.isDragging && "opacity-60 ring-2 ring-mkt",
        )}
        onClick={onClick}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className={cn("inline-flex items-center gap-1.5 rounded-[6px] border px-2 py-0.5 text-[11px] font-[700] uppercase tracking-wider", source.badgeClassName)}>
              <SourceIcon className="h-3.5 w-3.5" />
              <span>{source.label}</span>
            </div>
            <h5 className="mt-2 text-[14px] font-[600] text-text-primary leading-tight">{lead.name}</h5>
          </div>
          <span className="opacity-0 group-hover:opacity-100 transition-opacity rounded-[6px] border border-border bg-surface-muted px-2 py-1 text-[10px] font-[700] uppercase tracking-[0.08em] text-text-tertiary">
            Move
          </span>
        </div>

        <div className="mt-3 space-y-1 text-[12px] text-text-secondary leading-relaxed">
          {lead.phone ? <p>{lead.phone}</p> : null}
          {lead.email ? <p>{lead.email}</p> : null}
          {!lead.phone && !lead.email ? <p>No contact info</p> : null}
        </div>

        <div className="mt-4 flex items-center justify-between gap-3 border-t border-border pt-3">
          <div className="flex items-center gap-2">
            <ProtectedAvatar
              alt={lead.assigned_to_name ?? "Lead assignee"}
              avatarUrl={lead.assigned_to_avatar}
              className="h-6 w-6 shadow-sm ring-2 ring-background"
              fallbackClassName="bg-mkt text-white"
              iconClassName="h-3 w-3"
            />
            <span className="max-w-[8rem] truncate text-[12px] font-[500] text-text-primary">{lead.assigned_to_name ?? "Unassigned"}</span>
          </div>
          <span className="text-[13px] font-[600] text-text-primary">{formatIDR(lead.estimated_value)}</span>
        </div>

        <div className="mt-3 flex items-center gap-1.5 text-[11px] font-[600] text-text-tertiary uppercase tracking-wider">
          <Clock3 className="h-3 w-3" />
          <span>{new Date(lead.updated_at).toLocaleString("id-ID")}</span>
        </div>
      </Card>
    </div>
  );
}

function LeadOverlay({ lead }: { lead: Lead }) {
  return (
    <div className="w-[min(82vw,320px)] md:w-[320px]">
      <Card className="border-mkt shadow-2xl p-4 rotate-2 rounded-[12px]">
        <StatusBadge status={lead.pipeline_status} variant="lead-status" />
        <p className="mt-2 text-[14px] font-[600] text-text-primary leading-tight">{lead.name}</p>
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

  nextColumns[sourceColumnIndex]!.leads.splice(sourceLeadIndex, 1);

  let destinationColumnIndex = sourceColumnIndex;
  let destinationIndex = nextColumns[sourceColumnIndex]!.leads.length;

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

  nextColumns[destinationColumnIndex]!.leads.splice(destinationIndex, 0, {
    ...movingLead,
    pipeline_status: nextColumns[destinationColumnIndex]!.status,
  });

  return nextColumns;
}

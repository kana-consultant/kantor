import { useEffect, useMemo, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  CalendarClock,
  Download,
  ScrollText,
  Shield,
  UserRound,
} from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/shared/data-table";
import { EmptyState } from "@/components/shared/empty-state";
import { JsonDiffViewer } from "@/components/shared/json-diff-viewer";
import { ProtectedAvatar } from "@/components/shared/protected-avatar";
import { OverviewSkeleton } from "@/components/shared/skeletons";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Tooltip } from "@/components/ui/tooltip";
import { useRBAC } from "@/hooks/use-rbac";
import { permissions } from "@/lib/permissions";
import { ensureModuleAccess, ensurePermission } from "@/lib/rbac";
import {
  adminAuditKeys,
  exportAuditLogs,
  getAuditLogSummary,
  listAuditLogUsers,
  listAuditLogs,
} from "@/services/admin-audit";
import { toast } from "@/stores/toast-store";
import type {
  AuditLogFilters,
  AuditLogItem,
  AuditLogSummaryBucket,
} from "@/types/audit-log";

const defaultFilters: AuditLogFilters = {
  module: "",
  action: "",
  userId: "",
  resource: "",
  resourceId: "",
  dateFrom: "",
  dateTo: "",
  search: "",
  page: 1,
  perPage: 20,
};

const moduleOptions = [
  { value: "", label: "Semua Modul" },
  { value: "operational", label: "Operasional" },
  { value: "hris", label: "HRIS" },
  { value: "marketing", label: "Marketing" },
  { value: "admin", label: "Admin" },
];

const actionOptions = [
  { value: "", label: "Semua Aksi" },
  { value: "create", label: "Create" },
  { value: "update", label: "Update" },
  { value: "delete", label: "Delete" },
  { value: "view", label: "View" },
  { value: "approve", label: "Approve" },
  { value: "reject", label: "Reject" },
  { value: "login", label: "Login" },
  { value: "logout", label: "Logout" },
];

export const Route = createFileRoute("/_authenticated/admin/audit-logs")({
  beforeLoad: async () => {
    await ensureModuleAccess("admin");
    await ensurePermission(permissions.adminAuditLogView);
  },
  component: AuditLogsPage,
});

function AuditLogsPage() {
  const { hasPermission } = useRBAC();
  const canExport = hasPermission(permissions.adminAuditLogExport);
  const [filters, setFilters] = useState<AuditLogFilters>(defaultFilters);
  const [searchInput, setSearchInput] = useState("");
  const [expandedLogId, setExpandedLogId] = useState<string | null>(null);

  useEffect(() => {
    const timeoutId = window.setTimeout(() => {
      setFilters((current) =>
        current.search === searchInput.trim()
          ? current
          : { ...current, search: searchInput.trim(), page: 1 },
      );
    }, 250);

    return () => window.clearTimeout(timeoutId);
  }, [searchInput]);

  const summaryQuery = useQuery({
    queryKey: adminAuditKeys.summary(),
    queryFn: getAuditLogSummary,
  });

  const usersQuery = useQuery({
    queryKey: adminAuditKeys.users(),
    queryFn: () => listAuditLogUsers(),
  });

  const logsQuery = useQuery({
    queryKey: adminAuditKeys.logList(filters),
    queryFn: () => listAuditLogs(filters),
  });

  const summary = summaryQuery.data;
  const mostActiveModule = summary?.by_module?.[0] ?? null;
  const mostActiveUser = summary?.top_users?.[0] ?? null;

  const columns: Array<DataTableColumn<AuditLogItem>> = [
    {
      id: "timestamp",
      header: "Timestamp",
      accessor: "created_at",
      sortable: true,
      widthClassName: "min-w-[180px]",
      cell: (row) => (
        <Tooltip content={formatFullDate(row.created_at)} side="left">
          <span className="text-sm text-text-primary">{formatRelativeTime(row.created_at)}</span>
        </Tooltip>
      ),
    },
    {
      id: "user",
      header: "User",
      sortable: true,
      accessor: "user_name",
      widthClassName: "min-w-[200px]",
      cell: (row) => (
        <div className="flex items-center gap-3">
          <AvatarChip name={row.user_name} avatarUrl={row.user_avatar_url} />
          <div className="space-y-0.5">
            <p className="font-medium text-text-primary">{row.user_name}</p>
            <p className="text-xs text-text-secondary">{row.user_email || "-"}</p>
          </div>
        </div>
      ),
    },
    {
      id: "module",
      header: "Module",
      accessor: "module",
      sortable: true,
      cell: (row) => <StatusBadge status={row.module} variant="module" />,
    },
    {
      id: "action",
      header: "Action",
      accessor: "action",
      sortable: true,
      cell: (row) => <StatusBadge status={row.action} variant="audit-action" />,
    },
    {
      id: "resource",
      header: "Resource",
      accessor: "resource",
      sortable: true,
      cell: (row) => (
        <div>
          <p className="font-medium text-text-primary">{humanizeResource(row.resource)}</p>
          <p className="text-xs text-text-secondary">{row.resource}</p>
        </div>
      ),
    },
    {
      id: "resource_id",
      header: "Resource ID",
      accessor: "resource_id",
      sortable: true,
      widthClassName: "min-w-[170px]",
      cell: (row) => {
        const resourceLink = resolveResourceLink(row);
        if (!resourceLink) {
          return <code className="font-mono text-[12px] text-text-secondary">{row.resource_id}</code>;
        }

        return (
          <a
            className="font-mono text-[12px] text-module underline-offset-4 hover:underline"
            href={resourceLink}
          >
            {row.resource_id}
          </a>
        );
      },
    },
    {
      id: "ip_address",
      header: "IP Address",
      accessor: "ip_address",
      sortable: true,
      widthClassName: "min-w-[140px]",
      cell: (row) => <span className="font-mono text-[12px] text-text-secondary">{row.ip_address || "-"}</span>,
    },
  ];

  const downloadExport = async () => {
    try {
      const result = await exportAuditLogs(filters);
      const url = window.URL.createObjectURL(result.blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = result.filename || "audit-logs.csv";
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.URL.revokeObjectURL(url);
    } catch (error) {
      toast.error(
        "Gagal export audit log",
        error instanceof Error ? error.message : undefined,
      );
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.08em] text-error">Admin</p>
          <h1 className="mt-2 font-display text-[28px] font-[700] tracking-[-0.02em] text-text-primary">
            Audit Logs
          </h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-text-secondary">
            Telusuri perubahan penting lintas modul, lihat diff JSON, dan export jejak audit untuk investigasi atau compliance review.
          </p>
        </div>
        {canExport ? (
          <Button onClick={() => void downloadExport()} type="button" variant="outline">
            <Download className="h-4 w-4" />
            Export CSV
          </Button>
        ) : null}
      </div>

      {summaryQuery.isLoading ? (
        <OverviewSkeleton />
      ) : (
        <div className="grid gap-4 lg:grid-cols-4">
          <StatCard
            icon={ScrollText}
            label="Total Logs Today"
            tone="error"
            value={String(summary?.total_today ?? 0)}
          />
          <StatCard
            icon={CalendarClock}
            label="Total Logs This Week"
            tone="info"
            value={String(summary?.total_week ?? 0)}
          />
          <StatCard
            icon={Shield}
            label="Most Active Module"
            tone={moduleTone(mostActiveModule?.key)}
            value={moduleLabel(mostActiveModule)}
            helper={mostActiveModule ? `${mostActiveModule.count} log tercatat` : "Belum ada data"}
          />
          <StatCard
            icon={UserRound}
            label="Most Active User"
            tone="warning"
            value={mostActiveUser?.user_name ?? "-"}
            helper={mostActiveUser ? `${mostActiveUser.count} aksi | ${mostActiveUser.user_email || "tanpa email"}` : "Belum ada data"}
          />
        </div>
      )}

      <Card className="p-6">
        <div className="grid gap-4 xl:grid-cols-[180px_180px_180px_180px_220px_minmax(0,1fr)]">
          <select
            className="field-select"
            onChange={(event) =>
              setFilters((current) => ({ ...current, module: event.target.value, page: 1 }))
            }
            value={filters.module}
          >
            {moduleOptions.map((option) => (
              <option key={option.value || "all"} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <select
            className="field-select"
            onChange={(event) =>
              setFilters((current) => ({ ...current, action: event.target.value, page: 1 }))
            }
            value={filters.action}
          >
            {actionOptions.map((option) => (
              <option key={option.value || "all"} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <Input
            onChange={(event) =>
              setFilters((current) => ({ ...current, dateFrom: event.target.value, page: 1 }))
            }
            type="date"
            value={filters.dateFrom}
          />
          <Input
            onChange={(event) =>
              setFilters((current) => ({ ...current, dateTo: event.target.value, page: 1 }))
            }
            type="date"
            value={filters.dateTo}
          />
          <select
            className="field-select"
            onChange={(event) =>
              setFilters((current) => ({ ...current, userId: event.target.value, page: 1 }))
            }
            value={filters.userId}
          >
            <option value="">Semua User</option>
            {(usersQuery.data ?? []).map((user) => (
              <option key={user.user_id} value={user.user_id}>
                {user.user_name} {user.user_email ? `(${user.user_email})` : ""}
              </option>
            ))}
          </select>
          <Input
            onChange={(event) => setSearchInput(event.target.value)}
            placeholder="Cari action, resource, nilai JSON, atau user"
            value={searchInput}
          />
        </div>
      </Card>

      {logsQuery.isLoading && !logsQuery.data ? (
        <OverviewSkeleton />
      ) : (
        <DataTable
          columns={columns}
          data={logsQuery.data?.items ?? []}
          emptyDescription="Belum ada audit log yang cocok dengan filter aktif."
          emptyTitle="Audit log tidak ditemukan"
          getRowId={(row) => row.id}
          loading={logsQuery.isFetching && !logsQuery.data}
          onRowClick={(row) => setExpandedLogId((current) => (current === row.id ? null : row.id))}
          pagination={
            logsQuery.data?.meta
              ? {
                  page: logsQuery.data.meta.page,
                  perPage: logsQuery.data.meta.per_page,
                  total: logsQuery.data.meta.total,
                  onPageChange: (page) =>
                    setFilters((current) => ({
                      ...current,
                      page,
                    })),
                }
              : undefined
          }
          renderExpandedRow={(row) => (
            <div className="space-y-4">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <p className="text-sm font-semibold text-text-primary">
                    Detail perubahan untuk {humanizeResource(row.resource)}
                  </p>
                  <p className="text-xs text-text-secondary">
                    {row.user_name} | {formatFullDate(row.created_at)} | {row.module}/{row.action}
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <StatusBadge status={row.module} variant="module" />
                  <StatusBadge status={row.action} variant="audit-action" />
                </div>
              </div>
              <JsonDiffViewer oldValue={row.old_value ?? null} newValue={row.new_value ?? null} />
            </div>
          )}
          selectedRowId={expandedLogId}
        />
      )}
    </div>
  );
}

function AvatarChip({
  name,
  avatarUrl,
}: {
  name: string;
  avatarUrl?: string | null;
}) {
  return (
    <ProtectedAvatar
      alt={name}
      avatarUrl={avatarUrl}
      className="h-6 w-6 border border-border"
    />
  );
}

function humanizeResource(resource: string) {
  return resource
    .replace(/_/g, " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

function resolveResourceLink(item: AuditLogItem) {
  if (item.module === "operational" && item.resource === "project") {
    return `/operational/projects/${item.resource_id}`;
  }
  if (item.module === "hris" && item.resource === "employee") {
    return `/hris/employees/${item.resource_id}`;
  }
  if (item.module === "hris" && item.resource === "reimbursement") {
    return `/hris/reimbursements/${item.resource_id}`;
  }

  return null;
}

function formatFullDate(value: string) {
  return new Date(value).toLocaleString("id-ID", {
    dateStyle: "medium",
    timeStyle: "medium",
  });
}

function formatRelativeTime(value: string) {
  const target = new Date(value).getTime();
  const now = Date.now();
  const diffSeconds = Math.round((target - now) / 1000);
  const formatter = new Intl.RelativeTimeFormat("id-ID", { numeric: "auto" });

  const ranges: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ["day", 60 * 60 * 24],
    ["hour", 60 * 60],
    ["minute", 60],
    ["second", 1],
  ];

  for (const [unit, seconds] of ranges) {
    if (Math.abs(diffSeconds) >= seconds || unit === "second") {
      return formatter.format(Math.round(diffSeconds / seconds), unit);
    }
  }

  return formatFullDate(value);
}

function moduleTone(moduleID?: string) {
  switch (moduleID) {
    case "operational":
      return "ops" as const;
    case "hris":
      return "hr" as const;
    case "marketing":
      return "mkt" as const;
    case "admin":
      return "error" as const;
    default:
      return "info" as const;
  }
}

function moduleLabel(bucket: AuditLogSummaryBucket | null) {
  if (!bucket) {
    return "-";
  }

  return bucket.key === "operational"
    ? "Operasional"
    : bucket.key === "hris"
      ? "HRIS"
      : bucket.key === "marketing"
        ? "Marketing"
        : bucket.key === "admin"
          ? "Admin"
          : bucket.key;
}

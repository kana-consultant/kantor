# Architecture Overview

This document describes the high-level architecture of KANTOR.

## System Architecture

```
                    ┌─────────────────────────────┐
                    │       Browser / Client       │
                    │  React 19 + TanStack Router  │
                    └──────────────┬───────────────┘
                                   │
                    ┌──────────────▼───────────────┐
                    │       nginx (port 3000)       │
                    │  Static files + /api/ proxy   │
                    └──────────────┬───────────────┘
                                   │
                    ┌──────────────▼───────────────┐
                    │      Go Backend (port 8080)   │
                    │  Chi router + JWT + RBAC      │
                    └──────────────┬───────────────┘
                                   │
                    ┌──────────────▼───────────────┐
                    │    PostgreSQL 16 (port 5432)  │
                    │  Row-Level Security (RLS)     │
                    └──────────────────────────────┘
```

## Backend Layers

```
Request → Middleware → Handler → Service → Repository → PostgreSQL
```

| Layer | Responsibility | Example |
|-------|---------------|---------|
| **Middleware** | Auth, RBAC, tenant resolution, CORS, logging | `middleware/auth.go` |
| **Handler** | HTTP parsing, input validation, JSON responses | `handler/operational/projects.go` |
| **Service** | Business logic, orchestration, error classification | `service/operational/projects.go` |
| **Repository** | Pure SQL queries via pgx (no ORM) | `repository/operational/projects.go` |

Each layer only communicates with the layer directly below it. Handlers never talk to repositories directly.

## Multi-Tenancy

KANTOR uses PostgreSQL Row-Level Security for tenant isolation:

1. **TenantMiddleware** resolves the tenant from the `Host` header
2. A dedicated `*pgxpool.Conn` is acquired from the pool
3. The GUC `app.current_tenant` is set to the resolved tenant ID
4. All subsequent queries are automatically filtered by RLS policies
5. On request completion, `RESET ALL` clears the session state and the connection is returned to the pool

```
Host: kantor.company-a.com
  → Resolve domain → tenant_id = "abc-123"
  → SET app.current_tenant = 'abc-123'
  → All queries filtered by RLS (tenant_id = current_setting('app.current_tenant'))
  → RESET ALL on response
```

Global tables (without RLS): `tenants`, `tenant_domains`, `modules`, `permissions`

## RBAC Model

```
User ──┬── Module Assignment (user_module_roles)
       │       │
       │       ├── Role (per module)
       │       │     └── Permissions (role_permissions)
       │       │
       │       └── Permission format: module:resource:action
       │             e.g., operational:project:create
       │                   hris:employee:view
       │                   marketing:lead:manage
       │
       └── Super Admin flag (bypasses all checks)
```

Permission evaluation:
1. Super admins bypass all permission checks
2. Non-super-admins need both module assignment AND specific permission
3. Permissions are cached in memory with configurable TTL (default: 5 min)

## Frontend Architecture

```
TanStack Router (file-based routing)
  └── Route guards (beforeLoad: check auth + permissions)
       └── Page components
            ├── TanStack Query (server state, caching, mutations)
            ├── Zustand stores (client state: sidebar, theme)
            ├── React Hook Form + Zod (form handling + validation)
            └── shadcn/ui + Tailwind CSS (styling)
```

Key patterns:
- **PermissionGate** / **ModuleGate** components for conditional UI rendering
- **useRBAC()** hook for imperative permission checks
- **Optimistic updates** via TanStack Query cache manipulation (kanban drag-and-drop)
- **Notifications SSE stream** for lightweight realtime invalidation of the topbar notification center
- **File-based routing** — route structure mirrors the URL structure

## Notifications

```
Backend
  ├── GET /api/v1/notifications/unread-count
  ├── GET /api/v1/notifications
  ├── PATCH /api/v1/notifications/:id/read
  └── GET /api/v1/notifications/stream  (SSE)

Frontend
  ├── Topbar bell dropdown
  ├── SSE reconnect loop for realtime invalidation
  ├── Browser Notification API when tab is inactive
  └── Deep-link navigation to the related resource
```

The notification center uses REST endpoints for listing and marking reads, plus a server-sent events stream for realtime invalidation. Browser notifications are opt-in and depend on the user's browser permission state.

## Data Security

| Data Type | Protection |
|-----------|-----------|
| Passwords | bcrypt hashing |
| JWT tokens | HMAC-SHA256 signing |
| Salaries & bonuses | AES-256-GCM encryption at rest |
| Sensitive operations | Audit logging |
| Tenant data | PostgreSQL RLS isolation |
| File uploads | Tenant-prefixed storage paths |

Encryption key rotation is supported: set `DATA_ENCRYPTION_KEY_PREVIOUS` to the old key, and the app will re-encrypt on access.

## Chrome Extension (Activity Tracker)

```
Chrome Extension (Manifest V3)
  ├── Service Worker → 30s heartbeat to backend
  ├── Content Script → Page title extraction
  ├── Popup UI → Session start/stop, status display
  └── Options Page → Excluded domains, API settings

Backend:
  ├── POST /tracker/heartbeat → Record activity entry
  ├── POST /tracker/sessions/start → Start tracking session
  ├── GET /tracker/my-activity → Personal activity overview
  └── GET /tracker/team-activity → Team-wide analytics
```

Privacy: tracking requires explicit user consent (opt-in). Data retention is configurable via `TRACKER_RETENTION_DAYS`.
Access tokens are stored in `chrome.storage.session`, while persistent tracker settings remain in `chrome.storage.local`. Host permissions are limited to `http://*/*` and `https://*/*`.

## WhatsApp Integration

```
KANTOR Backend
  └── WAHA Client (HTTP)
       └── WAHA Server
            └── WhatsApp Web API

Features:
  ├── Template-based messages with variable placeholders
  ├── Per-tenant WA settings stored in `tenant_wa_configs`
  ├── Scheduled broadcasts (cron-based, configured per tenant)
  ├── Automated reminders (task due, overdue, weekly digest)
  ├── DB-backed daily rate limiting + random delays
  └── In-app notification sync for relevant WA events
```

## Database

Migrations are managed by [golang-migrate](https://github.com/golang-migrate/migrate) and run automatically on application startup.

Key conventions:
- UUIDs as primary keys (generated by PostgreSQL `gen_random_uuid()`)
- `created_at` / `updated_at` timestamps on all tables
- `tenant_id` column with RLS on all tenant-scoped tables
- Soft deletes via `is_active` flags (no hard deletes on critical data)
- Indexes on foreign keys and commonly filtered columns

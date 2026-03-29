# Production Deployment Guide

This guide covers deploying KANTOR to a production environment using Docker Compose.

> See also: [Architecture Overview](architecture.md) | [Contributing](../CONTRIBUTING.md)

## Prerequisites

- Docker Engine 24+ and Docker Compose v2
- A domain name with DNS configured
- TLS certificate (or a reverse proxy like Caddy/Traefik that handles ACME)

## 1. Environment Configuration

Copy `.env.example` and configure all values for production:

```bash
cp .env.example .env
```

### Required Secrets

| Variable | How to generate | Notes |
|----------|----------------|-------|
| `POSTGRES_PASSWORD` | `openssl rand -base64 32` | Database password |
| `JWT_SECRET` | `openssl rand -base64 48` | Must be at least 32 characters in production |
| `DATA_ENCRYPTION_KEY` | `openssl rand -base64 32` | Used for AES-256-GCM encryption of sensitive data (salaries, etc.) |

### Environment Variable Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `APP_ENV` | No | `development` | Set to `production` for production deployments |
| `PORT` | No | `8080` | Backend HTTP port (internal to Docker network) |
| `DATABASE_URL` | Yes | — | PostgreSQL connection string (overridden by Docker Compose) |
| `JWT_SECRET` | Yes | — | HMAC signing key for JWTs. Rejected if set to `change-me` in production |
| `DATA_ENCRYPTION_KEY` | Yes | — | Encryption key for sensitive data at rest |
| `DATA_ENCRYPTION_KEY_PREVIOUS` | No | — | Previous encryption key for key rotation (see below) |
| `UPLOADS_DIR` | No | `uploads` | File upload directory (overridden by Docker Compose) |
| `JWT_ACCESS_EXPIRY` | No | `15m` | Access token TTL |
| `JWT_REFRESH_EXPIRY` | No | `168h` | Refresh token TTL |
| `CORS_ORIGINS` | No | `http://localhost:3000` | Comma-separated allowed origins |
| `APP_URL` | No | `http://localhost:3000` | Public base URL used for deep links in WA messages and notifications |
| `POSTGRES_DB` | No | `internal_platform` | Database name |
| `POSTGRES_USER` | No | `dev` | Database user |
| `POSTGRES_PASSWORD` | Yes | — | Database password |

### WhatsApp Broadcast Configuration

WA Broadcast runtime settings are configured per tenant from the application UI and stored in `tenant_wa_configs`.
This includes:

- WAHA API URL
- WAHA API key
- Session name
- Daily limit
- Delay range
- Reminder and digest schedules

Production environment variables no longer carry those tenant-level WA settings. The backend still uses `APP_URL` to generate links included in messages and notifications.

### Seed Users

**Disable seed users in production:**

```env
SEED_SUPERADMIN_ENABLED=false
SEED_DEMO_USERS_ENABLED=false
```

If you need an initial admin account, enable the super admin seed on first deploy only, then disable it.

### Encryption Key Rotation

To rotate `DATA_ENCRYPTION_KEY`:

1. Set `DATA_ENCRYPTION_KEY_PREVIOUS` to the current key
2. Set `DATA_ENCRYPTION_KEY` to a new value
3. Deploy — the app will decrypt with the previous key and re-encrypt with the new key on access

## 2. Production .env Example

```env
APP_ENV=production
POSTGRES_DB=kantor
POSTGRES_USER=kantor
POSTGRES_PASSWORD=<generated-strong-password>
JWT_SECRET=<generated-min-32-chars>
DATA_ENCRYPTION_KEY=<generated-strong-key>
UPLOADS_DIR=/app/data/uploads
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=168h
CORS_ORIGINS=https://your-domain.com
APP_URL=https://your-domain.com
SEED_SUPERADMIN_ENABLED=false
SEED_DEMO_USERS_ENABLED=false
VITE_API_BASE_URL=/api/v1
```

## 3. Docker Compose Deployment

```bash
# Build and start
docker compose up -d --build

# Check service health
docker compose ps
docker compose logs -f backend
```

The stack exposes port **3000** (nginx frontend) which proxies `/api/` requests to the backend.

### Volume Mounts

| Volume | Purpose | Backup? |
|--------|---------|---------|
| `pgdata` | PostgreSQL data | Yes — critical |
| `uploads_data` | User-uploaded files (reimbursement receipts, campaign attachments) | Yes |

## 4. HTTPS / TLS Setup

The built-in nginx listens on port 80 (HTTP). For production, place a TLS-terminating reverse proxy in front:

### Option A: Caddy (recommended for simplicity)

```Caddyfile
your-domain.com {
    reverse_proxy localhost:3000
}
```

Caddy automatically provisions and renews Let's Encrypt certificates.

### Option B: nginx with certbot

Add an outer nginx config with TLS termination that proxies to `localhost:3000`.

### CORS Update

After setting up HTTPS, update `CORS_ORIGINS` to match your domain:

```env
CORS_ORIGINS=https://your-domain.com
```

### Browser Notifications

If you want browser notifications to work reliably, serve the app over HTTPS in production.
Most browsers only allow the Notification API on secure origins (or `localhost` during development).

## 5. Database

### Connection Pooling

The backend uses `pgxpool` with sensible defaults. For high traffic, consider:

- Tuning `max_connections` in PostgreSQL
- Adding PgBouncer for connection pooling

### Backups

```bash
# Daily backup via cron
docker compose exec db pg_dump -U kantor kantor | gzip > backup-$(date +%Y%m%d).sql.gz
```

### Migrations

Migrations run automatically on application startup via `golang-migrate`. No manual migration step is required.

## 6. Health Checks

| Endpoint | Purpose | Used by |
|----------|---------|---------|
| `GET /healthz` | Liveness check — always returns 200 | Load balancer |
| `GET /readyz` | Readiness check — verifies DB connectivity | Docker healthcheck, orchestrator |

The Docker Compose healthcheck already uses `/readyz`. External monitoring should poll this endpoint.

## 7. Monitoring

- **Logs**: The backend outputs structured JSON logs in production (`APP_ENV=production`). Pipe to your log aggregator (e.g., Loki, ELK, CloudWatch).
- **Audit trail**: All state-changing operations are logged to the `audit_logs` table for compliance.
- **Alerts**: Monitor `/readyz` for downtime and set up PostgreSQL monitoring for disk/connection usage.

## 8. Security Checklist

- [ ] `APP_ENV=production`
- [ ] `JWT_SECRET` is at least 32 random characters (not `change-me`)
- [ ] `DATA_ENCRYPTION_KEY` is a strong random value
- [ ] `SEED_SUPERADMIN_ENABLED=false`
- [ ] `SEED_DEMO_USERS_ENABLED=false`
- [ ] `CORS_ORIGINS` is set to your actual domain (not `*` or `localhost`)
- [ ] TLS is terminating before traffic reaches the app
- [ ] Database is not exposed to the public internet
- [ ] Upload volume is backed up
- [ ] PostgreSQL data volume is backed up

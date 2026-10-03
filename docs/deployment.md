# Production Deployment Guide

This guide covers deploying KANTOR to a production environment with Docker Compose or
the NixOS module (`module.nix`), plus the document engine (PDF payslips and contracts),
document email and the data-protection steps that run at startup, for both paths.
What the HRIS documents feature does and how HR uses it is described in
[HRIS documents](hris-documents.md).

> See also: [Architecture Overview](architecture.md) | [Contributing](../CONTRIBUTING.md)

## Prerequisites

- Docker Engine 24+ and Docker Compose v2, **or** a NixOS host using the flake's
  `nixosModules.default` (see [NixOS Deployment](#3b-nixos-deployment-modulenix))
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
| `SOFFICE_BIN` | No | auto-detect | LibreOffice `soffice` binary used to convert generated documents to PDF. Unset = found on `PATH` (`soffice`, `libreoffice`) or in the usual install locations; a path forces one; `off` disables PDF. See [Document engine](#9-document-engine-pdf) |
| `DOCUMENT_RENDER_TIMEOUT` | No | `90s` | Time limit for one `soffice` call (a batch gets 5 s more per extra document) |
| `DOCUMENT_LO_PROFILE_DIR` | No | `<user cache dir>/kantor/lo-profile-<PORT>` | Persistent LibreOffice profile, reused so conversions skip the first-start cost. Must be writable by the backend user. The default is per user and per port (e.g. `~/Library/Caches/kantor/lo-profile-8080`, `~/.cache/kantor/lo-profile-8080`); the Docker image and the NixOS module set it explicitly |
| `DOCUMENTS_ALLOW_DOCX_SEND` | No | `false` | Allow emailing documents as DOCX when no PDF exists. Keep `false` unless you deliberately run without LibreOffice |
| `DOCUMENT_MAIL_DEV_SMTP_ADDR` | No | `localhost:1025` in development | Development only: with `APP_ENV=development` set explicitly, document email is captured over plain SMTP by Mailpit at `localhost:1025` by default. Set another `host:port` to capture elsewhere, or `off` to use the real Gmail account. Ignored in any other `APP_ENV` (documents always go to `smtp.gmail.com`) |
| `BANK_ACCOUNT_CLEAR_PLAINTEXT` | No | `false` | Opt-in, irreversible: clear the plaintext bank account column and store numbers encrypted only. Enable only after a verified backup ([First deploy](#first-deploy-of-the-hr-documents-feature)) |
| `AUDIT_SCRUB_EXISTING` | No | `false` | Opt-in, irreversible: redact the denylisted values in existing `audit_logs` rows (new rows are always redacted). Enable only after a verified backup |

None of the document keys has to be set: the defaults work locally (install LibreOffice for
PDF, run Mailpit for email capture). Admin > Settings > Email Dokumen shows the effective PDF
and delivery status read-only; executable paths, directories and SMTP hosts are never
settable from the UI.

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

Demo accounts are no longer seeded automatically at boot. Run them on demand from the backend directory:

```bash
cd backend
go run ./cmd/seed
```

The CLI seeds the demo super admin and four module accounts (defined in `internal/seed/data.go`) for every tenant in `TENANTS`. **Do not run it in production** — the fixtures use a publicly known password.

### Encryption Key Rotation

> **Do not rotate `DATA_ENCRYPTION_KEY` for now.** There is no re-encryption tool yet.
> Everything below is encrypted with this key and nothing re-encrypts it in bulk: salary
> records and bonus amounts, employee identity data (NIK, birth data, KTP address), bank
> account numbers, payslip and contract snapshots, the stored document PDFs, the Gmail
> app password and the mail-delivery API key, among others. Losing the key, or rotating it and later dropping
> `DATA_ENCRYPTION_KEY_PREVIOUS`, makes that data unreadable. Back the key up together
> with the database, separately from it.
>
> Once a rotation has happened, `DATA_ENCRYPTION_KEY_PREVIOUS` must stay set **for good**.
> Key versions are positional: the previous key is version 1 and the new key version 2, so
> dropping the previous key makes the old data unreadable *and* breaks everything written
> since the rotation (it is version-2 ciphertext, which needs the key list to stay the same).

The code supports a decrypt-only previous key, for an emergency rotation (for example a
leaked key):

1. Set `DATA_ENCRYPTION_KEY_PREVIOUS` to the current key
2. Set `DATA_ENCRYPTION_KEY` to a new value
3. Deploy — new writes use the new key; existing ciphertext is **not** rewritten and stays
   readable only through `DATA_ENCRYPTION_KEY_PREVIOUS`. Keep both keys set from then on:
   never remove or change `DATA_ENCRYPTION_KEY_PREVIOUS`, not even after documents have
   been re-rendered (see the warning above). There is no supported way back to a single
   key.

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

To produce PDF payslips and contracts, build the backend with LibreOffice (see
[Document engine](#9-document-engine-pdf)); the backend finds `/usr/bin/soffice` by itself.
LibreOffice runs inside the backend container; there is no separate converter (sidecar)
service.

If your compose `.env` has `APP_ENV=development` (as `.env.example` does), document email is
captured at `localhost:1025` *inside the backend container*, where nothing listens, so sends
fail with the Mailpit message. Either set `APP_ENV=production` for a real deployment, run a
Mailpit service and set `DOCUMENT_MAIL_DEV_SMTP_ADDR=mailpit:1025`, or set
`DOCUMENT_MAIL_DEV_SMTP_ADDR=off` to send through Gmail.

### Volume Mounts

| Volume | Purpose | Backup? |
|--------|---------|---------|
| `pgdata` | PostgreSQL data | Yes — critical |
| `uploads_data` | User-uploaded files (reimbursement receipts, campaign attachments), the tenant logo (`branding/`) and the encrypted document PDFs (`documents/`) | Yes |

## 3b. NixOS Deployment (`module.nix`)

The flake exposes a NixOS module that runs the backend as the `kantor` systemd service,
PostgreSQL on the same host, and nginx (with ACME through Cloudflare DNS when domains are
set). A minimal host configuration:

```nix
{
  imports = [ kantor.nixosModules.default ];

  services.kantor = {
    enable = true;
    envFile = "/run/secrets/kantor.env";   # JWT_SECRET, DATA_ENCRYPTION_KEY, ...
    tenants = [
      { name = "PT Contoh"; slug = "default"; domains = [ "kantor.example.co.id" ]; }
    ];
    acmeEmail = "ops@example.co.id";
    cloudflareTokenFile = "/run/secrets/cloudflare-token";
    documents.pdf.enable = true;            # default: LibreOffice + Liberation fonts
  };
}
```

- **Secrets** go in `envFile` (root-readable only, e.g. sops-nix or agenix):
  `JWT_SECRET`, `DATA_ENCRYPTION_KEY` and, if used, `DATA_ENCRYPTION_KEY_PREVIOUS`,
  `DOCUMENTS_ALLOW_DOCX_SEND`. Never put them in the Nix store.
- The module sets `APP_ENV=production`, `DATABASE_URL` (Unix socket, peer auth as
  `kantor`), `UPLOADS_DIR=/var/lib/kantor/uploads`, `TENANTS`, `CORS_ORIGINS`, `APP_URL` and,
  with `documents.pdf.enable`, `SOFFICE_BIN` and `DOCUMENT_LO_PROFILE_DIR`.
  `DOCUMENT_MAIL_DEV_SMTP_ADDR` has no effect here (production).
- The `kantor` database user owns the tables but is not a superuser, so row-level
  security applies to every query the backend makes.
- State lives in `/var/lib/kantor` (uploads, including `documents/` and `branding/`, and the
  LibreOffice profile). Back it up with the database and the key.
- Deploys are `nixos-rebuild switch` (or the CI workflow in `.github/workflows/deploy.yml`);
  migrations and the startup data tasks run when the service starts
  (`journalctl -u kantor-backend`). After `switch-to-configuration` the CI job polls the
  backend's `/readyz` on the server for up to ~3 minutes and fails if it never answers 200;
  it logs only the unit state and restart count, so read the journal on the server.

### First deploy of the HR documents feature

The first start of this release does not change or delete existing data. The migrations
`20260930*` only add tables and one column (`employees.bank_account_encrypted`), and the
startup steps only add rows:

- the bank account backfill fills the new encrypted column and leaves
  `employees.bank_account_number` (and `updated_at`) as they are; every later write stores
  the number in both columns;
- RBAC grants the eight new HR document permissions to the built-in Admin role and records
  that in a new `rbac_baseline_v2` setting row; the existing `rbac_baseline_version` row is
  left as it is (the previous release reads it and still sees its own baseline as applied).
  It never removes or rewrites existing grants, and custom roles are not touched. Existing
  role rows are never written: system roles keep the description and hierarchy level their
  admins set, and a system role emptied on purpose stays empty (only a system role that
  does not exist yet is created, with its default grants);
- new settings (`company_profile`, `rbac_baseline_v2`) are inserted only when missing;
  existing setting rows are never updated;
- existing `audit_logs` rows are not rewritten (new rows are redacted when written).

Before merging to `main`:

1. **Back up** if you can: the database and `/var/lib/kantor` (Compose: the `pgdata` and
   `uploads_data` volumes), and keep `DATA_ENCRYPTION_KEY` safe, stored apart from both.
   New payslips, contracts and the encrypted bank column are unreadable without the key.
2. **Free disk**: keep at least 4 GB free on `/nix`. LibreOffice is a ~0.7-0.85 GiB download
   and ~2.2 GiB unpacked, on top of the previous generation.
3. **envFile** must not set `SOFFICE_BIN`, `DOCUMENT_LO_PROFILE_DIR`, `APP_ENV` or
   `DOCUMENT_MAIL_DEV_SMTP_ADDR`: systemd lets `EnvironmentFile` override the module's
   values (for example `APP_ENV=development` would send document email to a Mailpit that
   does not exist). Leave `BANK_ACCOUNT_CLEAR_PLAINTEXT` and `AUDIT_SCRUB_EXISTING` unset.
4. **Host flake lock**: the host configuration that imports this module (the `nix-config`
   repository the deploy workflow clones) locks its `kantor` input to older code (currently
   2026-07-01). After the merge, update that lock (`nix flake update kantor`) or deploy only
   through the CI workflow, which builds with `--override-input kantor .`. A
   `nixos-rebuild switch` from the stale lock deploys the old code, which is a rollback.

**Roll forward only.** Once migrations `20260930*` have run, the previous release no
longer starts: it crash-loops with `no migration found for version 20260930150000`. Do not
use GitHub "Revert" on the merge, re-run an older deploy, or switch back to an older NixOS
generation (`nixos-rebuild switch --rollback`, boot menu); fix forward with a new commit.
Never run a `20260930*` down migration on production data: they drop the new tables and
everything in them (payslips, contracts, identity data, delivery log) without a check
([details](#bank-account-numbers)). The existing data keeps the format the previous
release reads (the plaintext bank column is kept), so nothing is lost while you fix forward.

After the deploy: the CI health check passes, the log shows the two "off" lines below and
`unsealed=0` per tenant ([details](#bank-account-numbers)), the PDF checks in
[Verify](#verify) pass, and outbound TCP 587 or 465 to `smtp.gmail.com` is open
([check](#10-document-email-gmail)) before the first document email.

```
msg="bank account numbers: plaintext column kept next to the encrypted copy (dual-write); ..."
msg="audit log scrub of existing rows is off (new rows are redacted on insert); ..."
```

**Opt-in follow-ups, only after a verified backup.** Two clean-ups rewrite existing rows
and cannot be undone, so they are off by default. Run them later, one at a time, each
after a backup you have restored somewhere to check it (and with `DATA_ENCRYPTION_KEY`
stored safely): set the variable to `true` in the envFile, restart the backend, check the
log, then remove the variable again (leaving it set is harmless).

| Variable | What it does |
|----------|--------------|
| `BANK_ACCOUNT_CLEAR_PLAINTEXT=true` | Clears `employees.bank_account_number` once the encrypted copy holds the same number, and from then on writes the encrypted column only. Afterwards the numbers exist only encrypted: losing the key loses them, and the down migration of `20260930150000` refuses to run. |
| `AUDIT_SCRUB_EXISTING=true` | Replaces the denylisted values in existing `audit_logs` rows with `"[redacted]"` ([details](#audit-log-redaction)). |

A value that is not a boolean is treated as off and logged as a warning; it never stops the
server.

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
  The audit writer replaces the values of a fixed key denylist (account numbers, NIK,
  salary amounts, passwords and API keys, ...) with `"[redacted]"`, and the registration
  settings audit omits the registration code (see
  [Data protection at rest](#11-data-protection-at-rest)). Values under other keys are
  stored as sent, so treat `admin:audit_log:view` as a sensitive permission.
- **Alerts**: Monitor `/readyz` for downtime and set up PostgreSQL monitoring for disk/connection usage.

## 8. Security Checklist

- [ ] `APP_ENV=production`
- [ ] `JWT_SECRET` is at least 32 random characters (not `change-me`)
- [ ] `DATA_ENCRYPTION_KEY` is a strong random value
- [ ] `cmd/seed` is **not** run against the production database
- [ ] `CORS_ORIGINS` is set to your actual domain (not `*` or `localhost`)
- [ ] TLS is terminating before traffic reaches the app
- [ ] Database is not exposed to the public internet
- [ ] Upload volume is backed up
- [ ] PostgreSQL data volume is backed up
- [ ] Document email uses a dedicated send-only Gmail / Workspace account (see [Document email](#10-document-email-gmail))
- [ ] Backups of the uploads volume are stored with the same care as the database: `documents/` holds encrypted payslips and contracts, readable with `DATA_ENCRYPTION_KEY`
- [ ] `DATA_ENCRYPTION_KEY` is backed up separately and is **not** rotated (see [Encryption Key Rotation](#encryption-key-rotation))
- [ ] After the upgrade, the startup log shows `unsealed=0` for every tenant (see [Data protection at rest](#11-data-protection-at-rest))
- [ ] The opt-in clean-ups (`BANK_ACCOUNT_CLEAR_PLAINTEXT`, `AUDIT_SCRUB_EXISTING`) are only enabled after a verified backup

## 9. Document engine (PDF)

Payslips and contracts are rendered from the embedded DOCX templates and converted to PDF
by headless LibreOffice in a background worker. Only the PDF is stored, encrypted with
`DATA_ENCRYPTION_KEY`, under `UPLOADS_DIR/documents/<tenant_id>/<kind>/` (files `0640`,
directories `0750`); the DOCX is re-rendered on demand. Tenant logos (Admin > Settings >
Profil Perusahaan) are stored processed as `UPLOADS_DIR/branding/<tenant_id>/logo.png`.

`soffice` runs with a minimal environment (it never sees `DATA_ENCRYPTION_KEY`,
`JWT_SECRET` or `DATABASE_URL`), one conversion at a time, in its own process group that
is killed on timeout, with a temporary directory that is removed after every job.

**Finding LibreOffice.** With `SOFFICE_BIN` unset the backend looks for `soffice`, then
`libreoffice` on `PATH`, then at fixed install locations: on macOS
`/Applications/LibreOffice.app/Contents/MacOS/soffice` and
`~/Applications/LibreOffice.app/...`; on Linux `/usr/bin/soffice`, `/usr/local/bin/soffice`,
`/usr/lib/libreoffice/program/soffice`, `/usr/lib64/libreoffice/program/soffice`,
`/opt/libreoffice*/program/soffice` (the lexicographically last), `/snap/bin/libreoffice`,
`/run/current-system/sw/bin/soffice`; on Windows
`C:\Program Files\LibreOffice\program\soffice.exe`. The first executable file wins.
Detection never runs anything. `SOFFICE_BIN=<path>` forces a binary (a path that is not an
executable file disables PDF with a startup warning); `SOFFICE_BIN=off` disables PDF.

Without LibreOffice documents can still be generated and downloaded as DOCX, the pages
show that the PDF converter is missing, and sending returns `409 Konverter PDF belum
tersedia` (unless `DOCUMENTS_ALLOW_DOCX_SEND=true`). Admin > Settings > Email Dokumen shows
"Konversi PDF: aktif / nonaktif" with the reason.

**Profile directory.** With `DOCUMENT_LO_PROFILE_DIR` unset the profile goes to the user
cache dir, one per backend port (`~/Library/Caches/kantor/lo-profile-8080` on macOS,
`$XDG_CACHE_HOME` or `~/.cache/kantor/lo-profile-8080` on Linux; the system temp dir if
there is no cache dir), so two backends on one machine never share a profile lock. It is
created with mode `0700`. Production (NixOS module, Docker image) sets it explicitly.

The templates use Arial. Install the Liberation fonts (Liberation Sans is metric-compatible
with Arial); with a substitute font the layout shifts and the payslip no longer fits on
one page.

### NixOS (`module.nix`)

PDF is on by default:

```nix
services.kantor.documents.pdf.enable = true;        # default
services.kantor.documents.pdf.pinFontconfig = false; # see "Verify" below
```

This adds `SOFFICE_BIN=${pkgs.libreoffice}/bin/soffice` and
`DOCUMENT_LO_PROFILE_DIR=/var/lib/kantor/lo-profile` to `kantor-backend`, installs
`pkgs.liberation_ttf` through `fonts.packages`, and creates the profile directory (`0700`,
owned by `kantor`). Set `documents.pdf.enable = false` to deploy without LibreOffice (if
LibreOffice is still installed system-wide it is auto-detected; add `SOFFICE_BIN=off` to
`envFile` to force DOCX only).

LibreOffice adds about 2.2 GiB (unpacked) to the system closure, a ~0.7-0.85 GiB download;
keep at least 4 GB free on `/nix` for the first deploy. The deploy workflow copies with
`nix copy --substitute-on-destination`, so the server downloads store paths that exist on
a binary cache (cache.nixos.org, Cachix) itself instead of receiving them from the CI
runner over ssh.

### Docker Compose

The backend image does not include LibreOffice unless you ask for it:

```bash
docker compose build --build-arg WITH_LIBREOFFICE=true backend
```

or in `docker-compose.yml`:

```yaml
  backend:
    build:
      context: .
      dockerfile: Dockerfile.backend
      args:
        WITH_LIBREOFFICE: "true"
```

This installs `libreoffice-writer`, `ttf-liberation` and `fontconfig`. Nothing has to be
added to `.env`: the backend auto-detects `/usr/bin/soffice`, and the image already sets
`DOCUMENT_LO_PROFILE_DIR=/app/data/lo-profile`. (`SOFFICE_BIN` stays available as an
override, e.g. `SOFFICE_BIN=off`.)

The profile lives inside the container, so the first conversion after a container is
recreated takes longer (tens of seconds). Mount a volume on `/app/data/lo-profile` if you
want it to survive.

### Conversion temp files

While soffice runs, the rendered DOCX and its PDF exist in plaintext in a job directory
under `$TMPDIR/kantor-docgen/` (default `/tmp`). The job directory is removed when the
conversion ends, and at startup the backend removes every directory there that a crashed
or killed process left behind (each running backend holds a lock on its own). Keep that
location off persistent disks anyway:

- NixOS: the service runs with `PrivateTmp = true`, a private `/tmp` that systemd discards
  when the service stops.
- Compose: `docker-compose.yml` mounts a `tmpfs` on the backend's `/tmp`. Keep it if you
  write your own compose file; without it `/tmp` lives in the container's writable layer
  and survives restarts.

### Verify

1. The backend logs one line at startup: `document PDF conversion enabled` with
   `soffice_source=auto|env` and the binary when PDF is on; otherwise
   `LibreOffice not found` (`not_found`), `SOFFICE_BIN is not an executable file`
   (`env_invalid`) or `disabled by SOFFICE_BIN` (`off`). Admin > Settings > Email Dokumen
   shows the same status.
2. The binary runs as the service user:
   - NixOS: `sudo -u kantor $(systemctl show kantor-backend -p Environment | tr ' ' '\n' | sed -n 's/^SOFFICE_BIN=//p') --version`
   - Compose: `docker compose exec backend /usr/bin/soffice --version`
3. The fonts are visible: `fc-list | grep -i liberation` (Compose:
   `docker compose exec backend fc-list | grep -i liberation`).
4. Generate a payslip, download its PDF and run `pdffonts slip.pdf`. Every font must be
   embedded (`emb yes`) and be `LiberationSans`. If you see DejaVu or another substitute on
   NixOS, set `services.kantor.documents.pdf.pinFontconfig = true`, which points soffice at a
   fontconfig file (`FONTCONFIG_FILE`) built by `makeFontsConf` with the Liberation fonts,
   and check again. It does not narrow or isolate the font set: the file also includes
   `/etc/fonts/conf.d` (on NixOS, every `fonts.packages` directory), a minimal DejaVu and the
   usual impure font directories. It only makes a difference on hosts with
   `fonts.fontconfig.enable = false`.
5. The payslip PDF has exactly one page (`pdfinfo slip.pdf | grep Pages`).

### Retention

Generated PDFs and the `email_deliveries` log are kept until you delete them; there is no
automatic purge job. Keep them for as long as your payroll and employment record policy
requires, and include `UPLOADS_DIR/documents` in backups together with the database and
`DATA_ENCRYPTION_KEY` (without the key the PDFs cannot be read). If the key was ever rotated,
back up and keep `DATA_ENCRYPTION_KEY_PREVIOUS` too, permanently: re-rendering documents
does not make it removable (see [Encryption Key Rotation](#encryption-key-rotation)).

## 10. Document email (Gmail)

Payslips and contracts are emailed through Gmail SMTP (`smtp.gmail.com`, fixed in code),
configured per tenant in Admin > Settings > Email Dokumen. Password reset and
notifications keep using the separate mail-delivery (Resend) settings.

1. **Use a dedicated send-only account** (for example `slip@your-company.co.id` on Google
   Workspace, or a separate Gmail account). Do not use a person's mailbox: every document
   you send stays in that account's Sent folder.
2. Turn on 2-Step Verification for the account and create an **app password**
   (Google Account > Security > App passwords). Enter the address and the app password in
   Email Dokumen; the password is stored encrypted and never shown again.
3. Port `587` (STARTTLS) is the default; `465` (implicit TLS) is available when outbound
   587 is blocked.
4. **Check outbound SMTP from the server** before the first send:

   ```bash
   openssl s_client -starttls smtp -connect smtp.gmail.com:587 -crlf -quiet </dev/null
   # or, for port 465:
   openssl s_client -connect smtp.gmail.com:465 -crlf -quiet </dev/null
   ```

   A `220 smtp.gmail.com ESMTP` / `250` greeting after the certificate chain means the
   port is open. A timeout means the hosting provider blocks outbound SMTP; ask them to
   open it or use the other port.
5. Use "Kirim email uji" in Email Dokumen to send a test message to your own login
   address.

**Sent-mail retention.** Gmail keeps a copy of every message in Sent Mail, including the
PDF attachments (payslips with salary figures, contracts with the NIK and bank account
number). Restrict who can sign in to the account, and set a retention rule: on Google
Workspace, an admin retention rule (Google Vault) or a Gmail content-compliance policy for
that account; on a consumer Gmail account, periodically delete old Sent messages. KANTOR
itself stores only the delivery log (recipient, subject, attachment names and SHA-256), not
the message bodies.

For local testing, run Mailpit (`localhost:1025`, UI `http://localhost:8025`) with
`APP_ENV=development`: document email is then captured by Mailpit instead of being sent to
Gmail, with no extra `.env` key. Email Dokumen shows a "Mode development" banner while
capture is on. Mailpit is not part of the committed `docker-compose.yml`; start it with

```bash
docker run -d --name mailpit -p 127.0.0.1:1025:1025 -p 127.0.0.1:8025:8025 axllent/mailpit
```

If Mailpit is not running, sends fail with a message telling you to start it or to set
`DOCUMENT_MAIL_DEV_SMTP_ADDR=off` to send through Gmail. `DOCUMENT_MAIL_DEV_SMTP_ADDR=host:port` captures elsewhere. With any other `APP_ENV`
(or `APP_ENV` unset) capture never happens.

## 11. Data protection at rest

### Bank account numbers

`employees.bank_account_number` used to be stored in plaintext. Migration
`20260930150000_employee_bank_account_encrypted` adds `employees.bank_account_encrypted`
(AES-256-GCM with `DATA_ENCRYPTION_KEY`). This release is additive; the backend:

- **writes** every account number to both columns (HR form, `/auth/profile`, MCP): the
  ciphertext and the trimmed plaintext, so the plaintext column stays correct for older
  code and for a restore;
- **reads** a non-blank plaintext first (both columns hold the same number; a plaintext that
  differs can only come from an older binary or a manual fix and is the newer value), and
  the ciphertext when the plaintext is empty. The API returns the number trimmed, and a
  plaintext of only whitespace (spaces, TAB, NBSP, ...) reads as no number; the stored value
  is not changed. The app has always trimmed on write, so only values written outside it
  (SQL, imports) look different. To delete a number outside the app, clear **both**
  columns: with only `bank_account_number` set to `NULL`, reads fall back to the ciphertext
  (they cannot tell that row from one cleared by the opt-in below) and the old number comes
  back. Saving the employee with an empty number clears both;
- **backfills** at every start, per tenant: each row whose ciphertext is missing, holds
  another number or cannot be decrypted gets the plaintext number encrypted, with one
  guarded `UPDATE` that applies only if both columns are unchanged. The plaintext column and
  `updated_at` are never changed; rows already in sync and blank plaintexts are left alone.
  It is idempotent and safe while users are working; a failure is logged and retried on
  the next start (reads keep working through the plaintext).
- **fails soft** on a ciphertext it cannot decrypt (for example sealed under another
  `DATA_ENCRYPTION_KEY`) when no plaintext is stored: a warning with the employee id (never
  the value) is logged and the number reads as missing, so the employee list, detail,
  exports and documents keep working, and payslip / PKWT preflight report the account
  number as missing. Saving the employee without typing a new number keeps the stored
  value; entering a new number replaces it.

API masking does not change: the number is masked (`******7890`) unless the caller holds
`hris:employee_identity:view` or it is their own record; the payslip shows the mask; the
full number appears only in the PKWT. Exports and MCP tools go through the same handlers.

After the first start on the new release, check the log for each tenant:

```
msg="bank account encryption backfill" tenant=default plaintext_kept=true candidates=10 in_sync=0 encrypted=10 superseded=0 cleared=0 skipped=0 unsealed=0 remaining_plaintext=10
```

`unsealed` must be `0` (every stored number also has its encrypted copy; a plaintext of
only whitespace is not a number and is not counted). With the
plaintext kept, `remaining_plaintext` stays the number of rows with a value in the plaintext
column; on later starts `encrypted=0` and `in_sync` counts every stored number. The log carries counts only. To
check the database directly, connect as a superuser so row-level security does not hide
the rows:

- NixOS: `sudo -u postgres psql kantor`
- Compose: `docker compose exec db sh -c 'psql -U "$POSTGRES_USER" "$POSTGRES_DB"'`
  (single quotes: the variables are expanded inside the container, where they are set)

```sql
SELECT COUNT(*) FILTER (WHERE bank_account_number IS NOT NULL)    AS plaintext,
       COUNT(*) FILTER (WHERE bank_account_encrypted IS NOT NULL) AS encrypted,
       COUNT(*) FILTER (WHERE bank_account_number ~ '[^[:space:]\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]'
                          AND bank_account_encrypted IS NULL)     AS unsealed
FROM employees;
```

The pattern matches a value with at least one character that is not whitespace, the same
rule as the backend (`btrim` would strip only spaces and count a TAB-only value).

Inactive tenants are skipped by the backfill; their rows are converted once the tenant is
active again.

**Clearing the plaintext column** is the opt-in `BANK_ACCOUNT_CLEAR_PLAINTEXT=true`, only
after a verified backup ([First deploy](#first-deploy-of-the-hr-documents-feature)). With
it, the backfill sets the plaintext to `NULL` once the ciphertext holds the same number
(blank plaintexts too; counted as `cleared`, and `remaining_plaintext` drops to `0`), and
writes store the ciphertext only. Do not drop the column by hand; a later migration drops
it once every deployment has cleared it.

**Down migrations.** Do not run any `20260930*` down migration on production data
([roll forward only](#first-deploy-of-the-hr-documents-feature)); they are for development
databases. `20260930100000` to `20260930140000` drop `email_deliveries`,
`employee_hr_profiles` (the encrypted identity data), `document_sequences`, `payslips` and
`employment_contracts` without a check, and after the deploy those tables hold the only
copy of what users entered. The down migration of `20260930150000` drops
`bank_account_encrypted` and refuses (`refusing to drop employees.bank_account_encrypted
...`) while any employee of any tenant has a ciphertext next to an empty plaintext column:
after the clear opt-in, or after a plaintext-only deletion outside the app (above). When it
refuses, golang-migrate has already recorded version `20260930140000` as dirty while the
column is still there, and the backend no longer starts (`Dirty database version
20260930140000. Fix and force version.`). Put the version back before restarting, with
the backend's `DATABASE_URL` (on NixOS the migrations are linked at
`/var/lib/kantor/migrations`, and `nix shell nixpkgs#go-migrate` provides `migrate`; from a
checkout use `-path backend/migrations`). The nixpkgs `migrate` binary panics at startup
(`failed to parse CA certificate`, a bundled driver certificate with a negative serial) unless
`GODEBUG=x509negativeserial=1` is set:

```bash
GODEBUG=x509negativeserial=1 migrate -path /var/lib/kantor/migrations -database "$DATABASE_URL" force 20260930150000
```

### Audit log redaction

Every audit entry goes through one writer that replaces, at any depth (nested objects and
arrays, keys compared case-insensitively and ignoring `_`, `-` and spaces), the value of
these keys with `"[redacted]"`, keeping the key so the entry still shows which fields
changed: `bank_account_number`, `bank_account_encrypted`, `nik`, `identity`, `base_salary`,
`allowances`, `deductions`, `amount`, `net_salary`, `smtp_password`, `api_key`, `password`,
`app_password`. `null` values stay `null`. Because a redacted value looks the same whether
it changed or not, employee updates also record `changed_fields` (field names only): HR
edits list every changed field, and bank changes made by employees on their own profile
are audited with the field names only. Registration-settings updates are audited without
the registration code.

Rows written before this release are **not** changed by default. The opt-in
`AUDIT_SCRUB_EXISTING=true` (only after a verified backup, see
[First deploy](#first-deploy-of-the-hr-documents-feature)) scrubs them once per tenant in a
background job that starts right after the server (it does not delay startup or
`/readyz`):

```
msg="audit log redaction scrub" tenant=default resumed=false scanned=47 updated=12 duration_ms=840
```

The scrub walks `audit_logs` in primary-key windows of 1000 rows and saves its position in
a `system_settings` marker (`audit_redaction_version`) after each window, so an
interrupted run (restart, timeout) resumes where it stopped, and once it has finished
later starts skip it. It also replaces registration codes in old registration-settings
entries. The scrub rewrites the old values for good: earlier entries that showed a masked
account number (`******1234`) show `"[redacted]"` afterwards, so they no longer tell which
number was set.

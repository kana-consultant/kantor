# Kantor MCP Server

Kantor exposes its API to AI clients (Claude, custom agents) through a
[Model Context Protocol](https://modelcontextprotocol.io) server. Every tool maps
1:1 to a real `/api/v1` endpoint and is dispatched **through the normal request
chain**, so tenant isolation (RLS), authentication, and RBAC apply exactly as they
do for the web app. Superadmin-only endpoints are never exposed.

## Authentication — Personal Access Token

MCP clients authenticate with a Personal Access Token (PAT) bound to your user.
A PAT inherits your permissions: a tool call only succeeds if your role allows it.

Create one (while logged in):

```bash
curl -X POST https://app.yourtenant.com/api/v1/auth/pat \
  -H "Authorization: Bearer <your access token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "claude-desktop", "expires_in_days": 90}'
```

The plaintext token (`kantor_pat_…`) is returned **once** in `data.token`. Store it
securely. List tokens with `GET /api/v1/auth/pat`; revoke with
`DELETE /api/v1/auth/pat/{tokenID}`.

## Transport A — Remote HTTP (`/mcp`)

The backend serves Streamable HTTP at `/mcp`. Point any HTTP-capable MCP client at:

```
https://app.yourtenant.com/mcp
Authorization: Bearer kantor_pat_…
```

Each request is a single JSON-RPC message; the response is a single JSON-RPC reply.
The `Host` header selects the tenant, so use the tenant's own domain.

### Claude Desktop custom connector (OAuth)

Claude Desktop's **Settings → Connectors → Add custom connector** speaks OAuth, not
static tokens. The backend is an OAuth 2.1 authorization server for exactly this:

1. Name it anything; set **Remote MCP server URL** to `https://app.yourtenant.com/mcp`.
2. Leave **OAuth Client ID / Secret** blank — the server supports Dynamic Client
   Registration, so Claude registers itself.
3. Click **Add**. Claude hits `/mcp`, gets `401` + `WWW-Authenticate`, discovers the
   authorization server, and opens a browser to the Kantor consent page.
4. Log in with your Kantor account and click **Izinkan** (Allow). Tokens map to your
   user, so every tool call is enforced by your RBAC.

Flow: Authorization Code + PKCE (S256). Endpoints: `/.well-known/oauth-authorization-server`,
`/.well-known/oauth-protected-resource`, `/oauth/register`, `/oauth/authorize`, `/oauth/token`.

## Transport B — Local stdio (`kantor-mcp`)

For clients that speak stdio (e.g. Claude Desktop), run the bundled binary, which
proxies to the remote `/mcp`.

```bash
go build -o kantor-mcp ./backend/cmd/mcp
```

Claude Desktop `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "kantor": {
      "command": "/absolute/path/to/kantor-mcp",
      "env": {
        "KANTOR_BASE_URL": "https://app.yourtenant.com",
        "KANTOR_PAT": "kantor_pat_…",
        "KANTOR_TENANT_HOST": "app.yourtenant.com"
      }
    }
  }
}
```

`KANTOR_TENANT_HOST` is sent as the `Host` header so the server resolves the right
tenant; it usually equals the host part of `KANTOR_BASE_URL`.

## Tool surface

Tools are generated from the live route table at startup, so they always match the
deployed API. Names follow `{method}_{path}` (e.g. `get_hris_employees`,
`post_marketing_leads`, `put_hris_compensation_policy`). Path parameters are tool
arguments; pass query parameters under `query` (object) and request bodies under
`body` (object). The API validates everything and returns the standard response
envelope as the tool result; a non-2xx status sets `isError: true`.

Data-heavy list/query tools carry an enriched `inputSchema`: their real filters
are exposed as named, typed, described `query` properties (with enums where
applicable), and paginated endpoints surface `page`/`per_page` with their
defaults and caps — so a client can discover the available filters and page
through **all** rows instead of taking only the default first page. Tools also
expose MCP `annotations` (`readOnlyHint`/`destructiveHint`/`idempotentHint`)
derived from the HTTP method. Endpoints without a curated annotation keep the
generic free-form `query` object.

Excluded from the surface: superadmin endpoints (toggle super admin, registration
settings), public auth flows (login, register, password reset), and mail
credential / delivery routes (`/admin/settings/mail-delivery`,
`/admin/settings/document-mail`, `/email-deliveries`), and HR document data
(`/hris/employees/{id}/hr-profile` with the employee identity,
`/admin/settings/company-profile` including its `/logo`, every
`/hris/payslips` route: amounts, PDFs and sending, and every `/hris/contracts`
route: PKWT/NDA terms, compensation, PDFs with the full NIK, and sending).
Employee tools return the
bank account number masked (`******7890`) unless the token's user holds
`hris:employee_identity:view` or owns the record. The test
`internal/mcp/catalog_router_test.go` builds the catalog from the real app router
and fails if any of these reappear.

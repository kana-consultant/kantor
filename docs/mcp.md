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
derived from the HTTP method (plus `openWorldHint` on the tools that e-mail an HR
document). Endpoints without a curated annotation keep the
generic free-form `query` object.

Excluded from the surface: superadmin endpoints (toggle super admin, registration
settings), public auth flows (login, register, password reset), mail
credential / delivery routes (`/admin/settings/mail-delivery`,
`/admin/settings/document-mail`, the `/email-deliveries` history route), the
employee identity (`/hris/employees/{id}/hr-profile`), the company profile
(`/admin/settings/company-profile` including its `/logo`) and every rendered
document (`.../pdf`, `.../docx`): those are binary, and the PKWT prints the full NIK
and account number. Employee tools return the bank account number masked
(`******7890`) unless the token's user holds `hris:employee_identity:view` or owns
the record. For the mail settings, the HR profile, the company profile and the
rendered documents, the test `internal/mcp/catalog_router_test.go` builds the
catalog from the real app router and fails if one of them reappears.

### Payslips and contracts

The payslip and contract **workflow** is available as tools, under the caller's own
permissions (`hris:payslip:*` + `hris:salary:view`, `hris:contract:*`):

| Area | Tools |
|---|---|
| Payslips | `get_hris_payslips` (one month), `post_hris_payslips_generate`, `get_hris_payslips_payslipid`, `put_hris_payslips_payslipid` (note + manual lines), `get_hris_payslips_employee_employeeid`, `get_hris_payslips_payslipid_recipient`, `post_hris_payslips_send_preview`, `post_hris_payslips_payslipid_send`, `post_hris_payslips_send_batch`, `post_hris_payslips_payslipid_void_reissue` |
| Contracts | `get_hris_contracts`, `post_hris_contracts`, `get_hris_contracts_contractid`, `put_hris_contracts_contractid`, `get_hris_contracts_contractid_preflight`, `post_hris_contracts_contractid_generate`, `post_hris_contracts_contractid_renew`, `patch_hris_contracts_contractid_status`, `get_hris_contracts_contractid_recipient`, `post_hris_contracts_contractid_send` |

**What these tools hand to the AI client.** Salary amounts (payslip totals and
lines for every employee of the month, contract compensation), employee names,
departments, job titles, login and employee e-mail addresses, document numbers and
the latest delivery of each document. Using them puts payroll figures into the AI
provider's context. Not returned: the NIK, the full account number (payslips carry
it masked), the personal e-mail (masked on MCP calls even for callers who may see
it in the web app), the rendered files and any mail credential.

**Rules for sending.** The API applies these to every request the MCP server
builds (it marks them with `X-Kantor-Via: mcp`); the web app is not affected.

- **Login address only.** Through MCP a document is mailed only to the login e-mail
  of the employee's linked account, an address only that user can change, with
  their password. Employees without an account, the employee or personal e-mail,
  and contract `cc` are web-only: `send-preview` lists such slips under `skipped`
  (`mcp_recipient_not_login`) and a send returns `MCP_RECIPIENT_NOT_ALLOWED` /
  `MCP_CC_NOT_ALLOWED`. The reason: the employee e-mail of an employee without an
  account is ordinary HR data that `hris:employee:edit` can change, and it also
  decides which user account such an employee gets linked to. For the same reason
  an employee's e-mail cannot be changed through MCP at all
  (`put_hris_employees_employeeid` returns `EMPLOYEE_EMAIL_LOCKED`; use the web app).
- **The approved address is part of the call.** A send must carry
  `expected_recipient` (batch: `expected_recipients`, payslip id → address) with the
  address the human was shown. The server resolves the recipient again when it
  sends and refuses with `RECIPIENT_MISMATCH` (naming the slip) if it differs, so
  what was approved is what is mailed. Nothing is queued when one slip of a batch
  fails this check, and a batch job mails the address fixed at that moment.
- **`confirm: true`.** The three tools that e-mail an HR document
  (`post_hris_payslips_send_batch`, `post_hris_payslips_payslipid_send`,
  `post_hris_contracts_contractid_send`) do not call the API unless the top-level
  argument `confirm` is the literal `true`. This is a guardrail for the model, not
  an authorisation: the model sets it itself. The refusal tells it to show the
  recipients and wait for an explicit yes. These tools are also marked
  `destructiveHint` and `openWorldHint` (void-and-reissue and contract status
  changes are `destructiveHint`), so a client that honours annotations asks the
  human before running them. Do not set them to "always allow".
- **Same permissions, limits and audit as the web app.** A token whose user cannot
  send payslips in the browser cannot send them through MCP. Rate limits apply.
  The audit rows of sends made through MCP (request and outcome) carry `via: "mcp"`.

Other tools that notify people (reimbursement review, task assignment, WhatsApp
send) are not gated this way.

A typical month-end flow: `get_hris_payslips` → `post_hris_payslips_generate` →
poll `get_hris_payslips_payslipid` until `render_status` is `ready` →
`post_hris_payslips_send_preview` → show the recipients, the human approves →
`post_hris_payslips_send_batch` with `confirm: true` and `expected_recipients`.

Limits to know:

- **The token is a full API credential.** A PAT or OAuth token inherits all of its
  user's permissions on the REST API. The exclusions and the send rules above
  constrain the MCP tool surface, not a program that holds the token and calls the
  API directly. Give tokens only to clients you would trust with the web session.
- **Existing connections get the tools.** They appear for every existing PAT and
  OAuth connection whose user has the document permissions (by default only HRIS
  Admin and super admin).
- **Large tenants.** A tool result is cut at 100 KB and these lists are not
  paginated: `get_hris_payslips` reaches the limit at roughly 100 stored slips,
  `get_hris_contracts` returns up to 500 rows (use `limit`, `status`, `employee_id`).

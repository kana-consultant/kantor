# MCP Server

KANTOR ships a stdio Model Context Protocol server that exposes the HTTP API to MCP clients.
It is implemented as a thin gateway, so one MCP tool can call any current or future KANTOR API endpoint.

## Run

```bash
cd backend
KANTOR_API_BASE_URL=http://localhost:8080 \
KANTOR_TENANT_HOST=localhost \
KANTOR_API_TOKEN='<jwt-access-token>' \
go run ./cmd/mcp-server
```

Environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `KANTOR_API_BASE_URL` | `http://localhost:8080` | Backend HTTP origin. |
| `KANTOR_TENANT_HOST` | empty | Optional `Host` header used by tenant resolution. For local defaults, use `localhost`. |
| `KANTOR_API_TOKEN` | empty | Optional JWT bearer token used when a tool call does not provide `access_token`. |

## Tool

`kantor_api_request` calls a relative KANTOR API path.

Example arguments:

```json
{
  "method": "GET",
  "path": "/api/v1/hris/employees",
  "query": {
    "page": 1,
    "per_page": 20
  }
}
```

For authenticated endpoints, either set `KANTOR_API_TOKEN` or pass `access_token` in the tool arguments.
For tenant-scoped endpoints, either set `KANTOR_TENANT_HOST` or pass `tenant_host`.

The gateway only accepts relative paths and only supports `GET`, `POST`, `PUT`, `PATCH`, and `DELETE`.
JSON responses are decoded and returned as structured text. Non-text exports are returned as base64.

## Resources

The server exposes these MCP resources:

| URI | Description |
| --- | --- |
| `kantor://api/routes` | Route catalog for the KANTOR HTTP API. |
| `kantor://api/config` | Effective MCP runtime config. |

## Client Config Example

```json
{
  "mcpServers": {
    "kantor": {
      "command": "go",
      "args": ["run", "./cmd/mcp-server"],
      "cwd": "/path/to/kantor/backend",
      "env": {
        "KANTOR_API_BASE_URL": "http://localhost:8080",
        "KANTOR_TENANT_HOST": "localhost",
        "KANTOR_API_TOKEN": "<jwt-access-token>"
      }
    }
  }
}
```

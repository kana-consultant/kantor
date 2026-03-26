# OAuth Connector Between Extension and KANTOR

**Issue:** [#51](https://github.com/kana-consultant/kantor/issues/51)
**Date:** 2026-03-26
**Status:** Draft

## Problem

The Chrome extension currently requires users to manually configure API tokens — either by copy-pasting from the dashboard or entering them in the extension options page. This is friction-heavy for non-technical users.

## Solution Overview

Replace manual token configuration with an automatic connection flow. The Kantor backend acts as its own auth provider. When a user logs into the dashboard and clicks "Connect Extension," the dashboard requests a dedicated token pair for the extension and delivers it via `chrome.runtime.sendMessage`. The extension manages its own token lifecycle independently from the dashboard session.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| OAuth provider | Kantor backend itself | Self-contained, no third-party dependency |
| Connect trigger | User clicks "Connect Extension" on dashboard | Intentional action, respects privacy (activity tracking requires consent) |
| Token strategy | Dedicated extension token pair | Independent lifecycle, no rotation conflict with dashboard tokens |
| Token transfer | `chrome.runtime.sendMessage` | Secure — only target extension receives the message, no page script interception |
| Logout propagation | Direct message + backend revocation | Immediate disconnect via message, backend revocation as safety net |
| Extension access | Any authenticated user | Consent system still gates actual tracking |

## Backend Changes

### New Endpoint: `POST /api/v1/auth/extension-token`

- **Auth:** Requires `AuthMiddleware` (valid access token)
- **Request body:** None — uses authenticated user identity from JWT claims
- **Response:** `{ access_token, refresh_token, expires_in }`
- **Behavior:**
  - Issues a new token pair with claim `source: "extension"`
  - Access token expiry: 15 minutes (same as dashboard)
  - Refresh token expiry: 30 days (longer than dashboard's 7 days — re-connecting extension is less convenient)
  - Stores refresh token hash in `refresh_tokens` table with `source = 'extension'`

### New Endpoint: `POST /api/v1/auth/extension-disconnect`

- **Auth:** Requires `AuthMiddleware`
- **Request body:** None
- **Response:** `{ message: "disconnected" }`
- **Behavior:**
  - Revokes all refresh tokens with `source = 'extension'` for the authenticated user
  - Used by dashboard on logout and manual disconnect

### Database Migration

Add `source` column to `refresh_tokens` table:

```sql
ALTER TABLE refresh_tokens ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'dashboard';
```

### Auth Service Changes

- `GenerateExtensionToken(userID, tenantID)` — new method, issues token pair with `source: "extension"` claim and 30-day refresh expiry
- `RevokeExtensionTokens(userID)` — new method, revokes all extension refresh tokens for a user
- Existing `Login`, `Refresh` methods remain unchanged — the `source` claim propagates through refresh rotation automatically

### JWT Claims Update

Add `source` field to custom claims:

```go
type Claims struct {
    Type     string `json:"type"`
    TenantID string `json:"tenant_id"`
    Source   string `json:"source,omitempty"` // "dashboard" or "extension"
    jwt.RegisteredClaims
}
```

## Extension Changes

### Message Listener (`background.js`)

Replace/augment the existing `window.postMessage` bridge with `chrome.runtime.onMessageExternal`:

```
chrome.runtime.onMessageExternal.addListener((message, sender, sendResponse) => {
  // Validate sender.origin against configured dashboardUrl
  // Handle message types:
  //   KANTOR_TRACKER_CONNECT  → store tokens, respond success
  //   KANTOR_TRACKER_DISCONNECT → clear tokens, stop tracking, respond confirmation
  //   KANTOR_TRACKER_PING → respond with connection status
})
```

### Token Storage

- Store `accessToken`, `refreshToken`, `apiBaseUrl`, `dashboardUrl` in `chrome.storage.local`
- Remove manual token entry from options page (keep apiBaseUrl/dashboardUrl config as fallback)

### Token Refresh

- Existing 401 → refresh logic stays the same
- Update to use stored refresh token and persist new token pair on success
- On refresh failure (token revoked/expired): clear tokens, set state to disconnected

### Popup UI State

- **Connected:** Show user info, "Connected" status badge
- **Disconnected:** Show message: "Log in to your Kantor dashboard and click 'Connect Extension'"
- No manual token input fields needed

### Manifest Changes

- Add `"externally_connectable"` key to `manifest.json`:

```json
{
  "externally_connectable": {
    "matches": ["https://*.yourdomain.com/*", "http://localhost:*/*"]
  }
}
```

This restricts which origins can send messages to the extension.

## Dashboard (Frontend) Changes

### Extension Connection UI

- **Location:** Settings page (one-time setup action)
- **Extension detection:** Ping extension via `chrome.runtime.sendMessage(EXTENSION_ID, { type: 'KANTOR_TRACKER_PING' })`
  - No response → show "Install Extension" link
  - Disconnected → show "Connect Extension" button
  - Connected → show "Connected" status with "Disconnect" option

### Connect Flow

1. User clicks "Connect Extension"
2. Dashboard calls `POST /api/v1/auth/extension-token`
3. Dashboard sends tokens via `chrome.runtime.sendMessage(EXTENSION_ID, { type: 'KANTOR_TRACKER_CONNECT', accessToken, refreshToken, apiBaseUrl, dashboardUrl })`
4. Extension responds with success/failure
5. Dashboard shows updated connection status

### Disconnect Flow

1. User clicks "Disconnect" (or logs out)
2. Dashboard sends `KANTOR_TRACKER_DISCONNECT` via `chrome.runtime.sendMessage`
3. Dashboard calls `POST /api/v1/auth/extension-disconnect`
4. On logout: proceeds with normal logout flow

### Logout Integration

The existing logout handler in the auth store is extended to:
1. Send `KANTOR_TRACKER_DISCONNECT` to extension (fire-and-forget, don't block logout if extension isn't installed)
2. Call `POST /api/v1/auth/extension-disconnect`
3. Proceed with `POST /api/v1/auth/logout`

### Extension ID Configuration

- Environment variable: `VITE_EXTENSION_ID`
- Differs between dev and production builds

## Security

- **Token transfer:** `chrome.runtime.sendMessage` with explicit extension ID — only the target extension receives the message
- **Origin validation:** Extension validates `sender.origin` against configured dashboard URL via `externally_connectable` manifest key
- **Auth gate:** `/auth/extension-token` requires valid dashboard access token — no unauthenticated token issuance
- **Extension ID:** Public information (visible in Chrome Web Store), not a security secret — backend auth is the real gate
- **Token scope:** Extension tokens carry same permissions as user's dashboard tokens (same tenant, same roles). `source: "extension"` claim enables audit trail differentiation.
- **Refresh rotation:** Extension follows same rotation policy — old refresh token revoked on refresh
- **No new attack surface:** Both new endpoints require authentication. Token transfer channel (`chrome.runtime.sendMessage`) is not accessible to page scripts.

## Data Flows

### Connect
```
User clicks "Connect Extension" on Dashboard
  -> Dashboard: POST /api/v1/auth/extension-token
  -> Backend: issues token pair (source: "extension", refresh: 30 days)
  -> Dashboard: chrome.runtime.sendMessage(EXTENSION_ID, {CONNECT, tokens})
  -> Extension: stores tokens in chrome.storage.local
  -> Extension: responds success
  -> Dashboard: shows "Connected"
```

### Normal Operation
```
Extension makes API calls with its own access token
  -> On 401: POST /api/v1/auth/refresh with extension refresh token
  -> Backend: issues new token pair, revokes old refresh token
  -> Extension: stores new tokens, retries request
  -> On refresh failure: clears tokens, shows "Disconnected"
```

### Dashboard Logout
```
User clicks Logout on Dashboard
  -> Dashboard: chrome.runtime.sendMessage(EXTENSION_ID, {DISCONNECT})
  -> Extension: clears tokens, stops tracking
  -> Dashboard: POST /api/v1/auth/extension-disconnect
  -> Backend: revokes all extension refresh tokens for user
  -> Dashboard: POST /api/v1/auth/logout (normal logout)
```

### Manual Disconnect
```
User clicks "Disconnect" on Dashboard settings
  -> Same as logout flow, but dashboard stays logged in
```

## Files to Modify

### Backend
- `backend/internal/auth/jwt.go` — add `Source` field to Claims
- `backend/internal/service/auth/service.go` — add `GenerateExtensionToken`, `RevokeExtensionTokens` methods
- `backend/internal/handler/auth/` — add extension-token and extension-disconnect handlers
- `backend/internal/app/app.go` — register new routes
- `backend/internal/repository/` — add source-aware refresh token queries
- `backend/migrations/` — new migration for `source` column

### Extension
- `extension/manifest.json` — add `externally_connectable`
- `extension/background.js` — add `onMessageExternal` listener, update token storage/refresh
- `extension/popup/popup.js` + `popup.html` — update UI for connection state
- `extension/options/options.js` + `options.html` — remove manual token fields

### Frontend
- `frontend/src/services/auth.ts` — add `getExtensionToken`, `disconnectExtension` API calls
- `frontend/src/stores/auth-store.ts` — extend logout to disconnect extension
- New component for extension connection UI on settings page
- `frontend/.env` / `frontend/.env.example` — add `VITE_EXTENSION_ID`

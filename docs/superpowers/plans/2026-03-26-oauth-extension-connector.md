# OAuth Extension Connector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace manual API key/token configuration in the Chrome extension with automatic token delivery from the dashboard via `chrome.runtime.sendMessage`.

**Architecture:** The Kantor backend issues a dedicated extension token pair (access + refresh) via a new authenticated endpoint. The dashboard sends these tokens to the extension using `chrome.runtime.sendMessage`. The extension manages its own refresh cycle. Logout propagation uses both direct messaging and backend token revocation.

**Tech Stack:** Go (Chi router, JWT, PostgreSQL), Chrome Extension (Manifest V3, service worker), React (TanStack Router/Query, Zustand)

**Spec:** `docs/superpowers/specs/2026-03-26-oauth-extension-connector-design.md`

---

## File Structure

### New Files
- `backend/migrations/20260326010000_refresh_token_source.up.sql` — migration adding `source` column
- `backend/migrations/20260326010000_refresh_token_source.down.sql` — rollback migration
- `frontend/src/services/extension.ts` — extension communication service
- `frontend/src/components/settings/extension-connector.tsx` — extension connection UI component

### Modified Files
- `backend/internal/auth/jwt.go` — add `Source` to `AccessClaims`, add `GenerateRefreshTokenWithExpiry`
- `backend/internal/model/user.go` — add `Source` to `RefreshToken`
- `backend/internal/repository/auth/repository.go` — add `Source` to `CreateRefreshTokenParams`, update queries
- `backend/internal/service/auth/service.go` — add `GenerateExtensionToken`, `RevokeExtensionTokens`, update `Refresh`
- `backend/internal/handler/auth/handler.go` — add extension-token and extension-disconnect handlers, update refresh handler
- `backend/internal/app/app.go` — register new routes
- `backend/internal/dto/auth.go` — add `ExtensionTokenResponse`, update `TokenPair`
- `extension/manifest.json` — add `externally_connectable`
- `extension/background.js` — add `onMessageExternal`, update token storage/refresh
- `extension/popup/popup.js` — update for `accessToken` field name
- `extension/popup/popup.html` — no manual token input needed (manual setup kept as fallback)
- `extension/options/options.js` — update for `accessToken` field name
- `extension/content-script.js` — update `saveConfig` for new token field names
- `frontend/src/services/auth.ts` — add `disconnectExtension` call in logout
- `frontend/src/lib/env.ts` — add `VITE_EXTENSION_ID`
- `frontend/src/routes/_authenticated/operational/tracker.tsx` — update connect flow to use `chrome.runtime.sendMessage` + extension token endpoint
- `frontend/src/routes/_authenticated/admin/settings.tsx` — add extension connector section

---

## Task 1: Database Migration — Add `source` Column to `refresh_tokens`

**Files:**
- Create: `backend/migrations/20260326010000_refresh_token_source.up.sql`
- Create: `backend/migrations/20260326010000_refresh_token_source.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- backend/migrations/20260326010000_refresh_token_source.up.sql
ALTER TABLE refresh_tokens ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'dashboard';
```

- [ ] **Step 2: Write the down migration**

```sql
-- backend/migrations/20260326010000_refresh_token_source.down.sql
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS source;
```

- [ ] **Step 3: Verify migration files exist**

Run: `ls -la backend/migrations/20260326010000_*`
Expected: Both `.up.sql` and `.down.sql` files listed.

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/20260326010000_refresh_token_source.up.sql backend/migrations/20260326010000_refresh_token_source.down.sql
git commit -m "feat(auth): add source column migration for refresh_tokens"
```

---

## Task 2: Backend — JWT and Model Changes

**Files:**
- Modify: `backend/internal/auth/jwt.go:14-17` (AccessClaims), `backend/internal/auth/jwt.go:34-55` (GenerateAccessToken), `backend/internal/auth/jwt.go:77-85` (GenerateRefreshToken)
- Modify: `backend/internal/model/user.go:21-29` (RefreshToken)
- Modify: `backend/internal/repository/auth/repository.go:32-38` (CreateRefreshTokenParams)

- [ ] **Step 1: Add `Source` to `AccessClaims` in `jwt.go`**

In `backend/internal/auth/jwt.go`, update `AccessClaims`:

```go
type AccessClaims struct {
	Type     string `json:"type"`
	TenantID string `json:"tenant_id,omitempty"`
	Source   string `json:"source,omitempty"`
	jwt.RegisteredClaims
}
```

- [ ] **Step 2: Update `GenerateAccessToken` to accept source parameter**

Change signature and populate `Source`:

```go
func (m *TokenManager) GenerateAccessToken(userID string, tenantID string, source string, now time.Time) (string, time.Time, error) {
	expiresAt := now.Add(m.accessExpiry)
	claims := AccessClaims{
		Type:     "access",
		TenantID: tenantID,
		Source:   source,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}
```

- [ ] **Step 3: Add `GenerateRefreshTokenWithExpiry` method**

```go
func (m *TokenManager) GenerateRefreshTokenWithExpiry(expiry time.Duration) (string, time.Time, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", time.Time{}, err
	}

	token := base64.RawURLEncoding.EncodeToString(bytes)
	return token, time.Now().UTC().Add(expiry), nil
}
```

- [ ] **Step 4: Update `GenerateRefreshToken` to delegate**

```go
func (m *TokenManager) GenerateRefreshToken() (string, time.Time, error) {
	return m.GenerateRefreshTokenWithExpiry(m.refreshExpiry)
}
```

- [ ] **Step 5: Add `Source` to `RefreshToken` model**

In `backend/internal/model/user.go`:

```go
type RefreshToken struct {
	ID         string
	UserID     string
	TokenHash  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
	LastUsedAt *time.Time
	Source     string
}
```

- [ ] **Step 6: Add `Source` to `CreateRefreshTokenParams`**

In `backend/internal/repository/auth/repository.go`:

```go
type CreateRefreshTokenParams struct {
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UserAgent string
	IPAddress string
	Source    string
}
```

- [ ] **Step 7: Fix all callers of `GenerateAccessToken`**

Update `issueAuthResult` in `service.go` to pass empty source (dashboard default):

```go
accessToken, expiresAt, err := s.tokenManager.GenerateAccessToken(user.ID, tenantID, "", now)
```

- [ ] **Step 8: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Build succeeds.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/auth/jwt.go backend/internal/model/user.go backend/internal/repository/auth/repository.go backend/internal/service/auth/service.go
git commit -m "feat(auth): add source field to JWT claims, RefreshToken model, and CreateRefreshTokenParams"
```

---

## Task 3: Backend — Repository Updates (Source-Aware Queries)

**Files:**
- Modify: `backend/internal/repository/auth/repository.go:331-341` (CreateRefreshToken), `backend/internal/repository/auth/repository.go:343-371` (GetRefreshTokenByHash), `backend/internal/repository/auth/repository.go:373-406` (RotateRefreshToken)

- [ ] **Step 1: Update `CreateRefreshToken` query to include `source`**

```go
func (r *Repository) CreateRefreshToken(ctx context.Context, params CreateRefreshTokenParams) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address, source)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')::inet, COALESCE(NULLIF($6, ''), 'dashboard'))
	`

	_, err := repository.DB(ctx, r.db).Exec(ctx, query, params.UserID, params.TokenHash, params.ExpiresAt, params.UserAgent, params.IPAddress, params.Source)
	return err
}
```

- [ ] **Step 2: Update `GetRefreshTokenByHash` to scan `source`**

Add `source` to the SELECT and Scan:

```go
func (r *Repository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (model.RefreshToken, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT id::text, user_id::text, token_hash, expires_at, revoked_at, created_at, last_used_at, source
		FROM refresh_tokens
		WHERE token_hash = $1
	`

	var refreshToken model.RefreshToken
	err := repository.DB(ctx, r.db).QueryRow(ctx, query, tokenHash).Scan(
		&refreshToken.ID,
		&refreshToken.UserID,
		&refreshToken.TokenHash,
		&refreshToken.ExpiresAt,
		&refreshToken.RevokedAt,
		&refreshToken.CreatedAt,
		&refreshToken.LastUsedAt,
		&refreshToken.Source,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.RefreshToken{}, ErrNotFound
		}

		return model.RefreshToken{}, err
	}

	return refreshToken, nil
}
```

- [ ] **Step 3: Update `RotateRefreshToken` INSERT to include `source`**

```go
	insertQuery := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address, source)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')::inet, COALESCE(NULLIF($6, ''), 'dashboard'))
	`

	if _, err = tx.Exec(ctx, insertQuery, params.UserID, params.TokenHash, params.ExpiresAt, params.UserAgent, params.IPAddress, params.Source); err != nil {
		return err
	}
```

- [ ] **Step 4: Add `RevokeExtensionTokens` method**

```go
func (r *Repository) RevokeExtensionTokens(ctx context.Context, userID string) error {
	_, err := repository.DB(ctx, r.db).Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND source = 'extension' AND revoked_at IS NULL`,
		userID,
	)
	return err
}
```

- [ ] **Step 5: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/repository/auth/repository.go
git commit -m "feat(auth): update repository queries for source-aware refresh tokens"
```

---

## Task 4: Backend — Auth Service (Extension Token + Refresh Source Propagation)

**Files:**
- Modify: `backend/internal/service/auth/service.go:36-79` (authRepository interface), `backend/internal/service/auth/service.go:212-246` (Refresh), `backend/internal/service/auth/service.go:545-596` (issueAuthResult)

- [ ] **Step 1: Add `RevokeExtensionTokens` to `authRepository` interface**

In the `authRepository` interface, add:

```go
RevokeExtensionTokens(ctx context.Context, userID string) error
```

- [ ] **Step 2: Update `issueAuthResult` to accept and propagate `source`**

Change signature and usage:

```go
func (s *Service) issueAuthResult(ctx context.Context, user model.User, oldTokenHash string, source string, userAgent string, ipAddress string) (AuthResult, error) {
```

Inside the method, pass `source` to `GenerateAccessToken`:

```go
accessToken, expiresAt, err := s.tokenManager.GenerateAccessToken(user.ID, tenantID, source, now)
```

Use the appropriate refresh token expiry based on source (extension gets 30 days, dashboard gets default 7 days):

```go
var refreshToken string
var refreshExpiresAt time.Time
if source == "extension" {
	refreshToken, refreshExpiresAt, err = s.tokenManager.GenerateRefreshTokenWithExpiry(extensionRefreshExpiry)
} else {
	refreshToken, refreshExpiresAt, err = s.tokenManager.GenerateRefreshToken()
}
if err != nil {
	return AuthResult{}, err
}
```

Set `Source` on `refreshParams`:

```go
refreshParams := authrepo.CreateRefreshTokenParams{
	UserID:    user.ID,
	TokenHash: backendauth.HashRefreshToken(refreshToken),
	ExpiresAt: refreshExpiresAt,
	UserAgent: userAgent,
	IPAddress: ipAddress,
	Source:    source,
}
```

- [ ] **Step 3: Update all callers of `issueAuthResult`**

In `Register`:
```go
return s.issueAuthResult(ctx, user, "", "", userAgent, ipAddress)
```

In `Login`:
```go
return s.issueAuthResult(ctx, user, "", "", userAgent, ipAddress)
```

In `Refresh` — propagate source from the stored token:
```go
return s.issueAuthResult(ctx, user, tokenHash, storedToken.Source, userAgent, ipAddress)
```

- [ ] **Step 4: Add `GenerateExtensionToken` method**

```go
const extensionRefreshExpiry = 30 * 24 * time.Hour // 30 days

func (s *Service) GenerateExtensionToken(ctx context.Context, userID string, userAgent string, ipAddress string) (dto.TokenPair, error) {
	var tenantID string
	if info, ok := tenant.FromContext(ctx); ok {
		tenantID = info.ID
	}

	now := time.Now().UTC()
	accessToken, expiresAt, err := s.tokenManager.GenerateAccessToken(userID, tenantID, "extension", now)
	if err != nil {
		return dto.TokenPair{}, err
	}

	refreshToken, refreshExpiresAt, err := s.tokenManager.GenerateRefreshTokenWithExpiry(extensionRefreshExpiry)
	if err != nil {
		return dto.TokenPair{}, err
	}

	if err := s.repo.CreateRefreshToken(ctx, authrepo.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: backendauth.HashRefreshToken(refreshToken),
		ExpiresAt: refreshExpiresAt,
		UserAgent: userAgent,
		IPAddress: ipAddress,
		Source:    "extension",
	}); err != nil {
		return dto.TokenPair{}, err
	}

	return dto.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(time.Until(expiresAt).Seconds()),
	}, nil
}
```

- [ ] **Step 5: Add `RevokeExtensionTokens` method**

```go
func (s *Service) RevokeExtensionTokens(ctx context.Context, userID string) error {
	return s.repo.RevokeExtensionTokens(ctx, userID)
}
```

- [ ] **Step 6: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Build succeeds.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/auth/service.go
git commit -m "feat(auth): add GenerateExtensionToken, RevokeExtensionTokens, and source propagation in Refresh"
```

---

## Task 5: Backend — DTO Update (Extension Token Response + TokenPair Fix)

**Files:**
- Modify: `backend/internal/dto/auth.go:27-32` (TokenPair)

- [ ] **Step 1: Update `TokenPair` to conditionally expose `refresh_token`**

Currently `RefreshToken` has `json:"-"` which hides it from all responses. For the extension-token endpoint, we need to return it. Change to `omitempty`:

```go
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}
```

Note: The existing login/register/refresh handlers already return `TokenPair` in `AuthResponse`. The refresh token is delivered via httpOnly cookie for those flows, but including it in JSON doesn't hurt (the dashboard frontend ignores it — see `AuthTokens` type which only has `access_token`, `token_type`, `expires_in`). The extension endpoint needs it in the body.

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/dto/auth.go
git commit -m "feat(auth): expose refresh_token in TokenPair JSON for extension endpoint"
```

---

## Task 6: Backend — Handler (New Endpoints + Refresh Body Support)

**Files:**
- Modify: `backend/internal/handler/auth/handler.go:42-53` (RegisterRoutes), `backend/internal/handler/auth/handler.go:155-176` (refresh)

- [ ] **Step 1: Update `refresh` handler to accept body-based refresh token**

```go
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	// Try reading refresh token from JSON body first (extension), fall back to cookie (dashboard).
	var refreshToken string
	var bodyReq struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&bodyReq); err == nil && bodyReq.RefreshToken != "" {
		refreshToken = bodyReq.RefreshToken
	} else {
		var cookieErr error
		refreshToken, cookieErr = h.readRefreshTokenCookie(r)
		if cookieErr != nil {
			response.WriteError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Refresh token tidak ditemukan", nil)
			return
		}
	}

	result, err := h.service.Refresh(r.Context(), refreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	h.setRefreshTokenCookie(w, result.Tokens.RefreshToken)
	response.WriteJSON(w, http.StatusOK, dto.AuthResponse{
		User:         result.User,
		ModuleRoles:  result.ModuleRoles,
		Permissions:  result.Permissions,
		IsSuperAdmin: result.IsSuperAdmin,
		Tokens:       result.Tokens,
	}, nil)
}
```

- [ ] **Step 2: Add `ExtensionToken` handler**

```go
func (h *Handler) ExtensionToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi login tidak ditemukan", nil)
		return
	}

	tokens, err := h.service.GenerateExtensionToken(r.Context(), principal.UserID, r.UserAgent(), clientIP(r))
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal membuat token extension", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, tokens, nil)
}
```

- [ ] **Step 3: Add `ExtensionDisconnect` handler**

```go
func (h *Handler) ExtensionDisconnect(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi login tidak ditemukan", nil)
		return
	}

	if err := h.service.RevokeExtensionTokens(r.Context(), principal.UserID); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Gagal memutuskan koneksi extension", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "disconnected"}, nil)
}
```

- [ ] **Step 4: Register new routes in `app.go`**

In `backend/internal/app/app.go`, inside the `protected` group, after the existing `/auth/change-password` route:

```go
protected.Post("/auth/extension-token", authHandler.ExtensionToken)
protected.Post("/auth/extension-disconnect", authHandler.ExtensionDisconnect)
```

- [ ] **Step 5: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/auth/handler.go backend/internal/app/app.go
git commit -m "feat(auth): add extension-token and extension-disconnect endpoints, update refresh to accept body"
```

---

## Task 7: Extension — Manifest + External Message Listener

**Files:**
- Modify: `extension/manifest.json`
- Modify: `extension/background.js:1-16` (DEFAULT_STATE), add `onMessageExternal` listener

- [ ] **Step 1: Add `externally_connectable` to manifest.json**

```json
{
  "manifest_version": 3,
  "name": "KANTOR Activity Tracker",
  "version": "1.0.0",
  "description": "Track work activity for KANTOR platform",
  "permissions": ["tabs", "activeTab", "idle", "storage", "alarms"],
  "host_permissions": ["<all_urls>"],
  "externally_connectable": {
    "matches": ["http://localhost:*/*", "https://*.localhost:*/*"]
  },
  "content_scripts": [
    {
      "matches": ["<all_urls>"],
      "js": ["content-script.js"],
      "run_at": "document_start"
    }
  ],
  "background": {
    "service_worker": "background.js"
  },
  "action": {
    "default_popup": "popup/popup.html",
    "default_icon": "icons/icon48.png"
  },
  "options_page": "options/options.html",
  "icons": {
    "16": "icons/icon16.png",
    "48": "icons/icon48.png",
    "128": "icons/icon128.png"
  }
}
```

Note: `externally_connectable.matches` must be configured per deployment. `http://localhost:*/*` covers local dev.

- [ ] **Step 2: Update `DEFAULT_STATE` — rename `token` to `accessToken`, add `refreshToken`**

```js
const DEFAULT_STATE = {
  apiBaseUrl: "",
  dashboardUrl: "",
  accessToken: "",
  refreshToken: "",
  sessionId: "",
  consented: false,
  paused: false,
  idleTimeoutSeconds: 300,
  excludedDomains: [],
  queuedEntries: [],
  currentTab: null,
  trackerState: "stopped",
  lastSummary: null,
  lastHeartbeatAt: null,
  lastError: "",
};
```

- [ ] **Step 3: Add `chrome.runtime.onMessageExternal` listener**

Add after the existing `chrome.runtime.onMessage.addListener` block:

```js
chrome.runtime.onMessageExternal.addListener((message, sender, sendResponse) => {
  void (async () => {
    try {
      const state = await loadState();
      const dashboardUrl = state.dashboardUrl || state.apiBaseUrl;
      if (dashboardUrl && sender.origin) {
        const expectedOrigin = new URL(dashboardUrl).origin;
        if (sender.origin !== expectedOrigin) {
          sendResponse({ ok: false, error: "Origin not allowed" });
          return;
        }
      }

      switch (message?.type) {
        case "KANTOR_TRACKER_PING": {
          const currentState = await loadState();
          sendResponse({
            ok: true,
            connected: Boolean(currentState.accessToken),
            consented: currentState.consented,
          });
          break;
        }
        case "KANTOR_TRACKER_CONNECT": {
          const { accessToken, refreshToken, apiBaseUrl, dashboardUrl: newDashboardUrl } = message;
          if (!accessToken || !refreshToken || !apiBaseUrl) {
            sendResponse({ ok: false, error: "Missing required fields" });
            return;
          }
          await updateState({
            accessToken,
            refreshToken,
            apiBaseUrl: sanitizeApiBaseUrl(apiBaseUrl),
            dashboardUrl: sanitizeDashboardUrl(newDashboardUrl || ""),
            lastError: "",
          });
          await refreshConsent();
          await fetchTodaySummary();
          sendResponse({ ok: true });
          break;
        }
        case "KANTOR_TRACKER_DISCONNECT": {
          await stopTracking({ revokeConsent: false });
          await updateState({
            accessToken: "",
            refreshToken: "",
            sessionId: "",
            consented: false,
            trackerState: "stopped",
            lastError: "",
          });
          sendResponse({ ok: true });
          break;
        }
        default:
          sendResponse({ ok: false, error: "Unsupported action" });
      }
    } catch (error) {
      sendResponse({ ok: false, error: error instanceof Error ? error.message : "Extension error" });
    }
  })();
  return true;
});
```

- [ ] **Step 4: Commit**

```bash
git add extension/manifest.json extension/background.js
git commit -m "feat(extension): add externally_connectable manifest and onMessageExternal listener"
```

---

## Task 8: Extension — Update Token References (`token` → `accessToken`)

**Files:**
- Modify: `extension/background.js` — all references to `state.token`
- Modify: `extension/popup/popup.js:108` (render function)
- Modify: `extension/options/options.js:65-66` (renderState)
- Modify: `extension/content-script.js:49-50` (saveConfig)

- [ ] **Step 1: Update `background.js` — replace all `state.token` with `state.accessToken`**

Replace in `handleHeartbeatTick`:
```js
if (!state.apiBaseUrl || !state.accessToken || state.paused) {
```

Replace in `ensureActiveSession`:
```js
if (!state.consented || state.paused || !state.accessToken) {
```

Replace in `bestEffortEndSession`:
```js
if (!state.sessionId || !state.accessToken) {
```

Replace in `refreshConsent`:
```js
if (!state.apiBaseUrl || !state.accessToken) {
```

Replace in `fetchTodaySummary`:
```js
if (!state.apiBaseUrl || !state.accessToken || !state.consented) {
```

Replace in `authorizedRequest`:
```js
if (!state.apiBaseUrl || !state.accessToken) {
  throw new Error("Extension belum terhubung. Hubungkan dari dashboard KANTOR atau gunakan setup manual.");
}

const response = await fetch(`${sanitizeApiBaseUrl(state.apiBaseUrl)}${path}`, {
  ...init,
  headers: {
    "Content-Type": "application/json",
    Authorization: `Bearer ${state.accessToken}`,
    ...(init?.headers || {}),
  },
});
```

Note: Also remove `credentials: "include"` from `authorizedRequest` — the extension uses body-based tokens, not cookies.

- [ ] **Step 2: Update `refreshAccessToken` to use body-based refresh token**

```js
async function refreshAccessToken() {
  const state = await loadState();
  if (!state.refreshToken || !state.apiBaseUrl) {
    return false;
  }

  const response = await fetch(`${sanitizeApiBaseUrl(state.apiBaseUrl)}/auth/refresh`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ refresh_token: state.refreshToken }),
  });

  const payload = await response.json().catch(() => null);
  if (!response.ok || !payload?.success || !payload?.data?.tokens?.access_token) {
    await updateState({
      accessToken: "",
      refreshToken: "",
      trackerState: "stopped",
      lastError: "Sesi extension kedaluwarsa. Hubungkan ulang dari dashboard.",
    });
    return false;
  }

  await updateState({
    accessToken: payload.data.tokens.access_token,
    refreshToken: payload.data.tokens.refresh_token || state.refreshToken,
  });

  return true;
}
```

Update the call in `authorizedRequest` (remove `state.apiBaseUrl` argument):
```js
const refreshed = await refreshAccessToken();
```

- [ ] **Step 3: Update `tracker:save-config` message handler for backward compat**

In the `onMessage` listener, update the `tracker:save-config` case:
```js
case "tracker:save-config":
  await updateState({
    apiBaseUrl: sanitizeApiBaseUrl(message.payload.apiBaseUrl),
    dashboardUrl: sanitizeDashboardUrl(message.payload.dashboardUrl),
    accessToken: String(message.payload.token || message.payload.accessToken || "").trim(),
    refreshToken: String(message.payload.refreshToken || "").trim(),
  });
  await refreshConsent();
  await fetchTodaySummary();
  sendResponse({ ok: true });
  break;
```

- [ ] **Step 4: Update `popup/popup.js` — render function**

In the `render` function, change the `hasSetup` check:
```js
const hasSetup = Boolean(state.apiBaseUrl && state.accessToken);
```

Update input value bindings:
```js
elements.tokenInput.value = state.accessToken || "";
```

- [ ] **Step 5: Update `options/options.js` — renderState function**

```js
elements.token.value = state.accessToken || "";
```

And in `saveConfig` click handler:
```js
await sendMessage("tracker:save-config", {
  apiBaseUrl: elements.apiUrl.value,
  token: elements.token.value,
});
```

(Keep sending as `token` here since the `tracker:save-config` handler handles both `token` and `accessToken`.)

- [ ] **Step 6: Update `content-script.js` — saveConfig function**

```js
async function saveConfig(payload) {
  if (!payload || typeof payload !== "object") {
    throw new Error("Konfigurasi tracker tidak valid.");
  }

  const apiBaseUrl = String(payload.apiBaseUrl || "").trim();
  const dashboardUrl = String(payload.dashboardUrl || "").trim();
  const token = String(payload.token || "").trim();
  if (!apiBaseUrl || !token) {
    throw new Error("API URL atau token tracker belum tersedia.");
  }

  const response = await sendRuntimeMessage("tracker:save-config", { apiBaseUrl, dashboardUrl, token });
  if (!response?.ok) {
    throw new Error(response?.error || "Gagal menyimpan konfigurasi extension.");
  }
}
```

This stays the same — the content-script bridge still sends `token` (for backward compat with the old flow), and `tracker:save-config` handler maps it to `accessToken`.

- [ ] **Step 7: Verify extension loads without errors**

Load the extension in Chrome dev mode, open popup, check console for errors.

- [ ] **Step 8: Commit**

```bash
git add extension/background.js extension/popup/popup.js extension/options/options.js extension/content-script.js
git commit -m "feat(extension): rename token to accessToken/refreshToken, use body-based refresh"
```

---

## Task 9: Frontend — Extension Communication Service

**Files:**
- Create: `frontend/src/services/extension.ts`
- Modify: `frontend/src/lib/env.ts`

- [ ] **Step 1: Add `VITE_EXTENSION_ID` to env schema**

In `frontend/src/lib/env.ts`:

```ts
const envSchema = z.object({
  VITE_API_BASE_URL: apiBaseUrlSchema,
  VITE_EXTENSION_ID: z.string().optional().default(""),
});
```

- [ ] **Step 2: Add `VITE_EXTENSION_ID` to `.env.example`**

Append to `frontend/.env` (or `.env.example`):
```
VITE_EXTENSION_ID=
```

- [ ] **Step 3: Create `frontend/src/services/extension.ts`**

```ts
import { env } from "@/lib/env";
import { authPostJSON } from "@/lib/api-client";

const EXTENSION_ID = env.VITE_EXTENSION_ID;

interface ExtensionPingResponse {
  ok: boolean;
  connected?: boolean;
  consented?: boolean;
}

interface ExtensionActionResponse {
  ok: boolean;
  error?: string;
}

type ExtensionStatus = "not_installed" | "disconnected" | "connected";

function canUseChromeMessaging(): boolean {
  return Boolean(EXTENSION_ID && typeof chrome !== "undefined" && chrome.runtime?.sendMessage);
}

function sendToExtension<T>(message: Record<string, unknown>): Promise<T> {
  return new Promise((resolve, reject) => {
    if (!canUseChromeMessaging()) {
      reject(new Error("Chrome extension messaging not available"));
      return;
    }

    const timeout = setTimeout(() => {
      reject(new Error("Extension did not respond"));
    }, 2500);

    chrome.runtime.sendMessage(EXTENSION_ID, message, (response: T) => {
      clearTimeout(timeout);
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message ?? "Extension messaging failed"));
        return;
      }
      resolve(response);
    });
  });
}

export async function pingExtension(): Promise<ExtensionStatus> {
  if (!canUseChromeMessaging()) {
    return "not_installed";
  }

  try {
    const response = await sendToExtension<ExtensionPingResponse>({
      type: "KANTOR_TRACKER_PING",
    });
    if (response?.connected) {
      return "connected";
    }
    return "disconnected";
  } catch {
    return "not_installed";
  }
}

interface ExtensionTokenResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
}

export async function connectExtension(apiBaseUrl: string, dashboardUrl: string): Promise<void> {
  const tokens = await authPostJSON<ExtensionTokenResponse, Record<string, never>>(
    "/auth/extension-token",
    {},
  );

  const response = await sendToExtension<ExtensionActionResponse>({
    type: "KANTOR_TRACKER_CONNECT",
    accessToken: tokens.access_token,
    refreshToken: tokens.refresh_token,
    apiBaseUrl,
    dashboardUrl: dashboardUrl || window.location.origin,
  });

  if (!response?.ok) {
    throw new Error(response?.error ?? "Failed to connect extension");
  }
}

export async function disconnectExtension(): Promise<void> {
  try {
    await sendToExtension<ExtensionActionResponse>({
      type: "KANTOR_TRACKER_DISCONNECT",
    });
  } catch {
    // Fire-and-forget — extension may not be installed
  }

  await authPostJSON<{ message: string }, Record<string, never>>(
    "/auth/extension-disconnect",
    {},
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/services/extension.ts frontend/src/lib/env.ts
git commit -m "feat(frontend): add extension communication service and VITE_EXTENSION_ID env"
```

---

## Task 10: Frontend — Update Logout to Disconnect Extension

**Files:**
- Modify: `frontend/src/services/auth.ts:72-82` (logout function)

- [ ] **Step 1: Update `logout` to disconnect extension**

```ts
export async function logout() {
  const store = useAuthStore.getState();

  try {
    if (store.session) {
      // Disconnect extension (fire-and-forget)
      try {
        const { disconnectExtension } = await import("@/services/extension");
        await disconnectExtension();
      } catch {
        // Extension may not be installed — don't block logout
      }

      await revokeRefreshToken();
    }
  } finally {
    store.clearSession();
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/services/auth.ts
git commit -m "feat(frontend): disconnect extension on dashboard logout"
```

---

## Task 11: Frontend — Update Tracker Page Connect Flow

**Files:**
- Modify: `frontend/src/routes/_authenticated/operational/tracker.tsx`

- [ ] **Step 1: Update `requestExtensionAction` and `handleExtensionConnect` to use `chrome.runtime.sendMessage`**

Replace the `requestExtensionAction` function and update `handleExtensionConnect`:

```ts
import { connectExtension, disconnectExtension, pingExtension } from "@/services/extension";
```

Update the extension detection `useEffect` to use `pingExtension()`:
```ts
useEffect(() => {
  pingExtension().then((status) => {
    setExtensionInstalled(status !== "not_installed");
  });
}, []);
```

Update `handleExtensionConnect`:
```ts
async function handleExtensionConnect(enableTracking: boolean) {
  setIsConnectingExtension(true);
  try {
    const apiBaseUrl = trackerApiBaseUrl;
    const dashboardUrl = window.location.href;
    await connectExtension(apiBaseUrl, dashboardUrl);

    if (enableTracking) {
      // Grant consent via the old content-script bridge (still works for consent)
      // OR the extension auto-handles consent after connect
      toast.success("Tracker aktif di browser ini", "Extension sudah terhubung.");
      setConsentDialogOpen(false);
      await queryClient.invalidateQueries({ queryKey: trackerKeys.consent() });
      await queryClient.invalidateQueries({ queryKey: trackerKeys.consents() });
    } else {
      toast.success("Extension tersambung", "Browser ini sudah terhubung ke KANTOR Tracker.");
    }
    setExtensionInstalled(true);
  } catch (error) {
    toast.error(error instanceof Error ? error.message : "Gagal menghubungkan extension tracker");
  } finally {
    setIsConnectingExtension(false);
  }
}
```

Remove the old `requestExtensionAction` function, the `pendingExtensionRequests` ref, and the `window.postMessage`/`window.addEventListener("message")` code that handled extension responses. Keep the `TRACKER_WEB_SOURCE` / `TRACKER_EXTENSION_SOURCE` constants only if still used elsewhere in the file.

- [ ] **Step 2: Verify the page compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/routes/_authenticated/operational/tracker.tsx
git commit -m "feat(frontend): update tracker page to use chrome.runtime.sendMessage for extension connect"
```

---

## Task 12: Frontend — Extension Connector Component on Settings Page

**Files:**
- Create: `frontend/src/components/settings/extension-connector.tsx`
- Modify: `frontend/src/routes/_authenticated/admin/settings.tsx`

- [ ] **Step 1: Create the `ExtensionConnector` component**

```tsx
// frontend/src/components/settings/extension-connector.tsx
import { useEffect, useState } from "react";
import { PlugZap } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { connectExtension, disconnectExtension, pingExtension } from "@/services/extension";
import { env } from "@/lib/env";
import { toast } from "@/stores/toast-store";

type ExtensionStatus = "loading" | "not_installed" | "disconnected" | "connected";

export function ExtensionConnector() {
  const [status, setStatus] = useState<ExtensionStatus>("loading");
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    pingExtension().then((result) => setStatus(result));
  }, []);

  async function handleConnect() {
    setIsLoading(true);
    try {
      const apiBaseUrl = env.VITE_API_BASE_URL;
      await connectExtension(apiBaseUrl, window.location.origin);
      setStatus("connected");
      toast.success("Extension terhubung", "Chrome extension sudah terhubung ke akun Anda.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal menghubungkan extension");
    } finally {
      setIsLoading(false);
    }
  }

  async function handleDisconnect() {
    setIsLoading(true);
    try {
      await disconnectExtension();
      setStatus("disconnected");
      toast.success("Extension terputus", "Chrome extension sudah terputus dari akun Anda.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal memutuskan extension");
    } finally {
      setIsLoading(false);
    }
  }

  if (status === "loading") {
    return null;
  }

  return (
    <Card className="p-6">
      <div className="flex items-center gap-3 mb-4">
        <PlugZap className="h-5 w-5 text-muted-foreground" />
        <h3 className="text-lg font-semibold">Chrome Extension</h3>
      </div>

      {status === "not_installed" && (
        <div>
          <p className="text-sm text-muted-foreground mb-3">
            KANTOR Activity Tracker extension belum terdeteksi di browser ini.
          </p>
          <p className="text-sm text-muted-foreground">
            Instal extension dari halaman Operational → Activity Tracker, lalu kembali ke sini.
          </p>
        </div>
      )}

      {status === "disconnected" && (
        <div>
          <p className="text-sm text-muted-foreground mb-3">
            Extension terinstal tapi belum terhubung ke akun Anda.
          </p>
          <Button onClick={handleConnect} disabled={isLoading}>
            {isLoading ? "Menghubungkan..." : "Hubungkan Extension"}
          </Button>
        </div>
      )}

      {status === "connected" && (
        <div>
          <p className="text-sm text-muted-foreground mb-3">
            Extension terhubung ke akun Anda.
          </p>
          <Button variant="outline" onClick={handleDisconnect} disabled={isLoading}>
            {isLoading ? "Memutuskan..." : "Putuskan Extension"}
          </Button>
        </div>
      )}
    </Card>
  );
}
```

- [ ] **Step 2: Add `ExtensionConnector` to the admin settings page**

In `frontend/src/routes/_authenticated/admin/settings.tsx`, import and render the component. Add it at the bottom of the settings page layout (after existing settings sections):

```tsx
import { ExtensionConnector } from "@/components/settings/extension-connector";
```

Add `<ExtensionConnector />` in the JSX — place it outside the admin permission gates since any authenticated user can use it.

- [ ] **Step 3: Verify the page compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/settings/extension-connector.tsx frontend/src/routes/_authenticated/admin/settings.tsx
git commit -m "feat(frontend): add extension connector component to admin settings page"
```

---

## Task 13: Integration Testing — End to End Verification

- [ ] **Step 1: Start the backend**

Run: `cd backend && go run ./cmd/server/main.go`
Expected: Server starts, migrations apply (including the new `source` column).

- [ ] **Step 2: Verify new endpoints respond correctly**

Test extension-token (requires auth):
```bash
# Login first to get access token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"password123"}' \
  -c cookies.txt

# Call extension-token with the access token
curl -X POST http://localhost:8080/api/v1/auth/extension-token \
  -H "Authorization: Bearer <ACCESS_TOKEN>" \
  -H "Host: localhost"

# Expected: 200 with { access_token, refresh_token, token_type, expires_in }
```

Test refresh with body-based token:
```bash
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -H "Host: localhost" \
  -d '{"refresh_token":"<EXTENSION_REFRESH_TOKEN>"}'

# Expected: 200 with new token pair
```

Test extension-disconnect:
```bash
curl -X POST http://localhost:8080/api/v1/auth/extension-disconnect \
  -H "Authorization: Bearer <ACCESS_TOKEN>" \
  -H "Host: localhost"

# Expected: 200 with { "message": "disconnected" }
```

- [ ] **Step 3: Test extension in Chrome**

1. Load extension in Chrome dev mode (`chrome://extensions` → Load unpacked → `extension/`)
2. Start the frontend (`cd frontend && npm run dev`)
3. Log into the dashboard
4. Navigate to Admin → Settings
5. Click "Hubungkan Extension"
6. Verify extension popup shows "Connected" state
7. Log out of dashboard
8. Verify extension shows "Disconnected" state

- [ ] **Step 4: Commit any fixes found during testing**

```bash
git add -A
git commit -m "fix: integration testing fixes for extension OAuth connector"
```

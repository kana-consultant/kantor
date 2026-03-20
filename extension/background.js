const DEFAULT_STATE = {
  apiBaseUrl: "http://localhost:3000/api/v1",
  token: "",
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

const HEARTBEAT_ALARM = "kantor-heartbeat";
const HEARTBEAT_INTERVAL_MINUTES = 0.5;

chrome.runtime.onInstalled.addListener(() => {
  void initializeState();
});

chrome.runtime.onStartup.addListener(() => {
  void initializeState();
});

chrome.runtime.onSuspend.addListener(() => {
  void bestEffortEndSession();
});

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === HEARTBEAT_ALARM) {
    void handleHeartbeatTick();
  }
});

chrome.tabs.onActivated.addListener(() => {
  void updateCurrentTabSnapshot();
});

chrome.tabs.onUpdated.addListener((_tabId, changeInfo, tab) => {
  if (changeInfo.status === "complete" && tab.active) {
    void updateCurrentTabSnapshot();
  }
});

chrome.windows.onFocusChanged.addListener(() => {
  void updateCurrentTabSnapshot();
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  void (async () => {
    try {
      switch (message?.type) {
        case "tracker:get-state":
          sendResponse(await getExtensionState());
          break;
        case "tracker:save-config":
          await updateState({
            apiBaseUrl: sanitizeApiBaseUrl(message.payload.apiBaseUrl),
            token: String(message.payload.token || "").trim(),
          });
          await refreshConsent();
          await fetchTodaySummary();
          sendResponse({ ok: true });
          break;
        case "tracker:set-options":
          await updateState({
            idleTimeoutSeconds: normalizeIdleTimeout(message.payload.idleTimeoutSeconds),
            excludedDomains: normalizeExcludedDomains(message.payload.excludedDomains),
          });
          sendResponse({ ok: true });
          break;
        case "tracker:grant-consent":
          sendResponse(await grantConsent());
          break;
        case "tracker:revoke-consent":
          sendResponse(await revokeConsent());
          break;
        case "tracker:pause":
          await updateState({ paused: true, trackerState: "paused" });
          sendResponse({ ok: true });
          break;
        case "tracker:resume":
          await updateState({ paused: false });
          await ensureActiveSession();
          sendResponse({ ok: true });
          break;
        case "tracker:stop":
          await stopTracking();
          sendResponse({ ok: true });
          break;
        case "tracker:refresh":
          await refreshConsent();
          await fetchTodaySummary();
          sendResponse(await getExtensionState());
          break;
        default:
          sendResponse({ ok: false, error: "Unsupported action" });
      }
    } catch (error) {
      const messageText = error instanceof Error ? error.message : "Unexpected extension error";
      await updateState({ lastError: messageText });
      sendResponse({ ok: false, error: messageText });
    }
  })();

  return true;
});

async function initializeState() {
  const state = await loadState();
  await saveState({ ...DEFAULT_STATE, ...state });
  await chrome.alarms.clear(HEARTBEAT_ALARM);
  await chrome.alarms.create(HEARTBEAT_ALARM, {
    delayInMinutes: HEARTBEAT_INTERVAL_MINUTES,
    periodInMinutes: HEARTBEAT_INTERVAL_MINUTES,
  });
  await updateCurrentTabSnapshot();
  await refreshConsent();
  await ensureActiveSession();
}

async function getExtensionState() {
  const state = await loadState();
  return {
    ...state,
    dashboardUrl: toDashboardUrl(state.apiBaseUrl),
  };
}

async function handleHeartbeatTick() {
  const state = await loadState();
  if (!state.apiBaseUrl || !state.token || state.paused) {
    await updateState({ trackerState: state.paused ? "paused" : "stopped" });
    return;
  }

  const tabInfo = await getCurrentTabInfo(state.excludedDomains);
  if (!tabInfo) {
    await updateState({ currentTab: null, trackerState: "stopped" });
    return;
  }

  const idleState = await queryIdleState(state.idleTimeoutSeconds);
  const payload = {
    session_id: state.sessionId,
    url: tabInfo.url,
    domain: tabInfo.domain,
    page_title: tabInfo.title || tabInfo.domain,
    is_idle: idleState !== "active",
    timestamp: new Date().toISOString(),
  };

  await updateState({
    currentTab: {
      ...tabInfo,
      idleState,
      category: state.lastSummary?.top_domains?.find((item) => item.domain === tabInfo.domain)?.category || "uncategorized",
    },
    trackerState: idleState === "active" ? "active" : "idle",
  });

  await flushQueue();
  await sendHeartbeat(payload);
  await fetchTodaySummary();
}

async function sendHeartbeat(payload) {
  const state = await loadState();
  if (!navigator.onLine) {
    await queueEntry(payload);
    return;
  }

  let sessionId = state.sessionId;
  if (!sessionId) {
    sessionId = await ensureActiveSession();
    payload.session_id = sessionId;
  }

  try {
    const response = await authorizedRequest("/tracker/heartbeat", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    if (response?.data?.session?.id) {
      await updateState({
        sessionId: response.data.session.id,
        trackerState: payload.is_idle ? "idle" : "active",
        lastHeartbeatAt: new Date().toISOString(),
        lastError: "",
      });
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : "Failed to send heartbeat";
    if (message.includes("TRACKER_SESSION_NOT_FOUND")) {
      const nextSessionId = await ensureActiveSession(true);
      if (nextSessionId) {
        await sendHeartbeat({ ...payload, session_id: nextSessionId });
        return;
      }
    }
    if (message.includes("CONSENT_REQUIRED")) {
      await updateState({ consented: false, trackerState: "stopped", lastError: "Consent required" });
      return;
    }
    await queueEntry(payload);
    await updateState({ lastError: message });
  }
}

async function flushQueue() {
  const state = await loadState();
  if (!navigator.onLine || !state.queuedEntries.length) {
    return;
  }

  try {
    await authorizedRequest("/tracker/entries/batch", {
      method: "POST",
      body: JSON.stringify({ entries: state.queuedEntries }),
    });
    await updateState({ queuedEntries: [] });
  } catch (error) {
    await updateState({
      lastError: error instanceof Error ? error.message : "Failed to sync queued entries",
    });
  }
}

async function ensureActiveSession(forceRestart = false) {
  const state = await loadState();
  if (!state.consented || state.paused || !state.token) {
    return "";
  }
  if (state.sessionId && !forceRestart) {
    return state.sessionId;
  }

  try {
    const response = await authorizedRequest("/tracker/sessions/start", {
      method: "POST",
      body: JSON.stringify({}),
    });
    const sessionId = response?.data?.session_id || "";
    await updateState({
      sessionId,
      trackerState: "active",
      lastError: "",
    });
    return sessionId;
  } catch (error) {
    await updateState({
      trackerState: "stopped",
      lastError: error instanceof Error ? error.message : "Failed to start tracker session",
    });
    return "";
  }
}

async function stopTracking() {
  const state = await loadState();
  if (state.sessionId) {
    try {
      await authorizedRequest(`/tracker/sessions/${state.sessionId}/end`, {
        method: "PATCH",
        body: JSON.stringify({ timestamp: new Date().toISOString() }),
      });
    } catch {
      // Best-effort shutdown.
    }
  }

  await updateState({
    sessionId: "",
    paused: true,
    trackerState: "stopped",
  });
}

async function bestEffortEndSession() {
  const state = await loadState();
  if (!state.sessionId || !state.token) {
    return;
  }
  try {
    await authorizedRequest(`/tracker/sessions/${state.sessionId}/end`, {
      method: "PATCH",
      body: JSON.stringify({ timestamp: new Date().toISOString() }),
    });
  } catch {
    // Ignore suspend race conditions.
  }
}

async function grantConsent() {
  const response = await authorizedRequest("/tracker/consent", {
    method: "POST",
    body: JSON.stringify({}),
  });
  await updateState({ consented: true, paused: false, lastError: "" });
  await ensureActiveSession(true);
  return response;
}

async function revokeConsent() {
  await authorizedRequest("/tracker/consent", {
    method: "DELETE",
    body: JSON.stringify({}),
  });
  await updateState({
    consented: false,
    paused: true,
    sessionId: "",
    trackerState: "stopped",
  });
  return { ok: true };
}

async function refreshConsent() {
  const state = await loadState();
  if (!state.apiBaseUrl || !state.token) {
    return;
  }
  try {
    const response = await authorizedRequest("/tracker/consent", { method: "GET" });
    await updateState({
      consented: Boolean(response?.data?.consented),
      lastError: "",
    });
  } catch (error) {
    await updateState({
      consented: false,
      lastError: error instanceof Error ? error.message : "Failed to read consent",
    });
  }
}

async function fetchTodaySummary() {
  const state = await loadState();
  if (!state.apiBaseUrl || !state.token || !state.consented) {
    return;
  }
  const date = new Date().toISOString().slice(0, 10);
  try {
    const summary = await authorizedRequest(`/tracker/my-activity?date_from=${date}&date_to=${date}`, {
      method: "GET",
    });
    await updateState({
      lastSummary: summary.data,
      lastError: "",
    });
  } catch (error) {
    await updateState({
      lastError: error instanceof Error ? error.message : "Failed to fetch activity summary",
    });
  }
}

async function authorizedRequest(path, init) {
  const state = await loadState();
  if (!state.apiBaseUrl || !state.token) {
    throw new Error("API URL dan token harus diisi");
  }

  const response = await fetch(`${sanitizeApiBaseUrl(state.apiBaseUrl)}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${state.token}`,
      ...(init?.headers || {}),
    },
    credentials: "include",
  });

  const payload = await response.json().catch(() => null);
  if (response.status === 401) {
    const refreshed = await refreshAccessToken(state.apiBaseUrl);
    if (refreshed) {
      return authorizedRequest(path, init);
    }
  }

  if (!response.ok || !payload?.success) {
    const code = payload?.error?.code || `HTTP_${response.status}`;
    const message = payload?.error?.message || "Request failed";
    throw new Error(`${code}: ${message}`);
  }

  return payload;
}

async function refreshAccessToken(apiBaseUrl) {
  const response = await fetch(`${sanitizeApiBaseUrl(apiBaseUrl)}/auth/refresh`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({}),
    credentials: "include",
  });

  const payload = await response.json().catch(() => null);
  if (!response.ok || !payload?.success || !payload?.data?.tokens?.access_token) {
    return false;
  }

  await updateState({
    token: payload.data.tokens.access_token,
  });

  return true;
}

async function getCurrentTabInfo(excludedDomains) {
  const [tab] = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
  if (!tab?.url || !tab.active) {
    return null;
  }

  if (!isTrackableUrl(tab.url)) {
    return null;
  }

  const url = new URL(tab.url);
  const domain = url.hostname.toLowerCase();
  if (excludedDomains.includes(domain)) {
    return null;
  }

  return {
    url: tab.url,
    domain,
    title: tab.title || domain,
  };
}

async function updateCurrentTabSnapshot() {
  const state = await loadState();
  const tab = await getCurrentTabInfo(state.excludedDomains);
  await updateState({ currentTab: tab });
}

async function queueEntry(entry) {
  const state = await loadState();
  const nextQueue = [...state.queuedEntries, entry].slice(-250);
  await updateState({ queuedEntries: nextQueue });
}

async function queryIdleState(idleTimeoutSeconds) {
  return chrome.idle.queryState(normalizeIdleTimeout(idleTimeoutSeconds));
}

function isTrackableUrl(rawUrl) {
  return !/^chrome:|^chrome-extension:|^about:|^edge:|^file:/i.test(rawUrl);
}

function sanitizeApiBaseUrl(value) {
  return String(value || "").trim().replace(/\/+$/, "");
}

function normalizeExcludedDomains(items) {
  return Array.from(
    new Set(
      (Array.isArray(items) ? items : [])
        .map((item) => String(item || "").trim().toLowerCase())
        .filter(Boolean),
    ),
  );
}

function normalizeIdleTimeout(value) {
  const parsed = Number(value || 300);
  if (!Number.isFinite(parsed) || parsed < 60) {
    return 300;
  }
  return Math.round(parsed);
}

function toDashboardUrl(apiBaseUrl) {
  const value = sanitizeApiBaseUrl(apiBaseUrl);
  if (value.endsWith("/api/v1")) {
    return `${value.slice(0, -7)}/operational/tracker`;
  }
  return value.replace(/\/$/, "");
}

async function loadState() {
  const result = await chrome.storage.local.get(DEFAULT_STATE);
  return { ...DEFAULT_STATE, ...result };
}

async function saveState(nextState) {
  await chrome.storage.local.set(nextState);
}

async function updateState(partial) {
  const current = await loadState();
  await saveState({ ...current, ...partial });
}

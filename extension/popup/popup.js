const elements = {
  setupState: document.getElementById("setup-state"),
  consentState: document.getElementById("consent-state"),
  trackerState: document.getElementById("tracker-state"),
  apiUrlInput: document.getElementById("api-url-input"),
  tokenInput: document.getElementById("token-input"),
  saveConfigButton: document.getElementById("save-config-button"),
  toggleManualButton: document.getElementById("toggle-manual-button"),
  manualSetup: document.getElementById("manual-setup"),
  grantConsentButton: document.getElementById("grant-consent-button"),
  pauseButton: document.getElementById("pause-button"),
  resumeButton: document.getElementById("resume-button"),
  stopButton: document.getElementById("stop-button"),
  refreshButton: document.getElementById("refresh-button"),
  openSettingsButton: document.getElementById("open-settings-button"),
  dashboardLink: document.getElementById("dashboard-link"),
  loginLink: document.getElementById("login-link"),
  statusLabel: document.getElementById("status-label"),
  statusDot: document.getElementById("status-dot"),
  activeTimer: document.getElementById("active-timer"),
  totalActive: document.getElementById("total-active"),
  totalIdle: document.getElementById("total-idle"),
  currentDomain: document.getElementById("current-domain"),
  currentCategory: document.getElementById("current-category"),
  idleState: document.getElementById("idle-state"),
  topDomains: document.getElementById("top-domains"),
  errorBanner: document.getElementById("error-banner"),
};

document.addEventListener("DOMContentLoaded", () => {
  bindEvents();
  void refreshState();
});

function bindEvents() {
  elements.saveConfigButton.addEventListener("click", async () => {
    await sendMessage("tracker:save-config", {
      apiBaseUrl: elements.apiUrlInput.value,
      token: elements.tokenInput.value,
    });
    await refreshState();
  });

  elements.toggleManualButton.addEventListener("click", () => {
    const isHidden = elements.manualSetup.classList.contains("hidden");
    elements.manualSetup.classList.toggle("hidden", !isHidden);
    elements.toggleManualButton.textContent = isHidden ? "Sembunyikan setup manual" : "Setup manual untuk IT";
  });

  elements.grantConsentButton.addEventListener("click", async () => {
    await sendMessage("tracker:grant-consent");
    await refreshState();
  });

  elements.pauseButton.addEventListener("click", async () => {
    await sendMessage("tracker:pause");
    await refreshState();
  });

  elements.resumeButton.addEventListener("click", async () => {
    await sendMessage("tracker:resume");
    await refreshState();
  });

  elements.stopButton.addEventListener("click", async () => {
    await sendMessage("tracker:stop");
    await refreshState();
  });

  elements.refreshButton.addEventListener("click", async () => {
    await refreshState();
  });

  elements.openSettingsButton.addEventListener("click", () => {
    chrome.runtime.openOptionsPage();
  });
}

async function refreshState() {
  const state = await sendMessage("tracker:refresh");
  render(state);
}

function render(state) {
  const hasSetup = Boolean(state.apiBaseUrl && state.token);
  const hasConsent = Boolean(state.consented);

  elements.setupState.classList.toggle("hidden", hasSetup);
  elements.consentState.classList.toggle("hidden", !hasSetup || hasConsent);
  elements.trackerState.classList.toggle("hidden", !hasSetup || !hasConsent);

  elements.apiUrlInput.value = state.apiBaseUrl || "";
  elements.tokenInput.value = state.token || "";
  elements.dashboardLink.href = state.dashboardUrl || "#";
  elements.loginLink.href = state.dashboardUrl || "http://localhost:3000/operational/tracker";

  if (state.lastError) {
    elements.errorBanner.textContent = state.lastError;
    elements.errorBanner.classList.remove("hidden");
  } else {
    elements.errorBanner.classList.add("hidden");
  }

  renderTrackerState(state);
}

function renderTrackerState(state) {
  const summary = state.lastSummary || {};
  const status = state.paused ? "Paused" : state.trackerState === "idle" ? "Idle" : state.trackerState === "active" ? "Tracking Active" : "Stopped";
  const dotClass = state.trackerState === "idle" ? "idle" : state.trackerState === "active" ? "" : "offline";

  elements.statusLabel.textContent = status;
  elements.statusDot.className = `status-dot ${dotClass}`.trim();
  elements.activeTimer.textContent = formatSeconds(summary.total_active_seconds || 0);
  elements.totalActive.textContent = formatSeconds(summary.total_active_seconds || 0);
  elements.totalIdle.textContent = formatSeconds(summary.total_idle_seconds || 0);
  elements.currentDomain.textContent = state.currentTab?.domain || "-";
  elements.currentCategory.textContent = state.currentTab?.category || "uncategorized";
  elements.idleState.textContent = state.currentTab?.idleState === "idle" ? "Idle" : "Active";

  elements.pauseButton.classList.toggle("hidden", state.paused);
  elements.resumeButton.classList.toggle("hidden", !state.paused);

  const topDomains = Array.isArray(summary.top_domains) ? summary.top_domains.slice(0, 3) : [];
  elements.topDomains.innerHTML = topDomains.length
    ? topDomains
        .map(
          (item) => `
            <li>
              <code>${escapeHtml(item.domain)}</code>
              <span>${formatSeconds(item.duration_seconds || 0)}</span>
            </li>
          `,
        )
        .join("")
    : "<li><span>Belum ada domain yang terekam.</span></li>";
}

function formatSeconds(value) {
  const total = Number(value || 0);
  const hours = String(Math.floor(total / 3600)).padStart(2, "0");
  const minutes = String(Math.floor((total % 3600) / 60)).padStart(2, "0");
  const seconds = String(total % 60).padStart(2, "0");
  return `${hours}:${minutes}:${seconds}`;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function sendMessage(type, payload = {}) {
  return chrome.runtime.sendMessage({ type, payload }).then((response) => {
    if (!response) {
      throw new Error("Extension background did not respond");
    }
    if (response.ok === false) {
      throw new Error(response.error || "Extension action failed");
    }
    return response;
  });
}

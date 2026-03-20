(function () {
  const WEB_SOURCE = "KANTOR_WEB_APP";
  const EXTENSION_SOURCE = "KANTOR_TRACKER_EXTENSION";
  const SUPPORTED_TYPES = new Set(["KANTOR_TRACKER_PING", "KANTOR_TRACKER_CONNECT", "KANTOR_TRACKER_ENABLE"]);

  window.addEventListener("message", (event) => {
    if (event.source !== window || !event.data || typeof event.data !== "object") {
      return;
    }

    const data = event.data;
    if (data.source !== WEB_SOURCE || !SUPPORTED_TYPES.has(data.type)) {
      return;
    }

    void handleMessage(data).catch((error) => {
      postResult(data.requestId, false, undefined, error instanceof Error ? error.message : "Unexpected extension bridge error");
    });
  });

  async function handleMessage(message) {
    switch (message.type) {
      case "KANTOR_TRACKER_PING":
        postToPage({ type: "KANTOR_TRACKER_READY" });
        return;
      case "KANTOR_TRACKER_CONNECT":
        await ensureTrustedOrigin(message.payload?.apiBaseUrl);
        await saveConfig(message.payload);
        await refreshState(message.requestId);
        return;
      case "KANTOR_TRACKER_ENABLE":
        await ensureTrustedOrigin(message.payload?.apiBaseUrl);
        await saveConfig(message.payload);
        await sendRuntimeMessage("tracker:grant-consent");
        await refreshState(message.requestId);
        return;
      default:
        throw new Error("Unsupported extension bridge action");
    }
  }

  async function saveConfig(payload) {
    if (!payload || typeof payload !== "object") {
      throw new Error("Konfigurasi tracker tidak valid.");
    }

    const apiBaseUrl = String(payload.apiBaseUrl || "").trim();
    const token = String(payload.token || "").trim();
    if (!apiBaseUrl || !token) {
      throw new Error("API URL atau token tracker belum tersedia.");
    }

    const response = await sendRuntimeMessage("tracker:save-config", { apiBaseUrl, token });
    if (!response?.ok) {
      throw new Error(response?.error || "Gagal menyimpan konfigurasi extension.");
    }
  }

  async function refreshState(requestId) {
    const response = await sendRuntimeMessage("tracker:refresh");
    if (!response?.ok && response?.error) {
      throw new Error(response.error);
    }

    postResult(requestId, true, response);
  }

  async function ensureTrustedOrigin(apiBaseUrl) {
    const expectedOrigin = toDashboardOrigin(apiBaseUrl);
    if (!expectedOrigin) {
      throw new Error("API URL tracker tidak valid.");
    }

    if (window.location.origin !== expectedOrigin) {
      throw new Error("Extension hanya bisa dihubungkan dari dashboard KANTOR yang valid.");
    }
  }

  function toDashboardOrigin(apiBaseUrl) {
    try {
      const parsed = new URL(String(apiBaseUrl || ""), window.location.origin);
      return parsed.origin;
    } catch {
      return "";
    }
  }

  async function sendRuntimeMessage(type, payload) {
    return chrome.runtime.sendMessage({ type, payload });
  }

  function postResult(requestId, success, payload, error) {
    postToPage({
      type: "KANTOR_TRACKER_RESULT",
      requestId,
      success,
      payload,
      error,
    });
  }

  function postToPage(payload) {
    window.postMessage(
      {
        source: EXTENSION_SOURCE,
        ...payload,
      },
      window.location.origin,
    );
  }
})();

const elements = {
  apiUrl: document.getElementById("api-url"),
  token: document.getElementById("token"),
  idleTimeout: document.getElementById("idle-timeout"),
  domainInput: document.getElementById("domain-input"),
  domainList: document.getElementById("excluded-domain-list"),
  saveConfig: document.getElementById("save-config"),
  grantConsent: document.getElementById("grant-consent"),
  revokeConsent: document.getElementById("revoke-consent"),
  addDomain: document.getElementById("add-domain"),
  consentStatus: document.getElementById("consent-status"),
  notice: document.getElementById("notice"),
};

document.addEventListener("DOMContentLoaded", async () => {
  bindEvents();
  await renderState();
});

function bindEvents() {
  elements.saveConfig.addEventListener("click", async () => {
    await sendMessage("tracker:save-config", {
      apiBaseUrl: elements.apiUrl.value,
      token: elements.token.value,
    });
    await persistBehaviourSettings();
    await renderState("Konfigurasi berhasil disimpan.");
  });

  elements.grantConsent.addEventListener("click", async () => {
    await sendMessage("tracker:grant-consent");
    await renderState("Consent tracker sudah aktif.");
  });

  elements.revokeConsent.addEventListener("click", async () => {
    await sendMessage("tracker:revoke-consent");
    await renderState("Consent tracker sudah dicabut.");
  });

  elements.addDomain.addEventListener("click", async () => {
    const current = await sendMessage("tracker:get-state");
    const nextDomains = Array.from(
      new Set([...current.excludedDomains, elements.domainInput.value.trim().toLowerCase()].filter(Boolean)),
    );
    await sendMessage("tracker:set-options", {
      idleTimeoutSeconds: Number(elements.idleTimeout.value || 300),
      excludedDomains: nextDomains,
    });
    elements.domainInput.value = "";
    await renderState("Daftar excluded domains diperbarui.");
  });
}

async function persistBehaviourSettings() {
  const current = await sendMessage("tracker:get-state");
  await sendMessage("tracker:set-options", {
    idleTimeoutSeconds: Number(elements.idleTimeout.value || 300),
    excludedDomains: current.excludedDomains,
  });
}

async function renderState(message = "") {
  const state = await sendMessage("tracker:refresh");

  elements.apiUrl.value = state.apiBaseUrl || "";
  elements.token.value = state.token || "";
  elements.idleTimeout.value = String(state.idleTimeoutSeconds || 300);
  elements.consentStatus.textContent = state.consented
    ? "Consent aktif. Extension boleh mengirim heartbeat ke platform."
    : "Consent belum aktif. Tracking tidak akan berjalan sebelum Anda menyetujuinya.";

  elements.domainList.innerHTML = (state.excludedDomains || [])
    .map(
      (domain) => `
        <li>
          <span>${escapeHtml(domain)}</span>
          <button data-domain="${escapeHtml(domain)}" type="button">Hapus</button>
        </li>
      `,
    )
    .join("");

  elements.domainList.querySelectorAll("button").forEach((button) => {
    button.addEventListener("click", async () => {
      const nextDomains = (state.excludedDomains || []).filter((item) => item !== button.dataset.domain);
      await sendMessage("tracker:set-options", {
        idleTimeoutSeconds: Number(elements.idleTimeout.value || 300),
        excludedDomains: nextDomains,
      });
      await renderState("Domain berhasil dihapus dari exclusion list.");
    });
  });

  elements.notice.textContent = message || state.lastError || "";
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
      throw new Error("Background extension tidak merespons.");
    }
    if (response.ok === false) {
      throw new Error(response.error || "Aksi extension gagal.");
    }
    return response;
  });
}

const DEFAULT_SERVICE_URL = "http://127.0.0.1:1323";
const STORAGE_KEY = "serviceURL";

const form = document.querySelector("#settingsForm");
const serviceURLInput = document.querySelector("#serviceURL");
const baseURLInput = document.querySelector("#baseURL");
const modelInput = document.querySelector("#model");
const apiKeyInput = document.querySelector("#apiKey");
const promptInput = document.querySelector("#prompt");
const showApiKeyInput = document.querySelector("#showApiKey");
const connectionStatus = document.querySelector("#connectionStatus");
const connectionText = document.querySelector("#connectionText");
const promptCount = document.querySelector("#promptCount");
const saveStatus = document.querySelector("#saveStatus");
const saveButton = document.querySelector("#saveButton");
const reloadButton = document.querySelector("#reloadButton");

function normalizedServiceURL() {
  return serviceURLInput.value.trim().replace(/\/$/, "");
}

function setConnection(state, text) {
  connectionStatus.dataset.state = state;
  connectionText.textContent = text;
}

function setSaveStatus(state, text) {
  saveStatus.dataset.state = state;
  saveStatus.textContent = text;
}

function updatePromptCount() {
  promptCount.textContent = `${promptInput.value.length} 字符`;
}

async function loadSettings() {
  setConnection("checking", "正在连接");
  setSaveStatus("", "");

  const { [STORAGE_KEY]: savedURL } = await chrome.storage.local.get(STORAGE_KEY);
  if (savedURL && serviceURLInput.value === DEFAULT_SERVICE_URL) {
    serviceURLInput.value = savedURL;
  }

  try {
    const response = await fetch(`${normalizedServiceURL()}/settings`);
    if (!response.ok) {
      throw new Error(`服务返回 ${response.status}`);
    }

    const settings = await response.json();
    baseURLInput.value = settings.baseURL ?? "";
    modelInput.value = settings.model ?? "";
    apiKeyInput.value = settings.apiKey ?? "";
    promptInput.value = settings.prompt ?? "";
    updatePromptCount();
    setConnection("online", "服务已连接");
    setSaveStatus("success", "配置已从本地服务载入");
  } catch (error) {
    setConnection("offline", "服务未连接");
    setSaveStatus("error", `无法载入配置：${error.message}`);
  }
}

async function saveSettings(event) {
  event.preventDefault();
  if (!form.reportValidity()) {
    return;
  }

  saveButton.disabled = true;
  setSaveStatus("", "正在保存...");

  try {
    const serviceURL = normalizedServiceURL();
    await chrome.storage.local.set({ [STORAGE_KEY]: serviceURL });

    const response = await fetch(`${serviceURL}/settings`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        apiKey: apiKeyInput.value.trim(),
        baseURL: baseURLInput.value.trim().replace(/\/$/, ""),
        model: modelInput.value.trim(),
        prompt: promptInput.value.trim(),
      }),
    });

    if (!response.ok) {
      const body = await response.json().catch(() => null);
      throw new Error(body?.error || `服务返回 ${response.status}`);
    }

    setConnection("online", "服务已连接");
    setSaveStatus("success", "配置已保存");
  } catch (error) {
    setConnection("offline", "服务未连接");
    setSaveStatus("error", `保存失败：${error.message}`);
  } finally {
    saveButton.disabled = false;
  }
}

showApiKeyInput.addEventListener("change", () => {
  apiKeyInput.type = showApiKeyInput.checked ? "text" : "password";
});

promptInput.addEventListener("input", updatePromptCount);
serviceURLInput.addEventListener("change", loadSettings);
reloadButton.addEventListener("click", loadSettings);
form.addEventListener("submit", saveSettings);

document.addEventListener("DOMContentLoaded", loadSettings);

const SELECTOR_KEY = "anchorSelector"
const SERVICE_URL_KEY = "serviceURL"
const OPENED_TABS_KEY = "openedTabs"
const DEFAULT_SELECTOR = "a"
const DEFAULT_SERVICE_URL = "http://127.0.0.1:1323"

const selectorInput = document.querySelector("#anchorSelector")
const openLinksButton = document.querySelector("#openLinks")
const sendPagesButton = document.querySelector("#sendPages")
const analyzePagesButton = document.querySelector("#analyzePages")
const openOptionsButton = document.querySelector("#openOptions")
const openResultsButton = document.querySelector("#openResults")
const tabList = document.querySelector("#tabList")
const emptyState = document.querySelector("#emptyState")
const queueCount = document.querySelector("#queueCount")
const status = document.querySelector("#status")

let openedTabs = []

function setStatus(text, state = "") {
    status.textContent = text
    status.dataset.state = state
}

function renderQueue() {
    tabList.replaceChildren()
    queueCount.textContent = openedTabs.length
    emptyState.hidden = openedTabs.length > 0
    tabList.hidden = openedTabs.length === 0
    sendPagesButton.disabled = !openedTabs.some((item) => item.tabId && !item.error)

    for (const item of openedTabs) {
        const row = document.createElement("li")
        row.dataset.state = item.state || (item.error ? "error" : "waiting")

        const marker = document.createElement("span")
        marker.className = "tab-marker"
        marker.setAttribute("aria-hidden", "true")

        const url = document.createElement("span")
        url.className = "tab-url"
        url.textContent = item.url
        url.title = item.url

        const state = document.createElement("span")
        state.className = "tab-state"
        state.textContent = item.error ? "失败" : item.state === "sent" ? "已发送" : "待发送"

        row.append(marker, url, state)
        tabList.append(row)
    }
}

async function restoreState() {
    const local = await chrome.storage.local.get([SELECTOR_KEY])
    const session = await chrome.storage.session.get(OPENED_TABS_KEY)
    selectorInput.value = local[SELECTOR_KEY] || DEFAULT_SELECTOR
    openedTabs = session[OPENED_TABS_KEY] || []
    renderQueue()
}

async function getActivePage() {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
    if (!tab?.id) {
        throw new Error("无法找到当前页面")
    }
    return tab
}

async function sendTabMessage(tabId, message) {
    try {
        return await chrome.tabs.sendMessage(tabId, message)
    } catch (error) {
        await chrome.scripting.executeScript({
            target: { tabId },
            files: ["content.js"],
        })
        return chrome.tabs.sendMessage(tabId, message)
    }
}

async function openAllLinks() {
    const selector = selectorInput.value.trim()
    if (!selector) {
        setStatus("请输入链接选择器", "error")
        selectorInput.focus()
        return
    }

    openLinksButton.disabled = true
    setStatus("正在读取当前页面的链接...")

    try {
        await chrome.storage.local.set({ [SELECTOR_KEY]: selector })
        const activeTab = await getActivePage()
        const result = await sendTabMessage(activeTab.id, {
            type: "collectLinks",
            selector,
        })
        if (!result?.ok) {
            throw new Error(result?.error || "页面没有响应")
        }
        if (result.links.length === 0) {
            throw new Error("当前选择器没有匹配到链接")
        }

        const response = await chrome.runtime.sendMessage({
            type: "openLinks",
            links: result.links,
        })
        if (!response?.ok) {
            throw new Error(response?.error || "打开页面失败")
        }

        openedTabs = response.tabs
        renderQueue()
        setStatus(`已打开 ${openedTabs.filter((item) => item.tabId).length} 个页面`, "success")
    } catch (error) {
        setStatus(error.message, "error")
    } finally {
        openLinksButton.disabled = false
    }
}

async function waitForTab(tabId) {
    const tab = await chrome.tabs.get(tabId)
    if (tab.status === "complete") {
        return
    }

    await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => {
            chrome.tabs.onUpdated.removeListener(listener)
            reject(new Error("页面加载超时"))
        }, 30000)

        function listener(updatedTabId, changeInfo) {
            if (updatedTabId === tabId && changeInfo.status === "complete") {
                clearTimeout(timeout)
                chrome.tabs.onUpdated.removeListener(listener)
                resolve()
            }
        }

        chrome.tabs.onUpdated.addListener(listener)
    })
}

async function sendAllPages() {
    sendPagesButton.disabled = true
    analyzePagesButton.disabled = true
    openLinksButton.disabled = true
    const { [SERVICE_URL_KEY]: savedServiceURL } = await chrome.storage.local.get(SERVICE_URL_KEY)
    const serviceURL = (savedServiceURL || DEFAULT_SERVICE_URL).replace(/\/$/, "")
    let sent = 0
    let failed = 0

    for (const item of openedTabs) {
        if (!item.tabId || item.state === "sent") {
            continue
        }

        setStatus(`正在发送 ${sent + failed + 1}/${openedTabs.length}...`)
        try {
            await waitForTab(item.tabId)
            const page = await sendTabMessage(item.tabId, { type: "collectHTML" })
            if (!page?.ok) {
                throw new Error("无法读取页面 HTML")
            }

            const response = await fetch(`${serviceURL}/html`, {
                method: "POST",
                headers: {
                    "Content-Type": "text/html; charset=utf-8",
                    "x-url": page.url,
                },
                body: page.html,
            })
            if (!response.ok) {
                throw new Error(`接口返回 ${response.status}`)
            }

            item.state = "sent"
            delete item.error
            sent += 1
            try {
                await chrome.tabs.remove(item.tabId)
                delete item.tabId
            } catch {
                // The page may already have been closed by the user.
            }
        } catch (error) {
            item.state = "error"
            item.error = error.message
            failed += 1
        }

        await chrome.storage.session.set({ [OPENED_TABS_KEY]: openedTabs })
        renderQueue()
    }

    setStatus(
        failed ? `发送完成：成功 ${sent}，失败 ${failed}` : `已发送 ${sent} 个页面`,
        failed ? "error" : "success",
    )
    openLinksButton.disabled = false
    analyzePagesButton.disabled = false
    renderQueue()
}

async function analyzePages() {
    analyzePagesButton.disabled = true
    sendPagesButton.disabled = true
    openLinksButton.disabled = true
    setStatus("正在分析已持久化的页面...")

    try {
        const { [SERVICE_URL_KEY]: savedServiceURL } = await chrome.storage.local.get(SERVICE_URL_KEY)
        const serviceURL = (savedServiceURL || DEFAULT_SERVICE_URL).replace(/\/$/, "")
        const response = await fetch(`${serviceURL}/analyzer`)
        const result = await response.json().catch(() => null)

        if (!response.ok) {
            throw new Error(result?.error || `接口返回 ${response.status}`)
        }

        const count = Array.isArray(result?.success) ? result.success.length : 0
        setStatus(count ? `分析完成：已提取 ${count} 条数据` : "分析完成：没有生成新数据", "success")
    } catch (error) {
        setStatus(`分析失败：${error.message}`, "error")
    } finally {
        analyzePagesButton.disabled = false
        openLinksButton.disabled = false
        renderQueue()
    }
}

async function openResults() {
    const { [SERVICE_URL_KEY]: savedServiceURL } = await chrome.storage.local.get(SERVICE_URL_KEY)
    const serviceURL = (savedServiceURL || DEFAULT_SERVICE_URL).replace(/\/$/, "")
    await chrome.tabs.create({ url: `${serviceURL}/results` })
    window.close()
}

selectorInput.addEventListener("change", () => {
    chrome.storage.local.set({ [SELECTOR_KEY]: selectorInput.value.trim() })
})
openLinksButton.addEventListener("click", openAllLinks)
sendPagesButton.addEventListener("click", sendAllPages)
analyzePagesButton.addEventListener("click", analyzePages)
openResultsButton.addEventListener("click", openResults)
openOptionsButton.addEventListener("click", () => chrome.runtime.openOptionsPage())
document.addEventListener("DOMContentLoaded", restoreState)

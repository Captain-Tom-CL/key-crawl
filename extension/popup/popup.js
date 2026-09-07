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
const analysisStatus = document.querySelector("#analysisStatus")

let openedTabs = []
let isSendingPages = false
let isOpeningLinks = false
let isStartingAnalysis = false
let isLoadingState = true
let isAnalysisStatusUnknown = true
let analysisState = "idle"
let analysisPollTimer = null
let analysisRequestController = null
let analysisRequestVersion = 0
let isPopupClosed = false

function isBusy() {
    return isLoadingState || isSendingPages || isOpeningLinks || isStartingAnalysis
        || isAnalysisStatusUnknown || analysisState === "running"
}

function updateButtons() {
    const busy = isBusy()
    openLinksButton.disabled = busy
    sendPagesButton.disabled = isLoadingState || isSendingPages || isOpeningLinks || !openedTabs.some(canSendPage)
    analyzePagesButton.disabled = busy
    analyzePagesButton.textContent = isStartingAnalysis ? "正在启动…"
        : isAnalysisStatusUnknown ? "查询状态中…"
            : analysisState === "running" ? "分析中…" : "开始分析 →"
}

function canSendPage(item) {
    return item.tabId && item.state !== "sent" && item.state !== "skipped"
}

function setStatus(text, state = "") {
    status.textContent = text
    status.dataset.state = state
}

function renderQueue() {
    tabList.replaceChildren()
    queueCount.textContent = openedTabs.length
    emptyState.hidden = openedTabs.length > 0
    tabList.hidden = openedTabs.length === 0
    updateButtons()

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
        state.textContent = item.state === "skipped" ? "已跳过" : item.error ? "失败" : item.state === "sent" ? "已发送" : "待发送"
        state.title = item.skipReason || item.error || ""
        if (item.state === "skipped") {
            const reason = document.createElement("span")
            reason.className = "tab-reason"
            reason.textContent = item.skipReason || "页面不支持采集"
            row.append(marker, url, state, reason)
        } else {
            row.append(marker, url, state)
        }

        tabList.append(row)
    }
}

async function restoreState() {
    updateButtons()
    try {
        const local = await chrome.storage.local.get([SELECTOR_KEY])
        const session = await chrome.storage.session.get(OPENED_TABS_KEY)
        selectorInput.value = local[SELECTOR_KEY] || DEFAULT_SELECTOR
        openedTabs = session[OPENED_TABS_KEY] || []
    } catch (error) {
        setStatus(`载入采集队列失败：${error.message}`, "error")
    } finally {
        isLoadingState = false
        renderQueue()
    }
    await refreshAnalysisStatus()
}

async function getActivePage() {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
    if (!tab?.id) {
        throw new Error("无法找到当前页面")
    }
    return tab
}

async function sendTabMessage(tabId, message) {
    return chrome.runtime.sendMessage({ type: "pageCommand", tabId, command: message })
}

async function openAllLinks() {
    if (isBusy()) {
        return
    }
    const selector = selectorInput.value.trim()
    if (!selector) {
        setStatus("请输入链接选择器", "error")
        selectorInput.focus()
        return
    }

    isOpeningLinks = true
    updateButtons()
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
        isOpeningLinks = false
        updateButtons()
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

async function enablePageSelection(tabId) {
    await waitForTab(tabId)
    const tab = await chrome.tabs.get(tabId)
    const url = new URL(tab.url)
    if (!["http:", "https:"].includes(url.protocol)) {
        return { skipped: true, reason: "不是可采集的 HTTP/HTTPS 网页" }
    }
    if (/\.pdf$/i.test(url.pathname)) {
        return { skipped: true, reason: "PDF 页面，不采集 HTML" }
    }

    let page
    try {
        page = await sendTabMessage(tabId, { type: "enableSelection" })
    } catch (error) {
        return { skipped: true, reason: `无法访问页面内容（可能是 PDF 或浏览器受限页面）：${error.message}` }
    }
    if (page?.skipped) {
        return page
    }
    if (!page?.ok) {
        throw new Error(page?.error || "无法启用页面选择")
    }
    return page
}

async function selectPageContents() {
    if (isLoadingState || isSendingPages || isOpeningLinks) {
        return
    }
    isSendingPages = true
    updateButtons()
    let enabled = 0
    let failed = 0
    let skipped = 0
    let firstEnabledTabId = null

    try {
        const pending = openedTabs.filter(canSendPage)
        for (const item of pending) {
            setStatus(`正在启用选区 ${enabled + failed + skipped + 1}/${pending.length}...`)
            try {
                const page = await enablePageSelection(item.tabId)
                if (page.skipped) {
                    skipped += 1
                } else {
                    enabled += 1
                    firstEnabledTabId ??= item.tabId
                }
            } catch (error) {
                failed += 1
                setStatus(`启用失败：${error.message}`, "error")
            }
        }

        setStatus(`选区工具已启用：${enabled}，跳过 ${skipped}，失败 ${failed}`, failed ? "error" : "success")
        if (firstEnabledTabId) await chrome.tabs.update(firstEnabledTabId, { active: true })
    } catch (error) {
        setStatus(`启用选区中断：${error.message}`, "error")
    } finally {
        isSendingPages = false
        renderQueue()
    }
}

function setAnalysisStatus(text, state = "") {
    analysisStatus.textContent = text
    analysisStatus.dataset.state = state
}

function stopAnalysisPolling() {
    clearTimeout(analysisPollTimer)
    analysisPollTimer = null
    analysisRequestController?.abort()
    analysisRequestController = null
    analysisRequestVersion += 1
}

function scheduleAnalysisPoll(delay = 1000) {
    clearTimeout(analysisPollTimer)
    if (!isPopupClosed) {
        // Schedule after the last response, rather than overlapping setInterval requests.
        analysisPollTimer = setTimeout(refreshAnalysisStatus, delay)
    }
}

async function requestAnalysisStatus(method, controller) {
    const timeout = setTimeout(() => controller.abort(), 10000)
    try {
        const { [SERVICE_URL_KEY]: savedServiceURL } = await chrome.storage.local.get(SERVICE_URL_KEY)
        const serviceURL = (savedServiceURL || DEFAULT_SERVICE_URL).replace(/\/$/, "")
        const response = await fetch(`${serviceURL}/analyzer`, {
            method,
            cache: "no-store",
            signal: controller.signal,
        })
        const task = await response.json().catch(() => null)
        const validTask = ["idle", "running", "completed"].includes(task?.state)
            && [task.total, task.succeeded, task.failed].every(value => Number.isInteger(value) && value >= 0)
        if (!validTask) {
            throw new Error(task?.message || task?.error || `服务未返回有效任务状态（HTTP ${response.status}），请确认 Go 服务已更新`)
        }
        if (!response.ok && response.status !== 409 && !task.error) {
            throw new Error(`查询分析任务失败：HTTP ${response.status}`)
        }
        return task
    } finally {
        clearTimeout(timeout)
    }
}

function showAnalysisTask(task) {
    analysisState = task.state
    isAnalysisStatusUnknown = false
    analysisStatus.title = task.error || task.lastError || task.currentFile || ""
    if (task.state === "running") {
        const progress = task.succeeded + task.failed
        setAnalysisStatus(`分析中：${progress}/${task.total}，成功 ${task.succeeded}，失败 ${task.failed}；关闭弹窗仍会继续`)
        scheduleAnalysisPoll()
    } else {
        clearTimeout(analysisPollTimer)
        analysisPollTimer = null
        if (task.state === "idle") {
            setAnalysisStatus("分析未开始")
        } else if (task.error) {
            setAnalysisStatus(`分析异常结束：${task.error}（成功 ${task.succeeded}，失败 ${task.failed}）`, "error")
        } else if (task.total === 0) {
            setAnalysisStatus("没有待分析的页面，请先发送选区")
        } else if (task.failed > 0) {
            setAnalysisStatus(`分析结束：成功 ${task.succeeded}，失败 ${task.failed}，详情见服务日志`, "error")
        } else {
            setAnalysisStatus(`分析完成：成功 ${task.succeeded} 个页面`, "success")
        }
    }
    updateButtons()
}

async function refreshAnalysisStatus() {
    if (isPopupClosed || isStartingAnalysis) {
        return
    }
    stopAnalysisPolling()
    const version = analysisRequestVersion
    const controller = new AbortController()
    analysisRequestController = controller
    try {
        const task = await requestAnalysisStatus("GET", controller)
        if (!isPopupClosed && version === analysisRequestVersion) {
            showAnalysisTask(task)
        }
    } catch (error) {
        if (!isPopupClosed && version === analysisRequestVersion) {
            // A failed poll does not mean the server stopped its background job.
            isAnalysisStatusUnknown = true
            setAnalysisStatus(`暂时无法查询分析状态，将自动重试：${error.message}`, "error")
            updateButtons()
            scheduleAnalysisPoll(3000)
        }
    }
}

async function analyzePages() {
    if (isBusy()) {
        return
    }
    isStartingAnalysis = true
    stopAnalysisPolling()
    const version = analysisRequestVersion
    const controller = new AbortController()
    analysisRequestController = controller
    updateButtons()
    setAnalysisStatus("正在启动后台分析…")

    try {
        const task = await requestAnalysisStatus("POST", controller)
        if (!isPopupClosed && version === analysisRequestVersion) {
            // 409 carries the existing task, so keep observing instead of starting again.
            showAnalysisTask(task)
        }
    } catch (error) {
        if (!isPopupClosed && version === analysisRequestVersion) {
            // The POST may have reached Go even if its response was lost. Query only;
            // never automatically retry POST and accidentally launch another batch.
            isAnalysisStatusUnknown = true
            setAnalysisStatus(`启动结果未确认，正在查询任务状态：${error.message}`, "error")
            scheduleAnalysisPoll()
        }
    } finally {
        isStartingAnalysis = false
        if (!isPopupClosed) {
            updateButtons()
        }
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
sendPagesButton.addEventListener("click", selectPageContents)
analyzePagesButton.addEventListener("click", analyzePages)
openResultsButton.addEventListener("click", openResults)
openOptionsButton.addEventListener("click", () => chrome.runtime.openOptionsPage())
document.addEventListener("DOMContentLoaded", restoreState)
chrome.storage.onChanged.addListener((changes, area) => {
    if (area === "session" && changes[OPENED_TABS_KEY]) {
        openedTabs = changes[OPENED_TABS_KEY].newValue || []
        renderQueue()
    }
})
window.addEventListener("pagehide", () => {
    isPopupClosed = true
    stopAnalysisPolling()
})

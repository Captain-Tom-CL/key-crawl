const OPENED_TABS_KEY = "openedTabs"
const selectedUploads = new Set()
let queueUpdate = Promise.resolve()

function updateOpenedTabs(update) {
    const operation = queueUpdate.then(async () => {
        const stored = await chrome.storage.session.get(OPENED_TABS_KEY)
        await chrome.storage.session.set({ [OPENED_TABS_KEY]: update(stored[OPENED_TABS_KEY] || []) })
    })
    queueUpdate = operation.catch(() => { })
    return operation
}

async function requireServiceAvailable() {
    const stored = await chrome.storage.local.get("serviceURL")
    const serviceURL = (stored.serviceURL || "http://127.0.0.1:1323").replace(/\/$/, "")
    try {
        const response = await fetch(`${serviceURL}/health`, {
            cache: "no-store",
            signal: AbortSignal.timeout(3000),
        })
        if (!response.ok || (await response.json()).status !== "healthy") throw new Error("unhealthy")
    } catch {
        throw new Error("后台服务未启动或不可用，请先启动 Key Crawl 服务")
    }
}

async function sendPageCommand(tabId, command) {
    if (!Number.isInteger(tabId) || !["collectLinks", "enableSelection"].includes(command?.type)) {
        throw new Error("无效的页面操作")
    }
    const tab = await chrome.tabs.get(tabId)
    if (!/^https?:\/\//i.test(tab.url || "")) throw new Error("不是可采集的 HTTP/HTTPS 网页")
    await requireServiceAvailable()
    try {
        return await chrome.tabs.sendMessage(tabId, command)
    } catch {
        await chrome.scripting.executeScript({ target: { tabId }, files: ["content/content.js"] })
        return chrome.tabs.sendMessage(tabId, command)
    }
}

async function enableQueuedPage(tabId) {
    const stored = await chrome.storage.session.get(OPENED_TABS_KEY)
    if (!(stored[OPENED_TABS_KEY] || []).some((item) => item.tabId === tabId && item.state !== "sent")) return
    const tab = await chrome.tabs.get(tabId)
    if (tab.status === "complete") await sendPageCommand(tabId, { type: "enableSelection" })
}

chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
    if (changeInfo.status === "complete") enableQueuedPage(tabId).catch(() => { })
})

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.type === "pageCommand") {
        if (sender.tab) {
            sendResponse({ ok: false, error: "页面操作只能由扩展弹窗发起" })
            return false
        }
        sendPageCommand(message.tabId, message.command)
            .then(sendResponse)
            .catch((error) => sendResponse({ ok: false, error: error.message }))
        return true
    }
    if (message.type === "sendSelectedHTML") {
        sendSelectedHTML(message, sender)
            .then((result) => sendResponse(result))
            .catch((error) => sendResponse({ ok: false, error: error.message }))
        return true
    }
    if (message.type !== "openLinks") {
        return false
    }

    openLinks(message.links)
        .then((tabs) => sendResponse({ ok: true, tabs }))
        .catch((error) => sendResponse({ ok: false, error: error.message }))
    return true
})

async function openLinks(links) {
    await requireServiceAvailable()
    const uniqueLinks = [...new Set(links)]
    const openedTabs = []

    for (const url of uniqueLinks) {
        try {
            const tab = await chrome.tabs.create({ url, active: false })
            openedTabs.push({ tabId: tab.id, url })
        } catch (error) {
            openedTabs.push({ url, error: error.message })
        }
    }

    await updateOpenedTabs(() => openedTabs)
    await Promise.all(openedTabs.filter((item) => item.tabId).map((item) =>
        enableQueuedPage(item.tabId).catch(() => { })))
    return openedTabs
}

async function sendSelectedHTML(message, sender) {
    const tabId = sender.tab?.id
    if (!tabId || sender.frameId !== 0 || !/^https?:\/\//i.test(sender.url || "")) {
        throw new Error("只能从网页的主页面发送选区")
    }
    if (typeof message.html !== "string" || !/^<html(?:\s|>)/i.test(message.html)) {
        throw new Error("选区内容不是有效的 HTML")
    }
    if (new Blob([message.html]).size > 5 * 1024 * 1024) {
        throw new Error("HTML 超过 5 MiB 大小限制")
    }
    if (selectedUploads.has(tabId)) throw new Error("当前页面正在发送，请勿重复提交")
    selectedUploads.add(tabId)
    try {
        const stored = await chrome.storage.local.get("serviceURL")
        const serviceURL = (stored.serviceURL || "http://127.0.0.1:1323").replace(/\/$/, "")
        const response = await fetch(`${serviceURL}/html`, {
            method: "POST",
            headers: { "Content-Type": "text/html; charset=utf-8", "x-url": sender.url },
            body: message.html,
            signal: AbortSignal.timeout(30000),
        })
        if (!response.ok) {
            throw new Error(response.status === 413 ? "HTML 超过服务端大小限制（5 MiB）" : `接口返回 ${response.status}`)
        }
        try {
            await updateOpenedTabs((items) => items.map((item) => item.tabId === tabId
                ? { url: sender.url, state: "sent" } : item))
        } catch {
            return { ok: true, closed: false, warning: "已保存，但队列更新失败，请手动关闭页面" }
        }
        try {
            await chrome.tabs.remove(tabId)
            return { ok: true, closed: true }
        } catch {
            return { ok: true, closed: false, warning: "已保存，请手动关闭页面" }
        }
    } finally {
        selectedUploads.delete(tabId)
    }
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.type === "collectLinks") {
        try {
            const links = [...document.querySelectorAll(message.selector)]
                .map((element) => element.href || element.closest("a")?.href)
                .filter((url) => url?.startsWith("http://") || url?.startsWith("https://"))
            sendResponse({ ok: true, links: [...new Set(links)] })
        } catch (error) {
            sendResponse({ ok: false, error: `选择器无效：${error.message}` })
        }
    }

    if (message.type === "collectHTML") {
        const contentType = document.contentType?.toLowerCase()
        if (!["text/html", "application/xhtml+xml"].includes(contentType)
            || document.documentElement?.localName !== "html") {
            sendResponse({ ok: false, skipped: true, reason: `不是 HTML 页面（${contentType || "未知类型"}）` })
            return
        }

        const html = document.documentElement.outerHTML
        if (new Blob([html]).size > 5 * 1024 * 1024) {
            sendResponse({ ok: false, skipped: true, reason: "HTML 超过 5 MiB 大小限制" })
            return
        }
        sendResponse({
            ok: true,
            url: location.href,
            contentType,
            html,
        })
    }
})

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
        sendResponse({
            ok: true,
            url: location.href,
            html: document.documentElement.outerHTML,
        })
    }
})

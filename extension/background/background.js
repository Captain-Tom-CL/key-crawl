const OPENED_TABS_KEY = "openedTabs"

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.type !== "openLinks") {
        return false
    }

    openLinks(message.links)
        .then((tabs) => sendResponse({ ok: true, tabs }))
        .catch((error) => sendResponse({ ok: false, error: error.message }))
    return true
})

async function openLinks(links) {
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

    await chrome.storage.session.set({ [OPENED_TABS_KEY]: openedTabs })
    return openedTabs
}

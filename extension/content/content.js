(() => {
    if (globalThis.__keyCrawlContentInstalled) return
    globalThis.__keyCrawlContentInstalled = true

    function cleanHTML(root) {
        root.querySelectorAll("script, style, link").forEach((element) => {
            const relations = (element.getAttribute("rel") || "").toLowerCase().split(/\s+/)
            const resourceType = (element.getAttribute("as") || "").toLowerCase()
            if (element.localName !== "link"
                || relations.includes("stylesheet")
                || relations.includes("modulepreload")
                || (relations.some((relation) => ["preload", "prefetch"].includes(relation))
                    && ["style", "script"].includes(resourceType))) {
                element.remove()
            }
        })

        const sharedAttributes = new Set([
            "title", "lang", "dir", "aria-label", "itemprop", "itemscope", "itemtype", "itemid",
        ])
        const tagAttributes = new Map(Object.entries({
            a: ["href", "name"],
            img: ["src", "srcset", "alt", "data-src", "data-srcset", "data-original", "data-lazy-src"],
            source: ["src", "srcset", "type", "media", "data-src", "data-srcset"],
            video: ["src", "poster"],
            audio: ["src"],
            track: ["src", "kind", "srclang", "label"],
            meta: ["name", "property", "content", "charset", "http-equiv"],
            link: ["href", "rel", "type", "hreflang"],
            base: ["href"],
            td: ["colspan", "rowspan", "headers"],
            th: ["colspan", "rowspan", "headers", "scope", "abbr"],
            col: ["span"],
            colgroup: ["span"],
            ol: ["start", "reversed", "type"],
            li: ["value"],
            time: ["datetime"],
            del: ["datetime", "cite"],
            ins: ["datetime", "cite"],
            blockquote: ["cite"],
            q: ["cite"],
            data: ["value"],
            meter: ["value", "min", "max", "low", "high", "optimum"],
        }))
        const referencedIDs = new Set()
        root.querySelectorAll('[href^="#"], [headers]').forEach((element) => {
            const fragment = element.getAttribute("href")?.slice(1)
            if (fragment) {
                referencedIDs.add(fragment)
                try { referencedIDs.add(decodeURIComponent(fragment)) } catch { }
            }
            for (const identifier of (element.getAttribute("headers") || "").split(/\s+/)) {
                if (identifier) referencedIDs.add(identifier)
            }
        })

        const elements = [...root.querySelectorAll("*")]
        if (root.nodeType === Node.ELEMENT_NODE) elements.unshift(root)
        for (const element of elements) {
            for (const attribute of [...element.attributes]) {
                const name = attribute.name.toLowerCase()
                const isUnusedHTMLAttribute = element.namespaceURI === "http://www.w3.org/1999/xhtml"
                    && !sharedAttributes.has(name)
                    && !tagAttributes.get(element.localName)?.includes(name)
                    && !(name === "id" && referencedIDs.has(attribute.value))
                if (name === "style"
                    || name.startsWith("on")
                    || isUnusedHTMLAttribute
                    || ["href", "src", "action", "formaction", "xlink:href"].includes(name)
                    && /^javascript:/i.test(attribute.value.replace(/[\u0000-\u0020]/g, ""))) {
                    element.removeAttribute(attribute.name)
                }
            }
            if (element.localName === "template") cleanHTML(element.content)
        }

        const walker = document.createTreeWalker(root, NodeFilter.SHOW_COMMENT)
        const comments = []
        while (walker.nextNode()) comments.push(walker.currentNode)
        comments.forEach((comment) => comment.remove())

        const emptyContainers = root.querySelectorAll("div, span, p, section, article, aside, nav, header, footer, h1, h2, h3, h4, h5, h6, ul, ol, dl, a")
        for (const element of [...emptyContainers].reverse()) {
            if (element.namespaceURI === "http://www.w3.org/1999/xhtml"
                && element.attributes.length === 0
                && element.children.length === 0
                && !element.textContent.trim()) {
                element.remove()
            }
        }
    }

    let selectionUI = null

    function isHTMLPage() {
        return ["text/html", "application/xhtml+xml"].includes(document.contentType?.toLowerCase())
            && document.documentElement?.localName === "html"
    }

    function selectedHTML(elements) {
        const selected = [...elements].filter((element) => element.isConnected)
        const roots = selected.filter((element) => !selected.some((parent) => parent !== element && parent.contains(element)))
        roots.sort((first, second) => first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1)
        if (!roots.length) throw new Error("请先选择要发送的内容")

        const snapshot = document.documentElement.cloneNode(false)
        snapshot.append(document.head ? document.head.cloneNode(true) : document.createElement("head"))
        const body = document.createElement("body")
        for (const element of roots) {
            if (element === document.body) {
                for (const child of element.childNodes) body.append(child.cloneNode(true))
            } else {
                body.append(element.cloneNode(true))
            }
        }
        snapshot.append(body)
        snapshot.querySelectorAll("key-crawl-selection-ui").forEach((element) => element.remove())
        cleanHTML(snapshot)
        const html = snapshot.outerHTML
        if (new Blob([html]).size > 5 * 1024 * 1024) throw new Error("HTML 超过 5 MiB 大小限制，请缩小选区")
        return html
    }

    function enableSelection() {
        if (!isHTMLPage() || !document.body) return { ok: false, error: "当前页面不是可采集的 HTML 页面" }
        if (selectionUI) return { ok: true }

        const host = document.createElement("key-crawl-selection-ui")
        host.style.cssText = "all:initial!important;position:fixed!important;inset:0!important;z-index:2147483647!important;pointer-events:none!important;display:block!important;"
        const shadow = host.attachShadow({ mode: "closed" })
        shadow.innerHTML = `
        <style>
            :host { color-scheme: light; }
            * { box-sizing: border-box; letter-spacing: 0; }
            .toolbar { position: fixed; top: 12px; right: 12px; width: 320px; max-width: calc(100vw - 24px);
                padding: 12px; border: 1px solid #ccd6cd; border-radius: 8px; background: #fff;
                color: #24352f; box-shadow: 0 4px 20px #0002; pointer-events: auto;
                font: 14px/1.5 "Segoe UI", "Microsoft YaHei UI", sans-serif; }
            .heading { display: flex; justify-content: space-between; gap: 12px; margin-bottom: 10px; }
            .count { color: #67766f; font-variant-numeric: tabular-nums; }
            .actions { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
            button { min-width: 0; min-height: 38px; padding: 6px 10px; border: 1px solid #ccd6cd;
                border-radius: 6px; background: #f5f6f3; color: #24352f; font: inherit; cursor: pointer; }
            button[aria-pressed="true"] { background: #edf4ee; border-color: #23654d; }
            button.send { color: #fff; background: #23654d; border-color: #23654d; }
            button:focus-visible { outline: 2px solid #bd6926; outline-offset: 2px; }
            button:disabled { opacity: .5; cursor: default; }
            .status { margin: 8px 0 0; overflow-wrap: anywhere; font-size: 12px; }
            .status:empty { display: none; }
            .status[data-error="true"] { color: #ae4035; }
            .box { position: fixed; border: 2px solid #23654d; background: #23654d0a; pointer-events: none; }
            .hover { border-color: #bd6926; background: #bd692610; }
        </style>
        <div class="outlines" aria-hidden="true"></div>
        <section class="toolbar" aria-label="Key Crawl 选区采集">
            <div class="heading"><strong>Key Crawl</strong><span class="count" aria-live="polite">已选 0 项</span></div>
            <div class="actions">
                <button class="select" type="button" aria-pressed="false" title="点击选择元素；Ctrl 点击增减选区；Esc 结束选择">开始选择</button>
                <button class="send" type="button" disabled>开始发送</button>
            </div>
            <p class="status" role="status" aria-live="polite"></p>
        </section>`
        document.documentElement.append(host)
        selectionUI = host
        const selectButton = shadow.querySelector(".select")
        const sendButton = shadow.querySelector(".send")
        const count = shadow.querySelector(".count")
        const status = shadow.querySelector(".status")
        const outlines = shadow.querySelector(".outlines")
        const selected = new Set()
        let choosing = false
        let sending = false
        let saved = false
        let hovered = null
        let frame = null

        function render() {
            frame = null
            for (const element of selected) {
                if (!element.isConnected) selected.delete(element)
            }
            count.textContent = `已选 ${selected.size} 项`
            selectButton.textContent = choosing ? "结束选择" : "开始选择"
            selectButton.setAttribute("aria-pressed", String(choosing))
            selectButton.disabled = sending || saved
            sendButton.disabled = sending || saved || !selected.size
            sendButton.textContent = sending ? "发送中..." : saved ? "已发送" : "开始发送"
            outlines.replaceChildren()
            const targets = [...selected].map((element) => ({ element, hover: false }))
            if (choosing && hovered?.isConnected && !selected.has(hovered)) targets.push({ element: hovered, hover: true })
            for (const target of targets) {
                const rect = target.element.getBoundingClientRect()
                const left = Math.max(0, rect.left)
                const top = Math.max(0, rect.top)
                const width = Math.min(innerWidth, rect.right) - left
                const height = Math.min(innerHeight, rect.bottom) - top
                if (width <= 0 || height <= 0) continue
                const box = document.createElement("div")
                box.className = target.hover ? "box hover" : "box"
                box.style.cssText = `left:${left}px;top:${top}px;width:${width}px;height:${height}px;`
                outlines.append(box)
            }
        }

        function scheduleRender() {
            if (frame === null) frame = requestAnimationFrame(render)
        }

        function targetFor(event) {
            if (event.composedPath().includes(host)) return null
            const element = event.target
            return element instanceof Element && document.body.contains(element) ? element : null
        }

        selectButton.addEventListener("click", () => {
            choosing = !choosing
            hovered = null
            scheduleRender()
        })
        window.addEventListener("pointermove", (event) => {
            if (!choosing) return
            hovered = targetFor(event)
            scheduleRender()
        }, true)
        for (const eventName of ["pointerdown", "mousedown", "mouseup", "pointerup", "dblclick", "auxclick"]) {
            window.addEventListener(eventName, (event) => {
                if (!choosing || !targetFor(event)) return
                event.preventDefault()
                event.stopImmediatePropagation()
            }, true)
        }
        window.addEventListener("click", (event) => {
            const element = targetFor(event)
            if (!choosing || !element) return
            event.preventDefault()
            event.stopImmediatePropagation()
            status.textContent = ""
            const additive = event.ctrlKey || event.metaKey
            if (additive && selected.has(element)) {
                selected.delete(element)
            } else {
                if (!additive) selected.clear()
                for (const previous of selected) {
                    if (previous.contains(element) || element.contains(previous)) selected.delete(previous)
                }
                selected.add(element)
            }
            hovered = null
            scheduleRender()
        }, true)
        window.addEventListener("keydown", (event) => {
            if (event.key !== "Escape" || !choosing) return
            event.preventDefault()
            event.stopImmediatePropagation()
            choosing = false
            hovered = null
            scheduleRender()
        }, true)
        window.addEventListener("scroll", scheduleRender, true)
        window.addEventListener("resize", scheduleRender)
        const observer = new MutationObserver(scheduleRender)
        observer.observe(document.body, { subtree: true, childList: true, attributes: true, characterData: true })

        sendButton.addEventListener("click", async () => {
            if (sending || saved) return
            sending = true
            choosing = false
            hovered = null
            status.textContent = ""
            status.dataset.error = "false"
            scheduleRender()
            try {
                const html = selectedHTML(selected)
                const result = await chrome.runtime.sendMessage({ type: "sendSelectedHTML", html })
                if (!result?.ok) throw new Error(result?.error || "发送失败，未收到后台确认")
                saved = true
                status.textContent = result.warning || "已保存"
            } catch (error) {
                status.dataset.error = "true"
                status.textContent = error.message
            } finally {
                sending = false
                scheduleRender()
            }
        })
        render()
        return { ok: true }
    }

    chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
        if (message.type === "enableSelection") {
            sendResponse(enableSelection())
            return
        }
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

            const snapshot = document.documentElement.cloneNode(true)
            snapshot.querySelectorAll("key-crawl-selection-ui").forEach((element) => element.remove())
            cleanHTML(snapshot)
            const html = snapshot.outerHTML
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

})()

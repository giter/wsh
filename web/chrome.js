"use strict";

/* ============================================================
 * Custom window chrome (frameless windows)
 *
 * Windows/Linux run frameless so the whole window can match the app theme:
 * the native title bar and menu bar are gone, and this module draws a custom
 * title bar (brand + menu + window controls) instead.
 *
 * Wails does not load its runtime JS into this self-hosted frontend, but the
 * platform layer still injects `window._wails.invoke`, which is all we need:
 * the native side handles "wails:drag" and "wails:resize:<edge>" messages.
 * The gesture logic below is a port of the Wails v3 runtime's drag.ts.
 * ============================================================ */

// WINDOW_NAME comes from the "?win=" query param the Go side puts on each
// window URL ("main", "settings", "keys", ...). It is empty when the page is
// opened in a plain browser (headless debugging), where there is no chrome.
const WINDOW_NAME = new URLSearchParams(location.search).get("win") || "";

// Per-window chrome configuration.
const CHROME = {
    main: { brand: true, menu: true },
    settings: { title: "选项" },
    keys: { title: "密钥管理" },
};

// isMacOS reports whether the page runs in a macOS webview. macOS keeps the
// native frame and global menu, so no custom chrome is drawn there.
function isMacOS() {
    return /Mac|iPhone|iPad|iPod/.test(navigator.platform || navigator.userAgent || "");
}

// wailsInvoke sends a raw message to the native window (drag / resize).
function wailsInvoke(msg) {
    if (window._wails && typeof window._wails.invoke === "function") {
        window._wails.invoke(msg);
    }
}

/* ============================================================
 * Window gestures: drag + edge resize
 * Ported from the Wails v3 runtime (drag.ts). The resize path is required
 * because a frameless window's standard frame is hidden, which disables the
 * OS-driven resizing; it must be driven from the frontend.
 * ============================================================ */
function installWindowGestures() {
    if (isMacOS()) return;

    const MouseDown = 0;
    const MouseUp = 1;
    const MouseMove = 2;

    let canDrag = false;
    let dragging = false;
    let canResize = false;
    let resizing = false;
    let resizeEdge = "";
    let defaultCursor = "auto";
    let buttons = 0;

    const cursorForEdge = {
        "se-resize": "nwse-resize",
        "sw-resize": "nesw-resize",
        "nw-resize": "nwse-resize",
        "ne-resize": "nesw-resize",
        "w-resize": "ew-resize",
        "n-resize": "ns-resize",
        "s-resize": "ns-resize",
        "e-resize": "ew-resize",
    };

    function flag(name, fallback) {
        const flags = window._wails && window._wails.flags;
        const v = flags && flags[name];
        return typeof v === "number" ? v : fallback;
    }

    function eventTarget(event) {
        const t = event.target;
        if (t instanceof HTMLElement) return t;
        if (t instanceof Node && t.parentElement) return t.parentElement;
        return document.body;
    }

    function isDraggableEvent(event) {
        const target = eventTarget(event);
        const style = window.getComputedStyle(target);
        return (
            style.getPropertyValue("--wails-draggable").trim() === "drag" &&
            event.offsetX >= 0 &&
            event.offsetX < target.clientWidth &&
            event.offsetY >= 0 &&
            event.offsetY < target.clientHeight
        );
    }

    function setResize(edge) {
        if (edge) {
            if (!resizeEdge) defaultCursor = document.body.style.cursor;
            document.body.style.cursor = cursorForEdge[edge];
        } else if (resizeEdge) {
            document.body.style.cursor = defaultCursor;
        }
        resizeEdge = edge || "";
    }

    function primaryDown(event) {
        canDrag = false;
        canResize = false;

        if (resizeEdge) {
            // Only arm a resize from a real press (not a synthesized one).
            if (event.type !== "mousedown") return;
            canResize = true;
            return;
        }
        canDrag = isDraggableEvent(event);
    }

    function primaryUp() {
        canDrag = false;
        dragging = false;
        canResize = false;
        resizing = false;
    }

    function onMouseMove(event) {
        if (canResize && resizeEdge) {
            resizing = true;
            wailsInvoke("wails:resize:" + resizeEdge);
        } else if (canDrag) {
            dragging = true;
            wailsInvoke("wails:drag");
        }

        if (dragging || resizing) {
            canDrag = canResize = false;
            return;
        }

        const resizeHandleHeight = flag("system.resizeHandleHeight", 5);
        const resizeHandleWidth = flag("system.resizeHandleWidth", 5);
        const cornerExtra = flag("resizeCornerExtra", 10);

        // Scrollbars consume the strip at the window edge; shift the effective
        // edge inward so the resize zone sits just before the scrollbar.
        const scrollbarWidth = Math.max(0, window.innerWidth - document.documentElement.clientWidth);
        const scrollbarHeight = Math.max(0, window.innerHeight - document.documentElement.clientHeight);
        const rightEdge = window.innerWidth - scrollbarWidth;
        const bottomEdge = window.innerHeight - scrollbarHeight;

        const rightBorder = event.clientX < rightEdge && rightEdge - event.clientX < resizeHandleWidth;
        const leftBorder = event.clientX < resizeHandleWidth;
        const topBorder = event.clientY < resizeHandleHeight;
        const bottomBorder = event.clientY < bottomEdge && bottomEdge - event.clientY < resizeHandleHeight;

        const rightCorner = event.clientX < rightEdge && rightEdge - event.clientX < resizeHandleWidth + cornerExtra;
        const leftCorner = event.clientX < resizeHandleWidth + cornerExtra;
        const topCorner = event.clientY < resizeHandleHeight + cornerExtra;
        const bottomCorner = event.clientY < bottomEdge && bottomEdge - event.clientY < resizeHandleHeight + cornerExtra;

        if (!leftCorner && !topCorner && !bottomCorner && !rightCorner) setResize();
        else if (rightCorner && bottomCorner) setResize("se-resize");
        else if (leftCorner && bottomCorner) setResize("sw-resize");
        else if (leftCorner && topCorner) setResize("nw-resize");
        else if (topCorner && rightCorner) setResize("ne-resize");
        else if (leftBorder) setResize("w-resize");
        else if (topBorder) setResize("n-resize");
        else if (bottomBorder) setResize("s-resize");
        else if (rightBorder) setResize("e-resize");
        else setResize();
    }

    function update(event) {
        let eventType;
        let eventButtons = event.buttons;
        switch (event.type) {
            case "mousedown":
                eventType = MouseDown;
                eventButtons = buttons | (1 << event.button);
                break;
            case "mouseup":
                eventType = MouseUp;
                eventButtons = buttons & ~(1 << event.button);
                break;
            default:
                eventType = MouseMove;
                break;
        }

        let released = buttons & ~eventButtons;
        let pressed = eventButtons & ~buttons;
        buttons = eventButtons;

        // Windows can swallow the mouseup that ends a drag, so a press of an
        // already-pressed button is treated as a release-press sequence.
        if (eventType === MouseDown && !(pressed & event.button)) {
            released |= 1 << event.button;
            pressed |= 1 << event.button;
        }

        if ((eventType !== MouseMove && resizing) || (dragging && (eventType === MouseDown || event.button !== 0))) {
            event.stopImmediatePropagation();
            event.stopPropagation();
            event.preventDefault();
        }

        if (released & 1) primaryUp();
        if (pressed & 1) primaryDown(event);
        if (eventType === MouseMove) onMouseMove(event);
    }

    function suppressEvent(event) {
        // Suppress click events while a drag or resize is in progress so the
        // underlying UI does not react to the gesture.
        if (dragging || resizing) {
            event.stopImmediatePropagation();
            event.stopPropagation();
            event.preventDefault();
        }
    }

    window.addEventListener("mousedown", update, { capture: true });
    window.addEventListener("mousemove", update, { capture: true });
    window.addEventListener("mouseup", update, { capture: true });
    for (const ev of ["click", "contextmenu", "dblclick"]) {
        window.addEventListener(ev, suppressEvent, { capture: true });
    }
}

/* ============================================================
 * Menu bar (VSCode/Zed style, inside the title bar)
 * ============================================================ */
let openMenuEl = null;

function closeMenuBar() {
    if (!openMenuEl) return;
    openMenuEl.classList.remove("open");
    const dd = openMenuEl.querySelector(".menu-dropdown");
    if (dd) dd.remove();
    openMenuEl = null;
}

function openMenuDropdown(itemEl, menu) {
    if (openMenuEl === itemEl) return;
    closeMenuBar();

    const dd = document.createElement("div");
    dd.className = "menu-dropdown";
    menu.items.forEach((it) => {
        if (it.sep) {
            const s = document.createElement("div");
            s.className = "sep";
            dd.appendChild(s);
            return;
        }
        const row = document.createElement("div");
        row.className = "mi";
        const label = document.createElement("span");
        label.className = "mi-label";
        label.textContent = it.label;
        row.appendChild(label);
        if (it.accel) {
            const accel = document.createElement("span");
            accel.className = "mi-accel";
            accel.textContent = it.accel;
            row.appendChild(accel);
        }
        row.addEventListener("click", () => {
            closeMenuBar();
            it.run();
        });
        dd.appendChild(row);
    });

    itemEl.appendChild(dd);
    itemEl.classList.add("open");
    openMenuEl = itemEl;
}

function buildMenuBar(bar) {
    const menus = [
        {
            label: "文件",
            items: [
                { label: "新建连接", accel: "Ctrl+N", run: () => openConnEditor(null) },
                { label: "连接管理", accel: "Ctrl+M", run: () => openConnectionsPage() },
                { sep: true },
                { label: "退出", accel: "Ctrl+Q", run: () => runAppAction("quit") },
            ],
        },
        {
            label: "选项",
            items: [
                { label: "选项…", accel: "Ctrl+Shift+,", run: () => runAppAction("settings") },
                { label: "密钥管理…", accel: "Ctrl+Shift+K", run: () => runAppAction("keys") },
            ],
        },
        {
            label: "视图",
            items: [
                { label: "连接", accel: "Ctrl+1", run: () => navigateTo("home") },
                { label: "文件传输", accel: "Ctrl+2", run: () => navigateTo("sftp") },
                { label: "端口隧道", accel: "Ctrl+3", run: () => navigateTo("tunnels") },
            ],
        },
        {
            label: "帮助",
            items: [
                { label: "关于 wsh", run: showAbout },
                { label: "开发者工具", run: () => runAppAction("devtools") },
            ],
        },
    ];

    menus.forEach((menu) => {
        const item = document.createElement("div");
        item.className = "menu-item";
        const btn = document.createElement("button");
        btn.type = "button";
        btn.textContent = menu.label;
        item.appendChild(btn);
        btn.addEventListener("click", (e) => {
            e.stopPropagation();
            if (openMenuEl === item) closeMenuBar();
            else openMenuDropdown(item, menu);
        });
        // Hovering another top-level item while a menu is open switches to it.
        item.addEventListener("mouseenter", () => {
            if (openMenuEl) openMenuDropdown(item, menu);
        });
        bar.appendChild(item);
    });

    document.addEventListener("click", closeMenuBar);
    document.addEventListener("keydown", (e) => {
        if (e.key === "Escape") closeMenuBar();
    });
}

/* ============================================================
 * Title bar
 * ============================================================ */
const CONTROL_ICONS = {
    minimise: '<svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true"><rect x="0" y="4.5" width="10" height="1" fill="currentColor"/></svg>',
    maximise: '<svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true"><rect x="0.5" y="0.5" width="9" height="9" fill="none" stroke="currentColor"/></svg>',
    restore: '<svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true"><rect x="0.5" y="2.5" width="7" height="7" fill="none" stroke="currentColor"/><path d="M2.5 2.5V0.5H9.5V7.5H7.5" fill="none" stroke="currentColor"/></svg>',
    close: '<svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true"><path d="M0.5 0.5L9.5 9.5M9.5 0.5L0.5 9.5" fill="none" stroke="currentColor"/></svg>',
};

// windowControl asks the desktop shell to act on this window. The shell routes
// it to the window named by WINDOW_NAME.
function windowControl(op) {
    return RPC.call("window.control", { window: WINDOW_NAME, action: op });
}

function controlButton(kind, label, onClick) {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "tb-btn" + (kind === "close" ? " close" : "");
    b.title = label;
    b.setAttribute("aria-label", label);
    b.innerHTML = CONTROL_ICONS[kind];
    b.addEventListener("click", (e) => {
        e.stopPropagation();
        onClick();
    });
    return b;
}

// refreshMaximiseIcon keeps the maximise/restore glyph in sync with the window
// state (which can also change via snapping or double-click).
function refreshMaximiseIcon(btn) {
    windowControl("is-maximised")
        .then((max) => {
            btn.innerHTML = max ? CONTROL_ICONS.restore : CONTROL_ICONS.maximise;
            btn.title = max ? "还原" : "最大化";
        })
        .catch(() => {});
}

// buildChrome draws the custom title bar. It is a no-op when the page is not in
// a frameless Wails window (plain browser) or runs on macOS.
function buildChrome() {
    const bar = document.getElementById("titlebar");
    if (!bar || !WINDOW_NAME || isMacOS()) return;

    const cfg = CHROME[WINDOW_NAME] || {};

    // Brand / title.
    const brand = document.createElement("div");
    brand.className = "tb-brand";
    if (cfg.brand) {
        brand.innerHTML = '<span class="logo-dot"></span><span>WSH</span>';
    } else {
        brand.textContent = cfg.title || document.title;
    }
    bar.appendChild(brand);

    // Menu (main window only).
    if (cfg.menu) {
        const menubar = document.createElement("div");
        menubar.className = "menubar";
        bar.appendChild(menubar);
        buildMenuBar(menubar);
    }

    // Draggable filler.
    const spacer = document.createElement("div");
    spacer.className = "tb-spacer";
    bar.appendChild(spacer);

    // Window controls.
    const controls = document.createElement("div");
    controls.className = "tb-controls";
    controls.appendChild(controlButton("minimise", "最小化", () => windowControl("minimise")));
    const maxBtn = controlButton("maximise", "最大化", () => {
        windowControl("toggle-maximise").then(() => refreshMaximiseIcon(maxBtn)).catch(() => {});
    });
    controls.appendChild(maxBtn);
    controls.appendChild(controlButton("close", "关闭", () => windowControl("close")));
    bar.appendChild(controls);

    // Double-clicking the drag area toggles maximise, like a native title bar.
    spacer.addEventListener("dblclick", () => {
        windowControl("toggle-maximise").then(() => refreshMaximiseIcon(maxBtn)).catch(() => {});
    });

    let resizeTimer = null;
    window.addEventListener("resize", () => {
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(() => refreshMaximiseIcon(maxBtn), 160);
    });
    refreshMaximiseIcon(maxBtn);

    installWindowGestures();
}

// showAbout renders a themed About dialog instead of the native one.
function showAbout() {
    const body = document.createElement("div");
    body.className = "about";
    body.innerHTML = `
      <div class="about-logo"><span class="logo-dot"></span></div>
      <div class="about-title">wsh</div>
      <div class="about-desc">
        基于 Go + Wails v3 的跨平台 SSH 客户端<br>
        终端渲染：xterm.js
      </div>`;
    Modal.open("关于 wsh", body, [button("btn primary", "关闭", () => Modal.close())]);
}

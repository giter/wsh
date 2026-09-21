// Frameless window support.
//
// Windows/Linux run frameless so the whole window can match the app theme: the
// native title bar and menu bar are gone and <TitleBar> draws a custom one.
//
// Wails does not load its runtime JS into this self-hosted frontend, but the
// platform layer still injects `window._wails.invoke`, which is all we need:
// the native side handles "wails:drag" and "wails:resize:<edge>" messages. The
// gesture logic below is a port of the Wails v3 runtime's drag.ts.

import { rpc } from "./rpc.js";

// WINDOW_NAME comes from the "?win=" query param the Go side puts on each
// window URL ("main", "settings", "keys", ...). Empty when the page is opened
// in a plain browser (headless debugging), where there is no custom chrome.
export const WINDOW_NAME = new URLSearchParams(location.search).get("win") || "";

// isMacOS reports whether the page runs in a macOS webview. macOS keeps the
// native frame and global menu, so no custom chrome is drawn there.
export function isMacOS() {
    return /Mac|iPhone|iPad|iPod/.test(navigator.platform || navigator.userAgent || "");
}

// hasCustomChrome reports whether this window draws its own title bar.
export function hasCustomChrome() {
    return !!WINDOW_NAME && !isMacOS();
}

function wailsInvoke(msg) {
    if (window._wails && typeof window._wails.invoke === "function") {
        window._wails.invoke(msg);
    }
}

// windowControl asks the desktop shell to act on this window. The shell routes
// it to the window named by WINDOW_NAME.
export function windowControl(op) {
    return rpc.call("window.control", { window: WINDOW_NAME, action: op });
}

// installWindowGestures wires window dragging and edge resizing. The resize
// path is required because a frameless window's standard frame is hidden, which
// disables OS-driven resizing; it must be driven from the frontend.
export function installWindowGestures() {
    if (!hasCustomChrome()) return () => {};

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

    return () => {
        window.removeEventListener("mousedown", update, { capture: true });
        window.removeEventListener("mousemove", update, { capture: true });
        window.removeEventListener("mouseup", update, { capture: true });
        for (const ev of ["click", "contextmenu", "dblclick"]) {
            window.removeEventListener(ev, suppressEvent, { capture: true });
        }
    };
}

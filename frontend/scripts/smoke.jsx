// Headless boot smoke test.
//
// Mounts the real app in a jsdom document with a fake WebSocket backend and
// asserts that it actually renders. Its job is to catch the class of bug that
// leaves a blank window: a render-time throw (undefined access, a hook
// dependency array referencing a const declared further down, a bad import, ...)
// aborts React's first render, so the webview shows nothing but the page
// background — which is exactly what "打开以后黑屏" looks like.
//
// Run with: bun run smoke

import { JSDOM } from "jsdom";

const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
    url: "http://127.0.0.1:17777/?win=main",
    pretendToBeVisual: true,
});

// Install the browser globals the app reads at module load and during render.
// windowChrome.js reads location.search when it is imported, so this has to
// happen before the app modules are imported (hence the dynamic import below).
const { window } = dom;
globalThis.window = window;
globalThis.document = window.document;
globalThis.navigator = window.navigator;
globalThis.location = window.location;
globalThis.HTMLElement = window.HTMLElement;
globalThis.Element = window.Element;
globalThis.Node = window.Node;
globalThis.Event = window.Event;
globalThis.CustomEvent = window.CustomEvent;
globalThis.KeyboardEvent = window.KeyboardEvent;
globalThis.getComputedStyle = window.getComputedStyle.bind(window);
globalThis.requestAnimationFrame = window.requestAnimationFrame.bind(window);
globalThis.cancelAnimationFrame = window.cancelAnimationFrame.bind(window);

const errors = [];
window.addEventListener("error", (e) => errors.push(e.error || e.message));
window.addEventListener("unhandledrejection", (e) => errors.push(e.reason));

// ---- Fake backend -----------------------------------------------------------

// Canned replies for the RPCs the boot sequence makes. Anything unlisted gets an
// empty success, which is enough for the window to come up.
const REPLIES = {
    "settings.get": { theme: "dark" },
    "connections.list": [],
    "folders.list": [],
    "keys.list": [],
    "ai.status": { configured: false },
    "probes.snapshot": {},
};

class FakeWebSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    constructor(url) {
        this.url = url;
        this.readyState = FakeWebSocket.CONNECTING;
        this.sent = [];
        setTimeout(() => {
            this.readyState = FakeWebSocket.OPEN;
            this.onopen?.({});
        }, 0);
    }

    send(raw) {
        let req;
        try {
            req = JSON.parse(raw);
        } catch {
            return;
        }
        this.sent.push(req);
        const data = Object.prototype.hasOwnProperty.call(REPLIES, req.method) ? REPLIES[req.method] : {};
        setTimeout(() => {
            this.onmessage?.({ data: JSON.stringify({ id: req.id, ok: true, data }) });
        }, 0);
    }

    close() {
        this.readyState = FakeWebSocket.CLOSED;
    }
    addEventListener() {}
    removeEventListener() {}
}

globalThis.WebSocket = FakeWebSocket;
window.WebSocket = FakeWebSocket;

// ---- Mount -----------------------------------------------------------------

const fail = (msg) => {
    console.error(`✗ smoke: ${msg}`);
    process.exit(1);
};

let React;
let ReactDOMClient;
let App;
let AppProvider;
try {
    React = (await import("react")).default;
    ReactDOMClient = await import("react-dom/client");
    ({ default: App } = await import("../src/App.jsx"));
    ({ AppProvider } = await import("../src/state/store.jsx"));
} catch (e) {
    fail(`importing the app threw:\n${e?.stack || e}`);
}

const root = ReactDOMClient.createRoot(document.getElementById("root"));
try {
    // createElement rather than JSX: this script stays free of any build step.
    root.render(React.createElement(AppProvider, null, React.createElement(App)));
} catch (e) {
    fail(`the first render threw:\n${e?.stack || e}`);
}

// Give the boot effect (connect → load settings → setReady) a few ticks to run.
await new Promise((resolve) => setTimeout(resolve, 300));

if (errors.length) {
    const first = errors[0];
    fail(`the app reported an error during boot:\n${first?.stack || first}`);
}

const app = document.getElementById("app");
if (!app) {
    fail("nothing was rendered (#app is missing) — the window would be blank");
}

const titlebar = document.getElementById("titlebar");
if (!titlebar) {
    fail("the title bar did not render");
}

// The store's boot must have finished, otherwise the UI is stuck on the
// "connecting" screen even though nothing threw.
if (!app.textContent || !app.textContent.trim()) {
    fail("the app rendered an empty shell");
}

console.log(`✓ smoke: app booted and rendered (${app.textContent.trim().slice(0, 40)}…)`);
process.exit(0);

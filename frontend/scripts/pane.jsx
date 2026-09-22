// Reasoning-pane flow check.
//
// The smoke test only proves the app boots; this one drives the third column the
// way a user does and asserts on the DOM: open a tab, collapse the pane and bring
// it back through each of its three affordances, then ask a natural-language
// question and check that the question card, the answer card and the parsed
// thinking steps all render.
//
// It exists because the pane's behaviour (collapse cycle, conversation cards,
// streaming vs parsed reply) cannot be eyeballed in a headless environment.
//
// Run with: bun run pane

import { JSDOM } from "jsdom";

const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
    url: "http://127.0.0.1:17777/?win=main",
    pretendToBeVisual: true,
});
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
globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
};
// xterm queries the screen resolution on open(); jsdom has no matchMedia.
window.matchMedia = (query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent: () => false,
});
globalThis.matchMedia = window.matchMedia;

const errors = [];
window.addEventListener("error", (e) => errors.push(e.error || e.message));
window.addEventListener("unhandledrejection", (e) => errors.push(e.reason));

const REPLIES = {
    "settings.get": { theme: "dark", aiAutoRun: true },
    "connections.list": [],
    "folders.list": [],
    "keys.list": [],
    "ai.status": { configured: true },
    "probes.snapshot": {},
    "terminal.open": { sessionId: "sess-1" },
    // exec answers like the real gate: a yellow-zone command comes back needing
    // confirmation, anything else is written.
    "terminal.exec": (params) =>
        String(params?.command || "").startsWith("rm ")
            ? {
                  confirm: true,
                  result: { level: "caution", findings: [{ level: "caution", reason: "递归删除目录，需二次确认" }] },
              }
            : { written: true },
    "ai.ask": () => ({ requestId: `req-${++askSeq}`, kind: "command" }),
    "safety.confirm": { token: "tok-1" },
    "safety.allowed": { commands: [] },
};

// askSeq keeps every ai.ask reply distinguishable, the way the real backend does.
let askSeq = 0;

// The fake socket is kept so the test can deliver server pushes (ai.done).
let lastSocket = null;

class FakeWebSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;
    constructor() {
        this.readyState = 1;
        this.sent = [];
        this.replies = [];
        lastSocket = this;
        setTimeout(() => this.onopen?.({}), 0);
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
        const payload = typeof data === "function" ? data(req.params) : data;
        this.replies.push({ method: req.method, data: payload });
        setTimeout(() => this.onmessage?.({ data: JSON.stringify({ id: req.id, ok: true, data: payload }) }), 0);
    }
    close() {}
    addEventListener() {}
    removeEventListener() {}
}
globalThis.WebSocket = FakeWebSocket;
window.WebSocket = FakeWebSocket;

const React = (await import("react")).default;
const ReactDOMClient = await import("react-dom/client");
const { default: App } = await import("../src/App.jsx");
const { AppProvider } = await import("../src/state/store.jsx");

const root = ReactDOMClient.createRoot(document.getElementById("root"));
root.render(React.createElement(AppProvider, null, React.createElement(App)));

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const tick = async (n = 6) => {
    for (let i = 0; i < n; i++) await sleep(10);
};

await tick(20);

// Open a quick-connect tab so the third column exists at all.
const qc = document.querySelector("#quick-connect input");
if (!qc) throw new Error("quick connect input not found");
const setValue = (el, v) => {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set;
    setter.call(el, v);
    el.dispatchEvent(new window.Event("input", { bubbles: true }));
};
setValue(qc, "ssh://lee@192.168.8.33:22");
await tick();

const connectBtn = [...document.querySelectorAll("#quick-connect button")].find((b) => b.textContent.includes("连接"));
if (!connectBtn) throw new Error("connect button not found");
connectBtn.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
await tick(30);

const report = (label, ok, extra = "") => console.log(`${ok ? "✓" : "✗"} ${label}${extra ? " — " + extra : ""}`);

const pane = document.getElementById("reason-pane");
report("pane visible after opening a tab", !!pane);
if (!pane) {
    console.log("tabs in DOM:", document.querySelectorAll(".tab-page").length);
    process.exit(1);
}

// 1) Collapse via the header ✕, then reopen via the strip.
const closeBtn = [...document.querySelectorAll("#reason-header .icon-btn")].find((b) => b.textContent.includes("✕"));
closeBtn.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
await tick();
const strip = document.getElementById("reason-reopen");
report("collapsed pane leaves a reopen strip", !!strip, strip ? JSON.stringify(strip.textContent) : "");

if (strip) {
    strip.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await tick();
    report("clicking the strip reopens the pane", !!document.getElementById("reason-pane"));
}

// 2) Ctrl+Shift+A toggles it too.
const key = (code, shift) =>
    document.dispatchEvent(new window.KeyboardEvent("keydown", { code, key: code, ctrlKey: true, shiftKey: shift, bubbles: true }));
key("KeyA", true);
await tick();
report("Ctrl+Shift+A collapses the pane", !document.getElementById("reason-pane"));
key("KeyA", true);
await tick();
report("Ctrl+Shift+A brings it back", !!document.getElementById("reason-pane"));

// 3) The view menu carries a toggle.
const viewBtn = [...document.querySelectorAll(".menubar .menu-item > button")].find((b) => b.textContent === "视图");
viewBtn.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
await tick();
const labels = [...document.querySelectorAll(".menu-dropdown .mi-label")].map((s) => s.textContent);
report("view menu lists the pane toggle", labels.some((l) => l.includes("AI 推理窗格")), labels.join(" / "));

if (errors.length) {
    console.log("errors during run:", errors.slice(0, 3).map((e) => e?.stack || e));
    process.exit(1);
}

// 4) A natural-language turn shows the question and the answer in the pane.
const field = document.querySelector("#smart-input .si-field");
report("smart input rendered", !!field);
if (field) {
    setValue(field, "当前系统状态");
    await tick();
    field.dispatchEvent(
        new window.KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true }),
    );
    await tick();
    const ask = document.querySelector(".reason-card.ask");
    report("the question becomes its own card", !!ask, ask ? JSON.stringify(ask.textContent.slice(0, 30)) : "");
    const reply = [...document.querySelectorAll(".reason-card.cot .rc-title")].map((e) => e.textContent);
    report("the answer card exists", reply.includes("AI 回复"), reply.join(" / "));
    report("the input was cleared", field.value === "");

    // A green-zone command must run on its own, without a click.
    const before = lastSocket.sent.filter((r) => r.method === "terminal.exec").length;
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "ai.done",
            requestId: "req-1",
            ok: true,
            text: "[意图分析] 用户想查看系统状态\n[生成指令] uptime",
            steps: [
                { label: "意图分析", detail: "用户想查看系统状态" },
                { label: "生成指令", detail: "uptime" },
            ],
            command: "uptime",
            risk: { level: "safe" },
        }),
    });
    await tick();
    const execReqs = lastSocket.sent.filter((r) => r.method === "terminal.exec");
    report("a green command auto-executes", execReqs.length === before + 1, `terminal.exec × ${execReqs.length}`);
    const trackId = execReqs[execReqs.length - 1]?.params?.trackId;
    report("the automatic run is tagged with the card id", !!trackId, `trackId=${trackId}`);

    const steps = document.querySelectorAll(".reason-card.cot .rc-steps li");
    report("parsed steps are rendered", steps.length === 2, [...steps].map((s) => s.textContent).join(" | "));
    const rawSummary = document.querySelector(".rc-raw > summary");
    report("the raw thinking stays available", !!rawSummary, rawSummary ? rawSummary.textContent : "");
    const execStatus = document.querySelector(".rc-exec");
    report(
        "the card says it ran by itself",
        !!execStatus && execStatus.textContent.includes("自动执行"),
        execStatus ? execStatus.textContent : "",
    );

    // A yellow-zone command must not run on its own — but with auto-run on it
    // is submitted so the confirmation panel comes to the user instead of
    // waiting quietly in the card.
    const beforeYellow = lastSocket.sent.filter((r) => r.method === "terminal.exec").length;
    setValue(field, "清理构建目录");
    await tick();
    field.dispatchEvent(
        new window.KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true }),
    );
    await tick();
    const yellowReq = [...lastSocket.replies].reverse().find((r) => r.method === "ai.ask")?.data?.requestId;
    report("the second question starts a request", !!yellowReq, `requestId=${yellowReq}`);
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "ai.done",
            requestId: yellowReq,
            ok: true,
            text: "[生成指令] rm -rf /tmp/build",
            steps: [{ label: "生成指令", detail: "rm -rf /tmp/build" }],
            command: "rm -rf /tmp/build",
            risk: { level: "caution", reason: "递归删除目录" },
        }),
    });
    await tick();
    const yellowExecs = lastSocket.sent.filter((r) => r.method === "terminal.exec");
    report("a yellow command is submitted so it can be confirmed", yellowExecs.length === beforeYellow + 1);
    const dryRun = document.querySelector(".reason-card.dry-run");
    report("the confirmation panel opens by itself", !!dryRun);
    const levels = [...document.querySelectorAll(".reason-card.dry-run .rc-actions button")].map((b) =>
        b.textContent.trim(),
    );
    report(
        "the panel offers the three approval levels",
        levels.includes("仅此一次") && levels.includes("本会话允许") && levels.includes("始终允许"),
        levels.join(" / "),
    );

    // Choosing a level records it and runs the command in one gesture.
    const always = [...document.querySelectorAll(".reason-card.dry-run .rc-actions button")].find((b) =>
        b.textContent.includes("始终允许"),
    );
    always.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await tick();
    const confirmReq = lastSocket.sent.filter((r) => r.method === "safety.confirm").pop();
    report("the level is sent with the confirmation", confirmReq?.params?.scope === "always", JSON.stringify(confirmReq?.params));

    // A command the user approved is not an automatic one: reporting it as
    // "已自动执行（绿区）" misread both who decided and how risky it was (a sudo
    // command made this visible).
    const execRows = [...document.querySelectorAll(".rc-exec")].map((e) => e.textContent);
    const approvedRow = execRows.find((t) => t.includes("已确认"));
    report(
        "an approved command reads as confirmed, never as auto-run/绿区",
        !!approvedRow && !approvedRow.includes("自动执行") && !approvedRow.includes("绿区"),
        execRows.join(" | "),
    );

    // A command sitting at a password prompt says so, instead of spinning.
    const yellowTrack = yellowExecs[yellowExecs.length - 1]?.params?.trackId;
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "terminal.waiting",
            sessionId: "sess-1",
            trackId: yellowTrack,
            command: "rm -rf /tmp/build",
            waiting: true,
        }),
    });
    await tick();
    const waitRow = [...document.querySelectorAll(".rc-exec")].find((e) => e.textContent.includes("等待输入"));
    report("a command at a password prompt is shown as waiting", !!waitRow, waitRow ? waitRow.textContent : "");

    // 5) The analysis attaches to the card and is rendered with its weights.
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "terminal.output",
            sessionId: "sess-1",
            trackId,
            command: "uptime",
            text: " 10:53 up 3 days,  1 user,  load average: 0.12",
            durationMs: 400,
        }),
    });
    await tick();

    const asks = lastSocket.sent.filter((r) => r.method === "ai.ask");
    report("the captured output triggers a result analysis", asks.length >= 2, `ai.ask × ${asks.length}`);
    const analysisReq = [...lastSocket.replies].reverse().find((r) => r.method === "ai.ask")?.data?.requestId;
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "ai.done",
            requestId: analysisReq,
            ok: true,
            text: "[结论] ok",
            steps: [
                { label: "结论", detail: "命令执行成功，未发现异常" },
                { label: "要点", detail: "wglink 占用 0.7% CPU" },
                { label: "风险", detail: "1080/1081 端口疑似代理服务，需确认" },
            ],
            suggestions: [{ label: "路由追踪", command: "traceroute 223.5.5.5" }],
        }),
    });
    await tick();

    const tones = [...document.querySelectorAll(".rc-analysis .rc-step")].map((li) => li.className);
    report("analysis steps carry a tone class", tones.length === 3, tones.join(" | "));
    report(
        "the warning is singled out from the points",
        tones.some((c) => c.includes("tone-risk")) && tones.some((c) => c.includes("tone-point")),
    );
    const badge = [...document.querySelectorAll(".rc-head .risk-pill")].map((e) => e.textContent);
    report("the card header advertises the warning", badge.some((t) => t.includes("有风险提示")), badge.join(" | "));
    const narration = [...document.querySelectorAll(".rc-step.tone-narration")].length;
    report("narration is rendered recessively", narration > 0, `${narration} narration step(s)`);

    // Folding: once the conversation has moved on, older turns shrink to a
    // one-line recap so a long investigation stays scannable.
    const folded = document.querySelector(".reason-card.ask .rc-summary");
    report("an older turn folds to one line", !!folded, folded ? JSON.stringify(folded.textContent) : "");

    // Anchor linking: the outcome offers a way back to the raw terminal output.
    const revealBtn = [...document.querySelectorAll(".rc-execrow button")].find((b) =>
        b.textContent.includes("查看原始输出"),
    );
    report("the card links back to its raw output", !!revealBtn);

    // Quick follow-up: one click is the whole gesture — the chip records the
    // question and runs the recommended command without touching the input box.
    const chip = [...document.querySelectorAll(".rc-followups button")].find((b) => b.textContent.includes("路由追踪"));
    report("the analysis offers follow-up chips", !!chip);
    if (chip) {
        const beforeChip = lastSocket.sent.filter((r) => r.method === "terminal.exec").length;
        chip.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
        await tick();
        const asked = [...document.querySelectorAll(".reason-card.ask .rc-title")].some(
            (t) => t.textContent === "快捷追问",
        );
        report("the chip becomes the user's next turn", asked);
        const afterChip = lastSocket.sent.filter((r) => r.method === "terminal.exec");
        report("the chip runs its command in one click", afterChip.length === beforeChip + 1, `terminal.exec × ${afterChip.length}`);
    }

    // A command that will not end on its own is called out as a stream, and the
    // card offers the only useful key there: Ctrl+C.
    const streamTrack = lastSocket.sent.filter((r) => r.method === "terminal.exec").pop()?.params?.trackId;
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "terminal.streaming",
            sessionId: "sess-1",
            trackId: streamTrack,
            command: "traceroute 223.5.5.5",
            elapsedMs: 3200,
        }),
    });
    await tick();
    const streamRow = document.querySelector(".rc-exec.streaming");
    report(
        "a long-running command is labelled as streaming",
        !!streamRow && streamRow.textContent.includes("持续监听中"),
        streamRow ? streamRow.textContent : "",
    );
    const stopBtn = [...document.querySelectorAll(".rc-exec.streaming button")].find((b) => b.textContent.includes("停止"));
    report("the stream offers a stop button", !!stopBtn);
    if (stopBtn) {
        const beforeInput = lastSocket.sent.filter((r) => r.method === "terminal.input").length;
        stopBtn.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
        await tick();
        const inputs = lastSocket.sent.filter((r) => r.method === "terminal.input");
        report(
            "stopping sends Ctrl+C to the PTY",
            inputs.length === beforeInput + 1 && inputs[inputs.length - 1].params.data === "\u0003",
            JSON.stringify(inputs[inputs.length - 1]?.params),
        );
    }

    // Esc is the single "stop it" key: it abandons a generation that is taking too
    // long instead of leaving the box waiting.
    setValue(field, "系统负载");
    await tick();
    field.dispatchEvent(
        new window.KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true }),
    );
    await tick();
    const cancelReq = [...lastSocket.replies].reverse().find((r) => r.method === "ai.ask")?.data?.requestId;
    field.dispatchEvent(
        new window.KeyboardEvent("keydown", { key: "Escape", code: "Escape", bubbles: true, cancelable: true }),
    );
    await tick();
    const cancelCall = lastSocket.sent.filter((r) => r.method === "ai.cancel").pop();
    report("Esc abandons the in-flight generation", cancelCall?.params?.requestId === cancelReq, JSON.stringify(cancelCall?.params));

    // A segment that ended at a prompt for a secret is not a result: the card
    // must not bill it as a success (this is the sudo case from the field).
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "terminal.output",
            sessionId: "sess-1",
            trackId: streamTrack,
            command: "traceroute 223.5.5.5",
            text: "traceroute to 223.5.5.5\n[sudo] password for lee: ",
            durationMs: 900,
            waitingInput: true,
        }),
    });
    await tick();
    const stalledRow = [...document.querySelectorAll(".rc-exec")].find((e) => e.textContent.includes("输入提示"));
    report("a segment stuck at a prompt is not billed as success", !!stalledRow, stalledRow ? stalledRow.textContent : "");

    // A failed result analysis has its own retry: re-reading the output is not the
    // same thing as running the command again.
    const stalledAnalysis = [...lastSocket.replies].reverse().find((r) => r.method === "ai.ask");
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "ai.done",
            requestId: stalledAnalysis?.data?.requestId,
            ok: false,
            error: "AI 请求超时",
        }),
    });
    await tick();
    const analysisRetry = [...document.querySelectorAll(".rc-analysis .rc-actions button")].find((b) =>
        b.textContent.includes("重试分析"),
    );
    report("a failed analysis offers its own retry", !!analysisRetry);
    if (analysisRetry) {
        const execsBefore = lastSocket.sent.filter((r) => r.method === "terminal.exec").length;
        analysisRetry.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
        await tick();
        const execsAfter = lastSocket.sent.filter((r) => r.method === "terminal.exec").length;
        report("retrying the analysis does not re-run the command", execsAfter === execsBefore, `terminal.exec × ${execsAfter}`);
    }

    // A failed completion can be retried by hand, and the retry replays exactly the
    // same request rather than asking something else.
    setValue(field, "检查磁盘使用率");
    await tick();
    field.dispatchEvent(
        new window.KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true }),
    );
    await tick();
    const attemptsBefore = lastSocket.sent.filter((r) => r.method === "ai.ask").length;
    const failedReq = [...lastSocket.replies].reverse().find((r) => r.method === "ai.ask");
    lastSocket.onmessage({
        data: JSON.stringify({
            type: "ai.done",
            requestId: failedReq?.data?.requestId,
            ok: false,
            error: "dial tcp: connection refused",
        }),
    });
    await tick();
    const failedCard = document.querySelector(".reason-card.cot.error");
    report("a failed completion is marked as failed", !!failedCard, failedCard ? failedCard.textContent.slice(0, 40) : "");
    const retryBtn =
        failedCard && [...failedCard.querySelectorAll(".rc-actions button")].find((b) => b.textContent.trim() === "重试");
    report("a failed completion offers a manual retry", !!retryBtn);

    if (retryBtn) {
        retryBtn.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
        await tick();
        const attempts = lastSocket.sent.filter((r) => r.method === "ai.ask");
        report(
            "the retry replays the same request",
            attempts.length === attemptsBefore + 1 && attempts[attempts.length - 1].params.prompt === "检查磁盘使用率",
            JSON.stringify(attempts[attempts.length - 1].params),
        );
        const retryReq = [...lastSocket.replies].reverse().find((r) => r.method === "ai.ask");
        lastSocket.onmessage({
            data: JSON.stringify({
                type: "ai.done",
                requestId: retryReq?.data?.requestId,
                ok: true,
                text: "[生成指令] df -h",
                steps: [{ label: "生成指令", detail: "df -h" }],
                command: "df -h",
                risk: { level: "safe" },
            }),
        });
        await tick();
        report("the retried card leaves the failed state", !document.querySelector(".reason-card.cot.error"));
    }
}

if (errors.length) {
    console.log("errors during run:", errors.slice(0, 3).map((e) => e?.stack || e));
    process.exit(1);
}
process.exit(0);

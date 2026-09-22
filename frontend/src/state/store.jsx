import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { rpc } from "../lib/rpc.js";
import { WINDOW_NAME } from "../lib/windowChrome.js";

// The app runs in several native windows that all share one backend:
//
//   main     — the terminal sessions, with the session manager docked on the
//              left (quick connect bar + session tabs)
//   sftp     — file transfer, closable
//   tunnels  — port forwarding, closable
//   settings — global options, closable
//   keys     — key manager, closable
//
// Only the "main" window hosts terminal tabs. The tool windows ask for a session
// to be opened through the backend (see openSession below), which the session
// window picks up and turns into a tab.
export const WINDOW = WINDOW_NAME || "main";

// isSessionWindow reports whether this window hosts terminal tabs and the docked
// session manager.
export const isSessionWindow = WINDOW === "main";

const AppCtx = createContext(null);

export function useApp() {
    const ctx = useContext(AppCtx);
    if (!ctx) throw new Error("useApp must be used inside <AppProvider>");
    return ctx;
}

// DEFAULT_FONT_SIZE is the UI size that counts as 100% zoom. Settings store 0 for
// "unset", which resolves back to this.
export const DEFAULT_FONT_SIZE = 13;

// The UI is scaled by zooming the whole webview, not with a CSS font size: the
// stylesheet is fixed px throughout (so a root font-size changes almost nothing),
// and a native zoom also scales the terminal canvas while keeping 100vh layouts
// fitting the window. Windows clamps the webview zoom at 100%, hence the floor.
export const MIN_FONT_SIZE = DEFAULT_FONT_SIZE;
export const MAX_FONT_SIZE = 32;

// zoomFactorFor maps the stored UI size to a webview zoom factor.
export function zoomFactorFor(fontSize) {
    const px = parseInt(fontSize, 10);
    const base = px >= MIN_FONT_SIZE && px <= MAX_FONT_SIZE ? px : DEFAULT_FONT_SIZE;
    return base / DEFAULT_FONT_SIZE;
}

// zoomPercentFor is the same value as the percentage shown in the options window.
export function zoomPercentFor(fontSize) {
    return Math.round(zoomFactorFor(fontSize) * 100);
}

// applyZoom asks the desktop shell to zoom this window. Every window zooms itself
// as it applies settings, so the zoom follows the user across windows without the
// backend tracking a factor per window. Headless runs have no webview; the failed
// call is ignored.
export function applyZoom(fontSize) {
    rpc.call("app.action", { action: "zoom", window: WINDOW, zoom: zoomFactorFor(fontSize) }).catch(() => {});
}

// requestLatinInput asks the desktop shell to switch this window's OS input
// method to English, so a passphrase prompt starts out accepting ASCII instead
// of whatever IME the user had active. Only the native side can do this; headless
// runs and platforms without an IME ignore the failed call.
export function requestLatinInput() {
    rpc.call("app.action", { action: "ime-latin", window: WINDOW }).catch(() => {});
}

// applySettings pushes global options into the live UI: the zoom and the theme.
function applySettings(st) {
    applyZoom(st.fontSize);
    document.documentElement.dataset.theme = st.theme === "light" ? "light" : "dark";
}

export function AppProvider({ children }) {
    const [ready, setReady] = useState(false);
    const [bootError, setBootError] = useState(null);
    const [settings, setSettings] = useState({});
    const [connections, setConnections] = useState([]);
    const [folders, setFolders] = useState([]);
    const [keys, setKeys] = useState([]);
    const [tabs, setTabs] = useState([]);
    const [activeTab, setActiveTab] = useState(null);
    const [dialog, setDialog] = useState(null);
    // The session manager is docked on the left of the session window; like
    // XShell it can be collapsed and brought back (title bar button / Ctrl+1).
    const [sessionManagerOpen, setSessionManagerOpen] = useState(true);

    // ---- Reasoning pane (CoT / RCA / dry-run) ----
    // Cards are tagged with the tab that produced them, so the pane always shows
    // the reasoning for the session on screen.
    const [reason, setReason] = useState([]);
    const [reasonOpen, setReasonOpen] = useState(true);
    // draft is text another component wants to place into the Smart Input
    // (the "一键修复" action on a card).
    const [draft, setDraft] = useState(null);
    // pendingConfirm holds a yellow-zone command awaiting approval.
    const [pendingConfirm, setPendingConfirm] = useState(null);
    const [aiStatus, setAiStatus] = useState({ configured: false });
    // Host resource samples, keyed by connection ID (see probe.go).
    const [stats, setStats] = useState({});

    // focusByTab records, per tab, the card that most recently received content.
    // The pane keeps that card expanded as well as the newest one, so an answer
    // that lands while a newer question is already on screen is shown instead of
    // folding itself away the instant it arrives.
    const [focusByTab, setFocusByTab] = useState({});
    const noteFocus = useCallback((tabId, cardId) => {
        if (!tabId || !cardId) return;
        setFocusByTab((m) => (m[tabId] === cardId ? m : { ...m, [tabId]: cardId }));
    }, []);

    const reasonSeq = useRef(0);
    // Maps an in-flight AI request to the card (and nested field) it updates, so
    // several requests can be in flight and a result analysis lands on the card
    // that proposed the command instead of opening a new one.
    const aiRequests = useRef(new Map());
    // Long-lived push handlers must see the freshest reasoning pane and AI status.
    const reasonRef = useRef(reason);
    reasonRef.current = reason;
    const aiStatusRef = useRef(aiStatus);
    aiStatusRef.current = aiStatus;
    // Maps a terminal tab to its live session id, so a reasoning card can address
    // the PTY without the id being threaded through every component.
    const sessions = useRef(new Map());
    // Maps a terminal tab to the bridge its xterm exposes (mark a command's line,
    // scroll to it, highlight it). The terminal is the only place that knows where
    // a command was rendered, so the pane asks it instead of guessing.
    const terminals = useRef(new Map());
    // Lets the long-lived push handlers reach the latest runner: they are declared
    // before runCommand exists, and must not re-subscribe whenever it changes.
    const runCommandRef = useRef(null);

    const settingsRef = useRef(settings);
    settingsRef.current = settings;
    const connectionsRef = useRef(connections);
    connectionsRef.current = connections;
    const tabsRef = useRef(tabs);
    tabsRef.current = tabs;
    const activeTabRef = useRef(activeTab);
    activeTabRef.current = activeTab;
    // Tracks the font size last written to the backend so the debounced persist
    // effect below only fires for real zoom changes.
    const persistedFontSize = useRef(null);

    const loadSettings = useCallback(async () => {
        try {
            const st = await rpc.call("settings.get");
            persistedFontSize.current = parseInt(st?.fontSize, 10) || 0;
            setSettings(st || {});
        } catch {
            setSettings({});
        }
    }, []);

    const refreshKeys = useCallback(async () => {
        const ks = await rpc.call("keys.list");
        setKeys(ks);
        return ks;
    }, []);

    const refreshConnections = useCallback(async () => {
        const [conns, fds, ks] = await Promise.all([
            rpc.call("connections.list"),
            rpc.call("folders.list").catch(() => []),
            rpc.call("keys.list").catch(() => []),
        ]);
        setConnections(conns);
        setFolders(fds);
        setKeys(ks);
    }, []);

    const saveSettings = useCallback(async (payload) => {
        const st = await rpc.call("settings.save", payload);
        persistedFontSize.current = parseInt(st?.fontSize, 10) || 0;
        setSettings(st || {});
        return st;
    }, []);

    // zoomFont steps the UI size by delta px (DEFAULT_FONT_SIZE = 100%). The zoom
    // itself is a webview zoom applied by applySettings; the new size is persisted
    // (debounced) by the effect below.
    const zoomFont = useCallback((delta) => {
        const cur = parseInt(settingsRef.current.fontSize, 10);
        const base = cur >= MIN_FONT_SIZE && cur <= MAX_FONT_SIZE ? cur : DEFAULT_FONT_SIZE;
        const next = Math.min(MAX_FONT_SIZE, Math.max(MIN_FONT_SIZE, base + delta));
        setSettings((s) => ({ ...s, fontSize: next }));
    }, []);

    // resetZoom returns to 100%. It cannot read the stored size to do so: zoom is
    // persisted, so the stored value is already the zoomed one and resetting to it
    // would be a no-op.
    const resetZoom = useCallback(() => {
        setSettings((s) => ({ ...s, fontSize: DEFAULT_FONT_SIZE }));
    }, []);

    // refreshAIStatus loads whether a reasoning provider is configured. It is
    // defined up here, before the effects that depend on it: a useEffect
    // dependency array is evaluated during render, so referencing a const
    // declared further down would throw a TDZ ReferenceError and leave the
    // window blank.
    const refreshAIStatus = useCallback(async () => {
        try {
            const st = await rpc.call("ai.status");
            setAiStatus(st || { configured: false });
        } catch {
            setAiStatus({ configured: false });
        }
    }, []);

    // ---- Standing command approvals ("始终允许" in the dry-run panel) ----
    //
    // Declared here for the same reason as refreshAIStatus: the boot effect below
    // lists it in a dependency array, which is evaluated during render.
    const [allowedCommands, setAllowedCommands] = useState([]);

    const refreshAllowedCommands = useCallback(async () => {
        try {
            const res = await rpc.call("safety.allowed");
            setAllowedCommands(res?.commands || []);
        } catch {
            setAllowedCommands([]);
        }
    }, []);

    // revokeAllowed drops one standing approval, or all of them.
    const revokeAllowed = useCallback(async (command, all) => {
        const res = await rpc.call("safety.revoke", all ? { all: true } : { command });
        setAllowedCommands(res?.commands || []);
    }, []);

    // Persist font zoom after the user stops pressing the keys.
    useEffect(() => {
        if (!ready) return undefined;
        const fs = parseInt(settings.fontSize, 10);
        if (!fs || fs === persistedFontSize.current) return undefined;
        const timer = setTimeout(() => {
            const st = settingsRef.current;
            rpc.call("settings.save", {
                fontSize: fs,
                theme: st.theme || "dark",
                defaultPort: st.defaultPort || 0,
                defaultUser: st.defaultUser || "",
                tunnelLocalPort: st.tunnelLocalPort || 0,
                tunnelRemotePort: st.tunnelRemotePort || 0,
                // AI options are carried through unchanged: settings.save
                // replaces the whole record, so omitting them would wipe them.
                aiProvider: st.aiProvider || "",
                aiBaseUrl: st.aiBaseUrl || "",
                aiModel: st.aiModel || "",
                aiAutoAnalyze: !!st.aiAutoAnalyze,
                aiNoContext: !!st.aiNoContext,
                aiAutoRun: st.aiAutoRun !== false,
            })
                .then((saved) => {
                    persistedFontSize.current = parseInt(saved?.fontSize, 10) || 0;
                })
                .catch(() => {});
        }, 400);
        return () => clearTimeout(timer);
    }, [ready, settings.fontSize]);

    // Global options changes (from another window) are re-applied here.
    useEffect(() => {
        const off1 = rpc.on("ui.settings-changed", () => {
            loadSettings().catch(() => {});
            refreshAIStatus().catch(() => {});
        });
        const off2 = rpc.on("ui.keys-changed", () => {
            refreshConnections().catch(() => {});
        });
        const off3 = rpc.on("ui.connections-changed", () => {
            refreshConnections().catch(() => {});
        });
        return () => {
            off1();
            off2();
            off3();
        };
    }, [loadSettings, refreshConnections, refreshAIStatus]);

    useEffect(() => {
        applySettings(settings);
    }, [settings]);

    // Boot: connect the RPC socket, then load the initial state.
    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                await rpc.connect();
            } catch (e) {
                if (!cancelled) setBootError(e.message || String(e));
                return;
            }
            if (cancelled) return;
            await Promise.all([
                loadSettings(),
                refreshConnections().catch(() => {}),
                refreshAIStatus().catch(() => {}),
                refreshAllowedCommands().catch(() => {}),
            ]);
            if (!cancelled) setReady(true);
        })();
        return () => {
            cancelled = true;
        };
    }, [loadSettings, refreshConnections, refreshAIStatus, refreshAllowedCommands]);

    // ---- Tabs (session window only) ----

    const openTerminal = useCallback((conn) => {
        setTabs((ts) => (ts.some((t) => t.id === conn.id) ? ts : [...ts, { id: conn.id, kind: "terminal", connId: conn.id, title: conn.name }]));
        setActiveTab(conn.id);
    }, []);

    // openQuickTerminal opens a one-off session for an unsaved address (quick
    // connect). Every call gets its own tab, so the same host can be opened
    // more than once.
    const adhocSeq = useRef(0);
    const openQuickTerminal = useCallback((spec) => {
        const id = `adhoc-${++adhocSeq.current}`;
        const title =
            spec.port && spec.port !== 22
                ? `${spec.user}@${spec.host}:${spec.port}`
                : `${spec.user}@${spec.host}`;
        setTabs((ts) => [...ts, { id, kind: "terminal", host: spec.host, port: spec.port, user: spec.user, title }]);
        setActiveTab(id);
    }, []);

    // openSession opens a terminal for a saved connection or a one-off address.
    // In the tool windows (where the tabs don't exist) the request is delegated
    // to the backend, which broadcasts it to the session window.
    const openSession = useCallback(
        (target) => {
            if (!isSessionWindow) {
                const params = target.connId
                    ? { connId: target.connId }
                    : { host: target.host, port: target.port, user: target.user };
                rpc.call("session.open", params).catch((e) => {
                    setDialog({ type: "notice", title: "打开会话失败", message: e.message });
                });
                return;
            }
            if (target.connId) {
                const conn = connections.find((c) => c.id === target.connId);
                if (!conn) {
                    setDialog({ type: "notice", title: "打开会话失败", message: "连接不存在或已被删除" });
                    return;
                }
                openTerminal(conn);
                return;
            }
            openQuickTerminal({ host: target.host, port: target.port, user: target.user });
        },
        [connections, openTerminal, openQuickTerminal],
    );

    // Requests from other windows (or the native menus) to open a session.
    useEffect(() => {
        if (!isSessionWindow) return undefined;
        return rpc.on("ui.open-session", (m) => {
            if (m.connId) {
                const conn = connectionsRef.current.find((c) => c.id === m.connId);
                if (conn) openTerminal(conn);
                else setDialog({ type: "notice", title: "打开会话失败", message: "连接不存在或已被删除" });
                return;
            }
            if (m.host) openQuickTerminal({ host: m.host, port: m.port, user: m.user });
        });
    }, [openTerminal, openQuickTerminal]);

    // "New connection" from the native menu bar opens the connection editor
    // and reveals the session manager it belongs to.
    useEffect(() => {
        if (!isSessionWindow) return undefined;
        return rpc.on("ui.new-connection", () => {
            setSessionManagerOpen(true);
            setDialog({ type: "connection", conn: null });
        });
    }, []);

    // Rename a tab's session title.
    const renameTab = useCallback((id, title) => {
        const name = String(title || "").trim();
        if (!name) return;
        setTabs((ts) => ts.map((t) => (t.id === id ? { ...t, title: name } : t)));
    }, []);

    // renameConnection persists a new display name for a saved connection and
    // refreshes both the session tree and the open tab so the title matches.
    const renameConnection = useCallback(
        async (conn, name) => {
            await rpc.call("connections.save", {
                id: conn.id,
                name,
                host: conn.host,
                port: conn.port,
                user: conn.user,
                folderId: conn.folderId || "",
                // An empty password keeps the stored one on the backend.
                password: "",
                savePassword: conn.savePassword || false,
                privateKeyPath: conn.privateKeyPath || "",
                keyId: conn.keyId || "",
            });
            await refreshConnections();
            setTabs((ts) =>
                ts.map((t) => (t.connId === conn.id || t.savedConnectionId === conn.id ? { ...t, title: name } : t)),
            );
        },
        [refreshConnections],
    );

    // markTabSaved records that an ad-hoc session was saved as a connection,
    // adopting the connection's name as the tab title.
    const markTabSaved = useCallback((id, conn) => {
        if (!conn || !conn.id) return;
        setTabs((ts) => ts.map((t) => (t.id === id ? { ...t, savedConnectionId: conn.id, title: conn.name || t.title } : t)));
    }, []);

    const closeTab = useCallback((id) => {
        const current = tabsRef.current;
        const idx = current.findIndex((t) => t.id === id);
        if (idx < 0) return;
        const next = current.filter((t) => t.id !== id);
        setTabs(next);
        if (activeTabRef.current === id) {
            const neighbour = next[Math.min(idx, next.length - 1)];
            setActiveTab(neighbour ? neighbour.id : null);
        }
    }, []);

    const selectTab = useCallback((id) => setActiveTab(id), []);

    // toggleSessionManager collapses / reveals the docked session manager.
    const toggleSessionManager = useCallback(() => setSessionManagerOpen((v) => !v), []);

    // ---- Dialogs ----

    const openDialog = useCallback((d) => setDialog(d), []);
    const closeDialog = useCallback(() => setDialog(null), []);

    // ---- Reasoning pane ----

    const pushReason = useCallback((item) => {
        const id = `r${++reasonSeq.current}`;
        // Keep the list bounded: the pane is a working surface, not a log.
        setReason((rs) => [...rs, { id, ...item }].slice(-80));
        noteFocus(item.tabId, id);
        return id;
    }, [noteFocus]);

    const updateReason = useCallback((id, patch) => {
        setReason((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)));
    }, []);

    const clearReason = useCallback((tabId) => {
        setReason((rs) => (tabId ? rs.filter((r) => r.tabId !== tabId) : []));
    }, []);

    const toggleReason = useCallback(() => setReasonOpen((v) => !v), []);
    const openReason = useCallback(() => setReasonOpen(true), []);

    // patchCard merges a patch into a reasoning card, or into one of its nested
    // fields (key === null patches the card itself). A patched card is by
    // definition the one that just changed, so it takes the focus.
    const patchCard = useCallback((cardId, key, patch) => {
        const card = reasonRef.current.find((r) => r.id === cardId);
        if (card) noteFocus(card.tabId, cardId);
        setReason((rs) =>
            rs.map((r) => {
                if (r.id !== cardId) return r;
                if (!key) return { ...r, ...patch };
                return { ...r, [key]: { ...(r[key] || {}), ...patch } };
            }),
        );
    }, [noteFocus]);

    // askAI starts a streaming request. By default it opens a new card; passing
    // `into` streams into a field of an existing card instead, which is how a
    // result analysis stays attached to the command it interprets. It resolves
    // with { requestId, kind, cardId } so a caller can await the final result.
    //
    // autoRun asks for the proposed command to be submitted on its own when the
    // local engine finds nothing to confirm (see the ai.done handler).
    const askAI = useCallback(
        ({ tabId, sessionId, kind, prompt, excerpt, title, into, autoRun }) => {
            const key = into?.key || null;
            const id = into?.cardId
                ? into.cardId
                : pushReason({
                      tabId,
                      kind: "cot",
                      title: title || "AI 推理",
                      status: "streaming",
                      text: "",
                      steps: [],
                  });
            return rpc.call("ai.ask", { sessionId, kind, prompt, excerpt }).then(
                (res) => {
                    if (res && res.requestId) aiRequests.current.set(res.requestId, { id, key, tabId, autoRun: !!autoRun });
                    return { ...res, cardId: id };
                },
                (e) => {
                    patchCard(id, key, { status: "error", text: e.message || String(e) });
                    throw e;
                },
            );
        },
        [patchCard, pushReason],
    );

    // askCommand opens a natural-language turn in the reasoning pane: the user's
    // own question gets a card of its own, followed by the model's answer. The
    // bottom bar stays a plain input — the conversation lives entirely on the
    // right, so nothing is shown twice.
    const askCommand = useCallback(
        ({ tabId, sessionId, prompt, title }) => {
            pushReason({ tabId, kind: "ask", title: title || "我的提问", text: prompt });
            openReason();
            // autoRun carries through to the reply: a green command the model
            // just proposed runs without a second click.
            return askAI({ tabId, sessionId, kind: "command", prompt, title: "AI 回复", autoRun: true });
        },
        [askAI, openReason, pushReason],
    );

    // analyzeResult closes the loop: it asks the model to interpret the output of a
    // command the user just ran from a card, and stores the reply on that same
    // card, so the investigation continues where it started.
    const analyzeResult = useCallback(
        ({ tabId, sessionId, cardId, command, output }) => {
            const text = String(output || "").trim();
            if (!text) {
                patchCard(cardId, "analysis", {
                    status: "done",
                    text: "命令已执行，没有输出。",
                    steps: [],
                    suggestions: [],
                });
                return Promise.resolve(null);
            }
            patchCard(cardId, "analysis", { status: "streaming", text: "", steps: [], suggestions: [] });
            return askAI({
                tabId,
                sessionId,
                kind: "result",
                prompt: command,
                excerpt: text,
                into: { cardId, key: "analysis" },
            });
        },
        [askAI, patchCard],
    );

    // markExec records how a proposed command is doing (running → done) on the
    // card that proposed it.
    const markExec = useCallback(
        (cardId, patch) => {
            patchCard(cardId, "exec", patch);
        },
        [patchCard],
    );

    // runCommand submits a command through the local safety gate and handles the
    // two outcomes that must not execute it (blocked / needs approval) in one
    // place, so the Smart Input and the reasoning cards behave identically.
    //
    // trackCardId attributes the command's output back to a card. Only commands
    // the model proposed are tracked: the output of a hand-typed command does not
    // belong to any conversation.
    const runCommand = useCallback(
        async ({ tabId, sessionId, command, trackCardId, onWritten, auto }) => {
            const cmd = String(command || "").trim();
            if (!cmd) return { status: "empty" };
            const sid = sessionId || sessions.current.get(tabId) || "";
            if (!sid) return { status: "nosession", message: "会话尚未就绪" };

            const trackId = trackCardId || "";
            // Pinning the anchor before the write is what makes "回到原始输出" land on
            // the prompt line the command is echoed on: after the write, the cursor is
            // already past it.
            const term = terminals.current.get(tabId);
            const markAnchor = () => {
                if (trackId && term?.markAnchor) term.markAnchor(trackId);
            };
            const dropAnchor = () => {
                if (trackId && term?.dropAnchor) term.dropAnchor(trackId);
            };
            const send = (token) =>
                rpc.call("terminal.exec", { sessionId: sid, command: cmd, trackId, confirmToken: token || "" });
            // onWritten fires once the command is actually on the wire, which may
            // be after the user approves it in the dry-run panel.
            const written = () => {
                if (onWritten) onWritten();
            };

            if (trackId) markExec(trackId, { status: "running", startedAt: Date.now(), auto: !!auto });
            try {
                markAnchor();
                const res = await send("");
                if (res.blocked) {
                    // Nothing was written, so there is no line to go back to.
                    dropAnchor();
                    if (trackId) markExec(trackId, { status: "blocked", reason: res.result?.reason || "" });
                    pushReason({
                        tabId,
                        kind: "notice",
                        title: "已阻断高危命令",
                        status: "blocked",
                        text: res.result?.reason || "",
                        command: cmd,
                        risk: res.result,
                    });
                    openReason();
                    return {
                        status: "blocked",
                        message: res.result?.reason || "该命令已被本地安全引擎阻断",
                        result: res.result,
                    };
                }
                if (res.confirm) {
                    // Yellow zone: hand it to the dry-run panel and stop here. The
                    // command is only sent after the user approves it explicitly, and
                    // the panel lets that approval be one-shot, session-wide or
                    // permanent.
                    if (trackId) markExec(trackId, { status: "confirm", auto: !!auto });
                    setPendingConfirm({
                        tabId,
                        command: cmd,
                        result: res.result,
                        // auto marks a confirmation the model's own command opened,
                        // so the panel can say who started it.
                        auto: !!auto,
                        onConfirm: async (scope) => {
                            try {
                                // One gesture: record how far the approval reaches and
                                // get the authorisation for this run back.
                                const { token } = await rpc.call("safety.confirm", {
                                    sessionId: sid,
                                    command: cmd,
                                    scope: scope || "",
                                });
                                if (trackId) markExec(trackId, { status: "running", startedAt: Date.now(), auto: !!auto });
                                // Re-pin: the panel may have waited a while, and the marker must sit
                                // on the line the command is written to now.
                                markAnchor();
                                await rpc.call("terminal.exec", {
                                    sessionId: sid,
                                    command: cmd,
                                    trackId,
                                    confirmToken: token || "",
                                });
                                setPendingConfirm(null);
                                written();
                            } catch (e) {
                                setPendingConfirm(null);
                                if (trackId) markExec(trackId, { status: "error", reason: e.message || String(e) });
                            }
                        },
                        onCancel: () => {
                            setPendingConfirm(null);
                            if (trackId) markExec(trackId, { status: "idle" });
                        },
                    });
                    openReason();
                    return { status: "confirm", result: res.result };
                }
                written();
                return { status: "written", result: res.result, allowedBy: res.allowedBy || "" };
            } catch (e) {
                if (trackId) markExec(trackId, { status: "error", reason: e.message || String(e) });
                return { status: "error", message: e.message || String(e) };
            }
        },
        [markExec, openReason, pushReason],
    );
    // Keep the ref the push handlers use pointing at the latest runner.
    runCommandRef.current = runCommand;

    // runSuggestion promotes a follow-up suggestion into a command card of its own
    // and runs it, so every step of an investigation keeps its own output and
    // analysis instead of overwriting the card it came from.
    const runSuggestion = useCallback(
        async ({ tabId, sessionId, command, title }) => {
            const cmd = String(command || "").trim();
            if (!cmd) return { status: "empty" };
            const cardId = pushReason({
                tabId,
                kind: "cot",
                title: title || "下一步排查",
                status: "done",
                text: "",
                steps: [],
                command: cmd,
            });
            openReason();
            const res = await runCommand({ tabId, sessionId, command: cmd, trackCardId: cardId });
            // Show the real local classification rather than assuming 绿区.
            if (res?.result) patchCard(cardId, null, { risk: res.result });
            return res;
        },
        [openReason, patchCard, pushReason, runCommand],
    );

    // askFollowUp is what a 快捷追问 chip does: the chip's own label becomes the
    // user's turn in the thread, followed by the recommended command and its run.
    // One click is the whole gesture — the input box is never involved — and the
    // command is still classified and (if needed) confirmed like any other.
    const askFollowUp = useCallback(
        ({ tabId, sessionId, label, command, title }) => {
            pushReason({ tabId, kind: "ask", title: "快捷追问", text: label });
            openReason();
            return runSuggestion({ tabId, sessionId, command, title: title || label || "下一步排查" });
        },
        [openReason, pushReason, runSuggestion],
    );

    // interruptTab sends Ctrl+C to a tab's PTY. It is the escape hatch for a
    // command that will not end on its own (`ping` without `-c`, `tail -f`): the
    // stream stops, the sniffer closes the segment, and the card settles.
    const interruptTab = useCallback((tabId) => {
        const sid = sessions.current.get(tabId);
        if (!sid) return false;
        rpc.call("terminal.input", { sessionId: sid, data: "\u0003" }).catch(() => {});
        return true;
    }, []);

    // cancelAI abandons an in-flight generation (Esc in the Smart Input). The
    // backend stops streaming and closes the card, so a slow model does not hold
    // the UI hostage.
    const cancelAI = useCallback((requestId) => {
        if (!requestId) return;
        rpc.call("ai.cancel", { requestId }).catch(() => {});
    }, []);

    // registerTerminal hands the pane the xterm bridge of a tab (see TerminalTab).
    const registerTerminal = useCallback((tabId, api) => {
        if (api) terminals.current.set(tabId, api);
        else terminals.current.delete(tabId);
    }, []);

    // revealOutput scrolls the terminal to where the card's command ran and flashes
    // it. It returns false when that line is gone (scrolled out of the buffer), so
    // the card can say so instead of appearing to do nothing.
    const revealOutput = useCallback((tabId, cardId, lines) => {
        const term = terminals.current.get(tabId);
        return !!(term && term.reveal && term.reveal(cardId, lines));
    }, []);

    // hoverOutput highlights (and un-highlights) the region a card refers to while
    // the pointer is over it, so the right pane and the terminal stay anchored.
    const hoverOutput = useCallback((tabId, cardId, lines, on) => {
        const term = terminals.current.get(tabId);
        if (term && term.hover) term.hover(cardId, lines, on);
    }, []);

    // latestAICommand returns the newest command the model proposed for a tab. It
    // is what Tab in the Smart Input fills in, so the mouse never has to travel to
    // the reasoning pane.
    const latestAICommand = useCallback((tabId) => {
        const list = reasonRef.current;
        for (let i = list.length - 1; i >= 0; i--) {
            const item = list[i];
            if (item.tabId === tabId && item.command) return { command: item.command, cardId: item.id };
        }
        return null;
    }, []);

    // registerSession records the live session id of a tab (and clears it on
    // close), so cards can address the PTY.
    const registerSession = useCallback((tabId, sessionId) => {
        if (sessionId) sessions.current.set(tabId, sessionId);
        else sessions.current.delete(tabId);
    }, []);

    // Host samples arrive while sessions are open; the snapshot covers a window
    // that was just reloaded.
    useEffect(() => {
        const off = rpc.on("host.stats", (m) => {
            const st = m.stats;
            if (!st || !st.connId) return;
            setStats((s) => ({ ...s, [st.connId]: st }));
        });
        rpc.call("probes.snapshot")
            .then((snap) => {
                if (snap && Object.keys(snap).length) setStats((s) => ({ ...snap, ...s }));
            })
            .catch(() => {});
        return off;
    }, []);

    // AI pushes are correlated by requestId, so several requests can be in
    // flight (e.g. a diagnosis while a command is being generated). Each entry
    // names the card and, for a result analysis, the nested field to write into.
    useEffect(() => {
        const target = (m) => aiRequests.current.get(m.requestId);
        const offDelta = rpc.on("ai.delta", (m) => {
            const t = target(m);
            if (!t) return;
            const text = m.text || "";
            noteFocus(t.tabId, t.id);
            setReason((rs) =>
                rs.map((r) => {
                    if (r.id !== t.id) return r;
                    if (!t.key) return { ...r, text: (r.text || "") + text };
                    const cur = r[t.key] || {};
                    return { ...r, [t.key]: { ...cur, text: (cur.text || "") + text } };
                }),
            );
        });
        const offDone = rpc.on("ai.done", (m) => {
            const t = target(m);
            if (!t) return;
            aiRequests.current.delete(m.requestId);
            noteFocus(t.tabId, t.id);
            setReason((rs) =>
                rs.map((r) => {
                    if (r.id !== t.id) return r;
                    const cur = t.key ? r[t.key] || {} : r;
                    const patch = {
                        // A cancelled turn is neither a success nor a failure: say it was
                        // abandoned, so the partial text is not mistaken for an answer.
                        status: m.cancelled ? "cancelled" : m.ok ? "done" : "error",
                        text: m.cancelled ? "已取消" : m.ok ? m.text || cur.text || "" : m.error || "AI 请求失败",
                        steps: m.steps || [],
                        suggestions: m.suggestions || [],
                    };
                    if (!t.key) {
                        return { ...r, ...patch, command: m.command || "", risk: m.risk || null };
                    }
                    return { ...r, [t.key]: { ...cur, ...patch } };
                }),
            );

            // Auto-run: a command the model just proposed is submitted on its own.
            // The gate decides the rest, and it is the same gate a typed command
            // goes through:
            //
            //   green  → written straight to the PTY;
            //   yellow → the dry-run panel opens and waits for the user, which is
            //            the "pause for a dangerous command" step;
            //   red    → refused, and the card records the attempt so the user can
            //            see the model tried something dangerous.
            if (!t.autoRun || !m.ok || !m.command) return;
            if (settingsRef.current?.aiAutoRun === false) return;
            runCommandRef.current?.({
                tabId: t.tabId,
                command: m.command,
                trackCardId: t.id,
                auto: true,
            });
        });
        return () => {
            offDelta();
            offDone();
        };
    }, []);

    // The output of a tracked command comes back tagged with the card that
    // proposed it: fill that card in, then let the model interpret the result
    // without another click. This is the loop that turns "generated a command"
    // into "finished a diagnosis".
    useEffect(() => {
        const offOutput = rpc.on("terminal.output", (m) => {
            const cardId = m.trackId;
            if (!cardId) return;
            markExec(cardId, {
                status: m.timedOut ? "timeout" : "done",
                output: m.text || "",
                durationMs: m.durationMs || 0,
                truncated: !!m.truncated,
            });
            const ai = aiStatusRef.current || {};
            if (!ai.configured) return;
            if (ai.noContext) {
                // Context sharing is off, so there is nothing we are allowed to
                // send: say so instead of asking the model to analyse thin air.
                patchCard(cardId, "analysis", {
                    status: "done",
                    text: "已关闭终端上下文共享，未自动分析。可在「选项 → AI 推理」中开启后手动追问。",
                    steps: [],
                    suggestions: [],
                });
                return;
            }
            const card = reasonRef.current.find((r) => r.id === cardId);
            analyzeResult({
                tabId: card?.tabId,
                sessionId: m.sessionId,
                cardId,
                command: m.command || card?.command || "",
                output: m.text || "",
            });
        });
        // A command that keeps producing output is announced before it ends, so
        // the card can show "持续监听中" and offer Ctrl+C instead of a spinner that
        // looks stuck. The final terminal.output still arrives when it stops.
        const offStream = rpc.on("terminal.streaming", (m) => {
            if (!m.trackId) return;
            markExec(m.trackId, { status: "streaming", elapsedMs: m.elapsedMs || 0 });
        });
        // A session that ends mid-command can never deliver that output, so stop
        // showing a spinner that will not resolve.
        const offExit = rpc.on("terminal.exit", (m) => {
            let tabId = null;
            for (const [t, s] of sessions.current) {
                if (s === m.sessionId) tabId = t;
            }
            if (!tabId) return;
            setReason((rs) =>
                rs.map((r) =>
                    r.tabId === tabId && r.exec?.status === "running"
                        ? { ...r, exec: { ...r.exec, status: "error", reason: "会话已关闭，未收到输出" } }
                        : r,
                ),
            );
        });
        return () => {
            offOutput();
            offStream();
            offExit();
        };
    }, [analyzeResult, markExec, patchCard]);

    // appAction asks the desktop shell for a native action (open another
    // window, quit, DevTools). Surfaces a dialog when unavailable (headless).
    // "sessions" and "new-connection" are handled here: the session manager is
    // docked in this window, not a window of its own.
    const appAction = useCallback(
        (action, params) => {
            if (action === "sessions") {
                setSessionManagerOpen(true);
                return;
            }
            if (action === "new-connection") {
                setSessionManagerOpen(true);
                setDialog({ type: "connection", conn: null });
                return;
            }
            rpc.call("app.action", { action, ...(params || {}) }).catch((e) => {
                setDialog({ type: "notice", title: "操作不可用", message: e.message });
            });
        },
        [],
    );

    const value = useMemo(
        () => ({
            ready,
            bootError,
            settings,
            settingsRef,
            saveSettings,
            zoomFont,
            resetZoom,
            connections,
            folders,
            keys,
            refreshConnections,
            refreshKeys,
            tabs,
            activeTab,
            openTerminal,
            openQuickTerminal,
            openSession,
            sessionManagerOpen,
            toggleSessionManager,
            renameTab,
            renameConnection,
            markTabSaved,
            closeTab,
            selectTab,
            dialog,
            openDialog,
            closeDialog,
            appAction,
            reason,
            reasonOpen,
            toggleReason,
            openReason,
            pushReason,
            updateReason,
            clearReason,
            draft,
            setDraft,
            pendingConfirm,
            setPendingConfirm,
            aiStatus,
            askAI,
            askCommand,
            refreshAIStatus,
            runCommand,
            runSuggestion,
            askFollowUp,
            interruptTab,
            cancelAI,
            registerTerminal,
            revealOutput,
            hoverOutput,
            analyzeResult,
            latestAICommand,
            registerSession,
            allowedCommands,
            refreshAllowedCommands,
            revokeAllowed,
            stats,
            focusByTab,
        }),
        [
            ready,
            bootError,
            settings,
            saveSettings,
            zoomFont,
            resetZoom,
            connections,
            folders,
            keys,
            refreshConnections,
            refreshKeys,
            tabs,
            activeTab,
            openTerminal,
            openQuickTerminal,
            openSession,
            sessionManagerOpen,
            toggleSessionManager,
            renameTab,
            renameConnection,
            markTabSaved,
            closeTab,
            selectTab,
            dialog,
            openDialog,
            closeDialog,
            appAction,
            reason,
            reasonOpen,
            toggleReason,
            openReason,
            pushReason,
            updateReason,
            clearReason,
            draft,
            pendingConfirm,
            aiStatus,
            askAI,
            askCommand,
            refreshAIStatus,
            runCommand,
            runSuggestion,
            askFollowUp,
            interruptTab,
            cancelAI,
            registerTerminal,
            revealOutput,
            hoverOutput,
            analyzeResult,
            latestAICommand,
            registerSession,
            allowedCommands,
            refreshAllowedCommands,
            revokeAllowed,
            stats,
            focusByTab,
        ],
    );

    return <AppCtx.Provider value={value}>{children}</AppCtx.Provider>;
}

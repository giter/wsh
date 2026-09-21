import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { rpc } from "../lib/rpc.js";

// Pages that live in a tab, keyed by the "kind" used across the UI.
// Connection management is not a page: it lives in the sidebar session manager.
export const PAGES = {
    sftp: { id: "sftp-page", title: "文件传输" },
    tunnels: { id: "tunnels-page", title: "端口隧道" },
};

const AppCtx = createContext(null);

export function useApp() {
    const ctx = useContext(AppCtx);
    if (!ctx) throw new Error("useApp must be used inside <AppProvider>");
    return ctx;
}

// applySettings pushes global options into the live UI: font size (via the html
// font-size so all rem/em-based text follows) and theme (data-theme).
function applySettings(st) {
    const fs = parseInt(st.fontSize, 10);
    document.documentElement.style.fontSize = fs >= 8 && fs <= 32 ? fs + "px" : "";
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

    const settingsRef = useRef(settings);
    settingsRef.current = settings;
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

    // zoomFont changes the global font size by delta px; the new size is
    // persisted (debounced) by the effect below.
    const zoomFont = useCallback((delta) => {
        const st = settingsRef.current;
        const base = parseInt(st.fontSize, 10) >= 8 ? parseInt(st.fontSize, 10) : 13;
        const cur = parseInt(document.documentElement.style.fontSize, 10) || base;
        const next = Math.min(32, Math.max(8, cur + delta));
        document.documentElement.style.fontSize = next + "px";
        setSettings((s) => ({ ...s, fontSize: next }));
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
        });
        const off2 = rpc.on("ui.keys-changed", () => {
            refreshConnections().catch(() => {});
        });
        return () => {
            off1();
            off2();
        };
    }, [loadSettings, refreshConnections]);

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
            await Promise.all([loadSettings(), refreshConnections().catch(() => {})]);
            if (!cancelled) setReady(true);
        })();
        return () => {
            cancelled = true;
        };
    }, [loadSettings, refreshConnections]);

    // ---- Tabs ----

    const openTerminal = useCallback((conn) => {
        setTabs((ts) => (ts.some((t) => t.id === conn.id) ? ts : [...ts, { id: conn.id, kind: "terminal", connId: conn.id, title: conn.name }]));
        setActiveTab(conn.id);
    }, []);

    const openPage = useCallback((kind) => {
        const page = PAGES[kind];
        if (!page) return;
        setTabs((ts) => (ts.some((t) => t.id === page.id) ? ts : [...ts, { id: page.id, kind, title: page.title }]));
        setActiveTab(page.id);
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

    // ---- Dialogs ----

    const openDialog = useCallback((d) => setDialog(d), []);
    const closeDialog = useCallback(() => setDialog(null), []);

    // appAction asks the desktop shell for a native action (open a config
    // window, quit, DevTools). Surfaces a dialog when unavailable (headless).
    const appAction = useCallback(
        (action) => {
            rpc.call("app.action", { action }).catch((e) => {
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
            connections,
            folders,
            keys,
            refreshConnections,
            refreshKeys,
            tabs,
            activeTab,
            openTerminal,
            openPage,
            openQuickTerminal,
            closeTab,
            selectTab,
            dialog,
            openDialog,
            closeDialog,
            appAction,
        }),
        [
            ready,
            bootError,
            settings,
            saveSettings,
            zoomFont,
            connections,
            folders,
            keys,
            refreshConnections,
            refreshKeys,
            tabs,
            activeTab,
            openTerminal,
            openPage,
            openQuickTerminal,
            closeTab,
            selectTab,
            dialog,
            openDialog,
            closeDialog,
            appAction,
        ],
    );

    return <AppCtx.Provider value={value}>{children}</AppCtx.Provider>;
}

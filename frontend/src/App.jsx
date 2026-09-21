import { useCallback, useEffect } from "react";
import { useApp, WINDOW } from "./state/store.jsx";
import TitleBar from "./components/TitleBar.jsx";
import QuickConnectBar from "./components/QuickConnectBar.jsx";
import Sidebar from "./components/Sidebar.jsx";
import TabBar from "./components/TabBar.jsx";
import TerminalTab from "./components/TerminalTab.jsx";
import SftpPage from "./components/SftpPage.jsx";
import TunnelsPage from "./components/TunnelsPage.jsx";
import SettingsPage from "./components/SettingsPage.jsx";
import KeysPage from "./components/KeysPage.jsx";
import DialogHost from "./components/DialogHost.jsx";

// WINDOW_ROUTES maps the "?win=" value of a dedicated window to the page it
// renders. File transfer, tunnels and the configuration pages each run in their
// own native window, so they can be closed and reopened (or moved to another
// screen) independently of the terminal sessions.
const WINDOW_ROUTES = {
    sftp: { title: "文件传输", page: "sftp" },
    tunnels: { title: "端口隧道", page: "tunnels" },
    settings: { title: "选项", page: "settings" },
    keys: { title: "密钥管理", page: "keys" },
};

// windowRoute returns the dedicated-window configuration for this window, or
// null for the main session window. The "#keys" style hash is also accepted so
// dedicated windows can be opened in a plain browser while developing.
export function windowRoute() {
    const win = WINDOW;
    if (WINDOW_ROUTES[win]) return WINDOW_ROUTES[win];
    const route = (location.hash || "").replace(/^#/, "");
    return route ? WINDOW_ROUTES[route] || null : null;
}

// isTextInputFocused reports whether the caret is in the terminal or a form
// field, where control characters must reach the shell instead of being
// captured by the menu accelerators.
function isTextInputFocused() {
    const el = document.activeElement;
    if (!el) return false;
    if (el.closest && el.closest(".xterm")) return true;
    const tag = el.tagName;
    return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || el.isContentEditable;
}

// SessionWindow holds the terminal tabs, with the session manager docked on the
// left (Xshell style). The manager can be collapsed with its ✕ button and
// brought back from the menu bar or Ctrl+1; the other tools live in windows of
// their own so they never crowd the sessions.
function SessionWindow() {
    const app = useApp();

    // Keyboard accelerators for the menu. Matching uses e.code so it is layout
    // independent; bare-letter shortcuts are suppressed while typing so the
    // terminal keeps receiving Ctrl+N/Ctrl+M/Ctrl+Q.
    useEffect(() => {
        const onKeyDown = (e) => {
            if (!(e.ctrlKey || e.metaKey)) return;
            const code = e.code;

            // Font zoom (Ctrl/Cmd +/-/0). Ctrl+, also zooms; Ctrl+Shift+, is the
            // options window below.
            if (e.key === "=" || e.key === "+" || (e.key === "," && !e.shiftKey)) {
                e.preventDefault();
                app.zoomFont(1);
                return;
            }
            if (e.key === "-" || e.key === "_") {
                e.preventDefault();
                app.zoomFont(-1);
                return;
            }
            if (e.key === "0") {
                e.preventDefault();
                app.resetZoom();
                return;
            }

            if (e.shiftKey) {
                if (code === "KeyK") {
                    e.preventDefault();
                    app.appAction("keys");
                } else if (code === "Comma") {
                    e.preventDefault();
                    app.appAction("settings");
                }
                return;
            }
            if (code === "Digit1") {
                e.preventDefault();
                // Ctrl+1 toggles the docked panel, so it also closes it.
                app.toggleSessionManager();
            } else if (code === "Digit2") {
                e.preventDefault();
                app.appAction("sftp");
            } else if (code === "Digit3") {
                e.preventDefault();
                app.appAction("tunnels");
            } else if (!isTextInputFocused()) {
                if (code === "KeyN") {
                    e.preventDefault();
                    app.appAction("new-connection");
                } else if (code === "KeyQ") {
                    e.preventDefault();
                    app.appAction("quit");
                }
            }
        };

        document.addEventListener("keydown", onKeyDown);
        return () => document.removeEventListener("keydown", onKeyDown);
    }, [app]);

    return (
        <>
            <QuickConnectBar />
            <TabBar />
            <div id="content">
                {app.tabs.length === 0 ? (
                    <div className="center-box">
                        <div style={{ fontSize: 28 }}>🖥️</div>
                        <div>{app.sessionManagerOpen ? "双击「会话管理器」中的连接开始" : "按 Ctrl+1 展开会话管理器"}</div>
                    </div>
                ) : (
                    app.tabs.map((tab) => (
                        <div key={tab.id} className="tab-page" style={{ display: tab.id === app.activeTab ? "flex" : "none" }}>
                            <TerminalTab tab={tab} active={tab.id === app.activeTab} />
                        </div>
                    ))
                )}
            </div>
        </>
    );
}

export default function App() {
    const app = useApp();
    const route = windowRoute();

    useEffect(() => {
        // `config-window` is the flat layout used by the tool windows (no
        // sidebar, no session tabs).
        document.body.classList.toggle("config-window", !!route);
        document.body.dataset.window = WINDOW;
    }, [route]);

    // Suppress the webview's default right-click context menu.
    useEffect(() => {
        const onContextMenu = (e) => e.preventDefault();
        document.addEventListener("contextmenu", onContextMenu);
        return () => document.removeEventListener("contextmenu", onContextMenu);
    }, []);

    let content;
    if (app.bootError) {
        content = (
            <div className="center-box">
                <div className="err">{app.bootError}</div>
            </div>
        );
    } else if (!app.ready) {
        content = (
            <div className="center-box">
                <div className="spinner" />
                <div>正在连接…</div>
            </div>
        );
    } else if (!route) {
        content = <SessionWindow />;
    } else {
        switch (route.page) {
            case "sftp":
                content = <SftpPage />;
                break;
            case "tunnels":
                content = <TunnelsPage />;
                break;
            case "keys":
                content = <KeysPage />;
                break;
            case "settings":
                content = <SettingsPage />;
                break;
            default:
                content = null;
        }
    }

    return (
        <>
            <div id="app">
                <TitleBar />
                <div id="app-body">
                    {!route && app.sessionManagerOpen && <Sidebar />}
                    <main id="main">{content}</main>
                </div>
            </div>
            <DialogHost />
        </>
    );
}

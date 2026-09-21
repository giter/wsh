import { useCallback, useEffect } from "react";
import { useApp } from "./state/store.jsx";
import TitleBar from "./components/TitleBar.jsx";
import Sidebar from "./components/Sidebar.jsx";
import TabBar from "./components/TabBar.jsx";
import TerminalTab from "./components/TerminalTab.jsx";
import ConnectionsPage from "./components/ConnectionsPage.jsx";
import SftpPage from "./components/SftpPage.jsx";
import TunnelsPage from "./components/TunnelsPage.jsx";
import SettingsPage from "./components/SettingsPage.jsx";
import KeysPage from "./components/KeysPage.jsx";
import DialogHost from "./components/DialogHost.jsx";

// configRoute returns the standalone configuration page requested via the URL
// hash ("#settings" / "#keys"), or "" for the normal main window. Dedicated
// configuration windows opened from the title bar load these routes.
export function configRoute() {
    const route = (location.hash || "").replace(/^#/, "");
    return route === "settings" || route === "keys" ? route : "";
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

function MainContent() {
    const { tabs, activeTab } = useApp();

    if (tabs.length === 0) {
        return (
            <div className="center-box">
                <div style={{ fontSize: 28 }}>🖥️</div>
                <div>从左侧选择连接或功能开始</div>
            </div>
        );
    }

    return (
        <>
            {tabs.map((tab) => (
                <TabPane key={tab.id} tab={tab} active={tab.id === activeTab} />
            ))}
        </>
    );
}

function TabPane({ tab, active }) {
    let body = null;
    switch (tab.kind) {
        case "terminal":
            body = <TerminalTab tab={tab} active={active} />;
            break;
        case "sftp":
            body = <SftpPage />;
            break;
        case "tunnels":
            body = <TunnelsPage />;
            break;
        case "connections":
            body = <ConnectionsPage />;
            break;
        default:
            body = null;
    }
    return (
        <div className="tab-page" style={{ display: active ? "flex" : "none" }}>
            {body}
        </div>
    );
}

export default function App() {
    const app = useApp();
    const route = configRoute();

    useEffect(() => {
        document.body.classList.toggle("config-window", !!route);
    }, [route]);

    const navigateTo = useCallback(
        (kind) => {
            if (kind === "home") {
                if (app.tabs.length) app.selectTab(app.tabs[0].id);
                else app.selectTab(null);
                return;
            }
            app.openPage(kind);
        },
        [app],
    );

    // Keyboard accelerators for the menu. Matching uses e.code so it is layout
    // independent; bare-letter shortcuts are suppressed while typing so the
    // terminal keeps receiving Ctrl+N/Ctrl+M/Ctrl+Q.
    useEffect(() => {
        if (route) return undefined;

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
                const base = parseInt(app.settingsRef.current.fontSize, 10);
                document.documentElement.style.fontSize = (base >= 8 ? base : 13) + "px";
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
                navigateTo("home");
            } else if (code === "Digit2") {
                e.preventDefault();
                navigateTo("sftp");
            } else if (code === "Digit3") {
                e.preventDefault();
                navigateTo("tunnels");
            } else if (!isTextInputFocused()) {
                if (code === "KeyN") {
                    e.preventDefault();
                    app.openDialog({ type: "connection", conn: null });
                } else if (code === "KeyM") {
                    e.preventDefault();
                    navigateTo("connections");
                } else if (code === "KeyQ") {
                    e.preventDefault();
                    app.appAction("quit");
                }
            }
        };

        document.addEventListener("keydown", onKeyDown);
        return () => document.removeEventListener("keydown", onKeyDown);
    }, [route, app, navigateTo]);

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
    } else if (route) {
        content = route === "keys" ? <KeysPage /> : <SettingsPage />;
    } else {
        content = <MainContent />;
    }

    return (
        <>
            <div id="app">
                <TitleBar navigateTo={navigateTo} />
                <div id="app-body">
                    {!route && <Sidebar />}
                    <main id="main">
                        {!route && <TabBar />}
                        <div id="content">{content}</div>
                    </main>
                </div>
            </div>
            <DialogHost />
        </>
    );
}

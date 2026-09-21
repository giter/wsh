import { useEffect, useState } from "react";
import { useApp } from "../state/store.jsx";
import { WINDOW_NAME, hasCustomChrome, installWindowGestures, windowControl } from "../lib/windowChrome.js";

// Per-window chrome configuration, keyed by the "?win=" value. Windows not
// listed here (i.e. every dedicated tool window) show their own title.
const CHROME = {
    main: { brand: true, menu: true },
    sftp: { title: "文件传输" },
    tunnels: { title: "端口隧道" },
    settings: { title: "选项" },
    keys: { title: "密钥管理" },
};

const ICONS = {
    minimise:
        '<svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true"><rect x="0" y="4.5" width="10" height="1" fill="currentColor"/></svg>',
    maximise:
        '<svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true"><rect x="0.5" y="0.5" width="9" height="9" fill="none" stroke="currentColor"/></svg>',
    restore:
        '<svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true"><rect x="0.5" y="2.5" width="7" height="7" fill="none" stroke="currentColor"/><path d="M2.5 2.5V0.5H9.5V7.5H7.5" fill="none" stroke="currentColor"/></svg>',
    close: '<svg viewBox="0 0 10 10" width="10" height="10" aria-hidden="true"><path d="M0.5 0.5L9.5 9.5M9.5 0.5L0.5 9.5" fill="none" stroke="currentColor"/></svg>',
};

// TitleBar draws the custom title bar for the frameless windows. The menu bar is
// only drawn in the session window; the tool windows render just their title.
export default function TitleBar() {
    const app = useApp();
    const [openMenu, setOpenMenu] = useState(null);
    const [maximised, setMaximised] = useState(false);
    const custom = hasCustomChrome();
    const cfg = CHROME[WINDOW_NAME] || {};

    // Wire frameless window dragging / edge resizing once.
    useEffect(() => {
        if (!custom) return undefined;
        return installWindowGestures();
    }, [custom]);

    // Keep the maximise/restore glyph in sync (snapping and double-click can
    // change the state without going through the button).
    useEffect(() => {
        if (!custom) return undefined;
        let timer = null;
        const refresh = () => {
            clearTimeout(timer);
            timer = setTimeout(() => {
                windowControl("is-maximised")
                    .then((max) => setMaximised(!!max))
                    .catch(() => {});
            }, 160);
        };
        refresh();
        window.addEventListener("resize", refresh);
        return () => {
            clearTimeout(timer);
            window.removeEventListener("resize", refresh);
        };
    }, [custom]);

    useEffect(() => {
        if (!openMenu) return undefined;
        const close = () => setOpenMenu(null);
        const onKey = (e) => {
            if (e.key === "Escape") close();
        };
        document.addEventListener("click", close);
        document.addEventListener("keydown", onKey);
        return () => {
            document.removeEventListener("click", close);
            document.removeEventListener("keydown", onKey);
        };
    }, [openMenu]);

    if (!custom) {
        // macOS (native frame) and plain browsers render no title bar.
        return <div id="titlebar" className="titlebar" />;
    }

    const menus = [
        {
            label: "文件",
            items: [
                { label: "新建连接", accel: "Ctrl+N", run: () => app.appAction("new-connection") },
                { sep: true },
                { label: "退出", accel: "Ctrl+Q", run: () => app.appAction("quit") },
            ],
        },
        {
            label: "选项",
            items: [
                { label: "选项…", accel: "Ctrl+Shift+,", run: () => app.appAction("settings") },
                { label: "密钥管理…", accel: "Ctrl+Shift+K", run: () => app.appAction("keys") },
            ],
        },
        {
            label: "视图",
            items: [
                { label: "会话管理器", accel: "Ctrl+1", run: () => app.appAction("sessions") },
                { label: "文件传输", accel: "Ctrl+2", run: () => app.appAction("sftp") },
                { label: "端口隧道", accel: "Ctrl+3", run: () => app.appAction("tunnels") },
            ],
        },
        {
            label: "帮助",
            items: [
                { label: "关于 wsh", run: () => app.openDialog({ type: "about" }) },
                { label: "开发者工具", run: () => app.appAction("devtools") },
            ],
        },
    ];

    const toggleMaximise = () => {
        windowControl("toggle-maximise")
            .then(() => windowControl("is-maximised"))
            .then((max) => setMaximised(!!max))
            .catch(() => {});
    };

    return (
        <div id="titlebar" className="titlebar">
            <div className="tb-brand">
                {cfg.brand ? (
                    <>
                        <span className="logo-dot" />
                        <span>WSH</span>
                    </>
                ) : (
                    cfg.title || document.title
                )}
            </div>

            {cfg.menu && (
                <div className="menubar">
                    {menus.map((menu) => (
                        <div
                            key={menu.label}
                            className={"menu-item" + (openMenu === menu.label ? " open" : "")}
                            onMouseEnter={() => openMenu && setOpenMenu(menu.label)}
                        >
                            <button
                                type="button"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    setOpenMenu((cur) => (cur === menu.label ? null : menu.label));
                                }}
                            >
                                {menu.label}
                            </button>
                            {openMenu === menu.label && (
                                <div className="menu-dropdown">
                                    {menu.items.map((it, i) =>
                                        it.sep ? (
                                            <div className="sep" key={`sep-${i}`} />
                                        ) : (
                                            <div
                                                className="mi"
                                                key={it.label}
                                                onClick={() => {
                                                    setOpenMenu(null);
                                                    it.run();
                                                }}
                                            >
                                                <span className="mi-label">{it.label}</span>
                                                {it.accel && <span className="mi-accel">{it.accel}</span>}
                                            </div>
                                        ),
                                    )}
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            )}

            <div className="tb-spacer" onDoubleClick={toggleMaximise} />

            <div className="tb-controls">
                <button type="button" className="tb-btn" title="最小化" onClick={() => windowControl("minimise")} dangerouslySetInnerHTML={{ __html: ICONS.minimise }} />
                <button
                    type="button"
                    className="tb-btn"
                    title={maximised ? "还原" : "最大化"}
                    onClick={toggleMaximise}
                    dangerouslySetInnerHTML={{ __html: maximised ? ICONS.restore : ICONS.maximise }}
                />
                <button type="button" className="tb-btn close" title="关闭" onClick={() => windowControl("close")} dangerouslySetInnerHTML={{ __html: ICONS.close }} />
            </div>
        </div>
    );
}

import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { rpc } from "../lib/rpc.js";
import { useApp } from "../state/store.jsx";
import { base64Encode, base64Decode, triggerDownload } from "../lib/format.js";
import SmartInput from "./SmartInput.jsx";

// 终端报错特征对应的中文标题（与后端嗅探的 kind 对应）。
const ERROR_TITLES = {
    panic: "程序 panic",
    segfault: "段错误",
    oom: "内存不足 (OOM)",
    "disk-full": "磁盘空间不足",
    fatal: "致命错误",
    "core-dump": "核心转储",
    "port-in-use": "端口被占用",
    permission: "权限不足",
    "not-found": "命令或文件不存在",
    connection: "连接失败",
    denied: "认证失败",
    timeout: "操作超时",
    error: "错误输出",
};

function errorTitle(kind) {
    return ERROR_TITLES[kind] || "检测到错误";
}

// xterm palettes per app theme. The terminal keeps its own (dark by default)
// colours in dark mode; in light mode it follows the rest of the UI instead of
// staying a dark block in the middle of a light window.
const TERM_THEMES = {
    dark: {
        background: "#111118",
        foreground: "#e7e7f0",
        cursor: "#34d399",
        selectionBackground: "rgba(52,211,153,.25)",
    },
    light: {
        background: "#ffffff",
        foreground: "#1e1e26",
        cursor: "#0ea56e",
        selectionBackground: "rgba(14,165,110,.22)",
        black: "#1e1e26",
        brightBlack: "#5c5c6b",
        red: "#e11d48",
        brightRed: "#be123c",
        green: "#0ea56e",
        brightGreen: "#047857",
        yellow: "#b45309",
        brightYellow: "#d97706",
        blue: "#2563eb",
        brightBlue: "#1d4ed8",
        magenta: "#9333ea",
        brightMagenta: "#7e22ce",
        cyan: "#0e7490",
        brightCyan: "#155e75",
        white: "#e4e4ec",
        brightWhite: "#ffffff",
    },
};

// termThemeFor maps the app theme name to an xterm palette.
export function termThemeFor(theme) {
    return theme === "light" ? TERM_THEMES.light : TERM_THEMES.dark;
}

const TERM_OPTS = {
    fontFamily: '"Cascadia Code", "JetBrains Mono", Consolas, monospace',
    fontSize: 13,
    theme: TERM_THEMES.dark,
    scrollback: 5000,
};

export default function TerminalTab({ tab, active }) {
    const app = useApp();
    const wrapRef = useRef(null);
    const termRef = useRef(null);
    const fitRef = useRef(null);
    const sessionRef = useRef(null);
    const exitedRef = useRef(false);
    const [phase, setPhase] = useState("connecting"); // connecting | ready | error
    const [errorMsg, setErrorMsg] = useState("");
    const [zmodem, setZmodem] = useState(false);
    // altScreen tracks whether a full-screen (alternate screen) app is running,
    // which switches the Smart Input into raw passthrough.
    const [altScreen, setAltScreen] = useState(false);

    // Long-lived push handlers must not capture a stale store snapshot, so the
    // latest app object is kept in a ref.
    const appRef = useRef(app);
    appRef.current = app;

    // Mount once: create the terminal, subscribe to its pushes and connect.
    useEffect(() => {
        const wrap = wrapRef.current;
        const term = new Terminal(TERM_OPTS);
        const fit = new FitAddon();
        termRef.current = term;
        fitRef.current = fit;

        // Output pushed before the session id is known is buffered per session.
        const buffer = {};
        let sid = null;
        let disposed = false;

        const write = (data) => {
            if (!disposed) term.write(data);
        };

        const offData = rpc.on("terminal.data", (m) => {
            if (sid && m.sessionId === sid) write(m.data || "");
            else if (!sid) (buffer[m.sessionId] || (buffer[m.sessionId] = [])).push(m.data || "");
        });
        const offExit = rpc.on("terminal.exit", (m) => {
            if (m.sessionId !== sid) return;
            exitedRef.current = true;
            term.writeln("\r\n\x1b[90m[连接已关闭]\x1b[0m");
        });
        const offZSend = rpc.on("zmodem.send-file", (m) => {
            if (m.sessionId === sid) setZmodem(true);
        });
        const offZDownload = rpc.on("zmodem.download-start", (m) => {
            if (m.sessionId === sid) setZmodem(false);
        });
        const offZRecv = rpc.on("zmodem.receive", (m) => {
            if (m.sessionId !== sid) return;
            if (m.name) {
                term.writeln(`\r\n\x1b[90m[已接收 ${m.name}，正在保存…]\x1b[0m`);
                triggerDownload(m.name, base64Decode(m.data || ""));
            }
        });

        // Alternate-screen transitions (vim/htop/less) drive the input mode.
        const offMode = rpc.on("terminal.mode", (m) => {
            if (m.sessionId !== sid) return;
            setAltScreen(!!m.altScreen);
        });

        // An error feature in the output opens a diagnostic card on the right,
        // and (when enabled) immediately asks the model for a root cause.
        const offErr = rpc.on("terminal.error", (m) => {
            if (m.sessionId !== sid) return;
            const a = appRef.current;
            const analyze = () => {
                a.askAI({
                    tabId: tab.id,
                    sessionId: sid,
                    kind: "diagnose",
                    excerpt: m.excerpt,
                    title: "根因分析 · " + errorTitle(m.kind),
                });
            };
            a.pushReason({
                tabId: tab.id,
                kind: "error",
                title: errorTitle(m.kind),
                severity: m.severity,
                excerpt: m.excerpt,
                onAnalyze: analyze,
            });
            a.openReason();
            if (a.aiStatus?.configured && a.aiStatus?.autoAnalyze) analyze();
        });

        const onResize = term.onResize(({ cols, rows }) => {
            const s = sessionRef.current;
            if (s) rpc.call("terminal.resize", { sessionId: s, cols, rows }).catch(() => {});
        });
        const onData = term.onData((data) => {
            const s = sessionRef.current;
            if (s) rpc.call("terminal.input", { sessionId: s, data }).catch(() => {});
        });

        let opened = false;
        // user carries a username typed at the login prompt, used when the saved
        // connection has none. It applies to this login only.
        const connect = (password, keyPassphrase, user) => {
            setPhase("connecting");
            setErrorMsg("");
            // Saved connection (connId) or an ad-hoc quick-connect target.
            // keyPassphrase is the passphrase typed at a connect prompt for an
            // encrypted managed key; it stays in memory on the backend only.
            const target = tab.connId
                ? { connId: tab.connId, user: user || "", password: password || "", keyPassphrase: keyPassphrase || "" }
                : { host: tab.host, port: tab.port, user: user || tab.user, password: password || "", keyPassphrase: keyPassphrase || "" };
            rpc.call("terminal.open", target)
                .then((res) => {
                    if (disposed) return;
                    if (res && res.needUser) {
                        // The connection has no username, so ask for one and retry
                        // rather than failing with an opaque auth error.
                        setPhase("error");
                        setErrorMsg(res.message || "需要用户名");
                        app.openDialog({
                            type: "prompt",
                            title: "需要登录用户名",
                            label: "登录用户名",
                            onSubmit: (name) => connect(password, keyPassphrase, name),
                        });
                        return;
                    }
                    if (res && res.needPassphrase) {
                        setPhase("error");
                        setErrorMsg(res.message || "需要口令");
                        app.openDialog({
                            type: "passphrase",
                            message: res.message || "需要口令",
                            onSubmit: (pass) => connect(password, pass, user),
                        });
                        return;
                    }
                    if (res && res.needPassword) {
                        setPhase("error");
                        setErrorMsg(res.message || "需要密码");
                        app.openDialog({
                            type: "password",
                            message: res.message || "需要密码",
                            onSubmit: (pw) => connect(pw, keyPassphrase, user),
                        });
                        return;
                    }
                    sid = res.sessionId;
                    sessionRef.current = sid;
                    // term.open must only run once; a password retry reuses it.
                    if (!opened) {
                        opened = true;
                        term.loadAddon(fit);
                        term.open(wrap);
                        // Hardware acceleration for heavy log output; fall back to
                        // the default canvas renderer where WebGL is unavailable
                        // (some WebView2/WebKitGTK setups) rather than going blank.
                        try {
                            const webgl = new WebglAddon();
                            webgl.onContextLoss(() => webgl.dispose());
                            term.loadAddon(webgl);
                        } catch {
                            /* canvas renderer stays active */
                        }
                    }
                    try {
                        fit.fit();
                    } catch {
                        /* layout not ready yet */
                    }
                    const queued = buffer[sid];
                    if (queued) {
                        queued.forEach(write);
                        delete buffer[sid];
                    }
                    setPhase("ready");
                })
                .catch((err) => {
                    if (disposed) return;
                    setPhase("error");
                    setErrorMsg(err.message || String(err));
                });
        };

        connect("");

        return () => {
            disposed = true;
            const s = sessionRef.current;
            if (s && !exitedRef.current) rpc.call("terminal.close", { sessionId: s }).catch(() => {});
            onResize.dispose();
            onData.dispose();
            offData();
            offExit();
            offZSend();
            offZDownload();
            offZRecv();
            offMode();
            offErr();
            term.dispose();
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [tab.id]);

    // Follow the app theme: an existing terminal re-colours live when the user
    // switches theme in the options window.
    const themeName = app.settings.theme === "light" ? "light" : "dark";
    useEffect(() => {
        const term = termRef.current;
        if (!term) return;
        term.options.theme = termThemeFor(themeName);
    }, [themeName, phase]);

    // Closing a tab drops the reasoning cards that belonged to it.
    useEffect(() => {
        return () => app.clearReason(tab.id);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [tab.id]);

    // Fit and focus when this tab becomes visible. Hidden tabs have no size, so
    // fitting must not run while inactive.
    useEffect(() => {
        if (phase !== "ready" || !active) return undefined;
        const doFit = () => {
            try {
                fitRef.current?.fit();
            } catch {
                /* ignore */
            }
        };
        doFit();
        const t = setTimeout(() => termRef.current?.focus(), 60);
        window.addEventListener("resize", doFit);
        return () => {
            clearTimeout(t);
            window.removeEventListener("resize", doFit);
        };
    }, [phase, active]);

    return (
        <>
            <div className="term-wrap" ref={wrapRef} />
            {phase !== "ready" && (
                <div className="term-overlay">
                    {phase === "connecting" ? (
                        <div className="center-box">
                            <div className="spinner" />
                            <div>正在连接 {tab.title} …</div>
                        </div>
                    ) : (
                        <div className="center-box">
                            <div className="err">连接失败</div>
                            <div className="muted">{errorMsg}</div>
                        </div>
                    )}
                </div>
            )}
            {zmodem && <ZmodemBar sessionId={sessionRef.current} onDismiss={() => setZmodem(false)} term={termRef.current} />}
            {phase === "ready" && (
                <SmartInput
                    tabId={tab.id}
                    sessionId={sessionRef.current}
                    title={tab.title}
                    altScreen={altScreen}
                    onFocusTerminal={() => termRef.current?.focus()}
                />
            )}
        </>
    );
}

// ZmodemBar lets the user pick a file to send when the remote `rz` waits for
// one. The picker must be opened from a real user gesture (the push itself is
// not one), so a button is shown instead of clicking the input directly.
function ZmodemBar({ sessionId, onDismiss, term }) {
    const inputRef = useRef(null);

    const send = async (file) => {
        onDismiss();
        if (term) term.writeln(`\r\n\x1b[90m[正在发送 ${file.name} …]\x1b[0m`);
        try {
            // Chunked upload: a single base64 message would stall WebView2 on
            // large files. 1 MiB per chunk.
            const CHUNK = 1 << 20;
            const total = Math.ceil(file.size / CHUNK);
            await rpc.call("zmodem.sendBegin", { sessionId, name: file.name, size: file.size });
            for (let i = 0; i < total; i++) {
                const slice = file.slice(i * CHUNK, Math.min((i + 1) * CHUNK, file.size));
                const buf = await slice.arrayBuffer();
                await rpc.call("zmodem.sendChunk", { sessionId, index: i, data: base64Encode(buf) });
            }
            await rpc.call("zmodem.sendEnd", { sessionId });
            if (term) term.writeln("\r\n\x1b[90m[发送中，等待远端完成…]\x1b[0m");
        } catch (e) {
            if (term) term.writeln(`\r\n\x1b[90m[发送失败：${e.message}]\x1b[0m`);
        }
    };

    return (
        <div className="zmodem-bar">
            <span>远端 rz 正在等待接收文件</span>
            <input
                ref={inputRef}
                type="file"
                style={{ display: "none" }}
                onChange={(e) => {
                    const file = e.target.files?.[0];
                    if (file) send(file);
                }}
            />
            <button className="btn primary" onClick={() => inputRef.current?.click()}>
                选择文件…
            </button>
            <button
                className="btn"
                onClick={() => {
                    onDismiss();
                    rpc.call("zmodem.cancel", { sessionId }).catch(() => {});
                }}
            >
                取消
            </button>
        </div>
    );
}

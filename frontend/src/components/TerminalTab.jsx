import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { rpc } from "../lib/rpc.js";
import { useApp } from "../state/store.jsx";
import { base64Encode, base64Decode, triggerDownload } from "../lib/format.js";

const TERM_OPTS = {
    fontFamily: '"Cascadia Code", "JetBrains Mono", Consolas, monospace',
    fontSize: 13,
    theme: {
        background: "#111118",
        foreground: "#e7e7f0",
        cursor: "#34d399",
        selectionBackground: "rgba(52,211,153,.25)",
    },
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

        const onResize = term.onResize(({ cols, rows }) => {
            const s = sessionRef.current;
            if (s) rpc.call("terminal.resize", { sessionId: s, cols, rows }).catch(() => {});
        });
        const onData = term.onData((data) => {
            const s = sessionRef.current;
            if (s) rpc.call("terminal.input", { sessionId: s, data }).catch(() => {});
        });

        let opened = false;
        const connect = (password) => {
            setPhase("connecting");
            setErrorMsg("");
            rpc.call("terminal.open", { connId: tab.connId, password: password || "" })
                .then((res) => {
                    if (disposed) return;
                    if (res && res.needPassword) {
                        setPhase("error");
                        setErrorMsg(res.message || "需要密码");
                        app.openDialog({
                            type: "password",
                            message: res.message || "需要密码",
                            onSubmit: (pw) => connect(pw),
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
            term.dispose();
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [tab.connId]);

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

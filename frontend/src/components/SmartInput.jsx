import { useEffect, useMemo, useRef, useState } from "react";
import { rpc } from "../lib/rpc.js";
import { useApp } from "../state/store.jsx";
import { looksLikeNaturalLanguage, riskLabel } from "../lib/intent.js";

// SmartInput is the always-on input bar under the terminal. It carries both
// tracks: a shell command is sent to the remote PTY, natural language is turned
// into a command first, and every submission is classified by the local safety
// engine before anything is written (see PLAN.md M3/M5).
export default function SmartInput({ tabId, sessionId, title, altScreen, onFocusTerminal }) {
    const app = useApp();
    const [value, setValue] = useState("");
    const [risk, setRisk] = useState(null);
    const [mode, setMode] = useState("auto"); // auto | shell | ai
    const [preview, setPreview] = useState(null);
    const [note, setNote] = useState("");
    const [busy, setBusy] = useState(false);
    const inputRef = useRef(null);
    const disposedRef = useRef(false);

    useEffect(() => () => {
        disposedRef.current = true;
    }, []);

    const natural = useMemo(() => looksLikeNaturalLanguage(value, risk), [value, risk]);
    const level = risk?.level || "safe";
    const aiReady = !!app.aiStatus?.configured;

    // Live classification while typing, so a dangerous command is visible before
    // the user presses Enter.
    useEffect(() => {
        const cmd = value.trim();
        if (!cmd) {
            setRisk(null);
            return undefined;
        }
        const timer = setTimeout(() => {
            rpc.call("safety.check", { command: cmd })
                .then((res) => {
                    if (!disposedRef.current) setRisk(res);
                })
                .catch(() => {});
        }, 150);
        return () => clearTimeout(timer);
    }, [value]);

    // A reasoning card's fix button drops its command here.
    useEffect(() => {
        if (app.draft && app.draft.tabId === tabId) {
            setValue(app.draft.text || "");
            setPreview(null);
            setNote("");
            app.setDraft(null);
            inputRef.current?.focus();
        }
    }, [app.draft, tabId, app]);

    // Full-screen applications own the keyboard: typing into this box would
    // fight with vim/htop, so the box steps aside and keys go straight to the PTY.
    useEffect(() => {
        if (altScreen) onFocusTerminal?.();
    }, [altScreen, onFocusTerminal]);

    const runCommand = async (cmd) => {
        const command = String(cmd || "").trim();
        if (!command) return;
        if (!sessionId) {
            setNote("会话尚未就绪");
            return;
        }
        setNote("");
        try {
            const res = await rpc.call("terminal.exec", { sessionId, command });
            if (res.blocked) {
                setNote(res.result?.reason || "该命令已被本地安全引擎阻断");
                app.pushReason({
                    tabId,
                    kind: "notice",
                    title: "已阻断高危命令",
                    status: "blocked",
                    text: res.result?.reason || "",
                    command,
                    risk: res.result,
                });
                app.openReason();
                return;
            }
            if (res.confirm) {
                // Yellow zone: hand it to the dry-run panel and stop here. The
                // command is only sent after the user approves it explicitly.
                app.setPendingConfirm({
                    tabId,
                    command,
                    result: res.result,
                    onConfirm: async () => {
                        try {
                            const { token } = await rpc.call("safety.confirm", { sessionId, command });
                            await rpc.call("terminal.exec", { sessionId, command, confirmToken: token || "" });
                            app.setPendingConfirm(null);
                            setValue("");
                            setPreview(null);
                            setRisk(null);
                        } catch (e) {
                            setNote(e.message || String(e));
                        }
                    },
                    onCancel: () => app.setPendingConfirm(null),
                });
                app.openReason();
                return;
            }
            setValue("");
            setPreview(null);
            setRisk(null);
        } catch (e) {
            setNote(e.message || String(e));
        }
    };

    // requestPreview asks the model for a command and shows it *before* anything
    // runs. The model's proposal is classified locally, never trusted.
    const requestPreview = async () => {
        if (!aiReady) {
            setNote("未配置 AI 提供方，已按原样作为命令执行");
            return runCommand(value);
        }
        setBusy(true);
        setNote("");
        try {
            const req = await app.askAI({
                tabId,
                sessionId,
                kind: "command",
                prompt: value.trim(),
                title: "自然语言 → 命令",
            });
            const done = await waitForAI(req.requestId);
            if (disposedRef.current) return;
            if (!done || !done.ok) {
                setNote(done?.error || "AI 未返回可用结果");
                return;
            }
            if (!done.command) {
                setNote("AI 没有给出可执行的命令，请换一种说法");
                return;
            }
            setPreview({ command: done.command, risk: done.risk });
        } catch (e) {
            if (!disposedRef.current) setNote(e.message || String(e));
        } finally {
            if (!disposedRef.current) setBusy(false);
        }
    };

    const onKeyDown = (e) => {
        if (e.key === "Enter") {
            e.preventDefault();
            if (busy) return;
            if (preview) {
                runCommand(preview.command);
                return;
            }
            if (level === "blocked") {
                setNote(risk?.reason || "高危命令已禁用执行");
                return;
            }
            const useAI = mode === "ai" || (mode === "auto" && natural);
            if (useAI) {
                requestPreview();
                return;
            }
            runCommand(value);
            return;
        }
        if (e.key === "Tab" && preview) {
            // Fill the input for editing instead of executing.
            e.preventDefault();
            setValue(preview.command || "");
            setPreview(null);
            return;
        }
        if (e.key === "Escape") {
            setValue("");
            setPreview(null);
            setRisk(null);
            setNote("");
        }
    };

    if (altScreen) {
        return (
            <div id="smart-input" className="alt-passthrough">
                <span className="mode-pill">透传模式</span>
                <span className="muted">全屏应用运行中，键盘输入已直接转发给远端（Esc 退出全屏后恢复）</span>
            </div>
        );
    }

    return (
        <div id="smart-input" className={"level-" + level}>
            <div className="si-row">
                <span className="si-prompt">❯</span>
                <input
                    ref={inputRef}
                    className="si-field"
                    value={value}
                    placeholder={
                        aiReady
                            ? "输入命令，或用自然语言描述意图（如：帮我找出占用 8080 端口的进程）"
                            : "输入命令（配置 AI 后可用自然语言）"
                    }
                    onChange={(e) => setValue(e.target.value)}
                    onKeyDown={onKeyDown}
                    spellCheck={false}
                    autoComplete="off"
                />
                <select className="si-mode" value={mode} onChange={(e) => setMode(e.target.value)} title="解析模式">
                    <option value="auto">Auto</option>
                    <option value="shell">Shell</option>
                    <option value="ai">AI</option>
                </select>
                <span className={"si-lock level-" + level} title="本地 AST 安全引擎">
                    {level === "safe" ? "🔒 AST" : level === "caution" ? "⚠ AST" : "⛔ AST"}
                </span>
                {busy && <span className="spinner small" />}
            </div>

            {risk && value.trim() !== "" && (
                <div className={"si-hint level-" + level}>
                    {riskLabel(level)}
                    {risk.reason ? "：" + risk.reason : ""}
                    {natural && aiReady && !preview ? "（Enter 交给 AI 生成命令）" : ""}
                </div>
            )}

            {preview && (
                <div className="si-preview">
                    <div className="si-preview-head">
                        生成指令
                        <span className={"risk-pill level-" + (preview.risk?.level || "safe")}>
                            {riskLabel(preview.risk?.level || "safe")}
                        </span>
                    </div>
                    <code className="si-preview-cmd">{preview.command}</code>
                    <div className="si-preview-actions">
                        <span className="muted">Enter 执行 · Tab 填入输入框 · Esc 取消</span>
                        <button className="btn small" onClick={() => setValue(preview.command || "")}>
                            填入
                        </button>
                        <button className="btn primary small" onClick={() => runCommand(preview.command)}>
                            执行
                        </button>
                    </div>
                </div>
            )}

            {note && <div className="si-note err">{note}</div>}
        </div>
    );
}

// waitForAI resolves with the ai.done message for one request.
function waitForAI(requestId) {
    return new Promise((resolve) => {
        const off = rpc.on("ai.done", (m) => {
            if (m.requestId !== requestId) return;
            off();
            resolve(m);
        });
    });
}

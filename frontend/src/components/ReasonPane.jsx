import { useEffect, useState } from "react";
import { useApp } from "../state/store.jsx";
import { riskLabel, sevLabel } from "../lib/intent.js";

// ReasonPane is the right-hand column: everything that is "thinking" rather than
// "typing" lives here, so the terminal itself is never polluted by AI output.
// It renders four kinds of card (see PLAN.md M5/M6/M7):
//
//   - 影子推演 (dry-run) for a yellow-zone command awaiting approval;
//   - CoT cards streaming the model's reasoning;
//   - error/RCA cards raised by the output sniffer;
//   - blocked-command notices from the safety engine.
export default function ReasonPane({ tabId }) {
    const app = useApp();
    const items = app.reason.filter((r) => r.tabId === tabId);
    const pending = app.pendingConfirm && app.pendingConfirm.tabId === tabId ? app.pendingConfirm : null;

    return (
        <aside id="reason-pane">
            <div id="reason-header">
                <span className="logo-dot" />
                <span className="logo-text">AI 推理</span>
                <span className="grow" />
                {items.length > 0 && (
                    <button className="icon-btn" title="清空" onClick={() => app.clearReason(tabId)}>
                        🗑
                    </button>
                )}
                <button className="icon-btn" title="收起" onClick={() => app.toggleReason()}>
                    ✕
                </button>
            </div>

            <div id="reason-body">
                {!app.aiStatus?.configured && !pending && (
                    <div className="reason-empty">
                        <div>
                            未配置 AI 提供方。本地安全引擎仍然工作：高危命令会被直接阻断，黄区命令会在此处推演确认。
                        </div>
                        <div className="muted" style={{ marginTop: 6 }}>
                            在「选项 → AI 推理」中配置 OpenAI 兼容接口或本地 Ollama。
                        </div>
                    </div>
                )}

                {pending && <DryRunCard pending={pending} />}

                {items.length === 0 && !pending && app.aiStatus?.configured && (
                    <div className="reason-empty">
                        暂无推理内容。在底部输入自然语言，或让终端抛出错误后自动诊断。
                    </div>
                )}

                {items.map((item) => (
                    <ReasonCard key={item.id} item={item} tabId={tabId} />
                ))}
            </div>
        </aside>
    );
}

// DryRunCard is the 影子推演 panel: it lists what the command would do and waits
// for an explicit click before the command is sent.
function DryRunCard({ pending }) {
    const findings = pending.result?.findings || [];
    return (
        <div className="reason-card dry-run">
            <div className="rc-head">
                <span className="risk-pill level-caution">影子推演</span>
                <span className="rc-title">该命令需要确认</span>
            </div>
            <code className="rc-cmd">{pending.command}</code>
            <ul className="rc-findings">
                {findings.map((f, i) => (
                    <li key={i}>
                        <span className={"risk-pill level-" + f.level}>{riskLabel(f.level)}</span>
                        {f.reason}
                    </li>
                ))}
            </ul>
            <div className="rc-actions">
                <button className="btn small" onClick={pending.onCancel}>
                    取消
                </button>
                <button className="btn primary small" onClick={pending.onConfirm}>
                    确认执行
                </button>
            </div>
        </div>
    );
}

function ReasonCard({ item, tabId }) {
    const app = useApp();
    // A card that carries a command stays open: the command, its two action
    // buttons and the analysis of its output are the whole point of the card.
    const [open, setOpen] = useState(item.kind !== "cot" || item.status === "streaming" || !!item.command);

    // The loop must be visible: when the command runs or its analysis arrives,
    // reveal the card even if the user had collapsed it.
    useEffect(() => {
        if (item.exec || item.analysis) setOpen(true);
    }, [item.exec, item.analysis]);

    // fill drops a command into the Smart Input. The card id travels with it so
    // the command's output can still be attributed back here.
    const fill = (cmd, cardId) => {
        app.setDraft({ tabId, text: cmd, cardId: cardId || "" });
        app.openReason();
    };

    if (item.kind === "notice") {
        return (
            <div className="reason-card blocked">
                <div className="rc-head">
                    <span className="risk-pill level-blocked">已阻断</span>
                    <span className="rc-title">{item.title}</span>
                </div>
                <div className="rc-text">{item.text}</div>
                {item.command && <code className="rc-cmd">{item.command}</code>}
            </div>
        );
    }

    if (item.kind === "ask") {
        // The user's own turn in the conversation: without it the pane only shows
        // answers, and there is no way to tell what was asked.
        return (
            <div className="reason-card ask">
                <div className="rc-head">
                    <span className="rc-ask-mark">💬</span>
                    <span className="rc-title">{item.title || "我的提问"}</span>
                </div>
                <div className="rc-ask-text">{item.text}</div>
            </div>
        );
    }

    if (item.kind === "error") {
        return (
            <div className="reason-card error">
                <div className="rc-head">
                    <span className={"sev-pill sev-" + (item.severity || "low")}>{sevLabel(item.severity)}</span>
                    <span className="rc-title">{item.title}</span>
                </div>
                <pre className="rc-excerpt">{item.excerpt}</pre>
                {item.onAnalyze && (
                    <div className="rc-actions">
                        <button className="btn small" onClick={item.onAnalyze}>
                            分析根因
                        </button>
                    </div>
                )}
            </div>
        );
    }

    // CoT card.
    const steps = item.steps || [];
    const streaming = item.status === "streaming";
    return (
        <div className={"reason-card cot " + (item.status || "")}>
            <div className="rc-head clickable" onClick={() => setOpen((v) => !v)}>
                <span className="caret">{open ? "▾" : "▸"}</span>
                <span className="rc-title">{item.title}</span>
                {streaming && <span className="spinner small" />}
                {item.status === "error" && <span className="risk-pill level-blocked">失败</span>}
            </div>

            {open && (
                <>
                    {steps.length > 0 ? (
                        <ol className="rc-steps">
                            {steps.map((s, i) => (
                                <li key={i}>
                                    <span className="rc-step-label">[{s.label}]</span> {s.detail}
                                </li>
                            ))}
                        </ol>
                    ) : (
                        <pre className="rc-stream">{item.text || (streaming ? "思考中…" : "")}</pre>
                    )}

                    {item.command && <GeneratedCommand item={item} tabId={tabId} fill={fill} />}
                </>
            )}
        </div>
    );
}

// GeneratedCommand is the interactive part of a card: the command the model
// proposed, the two ways to run it, and — once it has run — the interpretation of
// its output, attached to this same card. That attachment is what turns "the
// model wrote a command" into "the model finished a diagnosis".
function GeneratedCommand({ item, tabId, fill }) {
    const app = useApp();
    const [note, setNote] = useState("");
    const exec = item.exec || {};
    const analysis = item.analysis;
    const running = exec.status === "running";

    const run = async () => {
        if (running) return;
        setNote("");
        const res = await app.runCommand({ tabId, command: item.command, trackCardId: item.id });
        if (res.status === "blocked" || res.status === "error" || res.status === "nosession") {
            setNote(res.message || "");
        }
    };

    // Ctrl+Enter on the focused card runs it, mirroring the Smart Input.
    const onKeyDown = (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
            e.preventDefault();
            run();
        }
    };

    return (
        <div className="rc-generated" tabIndex={0} onKeyDown={onKeyDown}>
            <div className="rc-generated-head">
                <span>生成指令</span>
                <span className={"risk-pill level-" + (item.risk?.level || "safe")}>
                    {riskLabel(item.risk?.level || "safe")}
                </span>
            </div>
            <code className="rc-cmd">{item.command}</code>
            <div className="rc-actions">
                <button className="btn small" onClick={() => fill(item.command, item.id)} title="也可以直接按 Tab">
                    填入 (Tab)
                </button>
                <button className="btn primary small" onClick={run} disabled={running} title="Ctrl+Enter">
                    {running ? "执行中…" : "🚀 立即执行 (Ctrl+↵)"}
                </button>
            </div>
            <ExecStatus exec={exec} />
            {note && <div className="rc-note err">{note}</div>}
            {analysis && <ResultAnalysis analysis={analysis} tabId={tabId} />}
        </div>
    );
}

// ExecStatus reports what happened to the command the card proposed.
function ExecStatus({ exec }) {
    const status = exec?.status;
    if (!status || status === "idle") return null;
    switch (status) {
        case "running":
            return (
                <div className="rc-exec running">
                    <span className="spinner small" /> 已下发，等待输出…
                </div>
            );
        case "blocked":
            return <div className="rc-exec blocked">⛔ 已被本地安全引擎阻断{exec.reason ? "：" + exec.reason : ""}</div>;
        case "confirm":
            return <div className="rc-exec caution">⚠ 需要确认，见上方影子推演</div>;
        case "error":
            return <div className="rc-exec blocked">执行失败{exec.reason ? "：" + exec.reason : ""}</div>;
        default: {
            const secs = ((exec.durationMs || 0) / 1000).toFixed(1);
            const tail = exec.truncated ? "（输出过长，已截断）" : "";
            if (status === "timeout") {
                return (
                    <div className="rc-exec caution">
                        🟡 命令仍在运行，已截取前 {secs}s 输出{tail}
                    </div>
                );
            }
            return (
                <div className="rc-exec done">
                    🟢 已执行（耗时 {secs}s）{tail}
                </div>
            );
        }
    }
}

// ResultAnalysis renders the model's reading of the output, plus one button per
// proposed next step. Clicking a suggestion runs it as a new card, so every step
// of the investigation keeps its own output and analysis.
function ResultAnalysis({ analysis, tabId }) {
    const app = useApp();
    const steps = analysis.steps || [];
    const suggestions = analysis.suggestions || [];
    const streaming = analysis.status === "streaming";

    const runSuggestion = (s) => {
        app.runSuggestion({ tabId, command: s.command, title: s.label || "下一步排查" });
    };

    return (
        <div className="rc-analysis">
            <div className="rc-analysis-head">
                <span>🤖 AI 结果分析</span>
                {streaming && <span className="spinner small" />}
                {analysis.status === "error" && <span className="risk-pill level-blocked">失败</span>}
            </div>
            {steps.length > 0 ? (
                <ol className="rc-steps">
                    {steps.map((s, i) => (
                        <li key={i}>
                            <span className="rc-step-label">[{s.label}]</span> {s.detail}
                        </li>
                    ))}
                </ol>
            ) : (
                <pre className="rc-stream">{analysis.text || (streaming ? "分析中…" : "")}</pre>
            )}
            {suggestions.length > 0 && (
                <div className="rc-followups">
                    <div className="rc-followups-head">
                        <span>快捷追问</span>
                        <span className="muted">点击即执行，仍需通过本地安全引擎</span>
                    </div>
                    <div className="rc-actions">
                        {suggestions.map((s, i) => (
                            <button key={i} className="btn small" onClick={() => runSuggestion(s)} title={s.command}>
                                {s.label}
                            </button>
                        ))}
                    </div>
                </div>
            )}
        </div>
    );
}

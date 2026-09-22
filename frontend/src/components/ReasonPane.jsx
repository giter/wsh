import { useEffect, useRef, useState } from "react";
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

    // Follow the tail while it grows (cards streaming in, analyses landing), but
    // stop the moment the user scrolls up: yanking the view back down while they
    // are reading is worse than missing the newest line.
    const bodyRef = useRef(null);
    const stick = useRef(true);

    const onScroll = () => {
        const el = bodyRef.current;
        if (!el) return;
        stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
    };

    useEffect(() => {
        const el = bodyRef.current;
        if (!el || !stick.current) return;
        el.scrollTop = el.scrollHeight;
    }, [items, pending]);

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
                <button className="icon-btn" title="收起（Ctrl+Shift+A 可重新打开）" onClick={() => app.toggleReason()}>
                    ✕
                </button>
            </div>

            <div id="reason-body" ref={bodyRef} onScroll={onScroll}>
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
// for an explicit choice before the command is sent. The three buttons are the
// approval *levels* — how far the user's decision reaches — rather than three ways
// to do the same thing.
function DryRunCard({ pending }) {
    const findings = pending.result?.findings || [];
    const confirm = pending.onConfirm;
    return (
        <div className="reason-card dry-run">
            <div className="rc-head">
                <span className="risk-pill level-caution">影子推演</span>
                <span className="rc-title">该命令需要确认</span>
                {pending.auto && <span className="rc-thinking">AI 自动发起</span>}
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
                <button className="btn small" onClick={() => confirm("")} title="只放行这一次">
                    仅此一次
                </button>
                <button className="btn small" onClick={() => confirm("session")} title="本会话内不再询问这条命令">
                    本会话允许
                </button>
                <button className="btn primary small" onClick={() => confirm("always")} title="永久放行这条命令，可在「选项 → AI 推理」中移除">
                    始终允许
                </button>
            </div>
            <div className="rc-allow-hint">
                「本会话允许」和「始终允许」只对这条命令本身生效（完全相同的一行），不会放宽其他命令。
            </div>
        </div>
    );
}

// stepTone maps a reasoning label to its visual weight. The model narrates its
// process ([意图分析]/[策略选择]/[安全判定]) and states findings ([结论]/[要点])
// and warnings ([风险]) in the same flat list, so without this the pane reads as
// a wall of equally important text and the one line that matters is lost in it.
function stepTone(label) {
    switch (label) {
        case "风险":
            return "risk";
        // Separate case labels, not `case "结论", "根因":` — the comma there is the
        // comma operator, which would only ever compare against the last value.
        case "结论":
        case "根因":
            return "conclusion";
        case "要点":
        case "影响":
            return "point";
        case "修复":
        case "后续":
        case "建议":
            return "action";
        case "生成指令":
        case "命令":
            return "command";
        default:
            // 意图分析 / 策略选择 / 安全判定 / 说明 / 状态 …
            return "narration";
    }
}

// hasWarning reports whether any step carries a real warning, so a collapsed card
// can show it in its header. The prompts ask the model to write 无 when there is
// nothing to flag, which is what makes this check meaningful.
function hasWarning(steps) {
    return (steps || []).some((s) => s.label === "风险" && isPresent(s.detail));
}

// nothingToFlag matches the ways a model says "there is nothing here". Kept
// explicit rather than a loose "starts with 无" test, so a real warning such as
// "无法确定是否为预期服务" is not swallowed by it.
const nothingToFlag = /^(无|没有)(风险|异常|明显|问题|特殊|需特别|需额外|需人工|需处理)?$/;

function isPresent(detail) {
    const s = String(detail || "").trim();
    if (s === "") return false;
    if (nothingToFlag.test(s)) return false;
    return !/^(none|n\/a|null)$/i.test(s);
}

// StepList renders the model's bracketed steps with the weight their label
// deserves: narration recedes, findings and warnings do not.
function StepList({ steps }) {
    return (
        <ol className="rc-steps">
            {steps.map((s, i) => {
                const tone = stepTone(s.label);
                return (
                    <li key={i} className={"rc-step tone-" + tone}>
                        <span className="rc-step-label">
                            {tone === "risk" && "⚠ "}
                            [{s.label}]
                        </span>
                        <span className="rc-step-detail">{s.detail}</span>
                    </li>
                );
            })}
        </ol>
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
    // The model's raw reply is the thinking process: it is what streams in. Once
    // the reply is parsed into steps, the steps are the readable form, so the raw
    // text is kept behind a toggle rather than thrown away.
    const raw = (item.text || "").trim();
    // Either the generation or its result analysis can raise the warning, and the
    // card is often collapsed, so the header has to carry it.
    const warn = hasWarning(steps) || hasWarning(item.analysis?.steps);
    return (
        <div className={"reason-card cot " + (item.status || "")}>
            <div className="rc-head clickable" onClick={() => setOpen((v) => !v)}>
                <span className="caret">{open ? "▾" : "▸"}</span>
                <span className="rc-title">{item.title}</span>
                {streaming && <span className="spinner small" />}
                {streaming && <span className="rc-thinking">思考中…</span>}
                {warn && <span className="risk-pill level-caution">⚠ 有风险提示</span>}
                {item.status === "error" && <span className="risk-pill level-blocked">失败</span>}
            </div>

            {open && (
                <>
                    {steps.length > 0 ? (
                        <StepList steps={steps} />
                    ) : (
                        <pre className="rc-stream">{raw || (streaming ? "思考中…" : "")}</pre>
                    )}

                    {steps.length > 0 && raw && (
                        <details className="rc-raw">
                            <summary>思考过程（模型原始回复）</summary>
                            <pre className="rc-stream">{raw}</pre>
                        </details>
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
    // A command that ran on its own says so: the user must never wonder why the
    // terminal moved without them pressing anything.
    const auto = !!exec.auto;
    switch (status) {
        case "running":
            return (
                <div className="rc-exec running">
                    <span className="spinner small" /> {auto ? "绿区命令已自动执行，等待输出…" : "已下发，等待输出…"}
                </div>
            );
        case "blocked":
            return <div className="rc-exec blocked">⛔ 已被本地安全引擎阻断{exec.reason ? "：" + exec.reason : ""}</div>;
        case "confirm":
            return <div className="rc-exec caution">⚠ 黄区命令，需确认 — 见上方影子推演</div>;
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
                    {auto ? "⚡ 已自动执行（绿区" : "🟢 已执行（"}
                    {secs}s）{tail}
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
                <StepList steps={steps} />
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

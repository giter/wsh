import { useState } from "react";
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
    const [open, setOpen] = useState(item.kind !== "cot" || item.status === "streaming");

    const fill = (cmd) => {
        app.setDraft({ tabId, text: cmd });
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

                    {item.command && (
                        <div className="rc-generated">
                            <div className="rc-generated-head">
                                <span>生成指令</span>
                                <span className={"risk-pill level-" + (item.risk?.level || "safe")}>
                                    {riskLabel(item.risk?.level || "safe")}
                                </span>
                            </div>
                            <code className="rc-cmd">{item.command}</code>
                            <div className="rc-actions">
                                <button className="btn small" onClick={() => fill(item.command)}>
                                    填入输入框
                                </button>
                            </div>
                        </div>
                    )}
                </>
            )}
        </div>
    );
}

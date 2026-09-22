import { useEffect, useRef, useState } from "react";
import { useApp } from "../state/store.jsx";
import { riskLabel, sevLabel } from "../lib/intent.js";
import { t, useT } from "../lib/i18n.js";

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
    const t = useT();
    const items = app.reason.filter((r) => r.tabId === tabId);
    // The card that most recently changed (see noteFocus in the store) is kept
    // open alongside the newest one.
    const focusId = app.focusByTab?.[tabId];
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
                <span className="logo-text">{t("reason.title")}</span>
                <span className="grow" />
                {items.length > 0 && (
                    <button className="icon-btn" title={t("reason.clear")} onClick={() => app.clearReason(tabId)}>
                        🗑
                    </button>
                )}
                <button className="icon-btn" title={t("reason.collapse")} onClick={() => app.toggleReason()}>
                    ✕
                </button>
            </div>

            <div id="reason-body" ref={bodyRef} onScroll={onScroll}>
                {!app.aiStatus?.configured && !pending && (
                    <div className="reason-empty">
                        <div>
                            {t("reason.notConfigured")}
                        </div>
                        <div className="muted" style={{ marginTop: 6 }}>
                            {t("reason.notConfiguredHint")}
                        </div>
                    </div>
                )}

                {pending && <DryRunCard pending={pending} />}

                {items.length === 0 && !pending && app.aiStatus?.configured && (
                    <div className="reason-empty">
                        {t("reason.empty")}
                    </div>
                )}

                {items.map((item, i) => (
                    <ReasonCard
                        key={item.id}
                        item={item}
                        tabId={tabId}
                        latest={i === items.length - 1}
                        focused={item.id === focusId}
                        onRetry={(key) => app.retryAI(item.id, key)}
                    />
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
                <span className="risk-pill level-caution">{t("reason.dryRun")}</span>
                <span className="rc-title">{t("reason.needsConfirm")}</span>
                {pending.auto && <span className="rc-thinking">{t("reason.autoInitiated")}</span>}
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
                    {t("reason.cancel")}
                </button>
                <button className="btn small" onClick={() => confirm("")} title={t("reason.onceTip")}>
                    {t("reason.once")}
                </button>
                <button className="btn small" onClick={() => confirm("session")} title={t("reason.allowSessionTip")}>
                    {t("reason.allowSession")}
                </button>
                <button className="btn primary small" onClick={() => confirm("always")} title={t("reason.alwaysAllowTip")}>
                    {t("reason.alwaysAllow")}
                </button>
            </div>
            <div className="rc-allow-hint">
                {t("reason.allowHint")}
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

// cardSummary reduces a card to the single line a folded card shows. It prefers
// the concrete outcome — what ran, how it ended, the conclusion the model drew —
// over the title, because the title is already in the header.
function cardSummary(item) {
    if (item.kind === "ask") return item.text || "";
    if (item.kind === "notice") return item.text || item.title || "";
    if (item.kind === "error") return item.excerpt || item.title || "";
    const exec = item.exec || {};
    const cmd = item.command || "";
    // Once there is an analysis, its reading is the point; before that, the
    // generation's own steps are all there is.
    const steps = exec.output ? item.analysis?.steps : item.steps;
    const hit = (steps || []).find((s) => s.label === "结论" || s.label === "根因" || s.label === "要点");
    const note = hit ? hit.detail : "";
    const secs = ((exec.durationMs || 0) / 1000).toFixed(1);
    // A failed generation has nothing else to show; a failed analysis must not be
    // summarised as if it had produced a reading. Both carry a retry in the card.
    if (item.status === "error") return t("reason.failedSummary", { title: item.title || t("reason.aiRequest") });
    if (item.status === "cancelled")
        return t("reason.cancelledSummary", { target: cmd || item.title || t("reason.aiRequest") });
    if (item.analysis?.status === "error") return t("reason.analysisFailedSummary", { cmd });
    switch (exec.status) {
        case "running":
            return t("reason.summaryRunning", { cmd });
        case "waiting":
            return t("reason.summaryWaiting", { cmd });
        case "streaming":
            return t("reason.summaryStreaming", { cmd });
        case "timeout":
            return t("reason.summaryTimeout", { cmd, secs });
        case "blocked":
            return t("reason.summaryBlocked", { cmd });
        case "confirm":
            return t("reason.summaryConfirm", { cmd });
        case "error":
            return t("reason.summaryError", { cmd });
        case "done":
            // A segment that ended at a password prompt is not a green result.
            if (exec.waitingInput) return t("reason.summaryStalled", { cmd });
            return t("reason.summaryDone", { cmd, secs }) + (note ? " — " + note : "");
        default:
            return (
                t("reason.summaryDefault", { target: cmd || item.title || t("reason.aiFallback") }) +
                (note ? " — " + note : "")
            );
    }
}

function ReasonCard({ item, tabId, latest, focused, onRetry }) {
    const app = useApp();
    // A card folds to a single summary line once the conversation moves on, so a
    // ten-step investigation stays scannable. Three cards stay open: the newest,
    // the one that just changed, and any with work still in flight. Clicking the
    // header overrides that, and the choice sticks.
    const foldable = item.kind === "ask" || item.kind === "cot";
    const streaming = item.status === "streaming";
    const execRunning = item.exec?.status === "running" || item.exec?.status === "streaming";
    const analysisStreaming = item.analysis?.status === "streaming";
    const auto = !!latest || !!focused || streaming || execRunning || analysisStreaming;
    const [manual, setManual] = useState(null);
    const open = foldable ? (manual === null ? auto : manual) : true;
    const toggle = () => setManual(!open);

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
                    <span className="risk-pill level-blocked">{t("reason.blockedPill")}</span>
                    <span className="rc-title">{item.title}</span>
                </div>
                <div className="rc-text">{item.text}</div>
                {item.command && <code className="rc-cmd">{item.command}</code>}
            </div>
        );
    }

    if (item.kind === "ask") {
        // The user's own turn in the conversation: without it the pane only shows
        // answers, and there is no way to tell what was asked. It folds like any
        // other card once the answer has pushed it up the thread.
        return (
            <div className="reason-card ask">
                <div className="rc-head clickable" onClick={toggle}>
                    <span className="caret">{open ? "▾" : "▸"}</span>
                    <span className="rc-ask-mark">💬</span>
                    <span className="rc-title">{item.title || t("reason.myQuestion")}</span>
                </div>
                {open ? <div className="rc-ask-text">{item.text}</div> : <div className="rc-summary">{item.text}</div>}
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
                            {t("reason.analyzeRca")}
                        </button>
                    </div>
                )}
            </div>
        );
    }

    // CoT card.
    const steps = item.steps || [];
    // The model's raw reply is the thinking process: it is what streams in. Once
    // the reply is parsed into steps, the steps are the readable form, so the raw
    // text is kept behind a toggle rather than thrown away.
    const raw = (item.text || "").trim();
    // Either the generation or its result analysis can raise the warning, and the
    // card is often folded, so the header has to carry it.
    const warn = hasWarning(steps) || hasWarning(item.analysis?.steps);
    return (
        <div className={"reason-card cot " + (item.status || "")}>
            <div className="rc-head clickable" onClick={toggle}>
                <span className="caret">{open ? "▾" : "▸"}</span>
                <span className="rc-title">{item.title}</span>
                {streaming && <span className="spinner small" />}
                {streaming && <span className="rc-thinking">{t("reason.thinking")}</span>}
                {item.status === "cancelled" && <span className="rc-thinking">{t("reason.cancelled")}</span>}
                {warn && <span className="risk-pill level-caution">{t("reason.warnPill")}</span>}
                {item.status === "error" && <span className="risk-pill level-blocked">{t("reason.failed")}</span>}
            </div>

            {!open && <div className="rc-summary">{cardSummary(item)}</div>}

            {open && (
                <>
                    {steps.length > 0 ? (
                        <StepList steps={steps} />
                    ) : (
                        <pre className="rc-stream">{raw || (streaming ? t("reason.thinking") : "")}</pre>
                    )}

                    {steps.length > 0 && raw && (
                        <details className="rc-raw">
                            <summary>{t("reason.rawReply")}</summary>
                            <pre className="rc-stream">{raw}</pre>
                        </details>
                    )}

                    {/* A completion that failed or was abandoned leaves the card
                        with nothing; retrying it is the whole recovery path. */}
                    {(item.status === "error" || item.status === "cancelled") && onRetry && (
                        <div className="rc-actions">
                            <button className="btn small" onClick={() => onRetry(null)}>
                                {t("reason.retry")}
                            </button>
                        </div>
                    )}

                    {item.command && <GeneratedCommand item={item} tabId={tabId} fill={fill} onRetry={onRetry} />}
                </>
            )}
        </div>
    );
}

// GeneratedCommand is the interactive part of a card: the command the model
// proposed, the two ways to run it, and — once it has run — the interpretation of
// its output, attached to this same card. That attachment is what turns "the
// model wrote a command" into "the model finished a diagnosis".
function GeneratedCommand({ item, tabId, fill, onRetry }) {
    const app = useApp();
    const [note, setNote] = useState("");
    const [revealNote, setRevealNote] = useState("");
    const exec = item.exec || {};
    const analysis = item.analysis;
    const running = exec.status === "running";

    // anchored means there is captured output and a marker in the terminal to
    // point at, so hover/reveal have something to do. The line count is the
    // command's echo plus its output, which is what gets highlighted.
    const output = exec.output || "";
    const lines = output ? output.split("\n").length + 1 : 0;
    const anchored = !!output && (exec.status === "done" || exec.status === "timeout" || exec.status === "streaming");

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

    const reveal = () => {
        setRevealNote("");
        if (!app.revealOutput(tabId, item.id, lines)) {
            setRevealNote(t("reason.outputGone"));
        }
    };

    return (
        <div
            className="rc-generated"
            tabIndex={0}
            onKeyDown={onKeyDown}
            onMouseEnter={() => anchored && app.hoverOutput(tabId, item.id, lines, true)}
            onMouseLeave={() => anchored && app.hoverOutput(tabId, item.id, lines, false)}
        >
            <div className="rc-generated-head">
                <span>{t("reason.generatedCommand")}</span>
                <span className={"risk-pill level-" + (item.risk?.level || "safe")}>
                    {riskLabel(item.risk?.level || "safe")}
                </span>
            </div>
            <code className="rc-cmd">{item.command}</code>
            <div className="rc-actions">
                <button className="btn small" onClick={() => fill(item.command, item.id)} title={t("reason.fillTabTip")}>
                    {t("reason.fillTab")}
                </button>
                <button className="btn primary small" onClick={run} disabled={running} title="Ctrl+Enter">
                    {running ? t("reason.running") : t("reason.runNow")}
                </button>
            </div>
            <div className="rc-execrow">
                <ExecStatus exec={exec} onStop={() => app.interruptTab(tabId)} />
                {anchored && (
                    <button className="btn small ghost" onClick={reveal} title={t("reason.viewOutputTip")}>
                        {t("reason.viewOutput")}
                    </button>
                )}
            </div>
            {revealNote && <div className="rc-note err">{revealNote}</div>}
            {note && <div className="rc-note err">{note}</div>}
            {analysis && <ResultAnalysis analysis={analysis} tabId={tabId} onRetry={onRetry} />}
        </div>
    );
}

// ExecStatus reports what happened to the command the card proposed. onStop lets
// the user end a command that will not end on its own, which is the whole point
// of recognising a stream: a bare spinner gives no way out. confirmed marks a run
// the user approved in the dry-run panel, which is a different event from an
// automatic one and is reported differently.
function ExecStatus({ exec, onStop }) {
    const status = exec?.status;
    if (!status || status === "idle") return null;
    // A command that ran on its own says so: the user must never wonder why the
    // terminal moved without them pressing anything.
    const auto = !!exec.auto;
    const secs = ((exec.durationMs || 0) / 1000).toFixed(1);
    const stop = onStop ? (
        <button className="btn small" onClick={onStop} title={t("reason.stopTip")}>
            {t("reason.stop")}
        </button>
    ) : null;
    switch (status) {
        case "running":
            return (
                <div className="rc-exec running">
                    <span className="spinner small" />
                    {exec.confirmed
                        ? t("reason.confirmedWaiting")
                        : auto
                          ? t("reason.autoWaiting")
                          : t("reason.sentWaiting")}
                    {stop}
                </div>
            );
        case "waiting":
            // Not running and not finished: the command is at a prompt for a
            // secret. Saying so beats a spinner that looks stuck.
            return (
                <div className="rc-exec caution">
                    {t("reason.waitingPassword")}
                    {stop}
                </div>
            );
        case "streaming": {
            const el = ((exec.elapsedMs || 0) / 1000).toFixed(1);
            return (
                <div className="rc-exec streaming">
                    <span className="spinner small" /> {t("reason.streamingStatus", { el })}
                    {stop}
                </div>
            );
        }
        case "blocked":
            return <div className="rc-exec blocked">{t("reason.blockedByEngine", { reason: exec.reason ? "：" + exec.reason : "" })}</div>;
        case "confirm":
            return <div className="rc-exec caution">{t("reason.needConfirmExec")}</div>;
        case "error":
            return <div className="rc-exec blocked">{t("reason.execFailed", { reason: exec.reason ? "：" + exec.reason : "" })}</div>;
        default: {
            const tail = exec.truncated ? t("reason.truncatedTail") : "";
            if (status === "timeout") {
                return (
                    <div className="rc-exec caution">
                        {t("reason.timeoutStatus", { secs, tail })}
                        {stop}
                    </div>
                );
            }
            if (exec.waitingInput) {
                // The segment ended at a prompt for a secret: the command did not
                // finish, so it is not a result and must not be shown as one.
                return (
                    <div className="rc-exec caution">
                        {t("reason.stoppedAtPrompt")}
                        {stop}
                    </div>
                );
            }
            return (
                <div className="rc-exec done">
                    {exec.confirmed
                        ? t("reason.doneConfirmed", { secs, tail })
                        : auto
                          ? t("reason.doneAuto", { secs, tail })
                          : t("reason.done", { secs, tail })}
                </div>
            );
        }
    }
}

// ResultAnalysis renders the model's reading of the output, plus one button per
// proposed next step. Clicking a suggestion runs it as a new card, so every step
// of the investigation keeps its own output and analysis.
function ResultAnalysis({ analysis, tabId, onRetry }) {
    const app = useApp();
    const steps = analysis.steps || [];
    const suggestions = analysis.suggestions || [];
    const streaming = analysis.status === "streaming";

    const runSuggestion = (s) => {
        // One click is the whole gesture: the chip's label becomes the user's turn,
        // then the recommended command runs through the same gate. The input box is
        // never involved.
        app.askFollowUp({ tabId, command: s.command, label: s.label || t("reason.nextStep") });
    };

    return (
        <div className="rc-analysis">
            <div className="rc-analysis-head">
                <span>{t("reason.analysisTitle")}</span>
                {streaming && <span className="spinner small" />}
                {analysis.status === "error" && <span className="risk-pill level-blocked">{t("reason.failed")}</span>}
            </div>
            {steps.length > 0 ? (
                <StepList steps={steps} />
            ) : (
                <pre className="rc-stream">{analysis.text || (streaming ? t("reason.analyzing") : "")}</pre>
            )}
            {/* The analysis is its own request, so it gets its own retry: re-running
                the command would be a different thing entirely. */}
            {analysis.status === "error" && onRetry && (
                <div className="rc-actions">
                    <button className="btn small" onClick={() => onRetry("analysis")}>
                        {t("reason.retryAnalysis")}
                    </button>
                </div>
            )}
            {suggestions.length > 0 && (
                <div className="rc-followups">
                    <div className="rc-followups-head">
                        <span>{t("reason.followups")}</span>
                        <span className="muted">{t("reason.followupsHint")}</span>
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

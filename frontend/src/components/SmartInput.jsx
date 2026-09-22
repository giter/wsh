import { useEffect, useMemo, useRef, useState } from "react";
import { rpc } from "../lib/rpc.js";
import { useApp } from "../state/store.jsx";
import { looksLikeNaturalLanguage } from "../lib/intent.js";
import { t, useT } from "../lib/i18n.js";

// SmartInput is the always-on input bar under the terminal. It carries both
// tracks: a shell command is sent to the remote PTY, natural language is turned
// into a command first, and every submission is classified by the local safety
// engine before anything is written (see PLAN.md M3/M5).
//
// It is only an *input*: a natural-language turn is handed to the reasoning pane
// (question card + answer card) instead of being previewed here, so a command is
// never shown twice and the whole conversation lives in one place.
export default function SmartInput({ tabId, sessionId, title, altScreen, onFocusTerminal }) {
    const app = useApp();
    const t = useT();
    const [value, setValue] = useState("");
    const [risk, setRisk] = useState(null);
    const [mode, setMode] = useState("auto"); // auto | shell | ai
    const [note, setNote] = useState("");
    const [busy, setBusy] = useState(false);
    // origin records that the current text was filled from a card ("填入"), so
    // submitting it unchanged still attributes the command's output back to that
    // card. Editing the text drops the link.
    const [origin, setOrigin] = useState(null); // { cardId, text }
    // The AI turn currently streaming, if any. Esc abandons it whether or not this
    // keystroke also runs the result, so a slow model can always be given up on.
    const activeReqRef = useRef(null);
    const inputRef = useRef(null);
    const disposedRef = useRef(false);

    useEffect(() => () => {
        disposedRef.current = true;
    }, []);

    const natural = useMemo(() => looksLikeNaturalLanguage(value, risk), [value, risk]);
    const level = risk?.level || "safe";
    const aiReady = !!app.aiStatus?.configured;
    // Whether a command is still running on this tab: Esc then means "interrupt
    // the terminal" rather than "clear the box".
    const running = (app.reason || []).some(
        (r) => r.tabId === tabId && (r.exec?.status === "running" || r.exec?.status === "streaming"),
    );

    // route is the single place that decides which track the current text takes,
    // so the hint line and the Enter handler can never disagree about it. Note
    // that it only picks *how the text is read*: whether anything is actually
    // executed (and whether it needs confirmation) is decided by the local safety
    // engine on the backend, never here.
    const route = useMemo(() => {
        if (!aiReady || mode === "shell") return "shell";
        if (mode === "ai") return "ai";
        return natural ? "ai" : "shell";
    }, [aiReady, mode, natural]);

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

    // A reasoning card drops text here: a command to review ("填入"). Keeping the
    // card id is what closes the loop when the command runs.
    useEffect(() => {
        if (app.draft && app.draft.tabId === tabId) {
            const text = app.draft.text || "";
            setValue(text);
            setOrigin(app.draft.cardId ? { cardId: app.draft.cardId, text: text.trim() } : null);
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

    // trackFor returns the card whose command this text is, so its output can be
    // attributed back to it. It returns "" once the text is edited by hand.
    const trackFor = (text) => (origin && origin.text === String(text).trim() ? origin.cardId : "");

    // execute submits a command through the shared safety gate (see store.jsx),
    // which is also what the reasoning cards' "立即执行" button uses.
    const execute = async (cmd, cardId) => {
        const text = String(cmd || "").trim();
        if (!text) return { status: "empty" };
        setNote("");
        const res = await app.runCommand({
            tabId,
            sessionId,
            command: text,
            trackCardId: cardId || "",
            onWritten: () => {
                setValue("");
                setRisk(null);
                setOrigin(null);
            },
        });
        if (res.status === "blocked" || res.status === "error" || res.status === "nosession") {
            setNote(res.message || "");
        }
        return res;
    };

    // ask hands a natural-language turn to the reasoning pane and clears the box:
    // the question and the answer both belong on the right. With run=true
    // (Ctrl+Enter) it waits for the generated command and runs it in the same
    // keystroke, still through the safety gate.
    const ask = ({ run = false } = {}) => {
        const prompt = value.trim();
        if (!prompt) return;
        if (!aiReady) {
            // No model configured: the text goes to the shell verbatim, which is
            // what this bar did before AI existed.
            execute(value, trackFor(value)).then((res) => {
                if (res?.status === "written") setNote(t("smart.noAIExecuted"));
            });
            return;
        }
        setNote("");
        setValue("");
        setRisk(null);
        setOrigin(null);

        const pending = app.askCommand({ tabId, sessionId, prompt }).catch((e) => {
            if (!disposedRef.current) setNote(e.message || String(e));
            return null;
        });
        // Remember the in-flight request (and forget it when its turn ends), so Esc
        // can abandon a generation that is taking too long.
        pending.then((req) => {
            if (!req || disposedRef.current) return null;
            activeReqRef.current = req.requestId;
            return waitForAI(req.requestId).then(() => {
                if (activeReqRef.current === req.requestId) activeReqRef.current = null;
            });
        });
        if (!run) return;

        setBusy(true);
        pending
            .then(async (req) => {
                if (!req) return;
                const done = await waitForAI(req.requestId);
                if (disposedRef.current) return;
                if (!done || !done.ok) {
                    setNote(done?.error || t("smart.aiNoResult"));
                    return;
                }
                if (!done.command) {
                    setNote(t("smart.aiNoCommand"));
                    return;
                }
                await execute(done.command, req.cardId);
            })
            .catch(() => {
                /* execute reports its own failures through the status line */
            })
            .finally(() => {
                if (!disposedRef.current) setBusy(false);
            });
    };

    const onKeyDown = (e) => {
        if (e.key === "Enter") {
            e.preventDefault();
            if (busy) return;
            // Ctrl/Cmd+Enter is the one-keystroke path: run what is here now, or
            // run what the model is about to produce, without a review step.
            const force = e.ctrlKey || e.metaKey;
            if (level === "blocked" && !force) {
                setNote(risk?.reason || t("smart.blockedDisabled"));
                return;
            }
            const useAI = route === "ai";
            if (useAI) {
                ask({ run: force });
                return;
            }
            execute(value, trackFor(value));
            return;
        }
        if (e.key === "Tab") {
            // Tab pulls the newest AI-proposed command into the box for editing, so
            // the hand never has to travel to the reasoning pane. With nothing to
            // pull, Tab keeps its default behaviour.
            const next = app.latestAICommand(tabId);
            if (!next || !next.command) return;
            e.preventDefault();
            setValue(next.command);
            setOrigin({ cardId: next.cardId, text: next.command.trim() });
            setNote("");
            return;
        }
        if (e.key === "Escape") {
            // Esc is the single "make it stop" key, in priority order: abandon the
            // model, then interrupt a command that will not end on its own, and only
            // then clear the box.
            const rid = activeReqRef.current;
            if (rid) {
                app.cancelAI(rid);
                activeReqRef.current = null;
                setBusy(false);
                setNote(t("smart.aiInterrupted"));
                return;
            }
            if (busy) {
                setBusy(false);
                setNote(t("smart.aiInterrupted"));
                return;
            }
            if (running && value.trim() === "") {
                app.interruptTab(tabId);
                setNote(t("smart.ctrlCSent"));
                return;
            }
            setValue("");
            setRisk(null);
            setOrigin(null);
            setNote("");
        }
    };

    if (altScreen) {
        return (
            <div id="smart-input" className="alt-passthrough">
                <span className="mode-pill">{t("smart.passthrough")}</span>
                <span className="muted">{t("smart.passthroughHint")}</span>
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
                    placeholder={aiReady ? t("smart.placeholder") : t("smart.placeholderNoAI")}
                    onChange={(e) => setValue(e.target.value)}
                    onKeyDown={onKeyDown}
                    spellCheck={false}
                    autoComplete="off"
                />
                <select
                    className="si-mode"
                    value={mode}
                    onChange={(e) => setMode(e.target.value)}
                    title={t("smart.modeTip")}
                >
                    <option value="auto">{t("smart.modeAuto")}</option>
                    <option value="shell">{t("smart.modeShell")}</option>
                    <option value="ai">{t("smart.modeAI")}</option>
                </select>
                <span className={"si-lock level-" + level} title={t("smart.astEngine")}>
                    {level === "safe" ? "🔒 AST" : level === "caution" ? "⚠ AST" : "⛔ AST"}
                </span>
                {busy && (
                    <span className="si-thinking" title={t("smart.escToInterrupt")}>
                        <span className="spinner small" /> {t("smart.thinking")}
                    </span>
                )}
            </div>

            {/* The status line is always present, even when it is empty. The
                terminal above is sized in pixels, so a line that came and went
                would reflow it on every keystroke. */}
            <StatusLine value={value} risk={risk} level={level} route={route} aiReady={aiReady} note={note} running={running} />
        </div>
    );
}

// StatusLine is the single fixed-height line under the input. It states what will
// happen to what is typed — which track reads it, and how the local engine
// classifies it — or shows the keyboard hint when the box is empty.
function StatusLine({ value, risk, level, route, aiReady, note, running }) {
    if (note) {
        return <div className="si-status err">{note}</div>;
    }
    if (value.trim() === "") {
        if (running) {
            // A command is still producing output: the useful key is Esc, not
            // another command.
            return <div className="si-status level-caution">{t("smart.runningHint")}</div>;
        }
        return (
            <div className="si-status idle">
                {aiReady ? t("smart.hintAI") : t("smart.hintEnter")}
            </div>
        );
    }
    if (route === "ai") {
        return (
            <div className="si-status level-ai">
                {t("smart.willAsk")}
            </div>
        );
    }
    // The classification is debounced; keep the line blank for the moment it takes
    // rather than flickering between hints.
    if (!risk) {
        return <div className="si-status" />;
    }
    if (level === "blocked") {
        return (
            <div className="si-status level-blocked">
                {t("smart.blockedZone", { reason: risk.reason ? "：" + risk.reason : "" })}
            </div>
        );
    }
    if (level === "caution") {
        return (
            <div className="si-status level-caution">
                {t("smart.cautionZone", { reason: risk.reason ? "：" + risk.reason : "" })}
            </div>
        );
    }
    return <div className="si-status">{t("smart.safeZone")}</div>;
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

import { useEffect, useState } from "react";
import { useApp, applyZoom, zoomPercentFor, DEFAULT_FONT_SIZE, MIN_FONT_SIZE, MAX_FONT_SIZE } from "../state/store.jsx";
import { LANGUAGES, useT } from "../lib/i18n.js";

// AllowedCommands lists the commands the user approved for good from the dry-run
// panel. A permanent approval the user cannot see or undo would be a trap, so the
// list is shown with a way to revoke each entry (or all of them).
function AllowedCommands() {
    const app = useApp();
    const t = useT();
    const [err, setErr] = useState("");

    useEffect(() => {
        app.refreshAllowedCommands().catch(() => {});
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const revoke = (command, all) => {
        setErr("");
        app.revokeAllowed(command, all).catch((e) => setErr(e.message || String(e)));
    };

    return (
        <div className="allowed-cmds">
            <div className="allowed-cmds-head">
                <span>{t("settings.allow.title")}</span>
                {app.allowedCommands.length > 0 && (
                    <button className="btn small danger" onClick={() => revoke("", true)}>
                        {t("settings.allow.clearAll")}
                    </button>
                )}
            </div>
            {app.allowedCommands.length === 0 ? (
                <p className="muted">
                    {t("settings.allow.empty")}
                </p>
            ) : (
                <ul className="allowed-cmds-list">
                    {app.allowedCommands.map((c) => (
                        <li key={c}>
                            <code>{c}</code>
                            <button className="btn small" onClick={() => revoke(c, false)}>
                                {t("settings.allow.remove")}
                            </button>
                        </li>
                    ))}
                </ul>
            )}
            {err && <p className="err">{err}</p>}
        </div>
    );
}

function fromSettings(st) {
    return {
        fontSize: String(st.fontSize || 13),
        theme: st.theme === "light" ? "light" : "dark",
        defaultPort: String(st.defaultPort || 22),
        defaultUser: st.defaultUser || "",
        tunnelLocalPort: String(st.tunnelLocalPort || 8080),
        tunnelRemotePort: String(st.tunnelRemotePort || 80),

        aiProvider: st.aiProvider || "",
        aiBaseUrl: st.aiBaseUrl || "",
        aiModel: st.aiModel || "",
        aiKey: "", // never sent back by the backend
        aiAutoAnalyze: !!st.aiAutoAnalyze,
        aiNoContext: !!st.aiNoContext,
        // The backend exposes the positive form; the stored field is inverted.
        aiAutoRun: st.aiAutoRun !== false,
        language: st.language || "auto",
    };
}

export default function SettingsPage() {
    const app = useApp();
    const t = useT();
    const [form, setForm] = useState(() => fromSettings(app.settings));
    const [status, setStatus] = useState({ msg: "", err: false });

    // Live preview: the zoom applies as it is edited, the same way Ctrl +/-/
    // does.
    useEffect(() => {
        applyZoom(form.fontSize);
    }, [form.fontSize]);

    useEffect(() => {
        document.documentElement.dataset.theme = form.theme;
    }, [form.theme]);

    const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }));

    const save = async () => {
        setStatus({ msg: "", err: false });
        try {
            await app.saveSettings({
                fontSize: parseInt(form.fontSize, 10) || 0,
                theme: form.theme,
                defaultPort: parseInt(form.defaultPort, 10) || 0,
                defaultUser: form.defaultUser.trim(),
                tunnelLocalPort: parseInt(form.tunnelLocalPort, 10) || 0,
                tunnelRemotePort: parseInt(form.tunnelRemotePort, 10) || 0,

                aiProvider: form.aiProvider,
                aiBaseUrl: form.aiBaseUrl.trim(),
                aiModel: form.aiModel.trim(),
                // An empty key keeps the stored one; clearing is explicit.
                aiKey: form.aiKey,
                aiAutoAnalyze: form.aiAutoAnalyze,
                aiNoContext: form.aiNoContext,
                aiAutoRun: form.aiAutoRun,
                language: form.language || "auto",
            });
            setForm((f) => ({ ...f, aiKey: "" }));
            setStatus({ msg: t("settings.saved"), err: false });
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

    const clearKey = async () => {
        setStatus({ msg: "", err: false });
        try {
            await app.saveSettings({
                fontSize: parseInt(form.fontSize, 10) || 0,
                theme: form.theme,
                defaultPort: parseInt(form.defaultPort, 10) || 0,
                defaultUser: form.defaultUser.trim(),
                tunnelLocalPort: parseInt(form.tunnelLocalPort, 10) || 0,
                tunnelRemotePort: parseInt(form.tunnelRemotePort, 10) || 0,
                aiProvider: form.aiProvider,
                aiBaseUrl: form.aiBaseUrl.trim(),
                aiModel: form.aiModel.trim(),
                clearAiKey: true,
                aiAutoAnalyze: form.aiAutoAnalyze,
                aiNoContext: form.aiNoContext,
                aiAutoRun: form.aiAutoRun,
            });
            setForm((f) => ({ ...f, aiKey: "" }));
            setStatus({ msg: t("settings.keyCleared"), err: false });
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

    const setCheck = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.checked }));

    const reset = () => {
        const next = fromSettings(app.settings);
        setForm(next);
        document.documentElement.dataset.theme = next.theme;
        applyZoom(next.fontSize);
        setStatus({ msg: "", err: false });
    };

    return (
        <div className="config-page">
            <header className="config-head">
                <div className="config-head-icon">⚙</div>
                <div className="config-head-text">
                    <h2>{t("settings.title")}</h2>
                    <p>{t("settings.desc")}</p>
                </div>
            </header>

            <div className="config-body">
                <section className="card">
                    <h3>{t("settings.appearance")}</h3>
                    <div className="grid">
                        <label className="field">
                            <span>{t("settings.zoom")}</span>
                            <input
                                type="number"
                                min={MIN_FONT_SIZE}
                                max={MAX_FONT_SIZE}
                                value={form.fontSize}
                                onChange={set("fontSize")}
                            />
                            <p className="field-hint">{t("settings.zoomHint", { percent: zoomPercentFor(form.fontSize), base: DEFAULT_FONT_SIZE })}</p>
                        </label>
                        <label className="field">
                            <span>{t("settings.theme")}</span>
                            <select value={form.theme} onChange={set("theme")}>
                                <option value="dark">{t("settings.theme.dark")}</option>
                                <option value="light">{t("settings.theme.light")}</option>
                            </select>
                        </label>
                        <label className="field">
                            <span>{t("settings.language")}</span>
                            <select value={form.language} onChange={set("language")}>
                                {LANGUAGES.map((l) => (
                                    <option key={l.value} value={l.value}>
                                        {l.value === "auto" ? t("settings.language.auto") : l.label}
                                    </option>
                                ))}
                            </select>
                        </label>
                    </div>
                </section>

                <section className="card">
                    <h3>{t("settings.defaults")}</h3>
                    <div className="grid">
                        <label className="field">
                            <span>{t("settings.defaults.port")}</span>
                            <input type="number" min="1" max="65535" value={form.defaultPort} onChange={set("defaultPort")} />
                        </label>
                        <label className="field">
                            <span>{t("settings.defaults.user")}</span>
                            <input placeholder="root" value={form.defaultUser} onChange={set("defaultUser")} />
                        </label>
                    </div>
                </section>

                <section className="card">
                    <h3>{t("settings.tunnel")}</h3>
                    <div className="grid">
                        <label className="field">
                            <span>{t("settings.tunnel.localPort")}</span>
                            <input type="number" min="1" max="65535" value={form.tunnelLocalPort} onChange={set("tunnelLocalPort")} />
                        </label>
                        <label className="field">
                            <span>{t("settings.tunnel.remotePort")}</span>
                            <input type="number" min="1" max="65535" value={form.tunnelRemotePort} onChange={set("tunnelRemotePort")} />
                        </label>
                    </div>
                </section>
                <section className="card">
                    <h3>{t("settings.ai.title")}</h3>
                    <div className="grid">
                        <label className="field">
                            <span>{t("settings.ai.provider")}</span>
                            <select value={form.aiProvider} onChange={set("aiProvider")}>
                                <option value="">{t("settings.ai.providerOff")}</option>
                                <option value="openai">{t("settings.ai.providerOpenai")}</option>
                                <option value="ollama">{t("settings.ai.providerOllama")}</option>
                            </select>
                        </label>
                        <label className="field">
                            <span>{t("settings.ai.model")}</span>
                            <input
                                placeholder={form.aiProvider === "ollama" ? "qwen2.5:7b" : "gpt-4o-mini"}
                                value={form.aiModel}
                                onChange={set("aiModel")}
                            />
                        </label>
                        <label className="field">
                            <span>{t("settings.ai.baseUrl")}</span>
                            <input
                                placeholder={
                                    form.aiProvider === "ollama"
                                        ? "http://127.0.0.1:11434"
                                        : "https://api.openai.com/v1"
                                }
                                value={form.aiBaseUrl}
                                onChange={set("aiBaseUrl")}
                            />
                        </label>
                        <label className="field">
                            <span>{app.settings.hasAiKey ? t("settings.ai.apiKeySaved") : t("settings.ai.apiKey")}</span>
                            <input
                                type="password"
                                placeholder={app.settings.hasAiKey ? t("settings.ai.keyPlaceholderKeep") : t("settings.ai.keyPlaceholderEmpty")}
                                value={form.aiKey}
                                onChange={set("aiKey")}
                                autoComplete="off"
                            />
                        </label>
                    </div>

                    <div className="check-row">
                        <label>
                            <input type="checkbox" checked={form.aiAutoRun} onChange={setCheck("aiAutoRun")} />
                            {t("settings.ai.autoRun")}
                        </label>
                    </div>
                    <div className="check-row">
                        <label>
                            <input type="checkbox" checked={form.aiAutoAnalyze} onChange={setCheck("aiAutoAnalyze")} />
                            {t("settings.ai.autoAnalyze")}
                        </label>
                    </div>
                    <div className="check-row">
                        <label>
                            <input type="checkbox" checked={form.aiNoContext} onChange={setCheck("aiNoContext")} />
                            {t("settings.ai.noContext")}
                        </label>
                    </div>
                    <p className="muted" style={{ marginTop: 8, lineHeight: 1.7 }}>
                        {t("settings.ai.privacyNote")}
                    </p>
                    <p className="muted" style={{ marginTop: 6, lineHeight: 1.7 }}>
                        {t("settings.ai.autoRunNote")}
                    </p>
                    <AllowedCommands />
                    {app.settings.hasAiKey && (
                        <div style={{ marginTop: 8 }}>
                            <button className="btn small danger" onClick={clearKey}>
                                {t("settings.ai.clearKey")}
                            </button>
                        </div>
                    )}
                </section>
            </div>

            <footer className="config-foot">
                <div className={"status-msg" + (status.err ? " err" : "")}>{status.msg}</div>
                <div className="foot-actions">
                    <button className="btn" onClick={reset}>
                        {t("common.reset")}
                    </button>
                    <button className="btn primary" onClick={save}>
                        {t("common.save")}
                    </button>
                </div>
            </footer>
        </div>
    );
}

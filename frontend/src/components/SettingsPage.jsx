import { useEffect, useState } from "react";
import { useApp, applyZoom, zoomPercentFor, DEFAULT_FONT_SIZE, MIN_FONT_SIZE, MAX_FONT_SIZE } from "../state/store.jsx";

function fromSettings(st) {
    return {
        fontSize: String(st.fontSize || 13),
        theme: st.theme === "light" ? "light" : "dark",
        defaultPort: String(st.defaultPort || 22),
        defaultUser: st.defaultUser || "",
        tunnelLocalPort: String(st.tunnelLocalPort || 8080),
        tunnelRemotePort: String(st.tunnelRemotePort || 80),
    };
}

export default function SettingsPage() {
    const app = useApp();
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
            });
            setStatus({ msg: "已保存", err: false });
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

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
                    <h2>选项</h2>
                    <p>调整界面外观与新连接、隧道的默认值</p>
                </div>
            </header>

            <div className="config-body">
                <section className="card">
                    <h3>外观</h3>
                    <div className="grid">
                        <label className="field">
                            <span>界面缩放</span>
                            <input
                                type="number"
                                min={MIN_FONT_SIZE}
                                max={MAX_FONT_SIZE}
                                value={form.fontSize}
                                onChange={set("fontSize")}
                            />
                            <p className="field-hint">{zoomPercentFor(form.fontSize)}%（{DEFAULT_FONT_SIZE} = 100%）</p>
                        </label>
                        <label className="field">
                            <span>主题</span>
                            <select value={form.theme} onChange={set("theme")}>
                                <option value="dark">暗色</option>
                                <option value="light">亮色</option>
                            </select>
                        </label>
                    </div>
                </section>

                <section className="card">
                    <h3>新连接默认值</h3>
                    <div className="grid">
                        <label className="field">
                            <span>默认端口</span>
                            <input type="number" min="1" max="65535" value={form.defaultPort} onChange={set("defaultPort")} />
                        </label>
                        <label className="field">
                            <span>默认用户</span>
                            <input placeholder="root" value={form.defaultUser} onChange={set("defaultUser")} />
                        </label>
                    </div>
                </section>

                <section className="card">
                    <h3>隧道默认值</h3>
                    <div className="grid">
                        <label className="field">
                            <span>本地端口</span>
                            <input type="number" min="1" max="65535" value={form.tunnelLocalPort} onChange={set("tunnelLocalPort")} />
                        </label>
                        <label className="field">
                            <span>远程端口</span>
                            <input type="number" min="1" max="65535" value={form.tunnelRemotePort} onChange={set("tunnelRemotePort")} />
                        </label>
                    </div>
                </section>
            </div>

            <footer className="config-foot">
                <div className={"status-msg" + (status.err ? " err" : "")}>{status.msg}</div>
                <div className="foot-actions">
                    <button className="btn" onClick={reset}>
                        恢复
                    </button>
                    <button className="btn primary" onClick={save}>
                        保存
                    </button>
                </div>
            </footer>
        </div>
    );
}

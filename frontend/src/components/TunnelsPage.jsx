import { useCallback, useEffect, useRef, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";
import { useT, t } from "../lib/i18n.js";

export default function TunnelsPage() {
    const tr = useT();
    const app = useApp();
    const [tunnels, setTunnels] = useState([]);

    // Credentials typed at a connect prompt, per connection. A tunnel rides on a
    // saved connection, which may deliberately have no stored password; the
    // secret is remembered for the rest of this window's lifetime.
    const credsRef = useRef({});

    const refresh = useCallback(async () => {
        try {
            setTunnels(await rpc.call("tunnels.list"));
        } catch {
            /* keep the previous list */
        }
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    // startTunnel answers credential prompts: the backend returns {needPassword}
    // / {needPassphrase} when the connection has no usable credentials, so the
    // user is asked once instead of the tunnel failing outright.
    const startTunnel = (t) => {
        const creds = credsRef.current[t.connectionId] || {};
        return rpc.call("tunnels.start", { id: t.id, ...creds }).then((res) => {
            if (!res || (!res.needPassword && !res.needPassphrase)) return res;

            return new Promise((resolve, reject) => {
                const onSubmit = async (secret) => {
                    const next =
                        res.needPassphrase
                            ? { ...creds, keyPassphrase: secret }
                            : { ...creds, password: secret };
                    credsRef.current = { ...credsRef.current, [t.connectionId]: next };
                    try {
                        resolve(await startTunnel(t));
                    } catch (e) {
                        reject(e);
                    }
                };
                app.openDialog({
                    type: res.needPassphrase ? "passphrase" : "password",
                    message: res.message || (res.needPassphrase ? t("dialog.needPassphrase") : t("dialog.needPassword")),
                    onSubmit,
                    onCancel: () => reject(new Error(t("common.cancelled"))),
                });
            });
        });
    };

    const toggle = async (t) => {
        try {
            if (t.running) await rpc.call("tunnels.stop", { id: t.id });
            else await startTunnel(t);
            refresh();
        } catch (e) {
            app.openDialog({ type: "notice", title: tr("tunnels.opFailed"), message: e.message });
        }
    };

    const remove = (t) =>
        app.openDialog({
            type: "confirm",
            title: tr("tunnels.deleteTitle"),
            body: (
                <p>
                    {tr("tunnels.deleteConfirm", { name: t.name })}
                </p>
            ),
            confirmLabel: tr("tunnels.delete"),
            onConfirm: async () => {
                await rpc.call("tunnels.delete", { id: t.id });
                refresh();
            },
        });

    return (
        <div className="page" style={{ overflow: "auto" }}>
            <h2>{tr("tunnels.title")}</h2>
            <div className="toolbar">
                <button className="btn primary" onClick={() => app.openDialog({ type: "tunnel", tunnel: null, onSaved: refresh })}>
                    {tr("tunnels.new")}
                </button>
            </div>
            <div className="list">
                {tunnels.length === 0 ? (
                    <div className="empty">{tr("tunnels.empty")}</div>
                ) : (
                    tunnels.map((t) => (
                        <TunnelRow
                            key={t.id}
                            tunnel={t}
                            onToggle={() => toggle(t)}
                            onEdit={() => app.openDialog({ type: "tunnel", tunnel: t, onSaved: refresh })}
                            onDelete={() => remove(t)}
                        />
                    ))
                )}
            </div>
        </div>
    );
}

function TunnelRow({ tunnel: t, onToggle, onEdit, onDelete }) {
    const tr = useT();
    const dirLabel = t.direction === "remote" ? tr("tunnels.remoteFwd") : tr("tunnels.localFwd");
    const flow =
        t.direction === "remote"
            ? `${t.remoteAddress}:${t.remotePort} → ${t.localAddress}:${t.localPort}`
            : `${t.localAddress}:${t.localPort} → ${t.remoteAddress}:${t.remotePort}`;

    return (
        <div className="row">
            <span className="dot" style={{ background: t.running ? "var(--accent)" : "var(--faint)" }} />
            <div className="row-main">
                <div className="row-title">{flow}</div>
                <div className="row-sub">
                    {t.name} · {dirLabel} · {tr("tunnels.via", { conn: t.connectionName })}
                </div>
                {t.remark && <div className="row-sub remark">{t.remark}</div>}
            </div>
            <span className={"badge " + (t.running ? "on" : "off")}>{t.running ? tr("tunnels.active") : tr("tunnels.stopped")}</span>
            <div className="row-actions">
                <button className="btn small" onClick={onToggle}>
                    {t.running ? tr("tunnels.stop") : tr("tunnels.start")}
                </button>
                <button className="icon-btn" title={tr("tunnels.edit")} onClick={onEdit}>
                    ✎
                </button>
                <button className="icon-btn danger" title={tr("tunnels.delete")} onClick={onDelete}>
                    🗑
                </button>
            </div>
        </div>
    );
}

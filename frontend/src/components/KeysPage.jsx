import { useCallback, useEffect, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";
import { useT } from "../lib/i18n.js";

// copyText copies to the clipboard, falling back to a hidden textarea when the
// async Clipboard API is unavailable or denied inside the webview.
function copyText(text) {
    if (!text) return;
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).catch(() => fallbackCopy(text));
        return;
    }
    fallbackCopy(text);
}

function fallbackCopy(text) {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try {
        document.execCommand("copy");
    } catch {
        /* ignore */
    }
    document.body.removeChild(ta);
}

export default function KeysPage() {
    const tr = useT();
    const app = useApp();
    const [keys, setKeys] = useState([]);
    const [error, setError] = useState("");

    const refresh = useCallback(async () => {
        try {
            setKeys(await rpc.call("keys.list"));
            setError("");
        } catch (e) {
            setError(e.message);
        }
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const remove = (k) =>
        app.openDialog({
            type: "confirm",
            title: tr("keys.deleteTitle"),
            body: (
                <>
                    <p>
                        {tr("keys.deleteConfirm", { name: k.name })}
                    </p>
                    <p className="muted" style={{ marginTop: 6 }}>
                        {tr("keys.deleteWarning")}
                    </p>
                </>
            ),
            confirmLabel: tr("keys.confirmDelete"),
            onConfirm: async () => {
                await rpc.call("keys.delete", { id: k.id });
                refresh();
            },
        });

    return (
        <div className="config-page">
            <header className="config-head">
                <div className="config-head-icon">🔑</div>
                <div className="config-head-text">
                    <h2>{tr("keys.title")}</h2>
                    <p>{tr("keys.desc")}</p>
                </div>
                <div className="spacer" />
                <button
                    className="btn primary"
                    onClick={() => app.openDialog({ type: "key", key: null, onSaved: refresh })}
                >
                    {tr("keys.submitBtn")}
                </button>
            </header>

            <div className="config-body">
                <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                    {error ? (
                        <div className="empty-state err">{error}</div>
                    ) : keys.length === 0 ? (
                        <div className="empty-state">
                            <div className="es-icon">🔑</div>
                            <div className="es-title">{tr("keys.emptyTitle")}</div>
                            <div className="es-hint">{tr("keys.emptyHint")}</div>
                        </div>
                    ) : (
                        keys.map((k) => <KeyCard key={k.id} item={k} onEdit={() => app.openDialog({ type: "key", key: k, onSaved: refresh })} onDelete={() => remove(k)} />)
                    )}
                </div>
            </div>

            <footer className="config-foot">
                <div className="muted">{tr("keys.count", { count: keys.length })}</div>
            </footer>
        </div>
    );
}

function KeyCard({ item: k, onEdit, onDelete }) {
    const tr = useT();
    return (
        <div className="card key-card">
            <div className="key-card-head">
                <div className="key-name">
                    {k.name}
                    {k.hasPassphrase && <span className="badge on">{tr("keys.encrypted")}</span>}
                    {k.passphraseSaved && <span className="badge off">{tr("keys.passSaved")}</span>}
                </div>
                <div className="key-actions">
                    <button className="btn small" onClick={() => copyText(k.publicKey)}>
                        {tr("keys.copyPub")}
                    </button>
                    <button className="btn small" onClick={onEdit}>
                        {tr("keys.edit")}
                    </button>
                    <button className="btn small danger" onClick={onDelete}>
                        {tr("keys.delete")}
                    </button>
                </div>
            </div>
            <div className="key-meta">
                <span className="tag mono">{k.keyType || tr("keys.unknownType")}</span>
                <span className="mono key-fp">{k.fingerprint || ""}</span>
            </div>
            <div className="key-pub mono">{k.publicKey || tr("keys.noPub")}</div>
            {k.comment && <div className="key-comment">{k.comment}</div>}
        </div>
    );
}

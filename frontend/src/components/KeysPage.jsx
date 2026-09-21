import { useCallback, useEffect, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

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
            title: "删除密钥",
            body: (
                <>
                    <p>
                        删除密钥 <b>{k.name}</b>？
                    </p>
                    <p className="muted" style={{ marginTop: 6 }}>
                        使用该密钥的连接将不再使用它，改回密码或其他方式认证。
                    </p>
                </>
            ),
            confirmLabel: "删除",
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
                    <h2>密钥管理</h2>
                    <p>提交私钥，连接登录时优先使用密钥认证</p>
                </div>
                <div className="spacer" />
                <button
                    className="btn primary"
                    onClick={() => app.openDialog({ type: "key", key: null, onSaved: refresh })}
                >
                    ＋ 提交密钥
                </button>
            </header>

            <div className="config-body">
                <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                    {error ? (
                        <div className="empty-state err">{error}</div>
                    ) : keys.length === 0 ? (
                        <div className="empty-state">
                            <div className="es-icon">🔑</div>
                            <div className="es-title">还没有密钥</div>
                            <div className="es-hint">点击右上角「提交密钥」导入你的私钥</div>
                        </div>
                    ) : (
                        keys.map((k) => <KeyCard key={k.id} item={k} onEdit={() => app.openDialog({ type: "key", key: k, onSaved: refresh })} onDelete={() => remove(k)} />)
                    )}
                </div>
            </div>

            <footer className="config-foot">
                <div className="muted">共 {keys.length} 个密钥</div>
            </footer>
        </div>
    );
}

function KeyCard({ item: k, onEdit, onDelete }) {
    return (
        <div className="card key-card">
            <div className="key-card-head">
                <div className="key-name">
                    {k.name}
                    {k.hasPassphrase && <span className="badge on">已加密</span>}
                </div>
                <div className="key-actions">
                    <button className="btn small" onClick={() => copyText(k.publicKey)}>
                        复制公钥
                    </button>
                    <button className="btn small" onClick={onEdit}>
                        编辑
                    </button>
                    <button className="btn small danger" onClick={onDelete}>
                        删除
                    </button>
                </div>
            </div>
            <div className="key-meta">
                <span className="tag mono">{k.keyType || "未知类型"}</span>
                <span className="mono key-fp">{k.fingerprint || ""}</span>
            </div>
            <div className="key-pub mono">{k.publicKey || "（无公钥）"}</div>
            {k.comment && <div className="key-comment">{k.comment}</div>}
        </div>
    );
}

import { useCallback, useEffect, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

export default function TunnelsPage() {
    const app = useApp();
    const [tunnels, setTunnels] = useState([]);

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

    const toggle = async (t) => {
        try {
            if (t.running) await rpc.call("tunnels.stop", { id: t.id });
            else await rpc.call("tunnels.start", { id: t.id });
            refresh();
        } catch (e) {
            app.openDialog({ type: "notice", title: "隧道操作失败", message: e.message });
        }
    };

    const remove = (t) =>
        app.openDialog({
            type: "confirm",
            title: "删除隧道",
            body: (
                <p>
                    删除隧道 <b>{t.name}</b>？
                </p>
            ),
            confirmLabel: "删除",
            onConfirm: async () => {
                await rpc.call("tunnels.delete", { id: t.id });
                refresh();
            },
        });

    return (
        <div className="page" style={{ overflow: "auto" }}>
            <h2>端口隧道</h2>
            <div className="toolbar">
                <button className="btn primary" onClick={() => app.openDialog({ type: "tunnel", tunnel: null, onSaved: refresh })}>
                    ＋ 新建隧道
                </button>
            </div>
            <div className="list">
                {tunnels.length === 0 ? (
                    <div className="empty">还没有隧道。点击「新建隧道」创建端口转发规则。</div>
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
    const dirLabel = t.direction === "remote" ? "远程转发" : "本地转发";
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
                    {t.name} · {dirLabel} · 通过 {t.connectionName}
                </div>
                {t.remark && <div className="row-sub remark">{t.remark}</div>}
            </div>
            <span className={"badge " + (t.running ? "on" : "off")}>{t.running ? "转发中" : "已停止"}</span>
            <div className="row-actions">
                <button className="btn small" onClick={onToggle}>
                    {t.running ? "停止" : "启动"}
                </button>
                <button className="icon-btn" title="编辑" onClick={onEdit}>
                    ✎
                </button>
                <button className="icon-btn danger" title="删除" onClick={onDelete}>
                    🗑
                </button>
            </div>
        </div>
    );
}

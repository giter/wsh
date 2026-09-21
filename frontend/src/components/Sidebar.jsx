import { useEffect, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

// Sidebar is the session manager (Xshell style): a folder/connection tree plus a
// properties pane for the selected connection. Connection management lives here
// instead of in its own tab, so the main area holds only session tabs.
export default function Sidebar() {
    const app = useApp();
    const [openFolders, setOpenFolders] = useState({});
    const [selectedId, setSelectedId] = useState(null);

    const selected = app.connections.find((c) => c.id === selectedId) || null;

    // Drop the selection if the connection disappears (deleted / reloaded).
    useEffect(() => {
        if (selectedId && !app.connections.some((c) => c.id === selectedId)) setSelectedId(null);
    }, [app.connections, selectedId]);

    const toggleFolder = (key) => setOpenFolders((s) => ({ ...s, [key]: !s[key] }));

    const newFolder = () =>
        app.openDialog({
            type: "prompt",
            title: "新建文件夹",
            label: "文件夹名称",
            onSubmit: async (name) => {
                await rpc.call("folders.save", { name });
                await app.refreshConnections();
            },
        });

    const renameFolder = (f) =>
        app.openDialog({
            type: "prompt",
            title: "重命名文件夹",
            label: "文件夹名称",
            initial: f.name,
            onSubmit: async (name) => {
                await rpc.call("folders.save", { id: f.id, name });
                await app.refreshConnections();
            },
        });

    const deleteFolder = (f) =>
        app.openDialog({
            type: "confirm",
            title: "删除文件夹",
            body: (
                <>
                    <p>
                        删除文件夹 <b>{f.name}</b>？
                    </p>
                    <p className="muted" style={{ marginTop: 6 }}>
                        文件夹内的连接不会被删除，只会移回「未分组」。
                    </p>
                </>
            ),
            confirmLabel: "删除",
            onConfirm: async () => {
                await rpc.call("folders.delete", { id: f.id });
                await app.refreshConnections();
            },
        });

    const deleteConn = (c) =>
        app.openDialog({
            type: "confirm",
            title: "删除连接",
            body: (
                <>
                    <p>
                        删除连接 <b>{c.name}</b>？
                    </p>
                    <p className="muted" style={{ marginTop: 6 }}>
                        该操作无法撤销，其依赖的隧道也会一并移除。
                    </p>
                </>
            ),
            confirmLabel: "删除",
            onConfirm: async () => {
                await rpc.call("connections.delete", { id: c.id });
                setSelectedId(null);
                await app.refreshConnections();
                app.closeTab(c.id);
            },
        });

    const connRow = (c) => (
        <div
            key={c.id}
            className={"tree-row conn" + (c.id === selectedId ? " selected" : "")}
            title={`${c.user}@${c.host}:${c.port}`}
            onClick={() => setSelectedId(c.id)}
            onDoubleClick={() => app.openTerminal(c)}
        >
            <span className="dot" style={{ background: c.color || "#34d399" }} />
            <span className="lbl">{c.name}</span>
        </div>
    );

    const folderBranch = (key, label, conns, folder) => {
        // Folders start expanded so saved connections are visible immediately.
        const open = openFolders[key] !== false;
        return (
            <div className="tree-node" key={key}>
                <div className="tree-row branch" onClick={() => toggleFolder(key)}>
                    <span className={"caret" + (open ? " open" : "")}>▶</span>
                    <span className="lbl">{label}</span>
                    {folder && (
                        <span className="row-actions-inline">
                            <button
                                title="重命名"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    renameFolder(folder);
                                }}
                            >
                                ✎
                            </button>
                            <button
                                className="danger"
                                title="删除"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    deleteFolder(folder);
                                }}
                            >
                                🗑
                            </button>
                        </span>
                    )}
                </div>
                {open && (
                    <div className="tree-children">
                        {conns.length ? conns.map(connRow) : <div className="tree-empty">（空）</div>}
                    </div>
                )}
            </div>
        );
    };

    const ungrouped = app.connections.filter((c) => !c.folderId);
    const hasAny = app.connections.length > 0 || app.folders.length > 0;

    return (
        <aside id="sidebar">
            <div id="sidebar-header">
                <span className="logo-dot" />
                <span className="logo-text">会话管理器</span>
            </div>

            <div className="sidebar-toolbar">
                <button className="side-mini primary" onClick={() => app.openDialog({ type: "connection", conn: null })}>
                    ＋ 新建
                </button>
                <button className="side-mini" onClick={newFolder}>
                    ＋ 文件夹
                </button>
            </div>

            <nav id="tree">
                {!hasAny && <div className="tree-empty">还没有连接，点击「＋ 新建」开始</div>}
                {app.folders.map((f) => folderBranch(f.id, f.name, app.connections.filter((c) => c.folderId === f.id), f))}
                {ungrouped.length > 0 && folderBranch("__none", "未分组", ungrouped, null)}

                <div className="tree-sep" />
                <div className="tree-row" onClick={() => app.openPage("sftp")}>
                    <span className="caret" />
                    <span className="lbl">📁 文件传输</span>
                </div>
                <div className="tree-row" onClick={() => app.openPage("tunnels")}>
                    <span className="caret" />
                    <span className="lbl">🔗 端口隧道</span>
                </div>
            </nav>

            {selected && (
                <div id="sidebar-details">
                    <div className="det-title">属性</div>
                    <DetailRow k="名称" v={selected.name} />
                    <DetailRow k="主机" v={selected.host} />
                    <DetailRow k="端口" v={String(selected.port)} />
                    <DetailRow k="用户" v={selected.user} />
                    <DetailRow k="认证" v={authLabel(selected, app.keys)} />
                    <DetailRow k="文件夹" v={folderName(selected, app.folders)} />
                    <div className="det-actions">
                        <button className="btn primary small" onClick={() => app.openTerminal(selected)}>
                            连接
                        </button>
                        <button className="btn small" onClick={() => app.openDialog({ type: "connection", conn: selected })}>
                            编辑
                        </button>
                        <button className="btn small danger" onClick={() => deleteConn(selected)}>
                            删除
                        </button>
                    </div>
                </div>
            )}
        </aside>
    );
}

function DetailRow({ k, v }) {
    return (
        <div className="det-row">
            <span className="det-k">{k}</span>
            <span className="det-v" title={v}>
                {v || "-"}
            </span>
        </div>
    );
}

function authLabel(conn, keys) {
    if (conn.keyId) {
        const key = keys.find((k) => k.id === conn.keyId);
        return key ? `密钥 · ${key.name}` : "密钥";
    }
    if (conn.hasPassword) return "密码";
    if (conn.privateKeyPath) return "私钥文件";
    return "无";
}

function folderName(conn, folders) {
    if (!conn.folderId) return "未分组";
    const f = folders.find((x) => x.id === conn.folderId);
    return f ? f.name : "未分组";
}

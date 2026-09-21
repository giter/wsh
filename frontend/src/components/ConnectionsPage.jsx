import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

export default function ConnectionsPage() {
    const app = useApp();
    const { connections, folders, refreshConnections, openDialog, openTerminal } = app;

    const addFolder = () =>
        openDialog({
            type: "prompt",
            title: "新建文件夹",
            label: "文件夹名称",
            onSubmit: async (name) => {
                await rpc.call("folders.save", { name });
                refreshConnections();
            },
        });

    const renameFolder = (f) =>
        openDialog({
            type: "prompt",
            title: "重命名文件夹",
            label: "文件夹名称",
            initial: f.name,
            onSubmit: async (name) => {
                await rpc.call("folders.save", { id: f.id, name });
                refreshConnections();
            },
        });

    const deleteFolder = (f) =>
        openDialog({
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
                refreshConnections();
            },
        });

    const deleteConn = (c) =>
        openDialog({
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
                await refreshConnections();
                app.closeTab(c.id);
            },
        });

    const byFolder = (fid) => connections.filter((c) => (c.folderId || "") === (fid || ""));
    const ungrouped = byFolder("");

    const section = (folder, conns) => (
        <div className="conn-folder" key={folder ? folder.id : "__none"}>
            <div className="folder-head">
                <span className="folder-title">{folder ? folder.name : "未分组"}</span>
                {folder && (
                    <>
                        <button className="btn small" onClick={() => renameFolder(folder)}>
                            重命名
                        </button>
                        <button className="btn small danger" onClick={() => deleteFolder(folder)}>
                            删除
                        </button>
                    </>
                )}
            </div>
            {conns.length === 0 ? (
                <div className="empty">（空）</div>
            ) : (
                conns.map((c) => (
                    <ConnRow
                        key={c.id}
                        conn={c}
                        onConnect={() => openTerminal(c)}
                        onMove={() => openDialog({ type: "moveConn", conn: c })}
                        onEdit={() => openDialog({ type: "connection", conn: c })}
                        onDelete={() => deleteConn(c)}
                    />
                ))
            )}
        </div>
    );

    return (
        <div className="page" style={{ overflow: "auto" }}>
            <h2>连接管理</h2>
            <div className="toolbar">
                <button className="btn primary" onClick={() => openDialog({ type: "connection", conn: null })}>
                    ＋ 新建连接
                </button>
                <button className="btn" onClick={addFolder}>
                    ＋ 新建文件夹
                </button>
                <span className="muted">
                    {connections.length} 台服务器 · {folders.length} 个文件夹
                </span>
            </div>
            <div className="list">
                {connections.length === 0 && folders.length === 0 ? (
                    <div className="empty">还没有保存的连接。点击「新建连接」开始。</div>
                ) : (
                    <>
                        {folders.map((f) => section(f, byFolder(f.id)))}
                        {(ungrouped.length > 0 || folders.length === 0) && section(null, ungrouped)}
                    </>
                )}
            </div>
        </div>
    );
}

function ConnRow({ conn, onConnect, onMove, onEdit, onDelete }) {
    return (
        <div className="row">
            <span className="dot" style={{ background: conn.color || "#34d399" }} />
            <div className="row-main">
                <div className="row-title">{conn.name}</div>
                <div className="row-sub">
                    {conn.user}@{conn.host}:{conn.port}
                </div>
            </div>
            <div className="row-actions">
                <button className="btn small" onClick={onConnect}>
                    连接
                </button>
                <button className="btn small" onClick={onMove}>
                    移动
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

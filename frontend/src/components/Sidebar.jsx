import { useEffect, useRef, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

// UNGROUPED is the pseudo folder key for connections with no folder. It is not a
// real folder ID; dropping there clears the connection's folderId.
const UNGROUPED = "__none";

// Sidebar is the session manager (Xshell style): a folder/connection tree plus a
// properties pane for the selected connection. It is docked on the left of the
// session window and can be collapsed with its ✕ button (Ctrl+1 brings it back).
// Opening a connection adds a terminal tab to the same window.
//
// The tree supports drag-and-drop: connections can be dragged into a folder, out
// to 未分组, or reordered within a group, and folders can be reordered. Every
// drop is persisted immediately, so the arrangement survives a restart.
export default function Sidebar() {
    const app = useApp();
    const [openFolders, setOpenFolders] = useState({});
    const [selectedId, setSelectedId] = useState(null);
    // What is being dragged, and where it would land. Kept in a ref for the
    // payload (read from drag handlers) and in state for the drop indicator.
    const dragRef = useRef(null);
    const [drop, setDrop] = useState(null); // {key, position: "into"|"before"|"after"}

    const selected = app.connections.find((c) => c.id === selectedId) || null;

    // Drop the selection if the connection disappears (deleted / reloaded).
    useEffect(() => {
        if (selectedId && !app.connections.some((c) => c.id === selectedId)) setSelectedId(null);
    }, [app.connections, selectedId]);

    const toggleFolder = (key) => setOpenFolders((s) => ({ ...s, [key]: !s[key] }));

    // A folder starts expanded, so saved connections are visible immediately.
    const isOpen = (key) => openFolders[key] !== false;

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
            },
        });

    // ---- Drag and drop ----

    // connsOf returns the connections of one group in display order.
    const connsOf = (key) => app.connections.filter((c) => (c.folderId || UNGROUPED) === key);

    const startDrag = (e, payload) => {
        dragRef.current = payload;
        e.dataTransfer.effectAllowed = "move";
        // Some webviews only start a drag when data is set.
        e.dataTransfer.setData("text/plain", payload.type === "conn" ? payload.id : payload.key);
    };

    const endDrag = () => {
        dragRef.current = null;
        setDrop(null);
    };

    // dropZone reports whether the pointer is in the upper or lower half of a
    // row, i.e. whether the dragged item should land before or after it.
    const dropZone = (e) => {
        const rect = e.currentTarget.getBoundingClientRect();
        return (e.clientY - rect.top) / Math.max(1, rect.height) < 0.5 ? "before" : "after";
    };

    // onConnDragOver proposes a drop for a connection row: reorder within the
    // row's group by inserting before / after it.
    const onConnDragOver = (e, key) => {
        const drag = dragRef.current;
        if (!drag || drag.type !== "conn") return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        setDrop({ key, position: dropZone(e) });
    };

    // onFolderDragOver proposes a drop for a folder header: a dragged connection
    // lands inside the folder, a dragged folder is inserted before / after it.
    const onFolderDragOver = (e, key) => {
        const drag = dragRef.current;
        if (!drag) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        setDrop({ key, position: drag.type === "conn" ? "into" : dropZone(e) });
    };

    const dropOnFolder = async (e, key) => {
        e.preventDefault();
        e.stopPropagation();
        const drag = dragRef.current;
        const target = drop;
        endDrag();
        if (!drag) return;

        if (drag.type === "conn") {
            // A connection cannot sit between two folders, so dropping it on a
            // folder row (anywhere) means "move it into this folder".
            await moveConnection(drag.id, key, -1);
            return;
        }

        // Reordering folders: insert before / after the hovered one.
        await insertFolder(drag.id, key, target?.position === "after" ? 1 : 0);
    };

    // insertFolder moves a folder next to another one, offset 0 = before it and
    // 1 = after it.
    const insertFolder = async (id, beforeKey, offset) => {
        const ids = app.folders.map((f) => f.id).filter((x) => x !== id);
        const at = ids.indexOf(beforeKey);
        if (at < 0) return;
        ids.splice(at + offset, 0, id);
        await rpc.call("folders.reorder", { ids });
        await app.refreshConnections();
    };

    // moveFolderToEnd reorders a dragged folder to the end of the folder list.
    const moveFolderToEnd = async (id) => {
        const ids = app.folders.map((f) => f.id).filter((x) => x !== id);
        ids.push(id);
        await rpc.call("folders.reorder", { ids });
        await app.refreshConnections();
    };

    // moveConnection persists a move and refreshes; index -1 appends.
    const moveConnection = async (id, key, index) => {
        const folderId = key === UNGROUPED ? "" : key;
        try {
            await rpc.call("connections.move", { id, folderId, index });
        } catch (err) {
            app.openDialog({ type: "notice", title: "移动失败", message: err.message });
        }
        await app.refreshConnections();
    };

    // dropOnConn inserts the dragged connection next to the hovered one.
    const dropOnConn = async (e, target) => {
        e.preventDefault();
        e.stopPropagation();
        const drag = dragRef.current;
        const zone = drop;
        endDrag();
        if (!drag) return;

        if (drag.type === "folder") {
            // Dropping a folder onto a connection row reorders it relative to the
            // folder that row belongs to, which is what the user sees.
            const targetFolder = app.connections.find((c) => c.id === target.id)?.folderId;
            if (targetFolder) await insertFolder(drag.id, targetFolder, zone?.position === "after" ? 1 : 0);
            return;
        }
        if (drag.id === target.id) return;

        const key = target.folderId || UNGROUPED;
        const ids = connsOf(key)
            .map((c) => c.id)
            .filter((id) => id !== drag.id);
        const at = ids.indexOf(target.id);
        if (at < 0) return;
        const index = zone?.position === "after" ? at + 1 : at;

        // A move inside the same group is a plain reorder; a cross-group move
        // carries the index into the destination group.
        if (drag.key === key) {
            ids.splice(index, 0, drag.id);
            try {
                await rpc.call("connections.reorder", { folderId: key === UNGROUPED ? "" : key, ids });
            } catch (err) {
                app.openDialog({ type: "notice", title: "排序失败", message: err.message });
            }
            await app.refreshConnections();
            return;
        }
        await moveConnection(drag.id, key, index);
    };

    // dropInGroup appends the dragged connection to the end of a group. Folders
    // dropped in a group's empty space are reordered to the end of the list.
    const dropInGroup = async (e, key) => {
        const drag = dragRef.current;
        endDrag();
        if (!drag) return;
        e.preventDefault();
        e.stopPropagation();
        if (drag.type === "conn") {
            await moveConnection(drag.id, key, -1);
            return;
        }
        await moveFolderToEnd(drag.id);
    };

    const dropClass = (key) => {
        if (!drop || drop.key !== key) return "";
        if (drop.position === "into") return " drop-into";
        return drop.position === "before" ? " drop-before" : " drop-after";
    };

    const connRow = (c, key) => (
        <div
            key={c.id}
            className={"tree-row conn" + (c.id === selectedId ? " selected" : "") + dropClass(c.id)}
            title={`${c.user}@${c.host}:${c.port}（可拖到文件夹）`}
            draggable
            onDragStart={(e) => startDrag(e, { type: "conn", id: c.id, key })}
            onDragEnd={endDrag}
            onDragOver={(e) => onConnDragOver(e, c.id)}
            onDrop={(e) => dropOnConn(e, c)}
            onClick={() => setSelectedId(c.id)}
            onDoubleClick={() => app.openSession({ connId: c.id })}
        >
            <span className="dot" style={{ background: c.color || "#34d399" }} />
            <span className="lbl">{c.name}</span>
        </div>
    );

    const folderBranch = (key, label, folder) => {
        const conns = connsOf(key);
        const open = isOpen(key);
        const isUngrouped = !folder;
        return (
            <div className="tree-node" key={key}>
                <div
                    className={"tree-row branch" + dropClass(key) + (isUngrouped ? " ungrouped" : "")}
                    onClick={() => toggleFolder(key)}
                    draggable={!isUngrouped}
                    onDragStart={isUngrouped ? undefined : (e) => startDrag(e, { type: "folder", id: folder.id, key })}
                    onDragEnd={endDrag}
                    onDragOver={(e) => onFolderDragOver(e, key)}
                    onDrop={(e) => dropOnFolder(e, key)}
                >
                    <span className={"caret" + (open ? " open" : "")}>▶</span>
                    <span className="lbl">{label}</span>
                    {folder && (
                        // The action buttons must not start a folder drag, so the
                        // drag is suppressed while the pointer is on them.
                        <span className="row-actions-inline" onDragStart={(e) => e.preventDefault()}>
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
                    <div
                        className="tree-children"
                        onDragOver={(e) => {
                            // Let the empty space under the rows accept a drop so a
                            // connection can be appended to the group.
                            if (dragRef.current) {
                                e.preventDefault();
                                setDrop({ key, position: "into" });
                            }
                        }}
                        onDrop={(e) => dropInGroup(e, key)}
                    >
                        {conns.length ? conns.map((c) => connRow(c, key)) : <div className="tree-empty">（空）</div>}
                    </div>
                )}
            </div>
        );
    };

    const hasAny = app.connections.length > 0 || app.folders.length > 0;
    const ungroupedCount = app.connections.filter((c) => !c.folderId).length;

    return (
        <aside id="sidebar">
            <div id="sidebar-header">
                <span className="logo-dot" />
                <span className="logo-text">会话管理器</span>
                <button className="panel-collapse" title="收起会话管理器（Ctrl+1 再展开）" onClick={() => app.toggleSessionManager()}>
                    ✕
                </button>
            </div>

            <div className="sidebar-toolbar">
                <button className="side-mini primary" onClick={() => app.openDialog({ type: "connection", conn: null })}>
                    ＋ 新建
                </button>
                <button className="side-mini" onClick={newFolder}>
                    ＋ 文件夹
                </button>
            </div>

            <nav
                id="tree"
                onDragOver={(e) => {
                    if (dragRef.current) e.preventDefault();
                }}
                onDrop={(e) => dropInGroup(e, UNGROUPED)}
            >
                {!hasAny && <div className="tree-empty">还没有连接，点击「＋ 新建」开始</div>}
                {app.folders.map((f) => folderBranch(f.id, f.name, f))}
                {/* 未分组 doubles as the drop target for "take out of the folder",
                    so it is shown whenever there is anything to organize. */}
                {(hasAny || ungroupedCount > 0) && folderBranch(UNGROUPED, "未分组", null)}

                <div className="tree-sep" />
                <div className="tree-row tool" onClick={() => app.appAction("sftp")} title="在新窗口中打开">
                    <span className="caret" />
                    <span className="lbl">📁 文件传输</span>
                    <span className="row-actions-inline always">↗</span>
                </div>
                <div className="tree-row tool" onClick={() => app.appAction("tunnels")} title="在新窗口中打开">
                    <span className="caret" />
                    <span className="lbl">🔗 端口隧道</span>
                    <span className="row-actions-inline always">↗</span>
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
                        <button className="btn primary small" onClick={() => app.openSession({ connId: selected.id })}>
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

import { useCallback, useEffect, useRef, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";
import ContextMenu from "./ContextMenu.jsx";

// UNGROUPED is the pseudo folder key for connections with no folder. It is not a
// real folder ID; dropping there clears the connection's folderId.
const UNGROUPED = "__none";

// ROOT is the pseudo key of the "所有会话" root node.
const ROOT = "__root";

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
    // Right-click menu: {x, y, items} of the open menu, or null. Creating,
    // renaming and deleting all live here instead of in a toolbar, so the tree
    // rows stay uncluttered.
    const [menu, setMenu] = useState(null);
    const closeMenu = useCallback(() => setMenu(null), []);
    // Search: a query narrows the tree to matching connections.
    const [search, setSearch] = useState("");
    const query = search.trim().toLowerCase();
    const searching = query.length > 0;

    const selected = app.connections.find((c) => c.id === selectedId) || null;

    // Drop the selection if the connection disappears (deleted / reloaded).
    useEffect(() => {
        if (selectedId && !app.connections.some((c) => c.id === selectedId)) setSelectedId(null);
    }, [app.connections, selectedId]);

    // toggleFolder flips the effective open state rather than the raw stored
    // value. An unset entry already means "expanded", so negating the raw value
    // would make the first click a no-op (undefined -> true is still expanded).
    const toggleFolder = (key) =>
        setOpenFolders((s) => {
            const open = s[key] !== false;
            return { ...s, [key]: !open };
        });

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

    // ---- Right-click menu ----

    const openMenu = (e, items) => {
        // Rows stop propagation so the menu for the row wins over the one for the
        // empty background behind it.
        e.preventDefault();
        e.stopPropagation();
        setMenu({ x: e.clientX, y: e.clientY, items });
    };

    // newConn opens the connection dialog with the target folder pre-selected;
    // an empty id means ungrouped.
    const newConn = (folderId) => app.openDialog({ type: "connection", conn: null, draft: { folderId: folderId || "" } });

    const connMenu = (c) => [
        { label: "连接", onClick: () => app.openSession({ connId: c.id }) },
        { label: "编辑", onClick: () => app.openDialog({ type: "connection", conn: c }) },
        { separator: true },
        { label: "删除", danger: true, onClick: () => deleteConn(c) },
    ];

    const folderMenu = (f) => [
        { label: "在此新建连接", onClick: () => newConn(f.id) },
        { separator: true },
        { label: "重命名", onClick: () => renameFolder(f) },
        { label: "删除", danger: true, onClick: () => deleteFolder(f) },
    ];

    // The root node, the ungrouped pseudo folder and the empty background all
    // offer the same "create here" actions.
    const createMenu = (folderId) => [
        { label: "新建连接", onClick: () => newConn(folderId) },
        { label: "新建文件夹", onClick: newFolder },
    ];

    // ---- Drag and drop ----

    // connsOf returns the connections of one group in display order.
    const connsOf = (key) => app.connections.filter((c) => (c.folderId || UNGROUPED) === key);

    // matchConn reports whether a connection is listed for the current query; an
    // empty query matches everything. Name, host and username are searched, which
    // is what a session list is looked up by.
    const matchConn = (c) =>
        !searching || [c.name, c.host, c.user].some((v) => String(v || "").toLowerCase().includes(query));

    // matchFolder reports whether a folder's own name matches.
    const matchFolder = (f) => f.name.toLowerCase().includes(query);

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
            onContextMenu={(e) => {
                // Right-clicking also selects, so the properties pane matches the
                // row the menu belongs to.
                setSelectedId(c.id);
                openMenu(e, connMenu(c));
            }}
        >
            <span className="node-icon conn-icon">
                <ServerIcon />
            </span>
            <span className="lbl">{c.name}</span>
        </div>
    );

    // folderBranch renders one folder (or the ungrouped pseudo folder) with its
    // connections nested underneath, Xshell style: a boxed expander, a folder
    // icon and a dashed guide line down the left of the children.
    const folderBranch = (key, label, folder) => {
        // A folder whose own name matched lists everything inside it, so the folder
        // stays useful to click; otherwise only the matching connections remain.
        const all = connsOf(key);
        const conns = searching && !(folder && matchFolder(folder)) ? all.filter(matchConn) : all;
        // While searching every surviving folder is expanded, so matches are
        // visible without opening folders by hand.
        const open = searching || isOpen(key);
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
                    onContextMenu={(e) => openMenu(e, folder ? folderMenu(folder) : createMenu(""))}
                >
                    <span className="expander">
                        <ExpanderIcon open={open} />
                    </span>
                    <span className="node-icon folder-icon">
                        <FolderIcon open={open} />
                    </span>
                    <span className="lbl">{label}</span>
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
    // While searching, a folder is kept when its name matches or it still holds a
    // matching connection, and the non-session rows (file transfer / tunnels) are
    // hidden so the list is nothing but results.
    const foldersShown = searching ? app.folders.filter((f) => matchFolder(f) || connsOf(f.id).some(matchConn)) : app.folders;
    const showUngrouped = searching ? connsOf(UNGROUPED).some(matchConn) : hasAny || ungroupedCount > 0;
    const noResults = searching && !foldersShown.length && !showUngrouped;
    const rootOpen = searching || isOpen(ROOT);

    return (
        <aside id="sidebar">
            <div id="sidebar-header">
                <span className="logo-dot" />
                <span className="logo-text">会话管理器</span>
                <button className="panel-collapse" title="收起会话管理器（Ctrl+1 再展开）" onClick={() => app.toggleSessionManager()}>
                    ✕
                </button>
            </div>

            <div className="sidebar-search">
                <input
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === "Escape") setSearch("");
                    }}
                    placeholder="搜索名称、主机、用户"
                    spellCheck={false}
                    aria-label="搜索会话"
                />
                {search !== "" && (
                    <button className="search-clear" title="清除搜索" onClick={() => setSearch("")}>
                        ✕
                    </button>
                )}
            </div>

            <nav
                id="tree"
                onDragOver={(e) => {
                    if (dragRef.current) e.preventDefault();
                }}
                onDrop={(e) => dropInGroup(e, UNGROUPED)}
                onContextMenu={(e) => openMenu(e, createMenu(""))}
            >
                {/* Root node, like Xshell's 所有会话: everything below is one level
                    in, under a dashed guide line. */}
                <div className="tree-row branch root" onClick={() => toggleFolder(ROOT)} onContextMenu={(e) => openMenu(e, createMenu(""))}>
                    <span className="expander">
                        <ExpanderIcon open={rootOpen} />
                    </span>
                    <span className="node-icon root-icon">
                        <FolderIcon open={rootOpen} />
                    </span>
                    <span className="lbl">所有会话</span>
                </div>

                {rootOpen && (
                    <div className="tree-children">
                        {!hasAny && !searching && <div className="tree-empty">还没有连接，在空白处右键新建</div>}
                        {noResults && <div className="tree-empty">没有匹配的会话</div>}
                        {foldersShown.map((f) => folderBranch(f.id, f.name, f))}
                        {/* 未分组 doubles as the drop target for "take out of the
                            folder", so it is shown whenever there is anything to
                            organize. */}
                        {showUngrouped && folderBranch(UNGROUPED, "未分组", null)}

                        {!searching && (
                            <>
                                <div className="tree-sep" />
                                <div className="tree-row tool" onClick={() => app.appAction("sftp")} title="在新窗口中打开">
                                    <span className="node-icon tool-icon">↗</span>
                                    <span className="lbl">文件传输</span>
                                </div>
                                <div className="tree-row tool" onClick={() => app.appAction("tunnels")} title="在新窗口中打开">
                                    <span className="node-icon tool-icon">↗</span>
                                    <span className="lbl">端口隧道</span>
                                </div>
                            </>
                        )}
                    </div>
                )}
            </nav>

            {selected && (
                <div id="sidebar-details">
                    <div className="det-head">
                        <span className="det-title">属性</span>
                        <button className="det-close" title="关闭属性" onClick={() => setSelectedId(null)}>
                            ✕
                        </button>
                    </div>
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
            {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={closeMenu} />}
        </aside>
    );
}

// ExpanderIcon is the boxed ⊞ / ⊟ control on a folder row. It is drawn as SVG
// rather than text because a font glyph at this size renders blurry.
//
// The geometry is snapped to whole device pixels so no stroke straddles a pixel
// boundary at 1x: the outline is 8x8 with its 1px stroke centred on 1.5 and 9.5,
// which fills columns 1..9 exactly and puts the box centre on the centre of
// column 5.5, so the arms can be centred there and still land on whole pixels.
// The viewBox is 11 so the 1px margins stay even on every side.
function ExpanderIcon({ open }) {
    return (
        <svg viewBox="0 0 11 11" width="11" height="11" aria-hidden="true">
            <rect x="1.5" y="1.5" width="8" height="8" rx="1.5" fill="none" stroke="currentColor" strokeWidth="1" />
            <line x1="3" y1="5.5" x2="8" y2="5.5" stroke="currentColor" strokeWidth="1" />
            {!open && <line x1="5.5" y1="3" x2="5.5" y2="8" stroke="currentColor" strokeWidth="1" />}
        </svg>
    );
}

// FolderIcon is the small folder glyph shown next to folders and the root node.
// Drawn inline (rather than an emoji) so it looks the same on every platform and
// can take the theme colours.
function FolderIcon({ open }) {
    return (
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            {open ? (
                <path
                    d="M1.5 3.5h4l1.4 1.6h7.6v7.4a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1z"
                    fill="#f6c445"
                    stroke="#c99a1e"
                    strokeWidth="0.8"
                    strokeLinejoin="round"
                />
            ) : (
                <path
                    d="M1.5 3.5h4.2l1.3 1.5h7.5v7.5a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1z"
                    fill="#f6c445"
                    stroke="#c99a1e"
                    strokeWidth="0.8"
                    strokeLinejoin="round"
                />
            )}
        </svg>
    );
}

// ServerIcon is the connection glyph, a small stack of server units.
function ServerIcon() {
    return (
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            <rect x="2" y="2.5" width="12" height="5" rx="1" fill="#e0524a" stroke="#b23b34" strokeWidth="0.8" />
            <rect x="2" y="8.5" width="12" height="5" rx="1" fill="#e0524a" stroke="#b23b34" strokeWidth="0.8" />
            <circle cx="4.2" cy="5" r="0.9" fill="#fff" />
            <circle cx="4.2" cy="11" r="0.9" fill="#fff" />
        </svg>
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

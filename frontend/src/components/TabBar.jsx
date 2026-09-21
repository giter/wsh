import { useApp } from "../state/store.jsx";
import { defaultConnName } from "../lib/format.js";

// A session tab's title can be renamed; the fixed utility pages cannot.
function isRenameable(t) {
    return t.kind === "terminal";
}

// isUnsavedSession reports whether a tab is an ad-hoc quick-connect session
// that has not been saved as a connection yet.
function isUnsavedSession(t) {
    return t.kind === "terminal" && !t.connId && !!t.host && !t.savedConnectionId;
}

export default function TabBar() {
    const app = useApp();
    const { tabs, activeTab, selectTab, closeTab, openDialog } = app;

    // rename edits the session title; for a saved connection the connection's
    // own name is updated too, so the sidebar and tab stay consistent. A quick
    // session that was saved already counts as a saved connection.
    const rename = (t) => {
        const connId = t.connId || t.savedConnectionId;
        const conn = connId ? app.connections.find((c) => c.id === connId) : null;
        openDialog({
            type: "prompt",
            title: conn ? "重命名连接" : "重命名标签页",
            label: conn ? "连接名称" : "标签页标题",
            initial: t.title,
            onSubmit: async (name) => {
                if (conn) await app.renameConnection(conn, name);
                else app.renameTab(t.id, name);
            },
        });
    };

    // saveSession turns a quick-connect session into a saved connection, with
    // the address pre-filled so only a name is left to confirm.
    const saveSession = (t) => {
        openDialog({
            type: "connection",
            conn: null,
            draft: { name: defaultConnName(t.user, t.host, t.port), host: t.host, port: t.port, user: t.user },
            onSaved: (saved) => app.markTabSaved(t.id, saved),
        });
    };

    return (
        <div id="tabbar">
            {tabs.map((t) => {
                const renameable = isRenameable(t);
                return (
                    <div
                        key={t.id}
                        className={"tab" + (t.id === activeTab ? " active" : "")}
                        onClick={() => selectTab(t.id)}
                        onDoubleClick={renameable ? () => rename(t) : undefined}
                    >
                        <span className="tab-title" title={renameable ? `${t.title}（双击重命名）` : t.title}>
                            {t.title}
                        </span>
                        {isUnsavedSession(t) && (
                            <button
                                className="tab-save"
                                title="保存为连接"
                                onDoubleClick={(e) => e.stopPropagation()}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    saveSession(t);
                                }}
                            >
                                💾
                            </button>
                        )}
                        <button
                            className="tab-close"
                            onClick={(e) => {
                                e.stopPropagation();
                                closeTab(t.id);
                            }}
                        >
                            ✕
                        </button>
                    </div>
                );
            })}
            <button className="tab-add" title="新建连接" onClick={() => openDialog({ type: "connection", conn: null })}>
                ＋
            </button>
        </div>
    );
}

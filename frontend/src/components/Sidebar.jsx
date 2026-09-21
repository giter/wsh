import { useState } from "react";
import { useApp } from "../state/store.jsx";

function TreeRow({ className, onClick, children, title }) {
    return (
        <div className={"tree-row " + (className || "")} onClick={onClick} title={title}>
            {children}
        </div>
    );
}

function TreeBranch({ label, open, onToggle, children }) {
    return (
        <div className="tree-node">
            <div className="tree-row branch" onClick={onToggle}>
                <span className={"caret" + (open ? " open" : "")}>▶</span>
                <span className="lbl">{label}</span>
            </div>
            <div className="tree-children" style={{ display: open ? "block" : "none" }}>
                {children}
            </div>
        </div>
    );
}

export default function Sidebar() {
    const app = useApp();
    const [connOpen, setConnOpen] = useState(true);
    const [openFolders, setOpenFolders] = useState({});

    const toggleFolder = (key) => setOpenFolders((s) => ({ ...s, [key]: !s[key] }));

    const connRow = (c) => (
        <TreeRow key={c.id} className="conn" title={`${c.user}@${c.host}:${c.port}`} onClick={() => app.openTerminal(c)}>
            <span className="dot" style={{ background: c.color || "#34d399" }} />
            <span className="lbl">{c.name}</span>
        </TreeRow>
    );

    const ungrouped = app.connections.filter((c) => !c.folderId);

    return (
        <aside id="sidebar">
            <div id="sidebar-header">
                <span className="logo-dot" />
                <span className="logo-text">WSH</span>
            </div>

            <nav id="tree">
                <TreeBranch label="连接" open={connOpen} onToggle={() => setConnOpen((v) => !v)}>
                    <TreeRow className="action" onClick={() => app.openDialog({ type: "connection", conn: null })}>
                        <span className="lbl">＋ 新建连接</span>
                    </TreeRow>
                    <TreeRow className="action" onClick={() => app.openPage("connections")}>
                        <span className="lbl">连接管理</span>
                    </TreeRow>
                    {app.folders.map((f) => (
                        <TreeBranch
                            key={f.id}
                            label={f.name}
                            open={!!openFolders[f.id]}
                            onToggle={() => toggleFolder(f.id)}
                        >
                            {app.connections.filter((c) => c.folderId === f.id).map(connRow)}
                        </TreeBranch>
                    ))}
                    {ungrouped.length > 0 && (
                        <TreeBranch label="未分组" open={!!openFolders.__none} onToggle={() => toggleFolder("__none")}>
                            {ungrouped.map(connRow)}
                        </TreeBranch>
                    )}
                </TreeBranch>

                <TreeRow onClick={() => app.openPage("sftp")}>
                    <span className="caret" />
                    <span className="lbl">📁 文件传输</span>
                </TreeRow>
                <TreeRow onClick={() => app.openPage("tunnels")}>
                    <span className="caret" />
                    <span className="lbl">🔗 端口隧道</span>
                </TreeRow>
            </nav>

            <div id="sidebar-footer">
                <button className="side-btn" title="新建连接" onClick={() => app.openDialog({ type: "connection", conn: null })}>
                    ＋ 新建连接
                </button>
            </div>
        </aside>
    );
}

import { useCallback, useEffect, useRef, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";
import { base64Decode, base64Encode, dirOf, fmtSize, joinLocal, triggerDownload } from "../lib/format.js";

export default function SftpPage() {
    const app = useApp();
    const [connId, setConnId] = useState("");
    const [localPath, setLocalPath] = useState("");
    const [local, setLocal] = useState([]);
    const [remotePath, setRemotePath] = useState("/");
    const [remote, setRemote] = useState([]);
    const [status, setStatus] = useState({ msg: "选择连接以浏览远程文件", err: false });
    const fileRef = useRef(null);

    const say = (msg, err = false) => setStatus({ msg, err });

    const loadLocal = useCallback(
        async (path) => {
            try {
                const res = await rpc.call("fs.list", { path }); // empty => home dir
                setLocalPath(res.path);
                setLocal(res.entries);
            } catch (e) {
                say("读取本地失败：" + e.message, true);
            }
        },
        [],
    );

    const loadRemote = useCallback(
        async (path, id = connId) => {
            if (!id) {
                setRemote([]);
                return;
            }
            const res = await rpc.call("sftp.list", { connId: id, path });
            setRemote(res.entries);
            setRemotePath(res.path);
        },
        [connId],
    );

    useEffect(() => {
        loadLocal("").catch(() => {});
    }, [loadLocal]);

    const navigateLocal = (name) => {
        if (name === "..") loadLocal(dirOf(localPath));
        else loadLocal(joinLocal(localPath, name));
    };

    const navigateRemote = (name) => {
        let next;
        if (name === "..") {
            if (remotePath === "/") return;
            const parts = remotePath.split("/").filter(Boolean);
            parts.pop();
            next = "/" + parts.join("/");
        } else {
            next = (remotePath === "/" ? "" : remotePath) + "/" + name;
        }
        loadRemote(next).catch((e) => say(e.message, true));
    };

    const onSelectConn = async (id) => {
        setConnId(id);
        if (!id) {
            setRemote([]);
            return;
        }
        say("连接中…");
        try {
            await loadRemote("/", id);
            say("");
        } catch (e) {
            say(e.message, true);
        }
    };

    const upload = async (file) => {
        if (!connId) return say("请先选择连接", true);
        say(`上传 ${file.name} …`);
        try {
            const buf = await file.arrayBuffer();
            await rpc.call("sftp.upload", {
                connId,
                remoteDir: remotePath,
                name: file.name,
                data: base64Encode(buf),
            });
            say(`已上传 ${file.name}`);
            loadRemote(remotePath).catch(() => {});
        } catch (e) {
            say(e.message, true);
        }
    };

    const download = async (name) => {
        if (!connId) return;
        const remoteFile = (remotePath === "/" ? "" : remotePath) + "/" + name;
        say(`下载 ${name} …`);
        try {
            const res = await rpc.call("sftp.download", { connId, path: remoteFile });
            triggerDownload(name, base64Decode(res.data));
            say(`已下载 ${name}`);
        } catch (e) {
            say(e.message, true);
        }
    };

    const mkdir = () =>
        app.openDialog({
            type: "prompt",
            title: "新建目录",
            label: "目录名",
            onSubmit: async (name) => {
                await rpc.call("sftp.mkdir", { connId, parent: remotePath, name });
                loadRemote(remotePath).catch(() => {});
            },
        });

    return (
        <div className="page" style={{ padding: "12px 16px" }}>
            <div className="toolbar">
                <label className="muted">连接</label>
                <select style={{ width: 220 }} value={connId} onChange={(e) => onSelectConn(e.target.value)}>
                    <option value="">选择连接…</option>
                    {app.connections.map((c) => (
                        <option key={c.id} value={c.id}>
                            {c.name}
                        </option>
                    ))}
                </select>
                <input
                    ref={fileRef}
                    type="file"
                    style={{ display: "none" }}
                    onChange={(e) => {
                        const f = e.target.files?.[0];
                        if (f) upload(f);
                        e.target.value = "";
                    }}
                />
                <button className="btn" onClick={() => fileRef.current?.click()}>
                    ⇧ 上传文件
                </button>
                <span className="muted" style={{ color: status.err ? "var(--danger)" : undefined }}>
                    {status.msg}
                </span>
            </div>

            <div className="sftp-body">
                <Pane title="本地" path={localPath} onUp={() => navigateLocal("..")}>
                    <Entry name=".." isDir openOnClick onOpen={() => navigateLocal("..")} />
                    {local.map((e) => (
                        <Entry
                            key={e.name}
                            name={e.name}
                            isDir={e.isDir}
                            size={e.size}
                            onOpen={e.isDir ? () => navigateLocal(e.name) : undefined}
                        />
                    ))}
                </Pane>

                <Pane title="远程" path={remotePath} onUp={() => navigateRemote("..")} onMkdir={mkdir}>
                    {remotePath !== "/" && <Entry name=".." isDir openOnClick onOpen={() => navigateRemote("..")} />}
                    {remote.map((e) => (
                        <Entry
                            key={e.name}
                            name={e.name}
                            isDir={e.isDir}
                            size={e.size}
                            onOpen={e.isDir ? () => navigateRemote(e.name) : undefined}
                            action={
                                !e.isDir ? (
                                    <button className="btn small" onClick={() => download(e.name)}>
                                        下载
                                    </button>
                                ) : null
                            }
                        />
                    ))}
                    {!connId && <div className="empty">请选择连接</div>}
                </Pane>
            </div>
        </div>
    );
}

function Pane({ title, path, onUp, onMkdir, children }) {
    return (
        <div className="sftp-pane">
            <div className="pane-head">
                <span className="muted">{title}</span>
                <span className="pane-path">{path}</span>
                <button className="icon-btn" title="上级" onClick={onUp}>
                    ↑
                </button>
                {onMkdir && (
                    <button className="icon-btn" title="新建目录" onClick={onMkdir}>
                        ＋
                    </button>
                )}
            </div>
            <div className="pane-list">{children}</div>
        </div>
    );
}

function Entry({ name, isDir, size, onOpen, openOnClick, action }) {
    const handlers = onOpen ? (openOnClick ? { onClick: onOpen } : { onDoubleClick: onOpen }) : {};
    return (
        <div className="sftp-entry" {...handlers}>
            <span>{isDir ? "📁" : "📄"}</span>
            <span className="entry-name">{name}</span>
            {!isDir && <span className="entry-size">{fmtSize(size)}</span>}
            {action}
        </div>
    );
}

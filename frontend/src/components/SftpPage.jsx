import { useCallback, useEffect, useRef, useState } from "react";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";
import { base64Decode, base64Encode, dirOf, fmtSize, joinLocal, triggerDownload } from "../lib/format.js";
import { useT, t } from "../lib/i18n.js";

export default function SftpPage() {
    const tr = useT();
    const app = useApp();
    const [connId, setConnId] = useState("");
    const [localPath, setLocalPath] = useState("");
    const [local, setLocal] = useState([]);
    const [remotePath, setRemotePath] = useState("/");
    const [remote, setRemote] = useState([]);
    const [status, setStatus] = useState({ msg: t("sftp.selectConnHint"), err: false });
    const fileRef = useRef(null);

    // Credentials typed at a connect prompt. Many connections deliberately do
    // not store a password, so the first SFTP call asks for one and the answer
    // is reused for the rest of the window's lifetime. Held in a ref because it
    // is read inside async callbacks that must see the latest value.
    const credsRef = useRef({ connId: "", password: "", keyPassphrase: "" });

    const say = (msg, err = false) => setStatus({ msg, err });

    // call runs an SFTP RPC, answering credential prompts transparently. The
    // backend returns {needPassword} / {needPassphrase} instead of failing when
    // the connection has no usable credentials; the user is asked once and the
    // call is retried with (and remembered for) the typed secret.
    const call = useCallback(
        (method, params, promptState) => {
            const creds = params.connId === credsRef.current.connId ? credsRef.current : { password: "", keyPassphrase: "" };
            return rpc.call(method, { ...params, ...creds }).then((res) => {
                if (!res || !res.needPassword && !res.needPassphrase) return res;

                if (res.needPassphrase) {
                    if (promptState?.passphrase) return Promise.reject(new Error(res.message || t("dialog.needPassphrase")));
                    return new Promise((resolve, reject) => {
                        app.openDialog({
                            type: "passphrase",
                            message: res.message || t("dialog.needPassphrase"),
                            onSubmit: async (pass) => {
                                credsRef.current = { connId: params.connId, password: creds.password, keyPassphrase: pass };
                                try {
                                    resolve(await call(method, params, { ...promptState, passphrase: true }));
                                } catch (e) {
                                    reject(e);
                                }
                            },
                            onCancel: () => reject(new Error(t("common.cancelled"))),
                        });
                    });
                }

                if (res.needPassword) {
                    if (promptState?.password) return Promise.reject(new Error(res.message || t("dialog.needPassword")));
                    return new Promise((resolve, reject) => {
                        app.openDialog({
                            type: "password",
                            message: res.message || t("dialog.needPassword"),
                            onSubmit: async (pw) => {
                                credsRef.current = { connId: params.connId, password: pw, keyPassphrase: creds.keyPassphrase };
                                try {
                                    resolve(await call(method, params, { ...promptState, password: true }));
                                } catch (e) {
                                    reject(e);
                                }
                            },
                            onCancel: () => reject(new Error(t("common.cancelled"))),
                        });
                    });
                }

                return res;
            });
        },
        [app],
    );

    const loadLocal = useCallback(
        async (path) => {
            try {
                const res = await rpc.call("fs.list", { path }); // empty => home dir
                setLocalPath(res.path);
                setLocal(res.entries);
            } catch (e) {
                say(t("sftp.loadLocalFailed", { msg: e.message }), true);
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
            const res = await call("sftp.list", { connId: id, path });
            setRemote(res.entries);
            setRemotePath(res.path);
        },
        [connId, call],
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
        // Switching connection invalidates any credential typed for the old one.
        if (credsRef.current.connId !== id) credsRef.current = { connId: id, password: "", keyPassphrase: "" };
        if (!id) {
            setRemote([]);
            return;
        }
        say(t("sftp.connecting"));
        try {
            await loadRemote("/", id);
            say("");
        } catch (e) {
            say(e.message, true);
        }
    };

    const upload = async (file) => {
        if (!connId) return say(t("sftp.selectConnFirst"), true);
        say(t("sftp.uploading", { name: file.name }));
        try {
            const buf = await file.arrayBuffer();
            await call("sftp.upload", {
                connId,
                remoteDir: remotePath,
                name: file.name,
                data: base64Encode(buf),
            });
            say(t("sftp.uploaded", { name: file.name }));
            loadRemote(remotePath).catch(() => {});
        } catch (e) {
            say(e.message, true);
        }
    };

    const download = async (name) => {
        if (!connId) return;
        const remoteFile = (remotePath === "/" ? "" : remotePath) + "/" + name;
        say(t("sftp.downloading", { name }));
        try {
            const res = await call("sftp.download", { connId, path: remoteFile });
            triggerDownload(name, base64Decode(res.data));
            say(t("sftp.downloaded", { name }));
        } catch (e) {
            say(e.message, true);
        }
    };

    const mkdir = () =>
        app.openDialog({
            type: "prompt",
            title: t("sftp.mkdir"),
            label: t("sftp.mkdirName"),
            onSubmit: async (name) => {
                await call("sftp.mkdir", { connId, parent: remotePath, name });
                loadRemote(remotePath).catch(() => {});
            },
        });

    return (
        <div className="page" style={{ padding: "12px 16px" }}>
            <div className="toolbar">
                <label className="muted">{tr("sftp.connection")}</label>
                <select style={{ width: 220 }} value={connId} onChange={(e) => onSelectConn(e.target.value)}>
                    <option value="">{tr("sftp.selectConnOption")}</option>
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
                <button className="btn" onClick={() => fileRef.current?.click()}>{tr("sftp.uploadFile")}</button>
                <span className="muted" style={{ color: status.err ? "var(--danger)" : undefined }}>
                    {status.msg}
                </span>
            </div>

            <div className="sftp-body">
                <Pane title={tr("sftp.local")} path={localPath} onUp={() => navigateLocal("..")}>
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

                <Pane title={tr("sftp.remote")} path={remotePath} onUp={() => navigateRemote("..")} onMkdir={mkdir}>
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
                                    <button className="btn small" onClick={() => download(e.name)}>{tr("sftp.download")}</button>
                                ) : null
                            }
                        />
                    ))}
                    {!connId && <div className="empty">{tr("sftp.selectConnEmpty")}</div>}
                </Pane>
            </div>
        </div>
    );
}

function Pane({ title, path, onUp, onMkdir, children }) {
    const tr = useT();
    return (
        <div className="sftp-pane">
            <div className="pane-head">
                <span className="muted">{title}</span>
                <span className="pane-path">{path}</span>
                <button className="icon-btn" title={tr("sftp.parent")} onClick={onUp}>
                    ↑
                </button>
                {onMkdir && (
                    <button className="icon-btn" title={tr("sftp.mkdir")} onClick={onMkdir}>
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

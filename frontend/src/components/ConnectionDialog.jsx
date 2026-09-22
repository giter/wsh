import { useRef, useState } from "react";
import Modal from "./Modal.jsx";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

// ConnectionDialog edits a saved connection (conn) or creates a new one. draft
// pre-fills the form for a new connection, e.g. from a quick-connect address or
// an open ad-hoc session. onSaved receives the saved connection view.
export default function ConnectionDialog({ conn, draft, onSaved, onClose }) {
    const app = useApp();
    const editing = !!conn;
    const initial = conn || draft || {};
    const [form, setForm] = useState(() => ({
        name: initial.name || "",
        host: initial.host || "",
        port: String(initial.port || app.settings.defaultPort || 22),
        user: initial.user || app.settings.defaultUser || "",
        password: "",
        savePassword: initial.savePassword || false,
        privateKeyPath: initial.privateKeyPath || "",
        keyId: initial.keyId || "",
        folderId: initial.folderId || "",
        jumpHostIds: [...(initial.jumpHostIds || [])],
    }));
    const [status, setStatus] = useState({ msg: "", err: false });
    // Passphrase typed at the test prompt for an encrypted managed key. Kept
    // for the dialog session only and passed along on retries.
    const keyPassRef = useRef("");

    const set = (key) => (e) => {
        const value = e.target.type === "checkbox" ? e.target.checked : e.target.value;
        setForm((f) => ({ ...f, [key]: value }));
    };

    // Jump hosts are stored in selection order: the first one picked is the
    // first hop. Toggling a bastion appends or removes it.
    const toggleJump = (id) =>
        setForm((f) => {
            const has = f.jumpHostIds.includes(id);
            return {
                ...f,
                jumpHostIds: has ? f.jumpHostIds.filter((x) => x !== id) : [...f.jumpHostIds, id],
            };
        });

    // Candidates exclude this connection itself (a self-hop is a cycle).
    const jumpCandidates = app.connections.filter((c) => !editing || c.id !== conn.id);
    const chainText = (targetName) =>
        ["Local", ...form.jumpHostIds.map((id) => app.connections.find((c) => c.id === id)?.name || id), targetName || "Target"].join(
            " → ",
        );

    const test = async () => {
        setStatus({ msg: "测试中…", err: false });
        try {
            const res = await rpc.call("connections.test", {
                id: editing ? conn.id : "",
                host: form.host,
                port: parseInt(form.port || "22", 10),
                user: form.user,
                password: form.password,
                keyPath: form.privateKeyPath,
                keyId: form.keyId,
                keyPassphrase: keyPassRef.current,
                jumpHostIds: form.jumpHostIds,
            });
            if (res && res.needPassphrase) {
                app.openDialog({
                    type: "passphrase",
                    message: res.message || "需要口令",
                    onSubmit: (pass) => {
                        keyPassRef.current = pass;
                        return test();
                    },
                });
                setStatus({ msg: res.message || "需要口令", err: true });
                return;
            }
            setStatus({ msg: "连接成功 ✓", err: false });
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

    const save = async () => {
        setStatus({ msg: "", err: false });
        try {
            const saved = await rpc.call("connections.save", {
                id: editing ? conn.id : "",
                name: form.name.trim(),
                host: form.host.trim(),
                port: parseInt(form.port || "22", 10),
                user: form.user.trim(),
                folderId: form.folderId,
                password: form.password,
                savePassword: form.savePassword,
                privateKeyPath: form.privateKeyPath.trim(),
                keyId: form.keyId,
                jumpHostIds: form.jumpHostIds,
            });
            await app.refreshConnections();
            onSaved?.(saved);
            onClose();
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

    return (
        <Modal
            title={editing ? "编辑连接" : "新建连接"}
            onClose={onClose}
            actions={
                <>
                    <button className="btn" onClick={onClose}>
                        取消
                    </button>
                    <button className="btn" onClick={test}>
                        测试连接
                    </button>
                    <button className="btn primary" onClick={save}>
                        保存
                    </button>
                </>
            }
        >
            <label className="field">
                <span>名称</span>
                <input autoFocus value={form.name} onChange={set("name")} />
            </label>
            <label className="field">
                <span>文件夹</span>
                <select value={form.folderId} onChange={set("folderId")}>
                    <option value="">未分组</option>
                    {app.folders.map((f) => (
                        <option key={f.id} value={f.id}>
                            {f.name}
                        </option>
                    ))}
                </select>
            </label>
            <label className="field">
                <span>主机</span>
                <input placeholder="example.com" value={form.host} onChange={set("host")} />
            </label>
            <label className="field">
                <span>端口</span>
                <input type="number" value={form.port} onChange={set("port")} />
            </label>
            <label className="field">
                <span>用户</span>
                <input placeholder="root" value={form.user} onChange={set("user")} />
            </label>
            <label className="field">
                <span>密码</span>
                <input type="password" autoComplete="off" value={form.password} onChange={set("password")} />
            </label>
            <label className="check-row">
                <input type="checkbox" checked={form.savePassword} onChange={set("savePassword")} />
                保存密码（加密落盘）
            </label>
            <label className="field">
                <span>密钥（优先使用）</span>
                <select value={form.keyId} onChange={set("keyId")}>
                    <option value="">不使用托管密钥</option>
                    {app.keys.map((k) => (
                        <option key={k.id} value={k.id}>
                            {k.name}
                        </option>
                    ))}
                </select>
            </label>
            <label className="field">
                <span>私钥文件路径</span>
                <input placeholder="可选：/path/to/id_rsa" value={form.privateKeyPath} onChange={set("privateKeyPath")} />
            </label>

            <div className="field">
                <span>跳板机（按选择顺序串联）</span>
                {jumpCandidates.length === 0 ? (
                    <div className="muted" style={{ fontSize: 12 }}>还没有其它连接可作为跳板机</div>
                ) : (
                    <div className="jump-list">
                        {jumpCandidates.map((c) => {
                            const idx = form.jumpHostIds.indexOf(c.id);
                            return (
                                <label key={c.id} className="jump-item">
                                    <input type="checkbox" checked={idx >= 0} onChange={() => toggleJump(c.id)} />
                                    <span className="jump-order">{idx >= 0 ? idx + 1 : ""}</span>
                                    <span className="lbl">{c.name}</span>
                                    <span className="muted">
                                        {c.user}@{c.host}
                                    </span>
                                </label>
                            );
                        })}
                    </div>
                )}
                {form.jumpHostIds.length > 0 && (
                    <div className="jump-chain muted" title={chainText(form.name.trim() || "Target")}>
                        {chainText(form.name.trim() || "Target")}
                    </div>
                )}
            </div>
            <div className={"status-msg" + (status.err ? " err" : "")}>{status.msg}</div>
        </Modal>
    );
}

import { useState } from "react";
import Modal from "./Modal.jsx";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

export default function ConnectionDialog({ conn, onClose }) {
    const app = useApp();
    const editing = !!conn;
    const [form, setForm] = useState(() => ({
        name: conn?.name || "",
        host: conn?.host || "",
        port: String(conn?.port || app.settings.defaultPort || 22),
        user: conn?.user || app.settings.defaultUser || "",
        password: "",
        savePassword: conn?.savePassword || false,
        privateKeyPath: conn?.privateKeyPath || "",
        keyId: conn?.keyId || "",
        folderId: conn?.folderId || "",
    }));
    const [status, setStatus] = useState({ msg: "", err: false });

    const set = (key) => (e) => {
        const value = e.target.type === "checkbox" ? e.target.checked : e.target.value;
        setForm((f) => ({ ...f, [key]: value }));
    };

    const test = async () => {
        setStatus({ msg: "测试中…", err: false });
        try {
            await rpc.call("connections.test", {
                id: editing ? conn.id : "",
                host: form.host,
                port: parseInt(form.port || "22", 10),
                user: form.user,
                password: form.password,
                keyPath: form.privateKeyPath,
                keyId: form.keyId,
            });
            setStatus({ msg: "连接成功 ✓", err: false });
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

    const save = async () => {
        setStatus({ msg: "", err: false });
        try {
            await rpc.call("connections.save", {
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
            });
            await app.refreshConnections();
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
            <div className={"status-msg" + (status.err ? " err" : "")}>{status.msg}</div>
        </Modal>
    );
}

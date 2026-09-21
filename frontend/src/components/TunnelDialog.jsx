import { useState } from "react";
import Modal from "./Modal.jsx";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

export default function TunnelDialog({ tunnel, onSaved, onClose }) {
    const app = useApp();
    const editing = !!tunnel;
    const [form, setForm] = useState(() => ({
        name: tunnel?.name || "",
        connectionId: tunnel?.connectionId || app.connections[0]?.id || "",
        direction: tunnel?.direction || "local",
        localAddress: tunnel?.localAddress || "127.0.0.1",
        localPort: String(tunnel?.localPort || app.settings.tunnelLocalPort || 8080),
        remoteAddress: tunnel?.remoteAddress || "127.0.0.1",
        remotePort: String(tunnel?.remotePort || app.settings.tunnelRemotePort || 80),
        remark: tunnel?.remark || "",
    }));
    const [status, setStatus] = useState({ msg: "", err: false });

    const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }));

    const remote = form.direction === "remote";
    const labels = remote
        ? { localAddr: "本地目标地址", localPort: "本地目标端口", remoteAddr: "远程监听地址", remotePort: "远程监听端口" }
        : { localAddr: "本地监听地址", localPort: "本地监听端口", remoteAddr: "远程目标地址", remotePort: "远程目标端口" };

    const save = async () => {
        setStatus({ msg: "", err: false });
        try {
            await rpc.call("tunnels.save", {
                id: editing ? tunnel.id : "",
                name: form.name.trim(),
                connectionId: form.connectionId,
                direction: form.direction,
                localAddress: form.localAddress.trim(),
                localPort: parseInt(form.localPort || "0", 10),
                remoteAddress: form.remoteAddress.trim(),
                remotePort: parseInt(form.remotePort || "0", 10),
                remark: form.remark.trim(),
            });
            await onSaved?.();
            onClose();
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

    return (
        <Modal
            title={editing ? "编辑隧道" : "新建隧道"}
            onClose={onClose}
            actions={
                <>
                    <button className="btn" onClick={onClose}>
                        取消
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
                <span>连接</span>
                <select value={form.connectionId} onChange={set("connectionId")}>
                    {app.connections.map((c) => (
                        <option key={c.id} value={c.id}>
                            {c.name}
                        </option>
                    ))}
                </select>
            </label>
            <label className="field">
                <span>方向</span>
                <select value={form.direction} onChange={set("direction")}>
                    <option value="local">本地转发（本地监听 → 远程）</option>
                    <option value="remote">远程转发（远程监听 → 本地）</option>
                </select>
            </label>
            <label className="field">
                <span>{labels.localAddr}</span>
                <input value={form.localAddress} onChange={set("localAddress")} />
            </label>
            <label className="field">
                <span>{labels.localPort}</span>
                <input type="number" value={form.localPort} onChange={set("localPort")} />
            </label>
            <label className="field">
                <span>{labels.remoteAddr}</span>
                <input value={form.remoteAddress} onChange={set("remoteAddress")} />
            </label>
            <label className="field">
                <span>{labels.remotePort}</span>
                <input type="number" value={form.remotePort} onChange={set("remotePort")} />
            </label>
            <label className="field">
                <span>备注</span>
                <input placeholder="备注（可选）" value={form.remark} onChange={set("remark")} />
            </label>
            <div className={"status-msg" + (status.err ? " err" : "")}>{status.msg}</div>
        </Modal>
    );
}

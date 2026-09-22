import { useState } from "react";
import Modal from "./Modal.jsx";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";
import { useT } from "../lib/i18n.js";

export default function TunnelDialog({ tunnel, onSaved, onClose }) {
    const tr = useT();
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
        ? { localAddr: tr("tunnels.localTargetAddr"), localPort: tr("tunnels.localTargetPort"), remoteAddr: tr("tunnels.remoteListenAddr"), remotePort: tr("tunnels.remoteListenPort") }
        : { localAddr: tr("tunnels.localListenAddr"), localPort: tr("tunnels.localListenPort"), remoteAddr: tr("tunnels.remoteTargetAddr"), remotePort: tr("tunnels.remoteTargetPort") };

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
            title={editing ? tr("tunnels.editTitle") : tr("tunnels.newTitle")}
            onClose={onClose}
            actions={
                <>
                    <button className="btn" onClick={onClose}>
                        {tr("common.cancel")}
                    </button>
                    <button className="btn primary" onClick={save}>
                        {tr("common.save")}
                    </button>
                </>
            }
        >
            <label className="field">
                <span>{tr("tunnels.name")}</span>
                <input autoFocus value={form.name} onChange={set("name")} />
            </label>
            <label className="field">
                <span>{tr("tunnels.connection")}</span>
                <select value={form.connectionId} onChange={set("connectionId")}>
                    {app.connections.map((c) => (
                        <option key={c.id} value={c.id}>
                            {c.name}
                        </option>
                    ))}
                </select>
            </label>
            <label className="field">
                <span>{tr("tunnels.direction")}</span>
                <select value={form.direction} onChange={set("direction")}>
                    <option value="local">{tr("tunnels.dirLocalOpt")}</option>
                    <option value="remote">{tr("tunnels.dirRemoteOpt")}</option>
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
                <span>{tr("tunnels.remark")}</span>
                <input placeholder={tr("tunnels.remarkPlaceholder")} value={form.remark} onChange={set("remark")} />
            </label>
            <div className={"status-msg" + (status.err ? " err" : "")}>{status.msg}</div>
        </Modal>
    );
}

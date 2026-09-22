import { useRef, useState } from "react";
import Modal from "./Modal.jsx";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";
import { useT } from "../lib/i18n.js";

// ConnectionDialog edits a saved connection (conn) or creates a new one. draft
// pre-fills the form for a new connection, e.g. from a quick-connect address or
// an open ad-hoc session. onSaved receives the saved connection view.
export default function ConnectionDialog({ conn, draft, onSaved, onClose }) {
    const tr = useT();
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
        setStatus({ msg: tr("conn.testing"), err: false });
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
                    message: res.message || tr("dialog.needPassphrase"),
                    onSubmit: (pass) => {
                        keyPassRef.current = pass;
                        return test();
                    },
                });
                setStatus({ msg: res.message || tr("dialog.needPassphrase"), err: true });
                return;
            }
            setStatus({ msg: tr("conn.testOk"), err: false });
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
            title={editing ? tr("conn.editTitle") : tr("conn.newTitle")}
            onClose={onClose}
            actions={
                <>
                    <button className="btn" onClick={onClose}>
                        {tr("common.cancel")}
                    </button>
                    <button className="btn" onClick={test}>
                        {tr("conn.test")}
                    </button>
                    <button className="btn primary" onClick={save}>
                        {tr("common.save")}
                    </button>
                </>
            }
        >
            <label className="field">
                <span>{tr("conn.name")}</span>
                <input autoFocus value={form.name} onChange={set("name")} />
            </label>
            <label className="field">
                <span>{tr("conn.folder")}</span>
                <select value={form.folderId} onChange={set("folderId")}>
                    <option value="">{tr("conn.ungrouped")}</option>
                    {app.folders.map((f) => (
                        <option key={f.id} value={f.id}>
                            {f.name}
                        </option>
                    ))}
                </select>
            </label>
            <label className="field">
                <span>{tr("conn.host")}</span>
                <input placeholder="example.com" value={form.host} onChange={set("host")} />
            </label>
            <label className="field">
                <span>{tr("conn.port")}</span>
                <input type="number" value={form.port} onChange={set("port")} />
            </label>
            <label className="field">
                <span>{tr("conn.user")}</span>
                <input placeholder="root" value={form.user} onChange={set("user")} />
            </label>
            <label className="field">
                <span>{tr("conn.password")}</span>
                <input type="password" autoComplete="off" value={form.password} onChange={set("password")} />
            </label>
            <label className="check-row">
                <input type="checkbox" checked={form.savePassword} onChange={set("savePassword")} />
                {tr("conn.savePassword")}
            </label>
            <label className="field">
                <span>{tr("conn.key")}</span>
                <select value={form.keyId} onChange={set("keyId")}>
                    <option value="">{tr("conn.noManagedKey")}</option>
                    {app.keys.map((k) => (
                        <option key={k.id} value={k.id}>
                            {k.name}
                        </option>
                    ))}
                </select>
            </label>
            <label className="field">
                <span>{tr("conn.keyPath")}</span>
                <input placeholder={tr("conn.keyPathPlaceholder")} value={form.privateKeyPath} onChange={set("privateKeyPath")} />
            </label>

            <div className="field">
                <span>{tr("conn.jumpHosts")}</span>
                {jumpCandidates.length === 0 ? (
                    <div className="muted" style={{ fontSize: 12 }}>{tr("conn.jumpHostsEmpty")}</div>
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

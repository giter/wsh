import { useState } from "react";
import Modal from "./Modal.jsx";
import { useApp } from "../state/store.jsx";
import { rpc } from "../lib/rpc.js";

// MoveConnDialog moves a connection into another folder.
export default function MoveConnDialog({ conn, onClose }) {
    const app = useApp();
    const [folderId, setFolderId] = useState(conn.folderId || "");
    const [err, setErr] = useState("");

    const save = async () => {
        try {
            await rpc.call("connections.save", { ...conn, folderId });
            await app.refreshConnections();
            onClose();
        } catch (e) {
            setErr(e.message);
        }
    };

    return (
        <Modal
            title={`移动「${conn.name}」`}
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
                <span>移动到</span>
                <select value={folderId} onChange={(e) => setFolderId(e.target.value)}>
                    <option value="">未分组</option>
                    {app.folders.map((f) => (
                        <option key={f.id} value={f.id}>
                            {f.name}
                        </option>
                    ))}
                </select>
            </label>
            <div className={"status-msg" + (err ? " err" : "")}>{err}</div>
        </Modal>
    );
}

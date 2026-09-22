import { useRef, useState } from "react";
import Modal from "./Modal.jsx";
import { rpc } from "../lib/rpc.js";
import { useT } from "../lib/i18n.js";

export default function KeyDialog({ item, onSaved, onClose }) {
    const tr = useT();
    const editing = !!item;
    const [name, setName] = useState(item?.name || "");
    const [comment, setComment] = useState(item?.comment || "");
    const [privateKey, setPrivateKey] = useState("");
    const [passphrase, setPassphrase] = useState("");
    // Opt-in only: unchecked by default the passphrase is asked again on every
    // connect and never touches disk.
    const [savePassphrase, setSavePassphrase] = useState(item?.passphraseSaved || false);
    const [status, setStatus] = useState({ msg: "", err: false });
    const fileRef = useRef(null);

    const onFile = async (e) => {
        const file = e.target.files?.[0];
        e.target.value = "";
        if (!file) return;
        setPrivateKey(await file.text());
        setName((n) => n.trim() || file.name.replace(/\.[^.]+$/, ""));
    };

    const save = async () => {
        setStatus({ msg: "", err: false });
        if (!name.trim()) {
            setStatus({ msg: tr("keys.nameRequired"), err: true });
            return;
        }
        if (!editing && !privateKey.trim()) {
            setStatus({ msg: tr("keys.privRequired"), err: true });
            return;
        }
        try {
            await rpc.call("keys.save", {
                id: editing ? item.id : "",
                name: name.trim(),
                comment: comment.trim(),
                privateKey,
                passphrase,
                savePassphrase,
            });
            await onSaved?.();
            onClose();
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

    return (
        <Modal
            title={editing ? tr("keys.editTitle") : tr("keys.newTitle")}
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
                <span>{tr("keys.name")}</span>
                <input autoFocus placeholder={tr("keys.namePlaceholder")} value={name} onChange={(e) => setName(e.target.value)} />
            </label>
            <label className="field">
                <span>{tr("keys.remark")}</span>
                <input placeholder={tr("keys.remarkPlaceholder")} value={comment} onChange={(e) => setComment(e.target.value)} />
            </label>

            <label className="field">
                <div className="field-row">
                    <span>{editing ? tr("keys.privContentKeep") : tr("keys.privContent")}</span>
                    <input ref={fileRef} type="file" style={{ display: "none" }} onChange={onFile} />
                    <button className="btn small" onClick={() => fileRef.current?.click()}>
                        {tr("keys.fromFile")}
                    </button>
                </div>
                <textarea
                    rows={8}
                    spellCheck={false}
                    placeholder={editing ? tr("keys.privPlaceholderEdit") : "-----BEGIN OPENSSH PRIVATE KEY-----\n..."}
                    value={privateKey}
                    onChange={(e) => setPrivateKey(e.target.value)}
                />
            </label>

            <label className="field">
                <span>{tr("keys.passphraseLabel")}</span>
                <input
                    type="password"
                    autoComplete="off"
                    placeholder={tr("keys.passphrasePlaceholder")}
                    value={passphrase}
                    onChange={(e) => setPassphrase(e.target.value)}
                />
            </label>
            <label className="check-row">
                <input type="checkbox" checked={savePassphrase} onChange={(e) => setSavePassphrase(e.target.checked)} />
                {tr("keys.rememberPass")}
            </label>

            <div className="hint">{tr("keys.hint")}</div>
            <div className={"status-msg" + (status.err ? " err" : "")}>{status.msg}</div>
        </Modal>
    );
}

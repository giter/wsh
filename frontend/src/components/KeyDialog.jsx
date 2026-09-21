import { useRef, useState } from "react";
import Modal from "./Modal.jsx";
import { rpc } from "../lib/rpc.js";

export default function KeyDialog({ item, onSaved, onClose }) {
    const editing = !!item;
    const [name, setName] = useState(item?.name || "");
    const [comment, setComment] = useState(item?.comment || "");
    const [privateKey, setPrivateKey] = useState("");
    const [passphrase, setPassphrase] = useState("");
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
            setStatus({ msg: "请输入密钥名称", err: true });
            return;
        }
        if (!editing && !privateKey.trim()) {
            setStatus({ msg: "请提交私钥内容", err: true });
            return;
        }
        try {
            await rpc.call("keys.save", {
                id: editing ? item.id : "",
                name: name.trim(),
                comment: comment.trim(),
                privateKey,
                passphrase,
            });
            await onSaved?.();
            onClose();
        } catch (e) {
            setStatus({ msg: e.message, err: true });
        }
    };

    return (
        <Modal
            title={editing ? "编辑密钥" : "提交密钥"}
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
                <input autoFocus placeholder="例如：生产服务器" value={name} onChange={(e) => setName(e.target.value)} />
            </label>
            <label className="field">
                <span>备注</span>
                <input placeholder="可选备注" value={comment} onChange={(e) => setComment(e.target.value)} />
            </label>

            <label className="field">
                <div className="field-row">
                    <span>{editing ? "私钥内容（留空保持不变）" : "私钥内容"}</span>
                    <input ref={fileRef} type="file" style={{ display: "none" }} onChange={onFile} />
                    <button className="btn small" onClick={() => fileRef.current?.click()}>
                        从文件读取…
                    </button>
                </div>
                <textarea
                    rows={8}
                    spellCheck={false}
                    placeholder={editing ? "留空则保持原有私钥不变" : "-----BEGIN OPENSSH PRIVATE KEY-----\n..."}
                    value={privateKey}
                    onChange={(e) => setPrivateKey(e.target.value)}
                />
            </label>

            <label className="field">
                <span>口令（私钥已加密时填写）</span>
                <input
                    type="password"
                    autoComplete="off"
                    placeholder="私钥未加密时留空"
                    value={passphrase}
                    onChange={(e) => setPassphrase(e.target.value)}
                />
            </label>

            <div className="hint">
                私钥仅保存在本机（加密落盘），不会上传。复制公钥并追加到服务器的 ~/.ssh/authorized_keys 即可用该密钥登录。
            </div>
            <div className={"status-msg" + (status.err ? " err" : "")}>{status.msg}</div>
        </Modal>
    );
}

import { useState } from "react";
import Modal from "./Modal.jsx";

// Small, reusable dialogs: confirm, text prompt, password prompt, notice and
// the About box.

export function ConfirmDialog({ title, body, confirmLabel = "确定", onConfirm, onClose }) {
    const [busy, setBusy] = useState(false);
    const run = async () => {
        setBusy(true);
        try {
            await onConfirm();
            onClose();
        } catch (e) {
            // Keep the dialog open so the user can retry; the error is shown by
            // the caller's own status handling where it matters.
            console.error("[confirm] action failed:", e);
        } finally {
            setBusy(false);
        }
    };
    return (
        <Modal
            title={title}
            onClose={onClose}
            actions={
                <>
                    <button className="btn" onClick={onClose} disabled={busy}>
                        取消
                    </button>
                    <button className="btn danger" onClick={run} disabled={busy}>
                        {confirmLabel}
                    </button>
                </>
            }
        >
            {body}
        </Modal>
    );
}

export function PromptDialog({ title, label, initial = "", onSubmit, onClose }) {
    const [value, setValue] = useState(initial);
    const [err, setErr] = useState("");
    const submit = async () => {
        const v = value.trim();
        if (!v) {
            setErr("不能为空");
            return;
        }
        try {
            await onSubmit(v);
            onClose();
        } catch (e) {
            setErr(e.message);
        }
    };
    return (
        <Modal
            title={title}
            onClose={onClose}
            actions={
                <>
                    <button className="btn" onClick={onClose}>
                        取消
                    </button>
                    <button className="btn primary" onClick={submit}>
                        保存
                    </button>
                </>
            }
        >
            <label className="field">
                <span>{label}</span>
                <input
                    autoFocus
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === "Enter") {
                            e.preventDefault();
                            submit();
                        }
                    }}
                />
            </label>
            <div className={"status-msg" + (err ? " err" : "")}>{err}</div>
        </Modal>
    );
}

export function PasswordDialog({ message, onSubmit, onClose }) {
    const [pw, setPw] = useState("");
    const submit = () => {
        onClose();
        onSubmit(pw);
    };
    return (
        <Modal title="需要密码" onClose={onClose}>
            <div className="status-msg err">{message}</div>
            <label className="field">
                <input
                    autoFocus
                    type="password"
                    placeholder="密码"
                    autoComplete="off"
                    value={pw}
                    onChange={(e) => setPw(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === "Enter") {
                            e.preventDefault();
                            submit();
                        }
                    }}
                />
            </label>
            <div className="modal-actions">
                <button className="btn" onClick={onClose}>
                    取消
                </button>
                <button className="btn primary" onClick={submit}>
                    连接
                </button>
            </div>
        </Modal>
    );
}

// PassphraseDialog asks for a managed key's passphrase at connect time. The
// input stays in memory for the app run only; it is never persisted unless the
// user opts in from the key manager.
export function PassphraseDialog({ message, onSubmit, onClose }) {
    const [pw, setPw] = useState("");
    const submit = () => {
        onClose();
        onSubmit(pw);
    };
    return (
        <Modal title="需要口令" onClose={onClose}>
            <div className="status-msg err">{message}</div>
            <label className="field">
                <input
                    autoFocus
                    type="password"
                    placeholder="私钥口令"
                    autoComplete="off"
                    value={pw}
                    onChange={(e) => setPw(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === "Enter") {
                            e.preventDefault();
                            submit();
                        }
                    }}
                />
            </label>
            <div className="modal-actions">
                <button className="btn" onClick={onClose}>
                    取消
                </button>
                <button className="btn primary" onClick={submit}>
                    连接
                </button>
            </div>
        </Modal>
    );
}

export function NoticeDialog({ title = "提示", message, onClose }) {
    return (
        <Modal
            title={title}
            onClose={onClose}
            actions={
                <button className="btn primary" onClick={onClose}>
                    关闭
                </button>
            }
        >
            <p>{message}</p>
        </Modal>
    );
}

export function AboutDialog({ onClose }) {
    return (
        <Modal
            title="关于 wsh"
            onClose={onClose}
            actions={
                <button className="btn primary" onClick={onClose}>
                    关闭
                </button>
            }
        >
            <div className="about">
                <div className="about-logo">
                    <span className="logo-dot" />
                </div>
                <div className="about-title">wsh</div>
                <div className="about-desc">
                    基于 Go + Wails v3 的跨平台 SSH 客户端
                    <br />
                    终端渲染：xterm.js
                </div>
            </div>
        </Modal>
    );
}

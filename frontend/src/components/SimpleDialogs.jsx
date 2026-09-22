import { useEffect, useState } from "react";
import Modal from "./Modal.jsx";
import { requestLatinInput } from "../state/store.jsx";
import { useT } from "../lib/i18n.js";

// Small, reusable dialogs: confirm, text prompt, password prompt, notice and
// the About box.

export function ConfirmDialog({ title, body, confirmLabel, onConfirm, onClose }) {
    const t = useT();
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
                        {t("common.cancel")}
                    </button>
                    <button className="btn danger" onClick={run} disabled={busy}>
                        {confirmLabel || t("dialog.confirm")}
                    </button>
                </>
            }
        >
            {body}
        </Modal>
    );
}

export function PromptDialog({ title, label, initial = "", onSubmit, onClose }) {
    const t = useT();
    const [value, setValue] = useState(initial);
    const [err, setErr] = useState("");
    const submit = async () => {
        const v = value.trim();
        if (!v) {
            setErr(t("dialog.promptEmpty"));
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
                        {t("common.cancel")}
                    </button>
                    <button className="btn primary" onClick={submit}>
                        {t("common.save")}
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

export function PasswordDialog({ message, onSubmit, onClose, onCancel }) {
    const t = useT();
    const [pw, setPw] = useState("");
    // A password is typed as plain ASCII, so start the IME in English rather than
    // whatever composition mode was left active elsewhere in the UI.
    useEffect(() => {
        requestLatinInput();
    }, []);
    const submit = () => {
        onClose();
        onSubmit(pw);
    };
    // Cancelling must be distinguishable from submitting, so callers awaiting a
    // credential can stop waiting instead of hanging forever.
    const cancel = () => {
        onClose();
        onCancel?.();
    };
    return (
        <Modal title={t("dialog.needPassword")} onClose={cancel}>
            <div className="status-msg err">{message}</div>
            <label className="field">
                <input
                    autoFocus
                    type="password"
                    placeholder={t("dialog.password")}
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
                <button className="btn" onClick={cancel}>
                    {t("common.cancel")}
                </button>
                <button className="btn primary" onClick={submit}>
                    {t("common.connect")}
                </button>
            </div>
        </Modal>
    );
}

// PassphraseDialog asks for a managed key's passphrase at connect time. The
// input stays in memory for the app run only; it is never persisted unless the
// user opts in from the key manager.
export function PassphraseDialog({ message, onSubmit, onClose, onCancel }) {
    const t = useT();
    const [pw, setPw] = useState("");
    // An SSH passphrase is typed as plain ASCII, so start the IME in English
    // rather than whatever composition mode was left active elsewhere in the UI.
    useEffect(() => {
        requestLatinInput();
    }, []);
    const submit = () => {
        onClose();
        onSubmit(pw);
    };
    const cancel = () => {
        onClose();
        onCancel?.();
    };
    return (
        <Modal title={t("dialog.needPassphrase")} onClose={cancel}>
            <div className="status-msg err">{message}</div>
            <label className="field">
                <input
                    autoFocus
                    type="password"
                    placeholder={t("dialog.passphrase")}
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
                <button className="btn" onClick={cancel}>
                    {t("common.cancel")}
                </button>
                <button className="btn primary" onClick={submit}>
                    {t("common.connect")}
                </button>
            </div>
        </Modal>
    );
}

export function NoticeDialog({ title, message, onClose }) {
    const t = useT();
    return (
        <Modal
            title={title || t("dialog.notice")}
            onClose={onClose}
            actions={
                <button className="btn primary" onClick={onClose}>
                    {t("common.close")}
                </button>
            }
        >
            <p>{message}</p>
        </Modal>
    );
}

export function AboutDialog({ onClose }) {
    const t = useT();
    return (
        <Modal
            title={t("dialog.about")}
            onClose={onClose}
            actions={
                <button className="btn primary" onClick={onClose}>
                    {t("common.close")}
                </button>
            }
        >
            <div className="about">
                <div className="about-logo">
                    <span className="logo-dot" />
                </div>
                <div className="about-title">wsh</div>
                <div className="about-desc">
                    {t("dialog.aboutDesc1")}
                    <br />
                    {t("dialog.aboutDesc2")}
                </div>
            </div>
        </Modal>
    );
}
